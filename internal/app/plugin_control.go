package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/api"
	pluginhost "github.com/elythi0n/virta/internal/plugin/host"
)

// pluginControl adapts the plugin Registry to the API's Plugins surface, including the
// config store, the sandboxed GUI server, and the scope-enforced HTTP bridge.
type pluginControl struct {
	reg       *pluginhost.Registry
	installer *pluginhost.Installer

	guiMu  sync.Mutex
	guis   map[string]*pluginhost.GUIPlugin // id@installDir → cached handler
	client *http.Client
}

func newPluginControl(reg *pluginhost.Registry, installer *pluginhost.Installer) *pluginControl {
	return &pluginControl{
		reg:       reg,
		installer: installer,
		guis:      map[string]*pluginhost.GUIPlugin{},
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

func infoFromEntry(e *pluginhost.Entry) api.PluginInfo {
	scopes := make([]string, 0, len(e.Manifest.Scopes))
	for _, s := range e.Manifest.Scopes {
		scopes = append(scopes, string(s))
	}
	panels := make([]api.PluginPanel, 0, len(e.Manifest.Contributes.Panels))
	for _, p := range e.Manifest.Contributes.Panels {
		panels = append(panels, api.PluginPanel{Kind: p.Kind, Title: p.Title, Icon: p.Icon})
	}
	return api.PluginInfo{
		ID:          e.Manifest.ID,
		Name:        e.Manifest.Name,
		Version:     e.Manifest.Version,
		Publisher:   e.Manifest.Publisher,
		Description: e.Manifest.Description,
		Tags:        e.Manifest.Tags,
		State:       string(e.State),
		Error:       e.Error,
		BuiltIn:     e.Manifest.BuiltIn,
		Scopes:      scopes,
		HasConfig:   len(e.Manifest.Config) > 0,
		Panels:      panels,
		HasGUI:      e.Manifest.Main.GUI != "",
	}
}

func (c *pluginControl) List() []api.PluginInfo {
	entries := c.reg.List()
	out := make([]api.PluginInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, infoFromEntry(e))
	}
	return out
}

func (c *pluginControl) Enable(id string) error {
	return c.reg.Enable(context.Background(), id)
}

func (c *pluginControl) Disable(id string) error {
	return c.reg.Disable(context.Background(), id)
}

func (c *pluginControl) Install(url string) (api.PluginInfo, error) {
	if c.installer == nil {
		return api.PluginInfo{}, fmt.Errorf("installer not configured")
	}
	result, err := c.installer.Install(context.Background(), url)
	if err != nil {
		return api.PluginInfo{}, err
	}
	if err := result.Manifest.Validate(); err != nil {
		return api.PluginInfo{}, fmt.Errorf("install: manifest validation failed: %w", err)
	}
	// Require explicit ScopeHTTP if the plugin contributes DataSources (they make network calls).
	if len(result.Manifest.Contributes.DataSources) > 0 && !result.Manifest.HasScope(pluginhost.ScopeHTTP) {
		return api.PluginInfo{}, fmt.Errorf(
			"install: plugin %q contributes DataSources but does not declare 'http' scope — installation rejected for safety",
			result.Manifest.ID,
		)
	}
	ctx := context.Background()
	if err := c.reg.RegisterRemote(ctx, result.Manifest, result.InstallDir); err != nil {
		return api.PluginInfo{}, err
	}
	if err := c.reg.Enable(ctx, result.Manifest.ID); err != nil {
		return api.PluginInfo{}, err
	}
	e, err := c.reg.Get(result.Manifest.ID)
	if err != nil {
		return api.PluginInfo{}, err
	}
	return infoFromEntry(e), nil
}

func (c *pluginControl) Uninstall(id string) error {
	e, err := c.reg.Get(id)
	if err != nil {
		return err
	}
	if e.Manifest.BuiltIn {
		return fmt.Errorf("built-in plugins cannot be uninstalled")
	}
	if err := c.reg.Remove(context.Background(), id); err != nil {
		return err
	}
	if c.installer != nil && e.InstallDir != "" {
		return c.installer.Uninstall(e.InstallDir)
	}
	return nil
}

// GetDetail returns the full plugin manifest including config schema and saved values.
func (c *pluginControl) GetDetail(id string) (api.PluginDetail, error) {
	e, err := c.reg.Get(id)
	if err != nil {
		return api.PluginDetail{}, err
	}
	detail := api.PluginDetail{PluginInfo: infoFromEntry(e)}
	if len(e.Manifest.Config) > 0 {
		var schema interface{}
		_ = json.Unmarshal(e.Manifest.Config, &schema) // ignore parse errors; nil schema is acceptable
		detail.ConfigSchema = schema
	}
	if len(e.Config) > 0 {
		var values interface{}
		_ = json.Unmarshal(e.Config, &values)
		detail.Config = values
	}
	return detail, nil
}

// GetConfig / SetConfig implement api.PluginConfigurer.
func (c *pluginControl) GetConfig(id string) (json.RawMessage, error) {
	return c.reg.GetConfig(id)
}

func (c *pluginControl) SetConfig(id string, cfg json.RawMessage) error {
	return c.reg.SetConfig(context.Background(), id, cfg)
}

// ── HTTP bridge (api.PluginProxier) ─────────────────────────────────────────

const proxyMaxBody = 2 << 20 // 2 MB bridged-response cap

// allowedPrefixes is the plugin's outbound allowlist: manifest-declared endpoints plus every
// http(s) URL the user saved in the plugin's config (configuring a base URL is consent to call it).
func allowedPrefixes(e *pluginhost.Entry) []string {
	prefixes := append([]string{}, e.Manifest.HTTPEndpoints...)
	if len(e.Config) > 0 {
		var values map[string]any
		if err := json.Unmarshal(e.Config, &values); err == nil {
			for _, v := range values {
				s, ok := v.(string)
				if ok && (strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://localhost") || strings.HasPrefix(s, "http://127.0.0.1")) {
					prefixes = append(prefixes, s)
				}
			}
		}
	}
	return prefixes
}

func urlAllowed(target string, prefixes []string) bool {
	for _, p := range prefixes {
		p = strings.TrimRight(p, "/")
		if p == "" {
			continue
		}
		if target == p || strings.HasPrefix(target, p+"/") || strings.HasPrefix(target, p+"?") {
			return true
		}
	}
	return false
}

func (c *pluginControl) ProxyHTTP(r *http.Request, id string, req api.PluginProxyRequest) (api.PluginProxyResponse, error) {
	e, err := c.reg.Get(id)
	if err != nil {
		return api.PluginProxyResponse{}, err
	}
	if e.State != pluginhost.StateEnabled {
		return api.PluginProxyResponse{}, fmt.Errorf("plugin %q is not enabled", id)
	}
	if !e.Manifest.HasScope(pluginhost.ScopeHTTP) {
		return api.PluginProxyResponse{}, fmt.Errorf("plugin %q does not declare the 'http' scope", id)
	}
	if !urlAllowed(req.URL, allowedPrefixes(e)) {
		return api.PluginProxyResponse{}, fmt.Errorf("url not allowed: %q is outside the plugin's declared endpoints", req.URL)
	}
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodPost {
		return api.PluginProxyResponse{}, fmt.Errorf("method %q not allowed (GET and POST only)", req.Method)
	}

	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}
	out, err := http.NewRequestWithContext(r.Context(), method, req.URL, body)
	if err != nil {
		return api.PluginProxyResponse{}, err
	}
	for k, v := range req.Headers {
		// The bridge never forwards ambient credentials; plugins set their own (e.g. an API key).
		if strings.EqualFold(k, "cookie") || strings.EqualFold(k, "host") {
			continue
		}
		out.Header.Set(k, v)
	}
	if out.Header.Get("User-Agent") == "" {
		out.Header.Set("User-Agent", "virta-plugin/"+id)
	}

	resp, err := c.client.Do(out)
	if err != nil {
		return api.PluginProxyResponse{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, proxyMaxBody))
	if err != nil {
		return api.PluginProxyResponse{}, err
	}
	return api.PluginProxyResponse{
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        string(data),
	}, nil
}

