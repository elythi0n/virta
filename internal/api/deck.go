package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Deck is the live-broadcast control surface: push title/category/tags across a list of
// channels in one call, and resolve a free-text game name to per-platform category ids.
// Implemented by the wiring layer (which fans out to each platform's adapter) and injected via
// SetDeck; strings keep the API decoupled from the platform model.
type Deck interface {
	// GetChannelInfo reads each target's currently-set stream info, so the Deck can pre-fill its
	// form with what the streamer already has live. Targets the engine isn't authenticated for
	// come back as excluded with an empty state.
	GetChannelInfo(ctx context.Context, targets []string) ([]DeckChannelState, error)
	// UpdateChannelInfo pushes the info patch to each target ("platform:slug") and reports the
	// per-target disposition. Targets the engine isn't authenticated for come back as excluded.
	UpdateChannelInfo(ctx context.Context, targets []string, info DeckInfo) ([]DeckResult, error)
	// SearchCategories resolves a free-text query to a platform's top matching categories.
	SearchCategories(ctx context.Context, platform, query string) ([]DeckCategory, error)
}

// DeckChannelState is one target's currently-live stream metadata, as served by
// GET /v1/deck/channel-info.
type DeckChannelState struct {
	Channel    string   `json:"channel"`             // "platform:slug"
	Title      string   `json:"title,omitempty"`
	Category   string   `json:"category,omitempty"`  // display name
	CategoryID string   `json:"category_id,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Status     string   `json:"status"`              // ok | excluded | error
	Reason     string   `json:"reason,omitempty"`
}

// DeckInfo is the patch the Deck pushes across platforms. Empty fields are skipped so a
// partial update only touches what the caller specified. Category is free text; the wiring layer
// resolves it to a platform-native id (or uses CategoryIDs if the caller pre-resolved them).
type DeckInfo struct {
	Title       string            `json:"title,omitempty"`
	Category    string            `json:"category,omitempty"`     // free-text game name
	CategoryIDs map[string]string `json:"category_ids,omitempty"` // platform -> pre-resolved id
	Tags        []string          `json:"tags,omitempty"`
}

// DeckResult is one target's disposition after a sync, served by POST /v1/deck/channel-info.
type DeckResult struct {
	Channel string `json:"channel"`          // "platform:slug"
	Status  string `json:"status"`           // ok | error | excluded | unsupported
	Reason  string `json:"reason,omitempty"` // why error/excluded
}

// DeckCategory is one platform's category match, served by GET /v1/deck/categories.
type DeckCategory struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	BoxArt   string `json:"box_art,omitempty"`
}

// Deck result statuses.
const (
	DeckOK          = "ok"          // the platform accepted the patch
	DeckError       = "error"       // the platform rejected the patch (reason filled)
	DeckExcluded    = "excluded"    // no authenticated account for that platform/channel
	DeckUnsupported = "unsupported" // the platform has no channel-info endpoint
)

// SetDeck installs the Deck controller. Until called, the Deck endpoints report unavailable.
func (s *Server) SetDeck(d Deck) { s.deck = d }

// deckUpdateRequest is the POST /v1/deck/channel-info body.
type deckUpdateRequest struct {
	Channels []string    `json:"channels"`
	Info     DeckInfo `json:"info"`
}

func (s *Server) handleDeckGet(w http.ResponseWriter, r *http.Request) {
	if s.deck == nil {
		http.Error(w, "deck unavailable", http.StatusServiceUnavailable)
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("channels"))
	if raw == "" {
		http.Error(w, "expected a channels query param", http.StatusBadRequest)
		return
	}
	channels := strings.Split(raw, ",")
	for i, c := range channels {
		channels[i] = strings.TrimSpace(c)
	}
	states, err := s.deck.GetChannelInfo(r.Context(), channels)
	if err != nil {
		s.channelError(w, err)
		return
	}
	if states == nil {
		states = []DeckChannelState{}
	}
	writeJSON(w, map[string]any{"channels": states})
}

func (s *Server) handleDeckUpdate(w http.ResponseWriter, r *http.Request) {
	if s.deck == nil {
		http.Error(w, "deck unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req deckUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Channels) == 0 {
		http.Error(w, "expected JSON body with channels and info", http.StatusBadRequest)
		return
	}
	results, err := s.deck.UpdateChannelInfo(r.Context(), req.Channels, req.Info)
	if err != nil {
		s.channelError(w, err)
		return
	}
	if results == nil {
		results = []DeckResult{}
	}
	writeJSON(w, map[string]any{"results": results})
}

func (s *Server) handleDeckCategories(w http.ResponseWriter, r *http.Request) {
	if s.deck == nil {
		http.Error(w, "deck unavailable", http.StatusServiceUnavailable)
		return
	}
	platform := strings.TrimSpace(r.URL.Query().Get("platform"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if platform == "" || q == "" {
		http.Error(w, "expected platform and q query params", http.StatusBadRequest)
		return
	}
	cats, err := s.deck.SearchCategories(r.Context(), platform, q)
	if err != nil {
		s.channelError(w, err)
		return
	}
	if cats == nil {
		cats = []DeckCategory{}
	}
	writeJSON(w, map[string]any{"categories": cats})
}
