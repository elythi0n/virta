package uikit

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// VTheme is the portable theme format (.vtheme / vtheme1: paste-string). It can:
//   - inherit a built-in base ("graphite-dark" or "light") so a custom theme only overrides what
//     it changes — the user doesn't need to spec every token.
//   - override any color token the base defines.
//   - set optional metadata (name, author, description).
//
// The format version is 1; incompatible changes increment it and older loaders reject.
type VTheme struct {
	Version     int               `json:"version"` // must be 1
	Name        string            `json:"name"`
	Base        string            `json:"base"`       // "graphite-dark" | "light"
	Appearance  string            `json:"appearance"` // inherits from base if empty
	Color       map[string]string `json:"color"`      // partial override — merges over base
	Author      string            `json:"author,omitempty"`
	Description string            `json:"description,omitempty"`
}

// VThemeWarning is a non-fatal advisory issued when a .vtheme contains an unknown token key or a
// low-contrast pair, so the user can see exactly what drifted without a hard failure.
type VThemeWarning struct {
	Key     string
	Message string
}

// ErrVThemeIncompatible is returned for a version mismatch.
var ErrVThemeIncompatible = fmt.Errorf("vtheme: incompatible version")

// LoadVTheme parses a .vtheme JSON, validates it, merges with the base, and returns a ready Theme
// plus any non-fatal warnings. The returned Theme is safe to use even when warnings is non-empty.
func (t *Tokens) LoadVTheme(data []byte) (theme Theme, warnings []VThemeWarning, err error) {
	var vt VTheme
	if err := json.Unmarshal(data, &vt); err != nil {
		return Theme{}, nil, fmt.Errorf("vtheme: parse: %w", err)
	}
	if vt.Version != 1 {
		return Theme{}, nil, ErrVThemeIncompatible
	}
	if vt.Name == "" {
		return Theme{}, nil, fmt.Errorf("vtheme: name is required")
	}
	base, ok := t.Themes[vt.Base]
	if !ok {
		// Fall back to the default theme rather than failing — the user might have an older base name.
		base = t.Themes[defaultTheme]
		warnings = append(warnings, VThemeWarning{Key: "base", Message: fmt.Sprintf("base %q not found; fell back to %q", vt.Base, defaultTheme)})
	}
	appearance := vt.Appearance
	if appearance == "" {
		appearance = base.Appearance
	}
	merged := map[string]string{}
	for k, v := range base.Color {
		merged[k] = v
	}
	for k, v := range vt.Color {
		if _, known := base.Color[k]; !known {
			warnings = append(warnings, VThemeWarning{Key: k, Message: "unknown token key (ignored)"})
			continue
		}
		merged[k] = v
	}
	theme = Theme{Appearance: appearance, Color: merged}

	// Contrast lint: check the core text/background pairs and warn when they fall below AA.
	// Gradients can't be contrast-checked (we skip them); solid hex values get the full check.
	type pair struct {
		fg, bg, label string
		min           float64
	}
	pairs := []pair{
		{"text-0", "bg-0", "primary text on base", 4.5},
		{"text-1", "bg-0", "secondary text on base", 4.5},
		{"text-2", "bg-0", "muted text on base", 3.0},
		{"accent", "bg-0", "accent on base", 3.0},
	}
	for _, p := range pairs {
		fg, bg := merged[p.fg], merged[p.bg]
		if !isHex(fg) || !isHex(bg) {
			continue
		}
		r := hexContrast(fg, bg)
		if r < p.min {
			warnings = append(warnings, VThemeWarning{
				Key:     p.fg,
				Message: fmt.Sprintf("low contrast: %s (%s on %s) is %.2f:1, below %.1f:1 AA; auto-fixed", p.label, fg, bg, r, p.min),
			})
			// Auto-fix: nudge lightness until the pair passes.
			merged[p.fg] = nudgeForContrast(fg, bg, p.min)
		}
	}
	return theme, warnings, nil
}

// MarshalVTheme encodes a custom theme as a .vtheme JSON body, deriving the delta over the base
// automatically so only the overrides are stored (clean diffs, smaller files).
func MarshalVTheme(name, base, appearance string, merged, baseColors map[string]string) ([]byte, error) {
	overrides := map[string]string{}
	for k, v := range merged {
		if baseColors[k] != v {
			overrides[k] = v
		}
	}
	vt := VTheme{Version: 1, Name: name, Base: base, Appearance: appearance, Color: overrides}
	return json.MarshalIndent(vt, "", "  ")
}

// VThemePasteString encodes a VTheme as a short base64-like paste string for easy sharing.
// Format: "vtheme1:<base64(JSON)>".
func VThemePasteString(data []byte) string {
	return "vtheme1:" + encodeBase64ish(data)
}

