package main

import "os"

// IntegrationFeature reports which rung of an OS-integration fallback chain (docs/14) is currently
// active, as a machine code the UI maps to user-facing copy. Detail is an optional machine reason
// (e.g. why a degraded rung is in effect), surfaced in diagnostics.
type IntegrationFeature struct {
	ID     string `json:"id"`
	Rung   string `json:"rung"`
	Detail string `json:"detail,omitempty"`
}

// IntegrationReport is the native-integration capability snapshot the desktop shell serves to its
// embedded UI (the "platform integration" settings panel renders it). The web-only build has no
// shell to serve it and substitutes an all-fallback default.
type IntegrationReport struct {
	OS       string               `json:"os"`
	Session  string               `json:"session,omitempty"` // "wayland" / "x11" / "" (non-Linux)
	Features []IntegrationFeature `json:"features"`
}

// resolveIntegration reports the active rung per feature for the given OS and session. It is the
// honest current state, not the eventual ceiling: features whose native path isn't wired yet report
// their working fallback rung (in-app banner, close-to-quit, visual flash), and bump up as the
// native implementations land. Pure (env is passed in) so the rung logic is unit-tested.
func resolveIntegration(goos, session string) IntegrationReport {
	wayland := goos == "linux" && session == "wayland"

	hotkeys := IntegrationFeature{ID: "hotkeys", Rung: "in_app", Detail: "not_implemented"}
	if wayland {
		hotkeys.Detail = "wayland_restricted"
	}
	window := IntegrationFeature{ID: "window", Rung: "native"}
	if wayland {
		window.Detail = "wayland_no_self_position"
	}

	return IntegrationReport{
		OS:      goos,
		Session: session,
		Features: []IntegrationFeature{
			window,
			{ID: "theme", Rung: "native"},         // system light/dark is followed
			{ID: "quicklaunch", Rung: "in_app"},   // Ctrl+K profile switcher is always present
			hotkeys,                               // in-app shortcuts; native/portal not yet wired
			{ID: "notifications", Rung: "in_app"}, // in-app banner; native toast not yet wired
			{ID: "tray", Rung: "none"},            // close-to-quit; native tray not yet wired
			{ID: "sounds", Rung: "visual"},        // visual flash; audio output not yet wired
		},
	}
}

// currentSession detects the windowing session from the environment (Linux): Wayland or X11. Empty
// on Windows/macOS, where the chain has no session split.
func currentSession() string {
	if t := os.Getenv("XDG_SESSION_TYPE"); t == "wayland" || t == "x11" {
		return t
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "wayland"
	}
	if os.Getenv("DISPLAY") != "" {
		return "x11"
	}
	return ""
}
