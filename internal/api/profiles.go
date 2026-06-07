package api

import (
	"context"
	"encoding/json"
	"net/http"
)

// Profiles is the workspace-profile control surface, implemented by the profile manager and
// injected via SetProfiles. Strings at the boundary keep the API decoupled from the manager.
type Profiles interface {
	List(ctx context.Context) ([]ProfileInfo, error)
	Create(ctx context.Context, name string) (ProfileInfo, error)
	Activate(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

// ProfileInfo is a profile's summary, as served by GET /v1/profiles.
type ProfileInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Active  bool   `json:"active"`
	Default bool   `json:"default"`
}

// SetProfiles installs the profile controller (wiring layer, after the manager exists).
func (s *Server) SetProfiles(p Profiles) { s.profiles = p }

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	if s.profiles == nil {
		http.Error(w, "profiles unavailable", http.StatusServiceUnavailable)
		return
	}
	list, err := s.profiles.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []ProfileInfo{}
	}
	writeJSON(w, map[string]any{"profiles": list})
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	if s.profiles == nil {
		http.Error(w, "profiles unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "expected JSON body with a name", http.StatusBadRequest)
		return
	}
	p, err := s.profiles.Create(r.Context(), req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, p)
}

func (s *Server) handleActivateProfile(w http.ResponseWriter, r *http.Request) {
	if s.profiles == nil {
		http.Error(w, "profiles unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing profile id", http.StatusBadRequest)
		return
	}
	if err := s.profiles.Activate(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	if s.profiles == nil {
		http.Error(w, "profiles unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing profile id", http.StatusBadRequest)
		return
	}
	if err := s.profiles.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
