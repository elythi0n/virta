package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/elythi0n/virta/internal/platform"
)

// Moments is the read/manage surface over detected hype moments. Implemented by the wiring
// layer over the store; injected via SetMoments.
type Moments interface {
	List(ctx context.Context, channel, before string, limit int) ([]platform.Moment, error)
	Delete(ctx context.Context, id string) error
}

// SetMoments installs the moments controller. Until called, the endpoints report unavailable.
func (s *Server) SetMoments(m Moments) { s.moments = m }

// parseMomentLimit reads a positive limit from the query, defaulting to 50 and capping at 200.
func parseMomentLimit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}

func (s *Server) handleListMoments(w http.ResponseWriter, r *http.Request) {
	if s.moments == nil {
		http.Error(w, "moments unavailable", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	list, err := s.moments.List(r.Context(), q.Get("channel"), q.Get("before"), parseMomentLimit(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []platform.Moment{}
	}
	writeJSON(w, map[string]any{"moments": list})
}

func (s *Server) handleDeleteMoment(w http.ResponseWriter, r *http.Request) {
	if s.moments == nil {
		http.Error(w, "moments unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.moments.Delete(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
