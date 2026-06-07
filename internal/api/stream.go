package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

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
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		expected := scheme + "://" + r.Host
		if origin != expected {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		opts.OriginPatterns = []string{r.Host}
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
			c.setSubscription(toSubscription(msg.Channels))
			if msg.Since > 0 {
				// Resume: replay buffered events past the client's cursor (at-least-once;
				// the client dedupes by seq).
				s.hub.replayTo(c, msg.Since)
			}
		}
	}
}

func toSubscription(channels []string) subscription {
	if len(channels) == 0 {
		return subscription{}
	}
	m := make(map[string]struct{}, len(channels))
	for _, ch := range channels {
		// Canonicalize to match the key incoming messages carry: slugs are case-insensitive, so
		// a client subscribing to "twitch:Shroud" must still match "twitch:shroud" on the wire.
		m[strings.ToLower(ch)] = struct{}{}
	}
	return subscription{channels: m}
}
