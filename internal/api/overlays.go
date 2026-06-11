package api

import (
	"context"
	"net/http"
)

// OverlayInfo describes one overlay instance contributed by a plugin.
type OverlayInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	PluginID string   `json:"plugin_id"`
	Types    []string `json:"types,omitempty"`
	URL      string   `json:"url,omitempty"` // tokenized URL, if token is available
}

// Overlays is the overlay registry surface. Implemented by the wiring layer; injected via SetOverlays.
type Overlays interface {
	List(ctx context.Context) ([]OverlayInfo, error)
}

// SetOverlays installs the overlay controller.
func (s *Server) SetOverlays(o Overlays) { s.overlays = o }

func (s *Server) handleListOverlays(w http.ResponseWriter, r *http.Request) {
	if s.overlays == nil {
		writeJSON(w, map[string]any{"overlays": []any{}})
		return
	}
	list, err := s.overlays.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []OverlayInfo{}
	}
	writeJSON(w, map[string]any{"overlays": list})
}
