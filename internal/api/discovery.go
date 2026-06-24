package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

// DiscoveryAPI is the search/browse surface for finding streamers across platforms. The
// Discovery pane uses it to list live channels (sorted by viewers, paginated) and to look up a
// streamer by name. Wired by the app layer; strings keep the API decoupled from the platform
// model. (Named DiscoveryAPI to avoid colliding with the daemon-endpoint Discovery struct used
// for the /__discovery handshake.)
type DiscoveryAPI interface {
	// SearchChannels resolves a free-text query to channel matches on one platform.
	SearchChannels(ctx context.Context, platform, query string, first int, liveOnly bool) ([]DiscoveryChannel, error)
	// TopStreams returns the currently-live channels on one platform, sorted by viewer count.
	// The cursor is opaque per platform; an empty cursor means "first page".
	TopStreams(ctx context.Context, platform string, first int, cursor, gameID string) (DiscoveryStreamsPage, error)
}

// DiscoveryChannel is one channel-search match, served by GET /v1/discovery/channels.
type DiscoveryChannel struct {
	Platform     string   `json:"platform"`
	Slug         string   `json:"slug"`         // login (Twitch) / slug (Kick)
	DisplayName  string   `json:"display_name"`
	IsLive       bool     `json:"is_live"`
	Title        string   `json:"title,omitempty"`
	Category     string   `json:"category,omitempty"`
	Thumbnail    string   `json:"thumbnail,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	StartedAt    string   `json:"started_at,omitempty"`
}

// DiscoveryStream is one live stream, served by GET /v1/discovery/streams.
type DiscoveryStream struct {
	Platform     string   `json:"platform"`
	Slug         string   `json:"slug"`
	DisplayName  string   `json:"display_name"`
	Title        string   `json:"title,omitempty"`
	Category     string   `json:"category,omitempty"`
	ViewerCount  int      `json:"viewer_count"`
	StartedAt    string   `json:"started_at,omitempty"`
	Thumbnail    string   `json:"thumbnail,omitempty"`
	Language     string   `json:"language,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// DiscoveryStreamsPage is one page of top streams plus the cursor for the next page (empty when
// the platform reports no further results).
type DiscoveryStreamsPage struct {
	Streams []DiscoveryStream `json:"streams"`
	Cursor  string            `json:"cursor,omitempty"`
}

// SetDiscovery installs the controller. Until called, the endpoints return service-unavailable.
func (s *Server) SetDiscovery(d DiscoveryAPI) { s.discovery = d }

func (s *Server) handleDiscoveryChannels(w http.ResponseWriter, r *http.Request) {
	if s.discovery == nil {
		http.Error(w, "discovery unavailable", http.StatusServiceUnavailable)
		return
	}
	platform := strings.TrimSpace(r.URL.Query().Get("platform"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if platform == "" || q == "" {
		http.Error(w, "expected platform and q query params", http.StatusBadRequest)
		return
	}
	first := parseInt(r.URL.Query().Get("first"), 20)
	liveOnly := r.URL.Query().Get("live_only") == "true"
	results, err := s.discovery.SearchChannels(r.Context(), platform, q, first, liveOnly)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if results == nil {
		results = []DiscoveryChannel{}
	}
	writeJSON(w, map[string]any{"results": results})
}

func (s *Server) handleDiscoveryStreams(w http.ResponseWriter, r *http.Request) {
	if s.discovery == nil {
		http.Error(w, "discovery unavailable", http.StatusServiceUnavailable)
		return
	}
	platform := strings.TrimSpace(r.URL.Query().Get("platform"))
	if platform == "" {
		http.Error(w, "expected platform query param", http.StatusBadRequest)
		return
	}
	first := parseInt(r.URL.Query().Get("first"), 20)
	cursor := r.URL.Query().Get("cursor")
	gameID := r.URL.Query().Get("game_id")
	page, err := s.discovery.TopStreams(r.Context(), platform, first, cursor, gameID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if page.Streams == nil {
		page.Streams = []DiscoveryStream{}
	}
	writeJSON(w, page)
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

