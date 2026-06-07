package api

import (
	"encoding/json"
	"net/http"
)

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
