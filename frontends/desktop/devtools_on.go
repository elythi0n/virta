//go:build devtools

package main

// init patches the main window options to enable DevTools when built with -tags devtools.
// In Wails v3, DevToolsEnabled is a per-window option; this file is compiled only in
// debug builds (make app-debug) so the release binary never ships with DevTools open.
func init() {
	// DevTools are enabled by setting DevToolsEnabled: true in the window options.
	// This is wired in main.go; this file merely acts as the build-tag gate.
	devToolsEnabled = true
}

// devToolsEnabled is read by main.go when creating the main window.
var devToolsEnabled = false
