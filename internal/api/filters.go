package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// FilterRule is one ordered filter rule on the wire (GET/PUT /v1/filters). It mirrors the engine's
// rule grammar with plain strings at the API boundary. Empty match categories don't constrain;
// within a category options are OR'd, across categories AND'd. Action is "hide" | "highlight" |
// "mask".
type FilterRule struct {
	ID        string   `json:"id"`
	Action    string   `json:"action"`
	Platforms []string `json:"platforms,omitempty"`
	Channels  []string `json:"channels,omitempty"`
	Authors   []string `json:"authors,omitempty"`
	Types     []string `json:"types,omitempty"`
	Keywords  []string `json:"keywords,omitempty"`
	Regexes   []string `json:"regexes,omitempty"`
}

// Filters is the filter-ruleset control surface, implemented by the profile manager and injected
// via SetFilters. The ruleset belongs to the active profile; setting it validates (a bad regex is
// rejected), hot-swaps the live ruleset, and persists.
type Filters interface {
	Filters() []FilterRule
	SetFilters(ctx context.Context, rules []FilterRule) error
}

// ErrInvalidRuleset is returned when a ruleset fails to compile (e.g. a bad regex), so the API can
// answer 400 rather than 500.
var ErrInvalidRuleset = errors.New("invalid filter ruleset")

// SetFilters installs the filter controller. Until set, the filter endpoints report unavailable.
func (s *Server) SetFilters(f Filters) { s.filters = f }

func (s *Server) handleListFilters(w http.ResponseWriter, _ *http.Request) {
	if s.filters == nil {
		http.Error(w, "filters unavailable", http.StatusServiceUnavailable)
		return
	}
	rules := s.filters.Filters()
	if rules == nil {
		rules = []FilterRule{}
	}
	writeJSON(w, map[string]any{"filters": rules})
}

func (s *Server) handleSetFilters(w http.ResponseWriter, r *http.Request) {
	if s.filters == nil {
		http.Error(w, "filters unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req struct {
		Filters []FilterRule `json:"filters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "expected JSON body with a filters array", http.StatusBadRequest)
		return
	}
	if err := s.filters.SetFilters(r.Context(), req.Filters); err != nil {
		if errors.Is(err, ErrInvalidRuleset) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"filters": s.filters.Filters()})
}
