package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// isLoopbackOrigin reports whether the given Origin header value is a trusted
// same-machine origin. Allowed cases:
//   - Exact match with the daemon's own http://host:port (standard same-origin)
//   - Any loopback HTTP origin (localhost / *.localhost / 127.0.0.1 / ::1) — covers the Electron
//     desktop shell, which serves its UI from http://localhost and connects the WebSocket here
func isLoopbackOrigin(origin, daemonHost string) bool {
	if origin == "http://"+daemonHost || origin == "https://"+daemonHost {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || strings.HasSuffix(h, ".localhost") || h == "127.0.0.1" || h == "::1"
}

const writeTimeout = 10 * time.Second

// handleStream upgrades the request to a WebSocket and streams pipeline events to the client.
// The client may send {"action":"subscribe","channels":[...],"since":N} to narrow what it
// receives; an empty channel list (or no subscribe) means all channels, and a non-zero "since"
// replays buffered events past that sequence number to resume after a reconnect.
//
// Origin checking: allow same-origin requests and, when the server is loopback-only, also
// allow any loopback origin (the embedded SPA and desktop webview are same-origin by default).
// Cross-origin requests from a different host are rejected.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	// Prevent the bearer token (passed as a query param on WebSocket handshakes) from leaking
	// via the Referer header if this response is navigated away from.
	w.Header().Set("Referrer-Policy", "no-referrer")
	opts := &websocket.AcceptOptions{}
	if origin := r.Header.Get("Origin"); origin != "" {
		if !isLoopbackOrigin(origin, r.Host) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		opts.OriginPatterns = []string{"*"}
	}
	conn, err := websocket.Accept(w, r, opts)
	if err != nil {
		return // Accept already wrote the error response
	}
	defer func() { _ = conn.CloseNow() }()

	c := newClient()
	if !s.hub.register(c) {
		_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
		return
	}
	defer s.hub.unregister(c)

	ctx := r.Context()

	// Write pump: drain encoded events to the socket until the client is unregistered
	// (which closes c.send) or a write fails.
	go func() {
		for b := range c.send {
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		}
	}()

	// Read pump: handle control messages until the connection closes or the server stops.
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return // client closed, read error, or ctx canceled on shutdown
		}
		var msg subscribeMessage
		if json.Unmarshal(data, &msg) == nil && msg.Action == "subscribe" {
			// Subscription guard: a client only ever sees events for channels it has joined.
			// In hosted mode this prevents user A from subscribing to user B's channel keys;
			// in single-user mode the joined set is the user's own channels and an empty
			// subscribe-list still defaults to "everything I've joined". An empty joined set
			// still produces an empty subscription (the client receives only broadcastAll
			// events — adapter health and similar process-wide signals).
			c.setSubscription(s.scopedSubscription(ctx, msg.Channels))
			if msg.Since > 0 {
				// Resume: replay buffered events past the client's cursor (at-least-once;
				// the client dedupes by seq).
				s.hub.replayTo(c, msg.Since)
			}
		}
	}
}

// toSubscription builds a raw subscription set from a channel-key list. Used by tests; the
// production read pump goes through scopedSubscription instead so per-user ownership is checked.
func toSubscription(channels []string) subscription {
	if len(channels) == 0 {
		return subscription{}
	}
	m := make(map[string]struct{}, len(channels))
	for _, ch := range channels {
		m[strings.ToLower(ch)] = struct{}{}
	}
	return subscription{channels: m}
}

// scopedSubscription returns the channel set a stream client is allowed to subscribe to,
// derived from the user's joined channels (per uid(ctx)). When the client sent a list, the
// result is the intersection — channels the client requested *and* the user owns. When the
// client sent nothing, the result is the full joined set, scoping "all events" to "all of
// MY events". A subscription with no channels means "no channel-keyed events"; the client
// still receives broadcast-to-all events (adapter health) through the hub's broadcastAll path.
func (s *Server) scopedSubscription(ctx context.Context, requested []string) subscription {
	owned := make(map[string]struct{})
	if s.channels != nil {
		for _, ch := range s.channels.List(ctx) {
			owned[strings.ToLower(ch.Platform+":"+ch.Slug)] = struct{}{}
		}
	}
	if len(requested) == 0 {
		// Default: subscribe to every channel the user owns. Equivalent to the old "empty =
		// all" behavior in single-user mode but never broader than the user's own joined set.
		return subscription{channels: owned}
	}
	allowed := make(map[string]struct{}, len(requested))
	for _, ch := range requested {
		// Canonicalize: slugs are case-insensitive on the wire, so the request "twitch:Shroud"
		// matches the joined key "twitch:shroud".
		key := strings.ToLower(ch)
		if _, ok := owned[key]; ok {
			allowed[key] = struct{}{}
		}
		// Keys outside the joined set are silently dropped — we don't tell the client whether
		// the channel exists or whether another user is in it.
	}
	return subscription{channels: allowed}
}
