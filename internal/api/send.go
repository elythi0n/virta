package api

import (
	"context"
	"encoding/json"
	"net/http"
)

// Send is the cross-posting control surface: preview where a message can go, and send it to many
// channels at once. Implemented by the dispatch layer and injected via SetSend; strings keep the
// API decoupled from the platform model (the wiring layer translates "platform:slug").
type Send interface {
	// Preview reports per-target send reachability without sending — the composer's chips.
	Preview(targets []string) ([]SendTarget, error)
	// Send cross-posts text to the targets, returning each target's disposition.
	Send(ctx context.Context, targets []string, text string) ([]SendResult, error)
}

// SendTarget is one channel's pre-send reachability, served by POST /v1/send/preview.
type SendTarget struct {
	Channel string `json:"channel"` // "platform:slug"
	CanSend bool   `json:"can_send"`
	Reason  string `json:"reason,omitempty"` // machine reason code when CanSend is false
}

// SendResult is one target's disposition after a cross-post, served by POST /v1/send.
type SendResult struct {
	Channel string `json:"channel"`          // "platform:slug"
	Status  string `json:"status"`           // queued | sent | dropped | excluded
	Reason  string `json:"reason,omitempty"` // why excluded/dropped (machine reason code)
}

// Send statuses.
const (
	SendQueued   = "queued"   // accepted, pacing through the governor
	SendSent     = "sent"     // delivered to the platform
	SendDropped  = "dropped"  // the platform rejected it
	SendExcluded = "excluded" // not reachable; never attempted
)

// SetSend installs the cross-posting controller. Until called, the send endpoints report the
// feature as unavailable.
func (s *Server) SetSend(c Send) { s.send = c }

// sendRequest is the POST /v1/send body.
type sendRequest struct {
	Channels []string `json:"channels"`
	Text     string   `json:"text"`
}

// previewRequest is the POST /v1/send/preview body.
type previewRequest struct {
	Channels []string `json:"channels"`
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if s.send == nil {
		http.Error(w, "send unavailable", http.StatusServiceUnavailable)
		return
	}
	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Channels) == 0 || req.Text == "" {
		http.Error(w, "expected JSON body with channels and text", http.StatusBadRequest)
		return
	}
	results, err := s.send.Send(r.Context(), req.Channels, req.Text)
	if err != nil {
		s.channelError(w, err)
		return
	}
	writeJSON(w, map[string]any{"results": results})
}

func (s *Server) handleSendPreview(w http.ResponseWriter, r *http.Request) {
	if s.send == nil {
		http.Error(w, "send unavailable", http.StatusServiceUnavailable)
		return
	}
	var req previewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Channels) == 0 {
		http.Error(w, "expected JSON body with channels", http.StatusBadRequest)
		return
	}
	targets, err := s.send.Preview(req.Channels)
	if err != nil {
		s.channelError(w, err)
		return
	}
	writeJSON(w, map[string]any{"targets": targets})
}
