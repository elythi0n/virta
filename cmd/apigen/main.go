// Command apigen generates the TypeScript wire types the frontends consume, reflected from the Go
// types that define the daemon's HTTP/WebSocket contract. UnifiedMessage and the event envelope
// are therefore never hand-duplicated in TS: edit the Go structs, run `make apigen`, and commit.
//
// The output is deterministic (types emitted in sorted order) so `make apigen-check` can fail a
// build whose generated file is stale.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/api"
)

// roots are the entry points of the wire contract; every type reachable from them is generated.
func roots() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf(api.WireEvent{}),
		reflect.TypeOf(api.Discovery{}),
		reflect.TypeOf(api.ChannelInfo{}),
		reflect.TypeOf(api.Capabilities{}),
		reflect.TypeOf(api.StreamInfo{}),
		reflect.TypeOf(api.EmoteInfo{}),
		reflect.TypeOf(api.FilterRule{}),
		reflect.TypeOf(api.AccountInfo{}),
		reflect.TypeOf(api.AuthConfig{}),
		reflect.TypeOf(api.DeviceSession{}),
		reflect.TypeOf(api.AuthSession{}),
		reflect.TypeOf(api.ProfileInfo{}),
		reflect.TypeOf(api.SendTarget{}),
		reflect.TypeOf(api.SendResult{}),
		reflect.TypeOf(api.QueueState{}),
		reflect.TypeOf(api.HeldMessage{}),
		reflect.TypeOf(api.LoggedMessage{}),
		reflect.TypeOf(api.TokenInfo{}),
		reflect.TypeOf(api.MintedToken{}),
		reflect.TypeOf(api.ThemeInfo{}),
		reflect.TypeOf(api.ProfileExport{}),
	}
}

var (
	timeType = reflect.TypeOf(time.Time{})
	rawType  = reflect.TypeOf(json.RawMessage(nil))
)

func main() {
	out := flag.String("out", "frontends/web/src/daemon/wire.gen.ts", "output TypeScript file")
	flag.Parse()

	g := &generator{decls: map[string]string{}}
	for _, r := range roots() {
		g.walk(r)
	}

	var b bytes.Buffer
	b.WriteString("// Generated from the Go wire types by cmd/apigen. Do not edit by hand.\n")
	b.WriteString("// Run `make apigen` after changing the daemon's API structs.\n\n")
	names := make([]string, 0, len(g.decls))
	for n := range g.decls {
		names = append(names, n)
	}
	sort.Strings(names)
	for i, n := range names {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(g.decls[n])
	}

	if err := os.WriteFile(*out, b.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "apigen: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("apigen: wrote %d types to %s\n", len(names), *out)
}

type generator struct {
	decls map[string]string // type name -> TS declaration
}

// walk emits a declaration for a named type and recurses into the types it references.
func (g *generator) walk(t reflect.Type) {
	t = deref(t)
	name := t.Name()
	if name == "" || g.decls[name] != "" {
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		if t == timeType {
			return // mapped inline to string
		}
		g.decls[name] = "" // mark in-progress to break cycles
		g.decls[name] = g.structDecl(t)
	case reflect.String:
		g.decls[name] = fmt.Sprintf("export type %s = string;\n", name)
	case reflect.Bool:
		g.decls[name] = fmt.Sprintf("export type %s = boolean;\n", name)
	default:
		if isNumeric(t.Kind()) {
			g.decls[name] = fmt.Sprintf("export type %s = number;\n", name)
		}
	}
}

func (g *generator) structDecl(t reflect.Type) string {
	var b strings.Builder
	fmt.Fprintf(&b, "export interface %s {\n", t.Name())
	g.structFields(&b, t)
	b.WriteString("}\n")
	return b.String()
}

func (g *generator) structFields(b *strings.Builder, t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue // unexported
		}
		// Inline anonymous (embedded) struct fields; JSON encoding flattens them.
		if f.Anonymous {
			g.structFields(b, deref(f.Type))
			continue
		}
		jsonName, omit, skip := jsonField(f)
		if skip {
			continue
		}
		optional := omit || f.Type.Kind() == reflect.Ptr
		fmt.Fprintf(b, "  %s%s: %s;\n", jsonName, optChar(optional), g.tsType(f.Type))
	}
}

// tsType maps a Go type to its TS representation, enqueuing any named types it references.
func (g *generator) tsType(t reflect.Type) string {
	t = deref(t)
	if t == timeType {
		return "string"
	}
	if t == rawType {
		return "unknown"
	}
	switch t.Kind() {
	case reflect.Bool:
		if named(t) {
			g.walk(t)
			return t.Name()
		}
		return "boolean"
	case reflect.String:
		if named(t) {
			g.walk(t)
			return t.Name()
		}
		return "string"
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return "string" // []byte encodes as a base64 string
		}
		return g.tsType(t.Elem()) + "[]"
	case reflect.Map:
		return fmt.Sprintf("Record<%s, %s>", g.tsType(t.Key()), g.tsType(t.Elem()))
	case reflect.Struct:
		if named(t) {
			g.walk(t)
			return t.Name()
		}
		return "unknown"
	case reflect.Interface:
		return "unknown"
	default:
		if isNumeric(t.Kind()) {
			if named(t) {
				g.walk(t)
				return t.Name()
			}
			return "number"
		}
		return "unknown"
	}
}

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func named(t reflect.Type) bool { return t.Name() != "" && t.PkgPath() != "" }

func isNumeric(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func optChar(optional bool) string {
	if optional {
		return "?"
	}
	return ""
}

// jsonField returns the wire name, whether it is omitempty, and whether the field is skipped.
func jsonField(f reflect.StructField) (name string, omitempty, skip bool) {
	tag := f.Tag.Get("json")
	if tag == "" {
		return f.Name, false, false
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "-" {
		return "", false, true
	}
	name = parts[0]
	if name == "" {
		name = f.Name
	}
	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty, false
}
