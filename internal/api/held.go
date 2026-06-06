package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/elythi0n/virta/internal/platform"
)

// HeldFrom converts a platform held message into its API/wire form. The author prefers the
// display name; held-at is unix milliseconds, or 0 when the platform gave no timestamp.
func HeldFrom(m platform.HeldMessage) HeldMessage {
	author := m.Author.DisplayName
	if author == "" {
		author = m.Author.Login
	}
	var heldAt int64
	if !m.HeldAt.IsZero() {
		heldAt = m.HeldAt.UnixMilli()
	}
	return HeldMessage{
		ID:       m.ID,
		Channel:  m.Channel.Key(),
		Author:   author,
		Text:     m.Text,
		Reason:   m.Reason,
		HeldAtMs: heldAt,
	}
}

// HeldMessage is one message a platform's AutoMod is holding for moderator review, as served by
// GET /v1/held. The id is the platform's own handle, used to approve or deny it. Strings keep the
// API decoupled from the platform model (the wiring layer translates the channel ref).
type HeldMessage struct {
	ID       string `json:"id"`
	Channel  string `json:"channel"` // "platform:slug"
	Author   string `json:"author"`
	Text     string `json:"text"`
	Reason   string `json:"reason,omitempty"`
	HeldAtMs int64  `json:"held_at_ms"` // unix milliseconds; 0 when unknown
}

// Held is the moderation hold-queue control surface: list what AutoMod is holding, then approve
// (post) or deny (drop) a message by id. Implemented by the wiring layer over the held queue and
// the dispatch sender; injected via SetHeld.
type Held interface {
	// List returns the pending held messages across all joined channels, oldest first.
	List() []HeldMessage
	// Approve posts a held message; Deny drops it. Both report ErrHeldNotFound for an unknown id.
	Approve(ctx context.Context, id string) error
	Deny(ctx context.Context, id string) error
}

// ErrHeldNotFound is returned when an approve/deny names an id no longer in the queue (already
// resolved, aged out, or never held), so the API can answer 404 rather than 502.
var ErrHeldNotFound = errors.New("held message not found")

// SetHeld installs the hold-queue controller. Until called, the held endpoints report the
// feature as unavailable.
func (s *Server) SetHeld(c Held) { s.held = c }

func (s *Server) handleListHeld(w http.ResponseWriter, _ *http.Request) {
	if s.held == nil {
		http.Error(w, "held queue unavailable", http.StatusServiceUnavailable)
		return
	}
	list := s.held.List()
	if list == nil {
		list = []HeldMessage{}
	}
	writeJSON(w, map[string]any{"held": list})
}

func (s *Server) handleApproveHeld(w http.ResponseWriter, r *http.Request) {
	s.resolveHeld(w, r, true)
}

func (s *Server) handleDenyHeld(w http.ResponseWriter, r *http.Request) {
	s.resolveHeld(w, r, false)
}

func (s *Server) resolveHeld(w http.ResponseWriter, r *http.Request, approve bool) {
	if s.held == nil {
		http.Error(w, "held queue unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing held message id", http.StatusBadRequest)
		return
	}
	var err error
	if approve {
		err = s.held.Approve(r.Context(), id)
	} else {
		err = s.held.Deny(r.Context(), id)
	}
	if errors.Is(err, ErrHeldNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
