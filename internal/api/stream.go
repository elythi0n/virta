package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

const writeTimeout = 10 * time.Second

// handleStream upgrades the request to a WebSocket and streams pipeline events to the client.
// The client may send {"action":"subscribe","channels":[...]} to narrow what it receives; an
// empty channel list (or no subscribe) means all channels.
//
// Origin checking is disabled because the listener is loopback-only — the only reachable
// clients are processes on this machine.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
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
		}
	}
}

func toSubscription(channels []string) subscription {
	if len(channels) == 0 {
		return subscription{}
	}
	m := make(map[string]struct{}, len(channels))
	for _, ch := range channels {
		m[ch] = struct{}{}
	}
	return subscription{channels: m}
}
