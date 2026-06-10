// Package crash writes structured local-only crash dumps when the daemon panics, so post-mortem
// debugging is possible without any telemetry leaving the machine (no external reporting
// by default). A dump is a JSON file under RuntimeDir/crashes/ containing the goroutine stack,
// build info, and timestamp. A notice is printed to stderr pointing at the file.
//
// Usage: defer crash.Handle(runtimeDir) at the top of main. Each unrecovered panic writes one
// file; old files beyond MaxDumps are pruned so the directory never grows unbounded.
package crash

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"time"
)

const MaxDumps = 20

// Dump is the structure written to a crash file.
type Dump struct {
	Time      string `json:"time"`
	Build     string `json:"build"`
	GoVersion string `json:"go_version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	Panic     string `json:"panic,omitempty"`
	Stack     string `json:"stack"`
}

// Handle is intended to be deferred at the top of main. It recovers the panic, writes a dump,
// prints a notice to stderr, and re-panics so the OS marks the exit non-zero.
func Handle(runtimeDir string) {
	r := recover()
	if r == nil {
		return
	}
	panicStr := fmt.Sprintf("%v", r)
	stack := string(debug.Stack())
	d := Dump{
		Time:      time.Now().UTC().Format(time.RFC3339),
		GoVersion: runtime.Version(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		Panic:     panicStr,
		Stack:     stack,
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		d.Build = bi.Main.Version
	}
	dir := filepath.Join(runtimeDir, "crashes")
	_ = os.MkdirAll(dir, 0o700)
	name := filepath.Join(dir, "crash-"+time.Now().UTC().Format("20060102-150405")+".json")
	if b, err := json.MarshalIndent(d, "", "  "); err == nil {
		_ = os.WriteFile(name, b, 0o600)
	}
	pruneOld(dir)
	fmt.Fprintf(os.Stderr, "\n[virta] CRASHED: %s\n[virta] Crash report written to: %s\n[virta] No data has been sent anywhere — this file stays on your machine.\n\n", panicStr, name)
	panic(r) // re-panic so the process exits non-zero
}

// ListDumps returns the crash dump file paths in the runtimeDir, newest first.
func ListDumps(runtimeDir string) []string {
	dir := filepath.Join(runtimeDir, "crashes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	return paths
}

func pruneOld(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= MaxDumps {
		return
	}
	// Sort oldest first and remove the excess.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names[:len(names)-MaxDumps] {
		_ = os.Remove(filepath.Join(dir, n))
	}
}
