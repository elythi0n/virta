package api

import (
	"context"
	"net/http"
	"strconv"
)

// LoggedMessage is one persisted message returned by search or history. Logging is opt-in, so these
// endpoints are empty until the user turns it on. Channel is "platform:slug"; the body is the
// rendered plain text the search index matches.
type LoggedMessage struct {
	ID       string `json:"id"`
	Channel  string `json:"channel"`
	Platform string `json:"platform"`
	Author   string `json:"author"`
	Body     string `json:"body"`
	SentAtMs int64  `json:"sent_at_ms"`
	Deleted  bool   `json:"deleted,omitempty"`
}

// SearchParams is a full-text query over the log. Text is required; the rest narrow it.
type SearchParams struct {
	Text    string
	Channel string // "platform:slug"; "" = every logged channel
	Author  string // "" = any
	Before  string // ULID cursor for paging
	Limit   int
}

// HistoryParams pages a single channel's log newest-first.
type HistoryParams struct {
	Channel string // "platform:slug"; required
	Before  string // ULID cursor for paging
	Limit   int
}

// History is the read surface over the message log: full-text Search and per-channel scrollback.
// Implemented by the wiring layer over the store; injected via SetHistory.
type History interface {
	Search(ctx context.Context, p SearchParams) ([]LoggedMessage, error)
	History(ctx context.Context, p HistoryParams) ([]LoggedMessage, error)
}

// SetHistory installs the log read controller. Until called, the endpoints report unavailable.
func (s *Server) SetHistory(c History) { s.history = c }

// parseLimit reads a positive limit from the query, defaulting to 100 and capping at 500.
func parseLimit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 {
		return 100
	}
	if n > 500 {
		return 500
	}
	return n
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		http.Error(w, "search unavailable", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	text := q.Get("q")
	if text == "" {
		http.Error(w, "missing q parameter", http.StatusBadRequest)
		return
	}
	msgs, err := s.history.Search(r.Context(), SearchParams{
		Text:    text,
		Channel: q.Get("channel"),
		Author:  q.Get("author"),
		Before:  q.Get("before"),
		Limit:   parseLimit(r),
	})
	if err != nil {
		s.channelError(w, err)
		return
	}
	if msgs == nil {
		msgs = []LoggedMessage{}
	}
	writeJSON(w, map[string]any{"messages": msgs})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		http.Error(w, "history unavailable", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	channel := q.Get("channel")
	if channel == "" {
		http.Error(w, "missing channel parameter", http.StatusBadRequest)
		return
	}
	msgs, err := s.history.History(r.Context(), HistoryParams{
		Channel: channel,
		Before:  q.Get("before"),
		Limit:   parseLimit(r),
	})
	if err != nil {
		s.channelError(w, err)
		return
	}
	if msgs == nil {
		msgs = []LoggedMessage{}
	}
	writeJSON(w, map[string]any{"messages": msgs})
}
