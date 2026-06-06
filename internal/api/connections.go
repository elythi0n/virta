package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// Connections is the per-platform connection-method control, implemented by the profile manager
// and injected via SetConnections. A method is one of "automatic" | "anonymous" | "authenticated"
// | "session"; it belongs to the active profile, and setting it reconnects that platform's
// channels with the new mode.
type Connections interface {
	Methods() map[string]string
	SetMethod(ctx context.Context, platform, method string) error
}

var validMethods = map[string]struct{}{
	"":              {}, // clears the pin (back to automatic)
	"automatic":     {},
	"anonymous":     {},
	"authenticated": {},
	"session":       {},
}

// SetConnections installs the connection-method controller.
func (s *Server) SetConnections(c Connections) { s.connections = c }

func (s *Server) handleListMethods(w http.ResponseWriter, _ *http.Request) {
	if s.connections == nil {
		http.Error(w, "connections unavailable", http.StatusServiceUnavailable)
		return
	}
	methods := s.connections.Methods()
	if methods == nil {
		methods = map[string]string{}
	}
	writeJSON(w, map[string]any{"methods": methods})
}

func (s *Server) handleSetMethod(w http.ResponseWriter, r *http.Request) {
	if s.connections == nil {
		http.Error(w, "connections unavailable", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Platform string `json:"platform"`
		Method   string `json:"method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Platform == "" {
		http.Error(w, "expected JSON body with platform and method", http.StatusBadRequest)
		return
	}
	if _, ok := validMethods[req.Method]; !ok {
		http.Error(w, "unknown connection method", http.StatusBadRequest)
		return
	}
	if err := s.connections.SetMethod(r.Context(), req.Platform, req.Method); err != nil {
		if errors.Is(err, ErrUnknownPlatform) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"methods": s.connections.Methods()})
}
