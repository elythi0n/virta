package uikit

import (
	"os"
	"strings"
	"testing"
)

// realTokens loads the committed token source, so these tests also validate that the source file
// itself is well-formed and within the design rules.
func realTokens(t *testing.T) *Tokens {
	t.Helper()
	b, err := os.ReadFile("../../frontends/ui-kit/tokens.json")
	if err != nil {
		t.Fatalf("read tokens.json: %v", err)
	}
	tk, err := Load(b)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return tk
}

func TestCSS_EmitsTokens(t *testing.T) {
	css := realTokens(t).CSS()
	for _, want := range []string{
		`--virta-font-ui: "Geist Variable"`,
		`--virta-font-mono: "Geist Mono Variable"`,
		"--virta-space-8: 8px;",
		"--virta-radius-sm: 5px;",
		"--virta-radius-lg: 8px;",
		"--virta-motion-base: 160ms;",
		"--virta-plat-twitch: #9146FF;",
		"--virta-bg-0: #0E0F12;", // default (graphite-dark) colors at :root
		`[data-theme="light"]`,
		"--virta-bg-0: #FAFBFC;", // light override
	} {
		if !strings.Contains(css, want) {
			t.Errorf("CSS missing %q", want)
		}
	}
}

func TestTS_EmitsConstsAndThemeUnion(t *testing.T) {
	ts := realTokens(t).TS()
	for _, want := range []string{
		`export const fonts = { ui: "Geist Variable", mono: "Geist Mono Variable" } as const;`,
		"export const space = [2, 4, 8, 12, 16, 24, 32] as const;",
		`"bg-0": "var(--virta-bg-0)"`,
		`"plat-twitch": "var(--virta-plat-twitch)"`,
		`export type ThemeName = "graphite-dark" | "light";`,
	} {
		if !strings.Contains(ts, want) {
			t.Errorf("TS missing %q", want)
		}
	}
}

func TestLoad_RejectsRadiusOverCeiling(t *testing.T) {
	bad := `{"font":{"ui":"Geist","mono":"Geist Mono"},"radius":{"lg":12},` +
		`"themes":{"graphite-dark":{"appearance":"dark","color":{}}}}`
	if _, err := Load([]byte(bad)); err == nil {
		t.Error("radius over the 8px ceiling should be rejected")
	}
}

func TestLoad_RequiresDefaultTheme(t *testing.T) {
	bad := `{"font":{"ui":"Geist","mono":"Geist Mono"},"themes":{"light":{"appearance":"light","color":{}}}}`
	if _, err := Load([]byte(bad)); err == nil {
		t.Error("missing the default theme should be rejected")
	}
}
