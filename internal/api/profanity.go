package api

import (
	"context"
	"encoding/json"
	"net/http"
)

// Profanity is the profanity-masking toggle surface. Implemented by the wiring layer; injected
// via SetProfanity.
type Profanity interface {
	Enabled() bool
	SetEnabled(ctx context.Context, enabled bool) error
}

// SetProfanity installs the profanity controller.
func (s *Server) SetProfanity(p Profanity) { s.profanity = p }

func (s *Server) handleGetProfanity(w http.ResponseWriter, _ *http.Request) {
	if s.profanity == nil {
		http.Error(w, "profanity filter unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"enabled": s.profanity.Enabled()})
}

func (s *Server) handleSetProfanity(w http.ResponseWriter, r *http.Request) {
	if s.profanity == nil {
		http.Error(w, "profanity filter unavailable", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "expected JSON with enabled", http.StatusBadRequest)
		return
	}
	if err := s.profanity.SetEnabled(r.Context(), req.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"enabled": req.Enabled})
}
