package main

import "testing"

func rung(r IntegrationReport, id string) IntegrationFeature {
	for _, f := range r.Features {
		if f.ID == id {
			return f
		}
	}
	return IntegrationFeature{}
}

func TestResolveIntegration_LinuxWayland(t *testing.T) {
	r := resolveIntegration("linux", "wayland")
	if r.OS != "linux" || r.Session != "wayland" {
		t.Fatalf("report meta = %+v", r)
	}
	if h := rung(r, "hotkeys"); h.Rung != "in_app" || h.Detail != "wayland_restricted" {
		t.Errorf("hotkeys = %+v, want in_app/wayland_restricted", h)
	}
	if w := rung(r, "window"); w.Rung != "native" || w.Detail != "wayland_no_self_position" {
		t.Errorf("window = %+v, want native/wayland_no_self_position", w)
	}
}

func TestResolveIntegration_LinuxX11(t *testing.T) {
	r := resolveIntegration("linux", "x11")
	if h := rung(r, "hotkeys"); h.Detail != "not_implemented" {
		t.Errorf("x11 hotkeys detail = %q, want not_implemented", h.Detail)
	}
	if w := rung(r, "window"); w.Detail != "" {
		t.Errorf("x11 window detail = %q, want empty", w.Detail)
	}
}

func TestResolveIntegration_Windows(t *testing.T) {
	r := resolveIntegration("windows", "")
	if r.Session != "" {
		t.Errorf("windows session = %q, want empty", r.Session)
	}
	// Fallback rungs hold cross-platform until the native paths are wired.
	for id, want := range map[string]string{"tray": "none", "notifications": "in_app", "sounds": "visual", "theme": "native"} {
		if got := rung(r, id).Rung; got != want {
			t.Errorf("%s rung = %q, want %q", id, got, want)
		}
	}
}

func TestResolveIntegration_EveryFeaturePresent(t *testing.T) {
	r := resolveIntegration("darwin", "")
	want := []string{"window", "theme", "quicklaunch", "hotkeys", "notifications", "tray", "sounds"}
	if len(r.Features) != len(want) {
		t.Fatalf("feature count = %d, want %d", len(r.Features), len(want))
	}
	for _, id := range want {
		if rung(r, id).ID == "" {
			t.Errorf("missing feature %q", id)
		}
	}
}
