package api

import (
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/elythi0n/virta/internal/platform"
)

// schemaVersion versions the wire envelope. A client reads this to fail loudly rather than
// misinterpret events from a newer server.
const schemaVersion = 1

// clientBuffer is how many encoded events a single client may fall behind before its oldest
// events are dropped. A slow client degrades only itself.
const clientBuffer = 256

// replayBuffer is how many recent events are retained for resume-on-reconnect. A client
// presents the highest seq it processed and the server replays buffered events past it.
// Bounded, so memory is constant; a client gone longer than this window misses the gap.
const replayBuffer = 1024

// wireEvent is the JSON envelope sent to clients. Exactly the fields relevant to Type are
// populated. Seq is a per-server monotonic sequence number: clients track the highest they've
// processed and present it as the resume cursor (delivery on resume is at-least-once — dedupe
// by Seq).
type wireEvent struct {
	Type              string                   `json:"type"`
	SchemaVersion     int                      `json:"schema_version"`
	Seq               uint64                   `json:"seq"`
	Message           *platform.UnifiedMessage `json:"message,omitempty"`
	Channel           *platform.ChannelRef     `json:"channel,omitempty"`
	PlatformMessageID string                   `json:"platform_message_id,omitempty"`
	MessageID         string                   `json:"message_id,omitempty"` // engine ULID of a deleted message, when resolved
	TargetUserID      string                   `json:"target_user_id,omitempty"`
	State             *platform.HealthStatus   `json:"state,omitempty"`
	Settings          *platform.ChatSettings   `json:"settings,omitempty"`
}

// replayEntry is one encoded event retained in the resume ring.
type replayEntry struct {
	seq   uint64
	key   string
	all   bool
	bytes []byte
}

// hub is the set of connected stream clients. It is a pipeline sink: every event the
// pipeline produces is broadcast to the clients whose subscription matches, and a bounded
// ring of recent events backs resume-on-reconnect.
type hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
	closed  bool

	seq    uint64        // monotonic event counter (guarded by mu)
	replay []replayEntry // ring of recent events; replay[rnext] is oldest when full
	rnext  int
}

func newHub() *hub {
	return &hub{clients: map[*client]struct{}{}}
}

func (h *hub) Name() string { return "wsclients" }

// Consume stamps the event with the next sequence number, encodes it once, records it in the
// resume ring, and delivers it to each matching client without blocking: if a client's buffer
// is full its oldest queued event is dropped to make room, so a slow reader never holds up the
// broadcast or other clients.
func (h *hub) Consume(_ context.Context, ev platform.Event) error {
	we, key, broadcastAll := toWire(ev)
	if we.Type == "" {
		return nil // event type we don't forward
	}
	// The pipeline drives one sink with a single worker goroutine, so Consume is never
	// concurrent with itself — seq and the JSON encode need no lock, keeping the expensive
	// marshal off the critical section that the client set and replay ring share.
	h.seq++
	we.Seq = h.seq
	b, err := json.Marshal(we)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.record(h.seq, key, broadcastAll, b)
	for c := range h.clients {
		if !broadcastAll && !c.wants(key) {
			continue
		}
		pushBytes(c, b)
	}
	return nil
}

// record appends an encoded event to the bounded resume ring (oldest overwritten when full).
func (h *hub) record(seq uint64, key string, all bool, b []byte) {
	if h.replay == nil {
		h.replay = make([]replayEntry, replayBuffer)
	}
	h.replay[h.rnext] = replayEntry{seq: seq, key: key, all: all, bytes: b}
	h.rnext = (h.rnext + 1) % len(h.replay)
}

// replayTo sends every buffered event past since that matches c's subscription, in sequence
// order. Used to resume a reconnecting client from its last processed cursor.
func (h *hub) replayTo(c *client, since uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var pending []replayEntry
	for _, e := range h.replay {
		if e.seq > since && (e.all || c.wants(e.key)) {
			pending = append(pending, e)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].seq < pending[j].seq })
	for _, e := range pending {
		pushBytes(c, e.bytes)
	}
}

// pushBytes enqueues b to a client without blocking, dropping its oldest queued event first
// if the buffer is full so the newest always makes it in.
func pushBytes(c *client, b []byte) {
	select {
	case c.send <- b:
	default:
		select {
		case <-c.send:
		default:
		}
		select {
		case c.send <- b:
		default:
		}
	}
}

func (h *hub) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for c := range h.clients {
		close(c.send)
		delete(h.clients, c)
	}
	return nil
}

func (h *hub) register(c *client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false
	}
	h.clients[c] = struct{}{}
	return true
}

func (h *hub) unregister(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
}

func (h *hub) clientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// client is one connected stream consumer. send carries pre-encoded event bytes to its
// write pump; sub is the (mutable) subscription.
type client struct {
	send chan []byte

	mu  sync.Mutex
	sub subscription
}

func newClient() *client {
	return &client{send: make(chan []byte, clientBuffer)}
}

// wants reports whether this client's subscription includes the given channel key.
func (c *client) wants(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sub.matches(key)
}

func (c *client) setSubscription(s subscription) {
	c.mu.Lock()
	c.sub = s
	c.mu.Unlock()
}

// subscription selects which channels a client receives. An empty set means "all channels".
type subscription struct {
	channels map[string]struct{} // keys like "twitch:forsen"
}

func (s subscription) matches(key string) bool {
	if len(s.channels) == 0 {
		return true
	}
	_, ok := s.channels[key]
	return ok
}

// subscribeMessage is the client→server control message on the stream.
type subscribeMessage struct {
	Action   string   `json:"action"`             // "subscribe"
	Channels []string `json:"channels,omitempty"` // "platform:slug"; empty/omitted = all
	Since    uint64   `json:"since,omitempty"`    // resume cursor: replay buffered events past this seq
}

func channelKey(ch platform.ChannelRef) string {
	return string(ch.Platform) + ":" + ch.Slug
}

// toWire converts a pipeline event into its wire envelope, the channel key used for
// per-client filtering, and whether it should be broadcast to everyone regardless of filter
// (adapter-wide health has no single channel).
func toWire(ev platform.Event) (we wireEvent, key string, broadcastAll bool) {
	switch e := ev.(type) {
	case platform.MessageEvent:
		m := e.Message
		return wireEvent{Type: "message", SchemaVersion: schemaVersion, Message: &m}, channelKey(m.Channel), false
	case platform.MessageDeletedEvent:
		ch := e.Channel
		return wireEvent{Type: "message_deleted", SchemaVersion: schemaVersion, Channel: &ch, PlatformMessageID: e.PlatformMessageID, MessageID: e.MessageID}, channelKey(ch), false
	case platform.ChannelClearEvent:
		ch := e.Channel
		return wireEvent{Type: "channel_clear", SchemaVersion: schemaVersion, Channel: &ch, TargetUserID: e.TargetUserID}, channelKey(ch), false
	case platform.HealthEvent:
		st := e.Status
		we = wireEvent{Type: "state", SchemaVersion: schemaVersion, State: &st}
		if e.Channel != nil {
			we.Channel = e.Channel
			return we, channelKey(*e.Channel), false
		}
		return we, "", true // adapter-wide: everyone hears it
	case platform.ChatSettingsEvent:
		ch := e.Channel
		s := e.Settings
		return wireEvent{Type: "chat_settings", SchemaVersion: schemaVersion, Channel: &ch, Settings: &s}, channelKey(ch), false
	default:
		return wireEvent{}, "", false
	}
}
