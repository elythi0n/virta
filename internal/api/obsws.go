package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/elythi0n/virta/internal/obsws"
)

// OBSWSController is the interface the API expects from the OBS WebSocket manager.
type OBSWSController interface {
	Status() obsws.Status
	GetConfig(ctx context.Context) (obsws.Config, bool, error)
	SetConfig(ctx context.Context, cfg obsws.Config, password string) error
	GetSources(ctx context.Context) ([]obsws.SourceInfo, error)
	GetScenes(ctx context.Context) (obsws.SceneList, error)
	TestSource(ctx context.Context, sourceName, value string) error
}

// SetOBSWS installs the OBS WebSocket controller.
func (s *Server) SetOBSWS(c OBSWSController) { s.obsws = c }

func (s *Server) handleGetOBSStatus(w http.ResponseWriter, r *http.Request) {
	if s.obsws == nil {
		http.Error(w, "OBS WebSocket integration unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.obsws.Status())
}

func (s *Server) handleGetOBSConfig(w http.ResponseWriter, r *http.Request) {
	if s.obsws == nil {
		http.Error(w, "OBS WebSocket integration unavailable", http.StatusServiceUnavailable)
		return
	}
	cfg, hasPassword, err := s.obsws.GetConfig(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"config": cfg, "has_password": hasPassword})
}

func (s *Server) handlePutOBSConfig(w http.ResponseWriter, r *http.Request) {
	if s.obsws == nil {
		http.Error(w, "OBS WebSocket integration unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req struct {
		Config   obsws.Config `json:"config"`
		Password string       `json:"password,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := s.obsws.SetConfig(r.Context(), req.Config, req.Password); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetOBSSources(w http.ResponseWriter, r *http.Request) {
	if s.obsws == nil {
		http.Error(w, "OBS WebSocket integration unavailable", http.StatusServiceUnavailable)
		return
	}
	sources, err := s.obsws.GetSources(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if sources == nil {
		sources = []obsws.SourceInfo{}
	}
	writeJSON(w, map[string]any{"sources": sources})
}

func (s *Server) handleGetOBSScenes(w http.ResponseWriter, r *http.Request) {
	if s.obsws == nil {
		http.Error(w, "OBS WebSocket integration unavailable", http.StatusServiceUnavailable)
		return
	}
	scenes, err := s.obsws.GetScenes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, scenes)
}

func (s *Server) handlePostOBSTestSource(w http.ResponseWriter, r *http.Request) {
	if s.obsws == nil {
		http.Error(w, "OBS WebSocket integration unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req struct {
		SourceName string `json:"source_name"`
		Value      string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SourceName == "" {
		http.Error(w, "expected JSON with source_name", http.StatusBadRequest)
		return
	}
	if err := s.obsws.TestSource(r.Context(), req.SourceName, req.Value); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePostOBSDetect(w http.ResponseWriter, r *http.Request) {
	detected, err := obsws.Detect(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"detected": detected})
}
