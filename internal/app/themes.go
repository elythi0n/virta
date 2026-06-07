package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/elythi0n/virta/internal/api"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/uikit"
)

// themeControl manages built-in and custom themes, satisfying api.Themes. Custom themes are
// persisted in the settings repo (scope "themes.<id>") as raw .vtheme JSON, loaded at construction.
type themeControl struct {
	mu       sync.RWMutex
	tokens   *uikit.Tokens
	custom   map[string][]byte // id → .vtheme JSON
	settings store.SettingsRepo
}

func newThemeControl(settings store.SettingsRepo) api.Themes {
	tokensPath := "frontends/ui-kit/tokens.json"
	var tok *uikit.Tokens
	if data, err := os.ReadFile(tokensPath); err == nil {
		if t, err := uikit.Load(data); err == nil {
			tok = t
		}
	}
	if tok == nil {
		// Fallback: an empty token set — built-ins won't list but import still works.
		tok, _ = uikit.Load([]byte(`{"font":{"ui":"Geist Variable","mono":"Geist Mono Variable"},"type":{},"space":[],"radius":{"sm":5,"md":6,"lg":8},"motion":{"fast":120,"base":160},"platform":{},"themes":{"graphite-dark":{"appearance":"dark","color":{}}}}`))
	}
	c := &themeControl{tokens: tok, custom: map[string][]byte{}, settings: settings}
	// Load any previously-persisted custom themes.
	if all, err := settings.All(context.Background()); err == nil {
		for _, s := range all {
			if strings.HasPrefix(s.Scope, "themes.") {
				id := strings.TrimPrefix(s.Scope, "themes.")
				c.custom[id] = s.Data
			}
		}
	}
	return c
}

func (c *themeControl) List() []api.ThemeInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var list []api.ThemeInfo
	for _, name := range c.tokens.ThemeNames() {
		th := c.tokens.Themes[name]
		list = append(list, api.ThemeInfo{ID: name, Name: name, Appearance: th.Appearance})
	}
	for id, data := range c.custom {
		var vt struct {
			Name       string `json:"name"`
			Base       string `json:"base"`
			Appearance string `json:"appearance"`
		}
		if err := json.Unmarshal(data, &vt); err == nil {
			list = append(list, api.ThemeInfo{ID: id, Name: vt.Name, Base: vt.Base, Appearance: vt.Appearance})
		}
	}
	return list
}

func (c *themeControl) Import(data []byte) (api.ThemeInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	theme, warnings, err := c.tokens.LoadVTheme(data)
	if err != nil {
		return api.ThemeInfo{}, err
	}
	var vt struct{ Name, Base, Appearance string }
	_ = json.Unmarshal(data, &vt)
	id := strings.ToLower(strings.ReplaceAll(vt.Name, " ", "-"))
	if id == "" {
		return api.ThemeInfo{}, fmt.Errorf("vtheme has no name")
	}
	c.custom[id] = data
	_ = c.settings.Put(context.Background(), store.Setting{Scope: "themes." + id, Data: data})
	warnStrs := make([]string, len(warnings))
	for i, w := range warnings {
		warnStrs[i] = w.Key + ": " + w.Message
	}
	_ = theme
	return api.ThemeInfo{ID: id, Name: vt.Name, Base: vt.Base, Appearance: vt.Appearance, Warnings: warnStrs}, nil
}

func (c *themeControl) Export(id string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if data, ok := c.custom[id]; ok {
		return data, nil
	}
	// Export a built-in: marshal it as a full .vtheme with no overrides.
	th, ok := c.tokens.Themes[id]
	if !ok {
		return nil, fmt.Errorf("theme %q not found", id)
	}
	return uikit.MarshalVTheme(id, id, th.Appearance, th.Color, th.Color)
}

func (c *themeControl) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.custom[id]; !ok {
		return fmt.Errorf("custom theme %q not found (built-ins cannot be deleted)", id)
	}
	delete(c.custom, id)
	_ = c.settings.Put(context.Background(), store.Setting{Scope: "themes." + id, Data: []byte("null")})
	return nil
}
