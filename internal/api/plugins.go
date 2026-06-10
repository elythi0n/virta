package api

import (
	"encoding/json"
	"net/http"
)

// PluginPanel is a panel contributed by a plugin (rendered by the generic sandboxed panel host).
type PluginPanel struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"`
}

// PluginInfo is the wire form of a registered plugin, served by GET /v1/plugins.
type PluginInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Publisher   string   `json:"publisher,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	State       string   `json:"state"` // enabled | disabled | error | installed
	Error       string   `json:"error,omitempty"`
	BuiltIn     bool     `json:"built_in"`
	Scopes      []string `json:"scopes,omitempty"`
	// HasConfig is true when the plugin declares a JSON Schema config.
	HasConfig bool `json:"has_config"`
	// Panels lists dock panels the plugin contributes. Remote plugins with a GUI entry render
	// through the sandboxed plugin panel host; built-ins render their own components.
	Panels []PluginPanel `json:"panels,omitempty"`
	// HasGUI is true when the plugin ships sandboxed GUI assets (main.gui).
	HasGUI bool `json:"has_gui,omitempty"`
}

// Plugins is the plugin management surface, implemented by the plugin host controller.
type Plugins interface {
	List() []PluginInfo
	Enable(id string) error
	Disable(id string) error
	Install(url string) (PluginInfo, error)
	Uninstall(id string) error
}

// SetPlugins installs the plugin controller.
func (s *Server) SetPlugins(p Plugins) { s.plugins = p }

func (s *Server) handleListPlugins(w http.ResponseWriter, _ *http.Request) {
	if s.plugins == nil {
		writeJSON(w, map[string]any{"plugins": []any{}})
		return
	}
	list := s.plugins.List()
	if list == nil {
		list = []PluginInfo{}
	}
	writeJSON(w, map[string]any{"plugins": list})
}

func (s *Server) handleEnablePlugin(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		http.Error(w, "plugin host unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing plugin id", http.StatusBadRequest)
		return
	}
	if err := s.plugins.Enable(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDisablePlugin(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		http.Error(w, "plugin host unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing plugin id", http.StatusBadRequest)
		return
	}
	if err := s.plugins.Disable(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInstallPlugin(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		http.Error(w, "plugin host unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, "expected JSON with url", http.StatusBadRequest)
		return
	}
	info, err := s.plugins.Install(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, info)
}

func (s *Server) handleUninstallPlugin(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		http.Error(w, "plugin host unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing plugin id", http.StatusBadRequest)
		return
	}
	if err := s.plugins.Uninstall(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PluginDetail extends PluginInfo with the full config schema for the settings form.
type PluginDetail struct {
	PluginInfo
	ConfigSchema interface{} `json:"config_schema,omitempty"`
	// Config holds the currently saved configuration values (object; absent when never saved).
	Config interface{} `json:"config,omitempty"`
}

// ── Plugin configuration values ──────────────────────────────────────────────

// PluginConfigurer is implemented by controllers that persist per-plugin config values.
type PluginConfigurer interface {
	GetConfig(id string) (json.RawMessage, error)
	SetConfig(id string, cfg json.RawMessage) error
}

func (s *Server) handleGetPluginConfig(w http.ResponseWriter, r *http.Request) {
	pc, ok := s.plugins.(PluginConfigurer)
	if s.plugins == nil || !ok {
		http.Error(w, "plugin config unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	cfg, err := pc.GetConfig(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"config":`))
	_, _ = w.Write(cfg)
	_, _ = w.Write([]byte(`}`))
}

func (s *Server) handlePutPluginConfig(w http.ResponseWriter, r *http.Request) {
	pc, ok := s.plugins.(PluginConfigurer)
	if s.plugins == nil || !ok {
		http.Error(w, "plugin config unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	var values map[string]any
	if err := json.NewDecoder(r.Body).Decode(&values); err != nil {
		http.Error(w, "expected a JSON object of config values", http.StatusBadRequest)
		return
	}
	raw, err := json.Marshal(values)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := pc.SetConfig(id, raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Plugin HTTP bridge ───────────────────────────────────────────────────────

// PluginProxyRequest is a plugin-initiated outbound HTTP request, executed daemon-side so the
// sandboxed GUI never makes (CORS-bound) network calls itself.
type PluginProxyRequest struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"` // GET (default) or POST
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// PluginProxyResponse is the bridged response.
type PluginProxyResponse struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type,omitempty"`
	Body        string `json:"body"`
}

// PluginProxier executes a bridged HTTP request on behalf of a plugin, enforcing the manifest's
// declared http_endpoints (plus origins from the plugin's saved config).
type PluginProxier interface {
	ProxyHTTP(r *http.Request, id string, req PluginProxyRequest) (PluginProxyResponse, error)
}

func (s *Server) handlePluginHTTP(w http.ResponseWriter, r *http.Request) {
	pp, ok := s.plugins.(PluginProxier)
	if s.plugins == nil || !ok {
		http.Error(w, "plugin http bridge unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req PluginProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, "expected JSON with url", http.StatusBadRequest)
		return
	}
	resp, err := pp.ProxyHTTP(r, id, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, resp)
}

// ── Plugin GUI serving ───────────────────────────────────────────────────────

// PluginGUIServer serves a plugin's sandboxed GUI assets (and the __virta.js SDK bootstrap).
type PluginGUIServer interface {
	ServeGUI(w http.ResponseWriter, r *http.Request, id, rest string)
}

func (s *Server) handlePluginGUI(w http.ResponseWriter, r *http.Request) {
	gs, ok := s.plugins.(PluginGUIServer)
	if s.plugins == nil || !ok {
		http.NotFound(w, r)
		return
	}
	gs.ServeGUI(w, r, r.PathValue("id"), r.PathValue("path"))
}

// Plugins interface extension — GetDetail returns manifest+schema for a single plugin.
// We use a type assertion so existing implementations don't break.
type PluginDetailer interface {
	GetDetail(id string) (PluginDetail, error)
}

func (s *Server) handleGetPlugin(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		http.Error(w, "plugin host unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing plugin id", http.StatusBadRequest)
		return
	}
	if pd, ok := s.plugins.(PluginDetailer); ok {
		detail, err := pd.GetDetail(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, detail)
		return
	}
	// Fallback: return summary only.
	for _, p := range s.plugins.List() {
		if p.ID == id {
			writeJSON(w, p)
			return
		}
	}
	http.Error(w, "plugin not found", http.StatusNotFound)
}
