package uikit

import (
	"strings"
	"testing"
)

func tokens(t *testing.T) *Tokens {
	t.Helper()
	b, err := loadTestTokens()
	if err != nil {
		t.Fatalf("load tokens: %v", err)
	}
	return b
}

func loadTestTokens() (*Tokens, error) {
	import_json := []byte(`{
		"font":{"ui":"Geist Variable","mono":"Geist Mono Variable"},
		"type":{"ui":{"size":13,"line":20,"weight":400}},
		"space":[4,8],"radius":{"sm":5,"md":6,"lg":8},"motion":{"fast":120,"base":160},
		"platform":{"twitch":"#9146FF"},
		"themes":{
			"graphite-dark":{
				"appearance":"dark",
				"color":{
					"bg-0":"#0E0F12","bg-1":"#15171C","bg-2":"#1C1F26","line":"#262A33",
					"text-0":"#E8EAF0","text-1":"#9AA1AE","text-2":"#5C6370",
					"accent":"#5B8CFF","ok":"#3FB950","warn":"#D29922","danger":"#F85149",
					"highlight-bg":"#3D2E12","highlight-rail":"#D29922","plat-x":"#E7E9EA",
					"scrollbar-thumb":"#2F333D","scrollbar-thumb-hover":"#3D4350"
				}
			},
			"light":{
				"appearance":"light",
				"color":{
					"bg-0":"#FAFBFC","bg-1":"#F1F3F5","bg-2":"#E7EAED","line":"#D5D9DF",
					"text-0":"#1A1D23","text-1":"#5C6370","text-2":"#7C8492",
					"accent":"#3B6FE0","ok":"#1A7F37","warn":"#9A6700","danger":"#CF222E",
					"highlight-bg":"#FFF8C5","highlight-rail":"#9A6700","plat-x":"#0F1419",
					"scrollbar-thumb":"#D0D7DE","scrollbar-thumb-hover":"#B0B8C1"
				}
			}
		}
	}`)
	return Load(import_json)
}

func TestVTheme_BasicRoundTrip(t *testing.T) {
	tok := tokens(t)
	data := []byte(`{"version":1,"name":"Midnight Purple","base":"graphite-dark","color":{"accent":"#A855F7"}}`)
	theme, warnings, err := tok.LoadVTheme(data)
	if err != nil {
		t.Fatalf("LoadVTheme: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if theme.Color["accent"] != "#A855F7" {
		t.Errorf("accent = %q, want #A855F7", theme.Color["accent"])
	}
	// Base tokens should be inherited.
	if theme.Color["bg-0"] != "#0E0F12" {
		t.Errorf("bg-0 not inherited: %q", theme.Color["bg-0"])
	}
	if theme.Appearance != "dark" {
		t.Errorf("appearance = %q, want dark", theme.Appearance)
	}
}

func TestVTheme_UnknownTokenWarns(t *testing.T) {
	tok := tokens(t)
	data := []byte(`{"version":1,"name":"Test","base":"graphite-dark","color":{"nonexistent":"#fff"}}`)
	_, warnings, err := tok.LoadVTheme(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, w := range warnings {
		if w.Key == "nonexistent" {
			found = true
		}
	}
	if !found {
		t.Error("expected warning for unknown token key")
	}
}

func TestVTheme_LowContrastAutoFix(t *testing.T) {
	tok := tokens(t)
	// text-0 = near-black on a dark background = low contrast (intentionally bad).
	data := []byte(`{"version":1,"name":"BadContrast","base":"graphite-dark","color":{"text-0":"#1A1D23"}}`)
	theme, warnings, err := tok.LoadVTheme(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hasFix := false
	for _, w := range warnings {
		if w.Key == "text-0" && strings.Contains(w.Message, "auto-fixed") {
			hasFix = true
		}
	}
	if !hasFix {
		t.Errorf("expected auto-fix warning for low-contrast text-0, got %v", warnings)
	}
	// After fix, text-0 should now pass 4.5:1.
	if r := hexContrast(theme.Color["text-0"], theme.Color["bg-0"]); r < 4.5 {
		t.Errorf("auto-fixed text-0 contrast = %.2f, still below 4.5", r)
	}
}

func TestVTheme_WrongVersionRejected(t *testing.T) {
	tok := tokens(t)
	data := []byte(`{"version":2,"name":"Future","base":"graphite-dark","color":{}}`)
	if _, _, err := tok.LoadVTheme(data); err == nil {
		t.Error("expected error for version 2")
	}
}

func TestVThemePasteString_RoundTrip(t *testing.T) {
	original := []byte(`{"version":1,"name":"Night Sky","base":"graphite-dark","color":{"accent":"#C084FC"}}`)
	paste := VThemePasteString(original)
	if !strings.HasPrefix(paste, "vtheme1:") {
		t.Fatalf("paste does not start with vtheme1:, got %q", paste[:12])
	}
	decoded, err := ParseVThemePasteString(paste)
	if err != nil {
		t.Fatalf("ParseVThemePasteString: %v", err)
	}
	if string(decoded) != string(original) {
		t.Errorf("round-trip mismatch: got %q", decoded)
	}
}

func TestMarshalVTheme_OnlyStoredDelta(t *testing.T) {
	base := map[string]string{"bg-0": "#0E0F12", "accent": "#5B8CFF", "text-0": "#E8EAF0"}
	merged := map[string]string{"bg-0": "#0E0F12", "accent": "#A855F7", "text-0": "#E8EAF0"} // only accent changed
	data, err := MarshalVTheme("Custom", "graphite-dark", "dark", merged, base)
	if err != nil {
		t.Fatalf("MarshalVTheme: %v", err)
	}
	if !strings.Contains(string(data), "A855F7") {
		t.Error("expected delta accent in output")
	}
	if strings.Contains(string(data), "E8EAF0") {
		t.Error("unchanged text-0 should not appear in delta")
	}
}
