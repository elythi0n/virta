package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// Channels is the join/leave control surface the API exposes, implemented by the engine and
// injected via SetChannels. Strings are used at this boundary so the API stays decoupled from
// the platform model; the wiring layer translates to/from platform types.
type Channels interface {
	// Join connects a channel. mode is a ConnMode string ("automatic", "anonymous", …);
	// an empty mode means the recommended default.
	Join(ctx context.Context, platform, slug, mode string) error
	// Leave parts a channel.
	Leave(ctx context.Context, platform, slug string) error
	// List returns the channels joined by the current user (scoped by user id in hosted mode).
	List(ctx context.Context) []ChannelInfo
	// Capabilities reports each platform's current capabilities, keyed by platform name.
	Capabilities() map[string]Capabilities
	// Streams returns live stream metadata for the current user's joined channels.
	Streams(ctx context.Context) []StreamInfo
	// Emotes returns the usable emotes across the current user's joined channels.
	Emotes(ctx context.Context) []EmoteInfo
}

// Capabilities mirrors a platform adapter's current capabilities for the wire, so a frontend can
// render send/moderation affordances without hardcoding platform knowledge.
type Capabilities struct {
	ReadAnonymous bool   `json:"read_anonymous"`
	ReadAuthed    bool   `json:"read_authed"`
	Send          bool   `json:"send"`
	Moderation    bool   `json:"moderation"`
	Replies       bool   `json:"replies"`
	HeldQueue     bool   `json:"held_queue"`
	Stability     string `json:"stability"`
}

// ChannelInfo is one joined channel's status, as served by GET /v1/channels.
type ChannelInfo struct {
	Platform string `json:"platform"`
	Slug     string `json:"slug"`
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
}

// channelRequest is the POST/DELETE /v1/channels body.
type channelRequest struct {
	Platform string `json:"platform"`
	Slug     string `json:"slug"`
	Mode     string `json:"mode,omitempty"`
}

// SetChannels installs the join/leave controller. Called by the wiring layer after the engine
// exists; until then the channel endpoints report the feature as unavailable.
func (s *Server) SetChannels(c Channels) { s.channels = c }

// ChannelList returns the current user's joined channels for non-HTTP callers (e.g. the intel
// controller needs the live list to inject into the AI system prompt).
func (s *Server) ChannelList(ctx context.Context) []ChannelInfo {
	if s.channels == nil {
		return nil
	}
	return s.channels.List(ctx)
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		http.Error(w, "channel control unavailable", http.StatusServiceUnavailable)
		return
	}
	list := s.channels.List(r.Context())
	if list == nil {
		list = []ChannelInfo{}
	}
	writeJSON(w, map[string]any{"channels": list})
}

func (s *Server) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	if s.channels == nil {
		http.Error(w, "channel control unavailable", http.StatusServiceUnavailable)
		return
	}
	caps := s.channels.Capabilities()
	if caps == nil {
		caps = map[string]Capabilities{}
	}
	writeJSON(w, map[string]any{"capabilities": caps})
}

func (s *Server) handleJoinChannel(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		http.Error(w, "channel control unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req channelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Platform == "" || req.Slug == "" {
		http.Error(w, "expected JSON body with platform and slug", http.StatusBadRequest)
		return
	}
	if err := s.channels.Join(r.Context(), req.Platform, req.Slug, req.Mode); err != nil {
		s.channelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLeaveChannel(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		http.Error(w, "channel control unavailable", http.StatusServiceUnavailable)
		return
	}
	// DELETE takes the channel from query params (platform, slug), avoiding a request body.
	platform := r.URL.Query().Get("platform")
	slug := r.URL.Query().Get("slug")
	if platform == "" || slug == "" {
		http.Error(w, "expected platform and slug query parameters", http.StatusBadRequest)
		return
	}
	if err := s.channels.Leave(r.Context(), platform, slug); err != nil {
		s.channelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// channelError maps a controller error to a status: an unknown platform is the caller's
// mistake (400); anything else is an upstream/connection failure (502).
func (s *Server) channelError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrUnknownPlatform) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, err.Error(), http.StatusBadGateway)
}

// ErrUnknownPlatform is returned by a Channels implementation when asked for a platform it has
// no adapter for, so the API can answer 400 rather than 502.
var ErrUnknownPlatform = errors.New("unknown platform")
