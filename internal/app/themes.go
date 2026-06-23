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
	"github.com/elythi0n/virta/internal/userctx"
)

// themeControl manages built-in and custom themes, satisfying api.Themes.
//
// Built-in themes (loaded from ui-kit tokens.json) are global — they're code, not user data.
// Custom themes a user imports via .vtheme are persisted in the settings repo (scope
// "themes.<id>") and live in a per-user in-memory map so user A can't list/export/delete user
// B's custom themes. Single-user mode keeps the empty user_id, so existing themes continue to
// load as before.
type themeControl struct {
	mu       sync.RWMutex
	tokens   *uikit.Tokens
	// custom maps user_id → { theme_id → .vtheme JSON }. Two users can independently import a
	// theme with the same id without colliding.
	custom   map[string]map[string][]byte
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
	c := &themeControl{tokens: tok, custom: map[string]map[string][]byte{}, settings: settings}
	// Reload persisted custom themes across every user. EachUser yields '' for the single-user
	// namespace, which carries every theme a pre-hosted install ever imported.
	_ = settings.EachUser(context.Background(), func(userID string) error {
		all, err := settings.AllForUser(context.Background(), userID)
		if err != nil {
			return nil
		}
		for _, s := range all {
			if !strings.HasPrefix(s.Scope, "themes.") {
				continue
			}
			if len(s.Data) == 0 || string(s.Data) == "null" {
				continue
			}
			id := strings.TrimPrefix(s.Scope, "themes.")
			c.userThemes(userID)[id] = s.Data
		}
		return nil
	})
	return c
}

// userThemes returns the per-user custom-theme map, creating an empty one on first access.
// Caller must hold c.mu (write lock for mutations).
func (c *themeControl) userThemes(userID string) map[string][]byte {
	m, ok := c.custom[userID]
	if !ok {
		m = map[string][]byte{}
		c.custom[userID] = m
	}
	return m
}

func (c *themeControl) List(ctx context.Context) []api.ThemeInfo {
	user := userctx.FromContext(ctx)
	c.mu.RLock()
	defer c.mu.RUnlock()
	var list []api.ThemeInfo
	for _, name := range c.tokens.ThemeNames() {
		th := c.tokens.Themes[name]
		list = append(list, api.ThemeInfo{ID: name, Name: name, Appearance: th.Appearance})
	}
	for id, data := range c.custom[user] {
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

func (c *themeControl) Import(ctx context.Context, data []byte) (api.ThemeInfo, error) {
	user := userctx.FromContext(ctx)
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
	c.userThemes(user)[id] = data
	_ = c.settings.Put(ctx, store.Setting{Scope: "themes." + id, Data: data})
	warnStrs := make([]string, len(warnings))
	for i, w := range warnings {
		warnStrs[i] = w.Key + ": " + w.Message
	}
	_ = theme
	return api.ThemeInfo{ID: id, Name: vt.Name, Base: vt.Base, Appearance: vt.Appearance, Warnings: warnStrs}, nil
}

func (c *themeControl) Export(ctx context.Context, id string) ([]byte, error) {
	user := userctx.FromContext(ctx)
	c.mu.RLock()
	defer c.mu.RUnlock()
	if data, ok := c.custom[user][id]; ok {
		return data, nil
	}
	// Export a built-in: built-ins are global, so any user can export them. Marshal as a full
	// .vtheme with no overrides.
	th, ok := c.tokens.Themes[id]
	if !ok {
		return nil, fmt.Errorf("theme %q not found", id)
	}
	return uikit.MarshalVTheme(id, id, th.Appearance, th.Color, th.Color)
}

func (c *themeControl) Delete(ctx context.Context, id string) error {
	user := userctx.FromContext(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.custom[user][id]; !ok {
		return fmt.Errorf("custom theme %q not found (built-ins cannot be deleted)", id)
	}
	delete(c.custom[user], id)
	_ = c.settings.Put(ctx, store.Setting{Scope: "themes." + id, Data: []byte("null")})
	return nil
}