// ── Sandboxed GUI serving (api.PluginGUIServer) ──────────────────────────────

func (c *pluginControl) ServeGUI(w http.ResponseWriter, r *http.Request, id, rest string) {
	e, err := c.reg.Get(id)
	if err != nil || e.Manifest.Main.GUI == "" || e.InstallDir == "" {
		http.NotFound(w, r)
		return
	}
	if e.State != pluginhost.StateEnabled {
		http.Error(w, "plugin disabled", http.StatusForbidden)
		return
	}

	// The SDK bootstrap is generated, not a file: same-origin script so CSP script-src 'self' holds.
	if rest == "__virta.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(pluginhost.HostSDKBootstrap(id, e.Manifest.Scopes)))
		return
	}

	key := id + "@" + e.InstallDir
	c.guiMu.Lock()
	gui, ok := c.guis[key]
	if !ok {
		gui, err = pluginhost.NewGUIPlugin(e.Manifest, e.InstallDir)
		if err == nil {
			c.guis[key] = gui
		}
	}
	c.guiMu.Unlock()
	if err != nil || gui == nil {
		http.NotFound(w, r)
		return
	}

	// Serve relative to the plugin's GUI dir; empty rest falls through to index.html.
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/" + rest
	gui.ServeHTTP(w, r2)
}
