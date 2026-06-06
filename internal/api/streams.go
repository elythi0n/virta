package api

import "net/http"

// StreamInfo is one joined channel's live stream metadata, as served by GET /v1/streams. Viewer
// count and thumbnail are populated where the platform exposes them anonymously; an offline
// channel reports Live=false with the rest zeroed.
type StreamInfo struct {
	Platform     string `json:"platform"`
	Slug         string `json:"slug"`
	Live         bool   `json:"live"`
	ViewerCount  int    `json:"viewer_count"`
	Title        string `json:"title,omitempty"`
	Category     string `json:"category,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	StartedAt    string `json:"started_at,omitempty"` // RFC3339; empty when unknown
}

func (s *Server) handleListStreams(w http.ResponseWriter, _ *http.Request) {
	if s.channels == nil {
		http.Error(w, "channel control unavailable", http.StatusServiceUnavailable)
		return
	}
	list := s.channels.Streams()
	if list == nil {
		list = []StreamInfo{}
	}
	writeJSON(w, map[string]any{"streams": list})
}
