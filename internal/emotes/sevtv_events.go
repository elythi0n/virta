// Package emotes — 7TV EventAPI WebSocket subscriber for live emote-set invalidation.
//
// When a streamer adds or removes a 7TV emote the REST snapshot (fetched every 24h) goes stale
// immediately. The EventAPI delivers an emote_set.update dispatch for each change, so we can
// call Refresh right away instead of waiting for the next poll cycle.
//
// Protocol (v3): each client → server message is JSON {op, d}. After connecting, subscribe with
// op 35 per channel. The server sends op 0 dispatches when an emote set changes. Opcodes:
//
//	 0  Dispatch (server → client): an emote set changed; refresh the affected channels.
//	 1  Hello    (server → client): session info; read heartbeat_interval but don't send heartbeats
//	            (the server initiates them with op 2).
//	 4  Reconnect (server → client): server wants us to redial (e.g. rolling restart).
//	35  Subscribe (client → server): start listening for a channel's emote-set changes.
//	36  Unsubscribe (client → server): stop listening.
package emotes

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/elythi0n/virta/internal/platform"
)

const sevenTVEventURL = "wss://events.7tv.io/v3"

const (
	stv7OpDispatch    = 0
	stv7OpHello       = 1
	stv7OpReconnect   = 4
	stv7OpSubscribe   = 35
	stv7OpUnsubscribe = 36
)

type stv7Msg struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
}

// SevenTVEvents subscribes to the 7TV EventAPI and triggers Resolver.Refresh when any
// subscribed channel's emote set changes. It manages the WS lifecycle: reconnects with
// exponential backoff, re-subscribes all channels after each reconnect.
type SevenTVEvents struct {
	resolver *Resolver
	log      *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu       sync.Mutex
	channels map[string]platform.ChannelRef // Key(ch) → ref (ID must be set at Subscribe time)
	conn     *websocket.Conn                // nil when disconnected
}

// NewSevenTVEvents builds the subscriber. Call Start() to begin the connection loop.
func NewSevenTVEvents(resolver *Resolver, log *slog.Logger) *SevenTVEvents {
	ctx, cancel := context.WithCancel(context.Background())
	return &SevenTVEvents{
		resolver: resolver,
		log:      log,
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
		channels: map[string]platform.ChannelRef{},
	}
}

// Subscribe registers ch for emote-set change notifications. ch.ID must be the numeric platform
// user id (Twitch broadcaster id). Channels without an ID are silently ignored.
func (s *SevenTVEvents) Subscribe(ch platform.ChannelRef) {
	if ch.ID == "" {
		return
	}
	key := Key(ch)
	s.mu.Lock()
	_, existed := s.channels[key]
	s.channels[key] = ch
	conn := s.conn
	s.mu.Unlock()
	if !existed && conn != nil {
		s.writeSub(conn, stv7OpSubscribe, ch)
	}
}

// Unsubscribe removes ch from emote-set notifications. The stored ref (which carries the ID) is
// used for the unsubscribe message, so callers need not re-supply the ID.
func (s *SevenTVEvents) Unsubscribe(ch platform.ChannelRef) {
	key := Key(ch)
	s.mu.Lock()
	stored, existed := s.channels[key]
	if existed {
		delete(s.channels, key)
	}
	conn := s.conn
	s.mu.Unlock()
	if existed && stored.ID != "" && conn != nil {
		s.writeSub(conn, stv7OpUnsubscribe, stored)
	}
}

// Start launches the connection loop in the background.
func (s *SevenTVEvents) Start() { go s.run() }

// Close stops the connection loop and waits for it to exit.
func (s *SevenTVEvents) Close() {
	s.cancel()
	<-s.done
}

func (s *SevenTVEvents) run() {
	defer close(s.done)
	attempt := 0
	for s.ctx.Err() == nil {
		if err := s.connect(); err != nil && s.ctx.Err() == nil {
			delay := stv7Backoff(attempt)
			s.log.Debug("7tv events reconnect", "attempt", attempt, "delay", delay, "err", err)
			attempt++
			select {
			case <-s.ctx.Done():
				return
			case <-time.NewTimer(delay).C:
			}
		} else {
			attempt = 0
		}
	}
}

func stv7Backoff(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func (s *SevenTVEvents) connect() error {
	conn, resp, err := websocket.Dial(s.ctx, sevenTVEventURL, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return err
	}
	conn.SetReadLimit(1 << 20)

	s.mu.Lock()
	s.conn = conn
	channels := make([]platform.ChannelRef, 0, len(s.channels))
	for _, ch := range s.channels {
		channels = append(channels, ch)
	}
	s.mu.Unlock()

	// Re-subscribe to all channels after a reconnect.
	for _, ch := range channels {
		s.writeSub(conn, stv7OpSubscribe, ch)
	}

	defer func() {
		s.mu.Lock()
		if s.conn == conn {
			s.conn = nil
		}
		s.mu.Unlock()
		_ = conn.CloseNow()
	}()

	for {
		_, raw, err := conn.Read(s.ctx)
		if err != nil {
			return err
		}
		s.handle(conn, raw)
	}
}

func (s *SevenTVEvents) handle(conn *websocket.Conn, raw []byte) {
	var msg stv7Msg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	switch msg.Op {
	case stv7OpDispatch:
		var d struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(msg.D, &d) != nil || d.Type != "emote_set.update" {
			return
		}
		// Refresh all subscribed channels — we only receive dispatches for subscribed channels,
		// and emote-set changes are rare, so over-refreshing is not a concern.
		s.mu.Lock()
		channels := make([]platform.ChannelRef, 0, len(s.channels))
		for _, ch := range s.channels {
			channels = append(channels, ch)
		}
		s.mu.Unlock()
		for _, ch := range channels {
			go func(ch platform.ChannelRef) { _ = s.resolver.Refresh(context.Background(), ch) }(ch)
		}

	case stv7OpReconnect:
		// Server wants us to redial; closing the conn causes connect() to return and run() to retry.
		_ = conn.Close(websocket.StatusNormalClosure, "reconnect requested")
	}
}

func (s *SevenTVEvents) writeSub(conn *websocket.Conn, op int, ch platform.ChannelRef) {
	platform7TV := stv7Platform(ch.Platform)
	if platform7TV == "" || ch.ID == "" {
		return
	}
	d, err := json.Marshal(map[string]any{
		"type": "emote_set.update",
		"condition": map[string]string{
			"ctx":      "channel",
			"platform": platform7TV,
			"id":       ch.ID,
		},
	})
	if err != nil {
		return
	}
	b, err := json.Marshal(stv7Msg{Op: op, D: d})
	if err != nil {
		return
	}
	_ = conn.Write(s.ctx, websocket.MessageText, b)
}

func stv7Platform(p platform.Platform) string {
	switch p {
	case platform.Twitch:
		return "TWITCH"
	case platform.Kick:
		return "KICK"
	default:
		return ""
	}
}
