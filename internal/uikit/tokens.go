// Package uikit turns the design-system token source (frontends/ui-kit/tokens.json) into the
// generated artifacts every surface consumes: CSS custom properties for the web/desktop UI and
// typed constants for TypeScript. It is the one place the tokens are interpreted, so the design
// system has a single source of truth. The cmd/tokengen binary wires this to the filesystem.
package uikit

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// maxRadius is the hard ceiling on corner rounding: the app reads modern, not bubbly.
const maxRadius = 8

// fallback font stacks appended after the chosen family, so text renders before the bundled
// font loads and on systems missing it.
const (
	uiFallback   = `system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`
	monoFallback = `ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`
)

// TypeRole is one typographic role's size, line height, and weight (in px / unitless).
type TypeRole struct {
	Size   int `json:"size"`
	Line   int `json:"line"`
	Weight int `json:"weight"`
}

// Theme is a named set of color tokens plus its light/dark appearance.
type Theme struct {
	Appearance string            `json:"appearance"`
	Color      map[string]string `json:"color"`
}

// Tokens is the parsed token source.
type Tokens struct {
	Font     map[string]string   `json:"font"`
	Type     map[string]TypeRole `json:"type"`
	Space    []int               `json:"space"`
	Radius   map[string]int      `json:"radius"`
	Motion   map[string]int      `json:"motion"`
	Platform map[string]string   `json:"platform"`
	Themes   map[string]Theme    `json:"themes"`
}

// defaultTheme is applied at :root so the app has a theme with no data-theme attribute set.
const defaultTheme = "graphite-dark"

// Load parses and validates the token source.
func Load(b []byte) (*Tokens, error) {
	var t Tokens
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("uikit: parse tokens: %w", err)
	}
	if t.Font["ui"] == "" || t.Font["mono"] == "" {
		return nil, fmt.Errorf("uikit: font.ui and font.mono are required")
	}
	for name, r := range t.Radius {
		if r > maxRadius {
			return nil, fmt.Errorf("uikit: radius %q is %dpx, over the %dpx ceiling", name, r, maxRadius)
		}
	}
	if _, ok := t.Themes[defaultTheme]; !ok {
		return nil, fmt.Errorf("uikit: missing the default theme %q", defaultTheme)
	}
	return &t, nil
}

// ThemeNames returns the theme names sorted, default first.
func (t *Tokens) ThemeNames() []string {
	names := make([]string, 0, len(t.Themes))
	for n := range t.Themes {
		if n != defaultTheme {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return append([]string{defaultTheme}, names...)
}

// CSS renders the tokens as CSS custom properties: theme-independent tokens (fonts, type, spacing,
// radii, motion, platform colors) at :root, the default theme's colors at :root, and every theme's
// colors under a [data-theme="…"] selector for explicit selection.
func (t *Tokens) CSS() string {
	var b strings.Builder
	b.WriteString("/* Generated from tokens.json by cmd/tokengen. Do not edit by hand. */\n")
	b.WriteString(":root {\n")
	fmt.Fprintf(&b, "  --virta-font-ui: %q, %s;\n", t.Font["ui"], uiFallback)
	fmt.Fprintf(&b, "  --virta-font-mono: %q, %s;\n", t.Font["mono"], monoFallback)
	for _, role := range sortedKeys(typeKeys(t.Type)) {
		r := t.Type[role]
		fmt.Fprintf(&b, "  --virta-type-%s-size: %dpx;\n", role, r.Size)
		fmt.Fprintf(&b, "  --virta-type-%s-line: %dpx;\n", role, r.Line)
		if r.Weight != 0 {
			fmt.Fprintf(&b, "  --virta-type-%s-weight: %d;\n", role, r.Weight)
		}
	}
	for _, px := range t.Space {
		fmt.Fprintf(&b, "  --virta-space-%d: %dpx;\n", px, px)
	}
	for _, k := range sortedKeys(intKeys(t.Radius)) {
		fmt.Fprintf(&b, "  --virta-radius-%s: %dpx;\n", k, t.Radius[k])
	}
	for _, k := range sortedKeys(intKeys(t.Motion)) {
		fmt.Fprintf(&b, "  --virta-motion-%s: %dms;\n", k, t.Motion[k])
	}
	for _, k := range sortedKeys(strKeys(t.Platform)) {
		fmt.Fprintf(&b, "  --virta-plat-%s: %s;\n", k, t.Platform[k])
	}
	// Default theme colors live at :root too.
	writeColors(&b, t.Themes[defaultTheme].Color)
	b.WriteString("}\n")
	for _, name := range t.ThemeNames() {
		fmt.Fprintf(&b, "[data-theme=%q] {\n", name)
		writeColors(&b, t.Themes[name].Color)
		b.WriteString("}\n")
	}
	return b.String()
}

func writeColors(b *strings.Builder, color map[string]string) {
	for _, k := range sortedKeys(strKeys(color)) {
		fmt.Fprintf(b, "  --virta-%s: %s;\n", k, color[k])
	}
}

// TS renders typed constants and the theme-name union for TypeScript consumers.
func (t *Tokens) TS() string {
	var b strings.Builder
	b.WriteString("// Generated from tokens.json by cmd/tokengen. Do not edit by hand.\n\n")
	fmt.Fprintf(&b, "export const fonts = { ui: %q, mono: %q } as const;\n\n", t.Font["ui"], t.Font["mono"])

	b.WriteString("export const space = [")
	for i, px := range t.Space {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%d", px)
	}
	b.WriteString("] as const;\n\n")

	b.WriteString("export const radius = {")
	for i, k := range sortedKeys(intKeys(t.Radius)) {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, " %s: %d", k, t.Radius[k])
	}
	b.WriteString(" } as const;\n\n")

	// Every color token as a CSS-var reference, so components reference tokens by name in TS too.
	b.WriteString("export const color = {\n")
	for _, k := range sortedKeys(strKeys(t.Themes[defaultTheme].Color)) {
		fmt.Fprintf(&b, "  %q: %q,\n", k, "var(--virta-"+k+")")
	}
	for _, k := range sortedKeys(strKeys(t.Platform)) {
		fmt.Fprintf(&b, "  %q: %q,\n", "plat-"+k, "var(--virta-plat-"+k+")")
	}
	b.WriteString("} as const;\n\n")

	b.WriteString("export type ThemeName =")
	for i, name := range t.ThemeNames() {
		if i > 0 {
			b.WriteString(" |")
		}
		fmt.Fprintf(&b, " %q", name)
	}
	b.WriteString(";\n")
	return b.String()
}

func typeKeys(m map[string]TypeRole) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
func intKeys(m map[string]int) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
func strKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
func sortedKeys(ks []string) []string {
	sort.Strings(ks)
	return ks
}