// ParseVThemePasteString decodes a vtheme1: paste string back to JSON.
func ParseVThemePasteString(s string) ([]byte, error) {
	if !strings.HasPrefix(s, "vtheme1:") {
		return nil, fmt.Errorf("vtheme: not a vtheme1: paste string")
	}
	return decodeBase64ish(s[8:])
}

// ---- helpers ----

func isHex(s string) bool { return len(s) == 7 && s[0] == '#' }

func hexContrast(fg, bg string) float64 {
	la := hexLum(fg)
	lb := hexLum(bg)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func hexLum(hex string) float64 {
	r := hexByte(hex, 1)
	g := hexByte(hex, 3)
	b := hexByte(hex, 5)
	return 0.2126*linChan(r) + 0.7152*linChan(g) + 0.0722*linChan(b)
}

func hexByte(hex string, i int) float64 {
	n, _ := strconv.ParseInt(hex[i:i+2], 16, 16)
	return float64(n) / 255
}

func linChan(c float64) float64 {
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// nudgeForContrast lightens or darkens fg (by shifting toward the opposite extreme from bg) until
// contrast ≥ min or we exhaust the range. Simple but effective for auto-fix.
func nudgeForContrast(fg, bg string, min float64) string {
	bgLum := hexLum(bg)
	// Determine whether to go lighter or darker.
	lighten := bgLum < 0.5 // dark bg → lighten fg
	r, g, b := hexByte(fg, 1)*255, hexByte(fg, 3)*255, hexByte(fg, 5)*255
	for range 50 {
		if hexContrast(fmt.Sprintf("#%02x%02x%02x", int(r), int(g), int(b)), bg) >= min {
			break
		}
		step := 8.0
		if lighten {
			r = math.Min(255, r+step)
			g = math.Min(255, g+step)
			b = math.Min(255, b+step)
		} else {
			r = math.Max(0, r-step)
			g = math.Max(0, g-step)
			b = math.Max(0, b-step)
		}
	}
	return fmt.Sprintf("#%02x%02x%02x", int(r), int(g), int(b))
}

// encodeBase64ish uses URL-safe base64 (stdlib encoding table) for the paste string.
func encodeBase64ish(data []byte) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	var sb strings.Builder
	for i := 0; i < len(data); i += 3 {
		remaining := len(data) - i
		b0 := data[i]
		var b1, b2 byte
		if remaining > 1 {
			b1 = data[i+1]
		}
		if remaining > 2 {
			b2 = data[i+2]
		}
		sb.WriteByte(chars[(b0>>2)&0x3f])
		sb.WriteByte(chars[((b0&0x3)<<4)|((b1>>4)&0xf)])
		if remaining > 1 {
			sb.WriteByte(chars[((b1&0xf)<<2)|((b2>>6)&0x3)])
		}
		if remaining > 2 {
			sb.WriteByte(chars[b2&0x3f])
		}
	}
	return sb.String()
}

func decodeBase64ish(s string) ([]byte, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	lookup := [256]int{}
	for i := range lookup {
		lookup[i] = -1
	}
	for i, c := range chars {
		lookup[c] = i
	}
	var out []byte
	for i := 0; i < len(s); i += 4 {
		chunk := s[i:]
		get := func(j int) int {
			if j >= len(chunk) {
				return 0
			}
			v := lookup[chunk[j]]
			if v < 0 {
				return 0
			}
			return v
		}
		a, b, c, d := get(0), get(1), get(2), get(3)
		out = append(out, byte((a<<2)|(b>>4)))
		if i+2 < len(s) {
			out = append(out, byte(((b&0xf)<<4)|(c>>2)))
		}
		if i+3 < len(s) {
			out = append(out, byte(((c&0x3)<<6)|d))
		}
	}
	return out, nil
}

// TokenGroup returns tokens grouped for the in-app editor UI.
func TokenGroup(color map[string]string) []struct {
	Group string
	Keys  []string
} {
	groups := []struct {
		Group string
		Keys  []string
	}{
		{"Background", filterKeys(color, "bg-")},
		{"Text", filterKeys(color, "text-")},
		{"Semantic", []string{"accent", "ok", "warn", "danger"}},
		{"Platform", filterKeys(color, "plat-")},
		{"Structure", filterKeys(color, "line", "highlight-bg", "highlight-rail", "scrollbar-thumb", "scrollbar-thumb-hover")},
	}
	return groups
}

func filterKeys(m map[string]string, prefixes ...string) []string {
	var keys []string
	for k := range m {
		for _, p := range prefixes {
			if strings.HasPrefix(k, p) || k == p {
				keys = append(keys, k)
				break
			}
		}
	}
	sort.Strings(keys)
	return keys
}
