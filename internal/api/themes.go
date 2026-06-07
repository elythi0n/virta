package api

import (
	"encoding/json"
	"net/http"
)

// ThemeInfo describes a built-in or custom theme.
type ThemeInfo struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Base       string   `json:"base,omitempty"`
	Appearance string   `json:"appearance"`
	Warnings   []string `json:"warnings,omitempty"` // non-fatal lint notes
}

// Themes is the theme management surface. Implemented by the wiring layer; injected via SetThemes.
type Themes interface {
	List() []ThemeInfo
	Import(data []byte) (ThemeInfo, error)      // parse, lint, persist
	Export(id string) ([]byte, error)           // retrieve .vtheme JSON
	Delete(id string) error
}

// SetThemes installs the theme controller.
func (s *Server) SetThemes(t Themes) { s.themes = t }

func (s *Server) handleListThemes(w http.ResponseWriter, _ *http.Request) {
	if s.themes == nil {
		http.Error(w, "themes unavailable", http.StatusServiceUnavailable)
		return
	}
	list := s.themes.List()
	if list == nil {
		list = []ThemeInfo{}
	}
	writeJSON(w, map[string]any{"themes": list})
}

func (s *Server) handleImportTheme(w http.ResponseWriter, r *http.Request) {
	if s.themes == nil {
		http.Error(w, "themes unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	data, err := readBody(r)
	if err != nil || len(data) == 0 {
		http.Error(w, "expected .vtheme JSON body", http.StatusBadRequest)
		return
	}
	info, err := s.themes.Import(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, info)
}

func (s *Server) handleExportTheme(w http.ResponseWriter, r *http.Request) {
	if s.themes == nil {
		http.Error(w, "themes unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	data, err := s.themes.Export(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.vtheme"`)
	_, _ = w.Write(data)
}

func (s *Server) handleDeleteTheme(w http.ResponseWriter, r *http.Request) {
	if s.themes == nil {
		http.Error(w, "themes unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if err := s.themes.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func readBody(r *http.Request) ([]byte, error) {
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}
