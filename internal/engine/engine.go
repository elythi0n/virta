// Package engine is the seam between platform adapters and the message pipeline. It owns the
// set of registered adapters, routes channel join/leave to the right one, and performs the
// two ingest-time jobs that need engine-wide state: assigning each message a time-sortable
// ULID, and resolving a deletion's platform id back to the ULID of the message it removes.
// Everything else — annotation, fan-out, delivery — belongs to the pipeline and sinks.
package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
)

// defaultIDMapCap bounds how many recent platform-id→ULID mappings are retained for deletion
// resolution. A few thousand covers the realistic window between a message and its deletion
// across busy channels while keeping memory flat.
const defaultIDMapCap = 8192

// seenCapPerChannel bounds the first-time-chatter set per channel. Beyond it we stop claiming
// first-time (rather than over-claim) — far above any real channel's distinct chatters.
const seenCapPerChannel = 100_000

// Submitter is the pipeline entry point the engine feeds. *pipeline.Runner satisfies it.
type Submitter interface {
	Submit(ev platform.Event)
}

// Engine wires adapters to a pipeline Submitter. It is safe for concurrent use.
type Engine struct {
	out     Submitter
	gen     id.Generator
	ids     *idMap
	logging atomic.Bool // when false (default), messages are marked ephemeral and never persisted

	mu       sync.Mutex
	adapters map[platform.Platform]platform.Adapter
	joined   map[string]platform.ChannelRef // channelKey → ref, for listing
	seen     map[string]map[string]struct{} // channelKey → author ids seen this session
	closed   bool

	wg sync.WaitGroup // adapter event forwarders
}

// New builds an engine that mints ULIDs with gen and submits processed events to out.
func New(out Submitter, gen id.Generator) *Engine {
	return &Engine{
		out:      out,
		gen:      gen,
		ids:      newIDMap(defaultIDMapCap),
		adapters: map[platform.Platform]platform.Adapter{},
		joined:   map[string]platform.ChannelRef{},
		seen:     map[string]map[string]struct{}{},
	}
}

// Register adds an adapter and starts forwarding its events into the pipeline. Call before
// Start/Join; not safe to call concurrently with Close.
func (e *Engine) Register(a platform.Adapter) {
	e.mu.Lock()
	e.adapters[a.Platform()] = a
	e.mu.Unlock()
	e.wg.Add(1)
	go e.forward(a)
}

// forward drains one adapter's events through ingest until the adapter closes its channel.
func (e *Engine) forward(a platform.Adapter) {
	defer e.wg.Done()
	for ev := range a.Events() {
		e.ingest(ev)
	}
}

// ingest applies the engine's ingest-time transforms and submits the event. Messages get a
// ULID (and are recorded for deletion lookup); deletions get their original message's ULID
// resolved; everything else passes through untouched.
func (e *Engine) ingest(ev platform.Event) {
	switch t := ev.(type) {
	case platform.MessageEvent:
		if t.Message.ID == "" {
			t.Message.ID = e.gen.New()
		}
		if t.Message.PlatformMessageID != "" {
			e.ids.put(idKey(t.Message.Channel, t.Message.PlatformMessageID), t.Message.ID)
		}
		// Ephemeral unless logging is on — the flag the store's choke point enforces, so
		// logging-off can never persist chat (ADR-014).
		t.Message.Ephemeral = !e.logging.Load()
		e.markFirstTime(&t.Message)
		e.out.Submit(t)
	case platform.MessageDeletedEvent:
		if t.PlatformMessageID != "" {
			t.MessageID = e.ids.get(idKey(t.Channel, t.PlatformMessageID))
		}
		e.out.Submit(t)
	default:
		e.out.Submit(ev)
	}
}

// markFirstTime flags a chatter's first message of the session. An adapter that already set the
// flag from an authoritative platform tag (e.g. Twitch's first-msg) wins; otherwise we mark the
// first time we've seen this author in the channel. Bounded per channel.
func (e *Engine) markFirstTime(m *platform.UnifiedMessage) {
	if m.Type != platform.TypeChat {
		return
	}
	if m.Annotations != nil && m.Annotations.FirstTime {
		return
	}
	author := m.Author.ID
	if author == "" {
		author = m.Author.Login
	}
	if author == "" {
		return
	}
	key := channelKey(m.Channel)
	e.mu.Lock()
	set := e.seen[key]
	if set == nil {
		set = make(map[string]struct{})
		e.seen[key] = set
	}
	_, known := set[author]
	first := !known && len(set) < seenCapPerChannel
	if first {
		set[author] = struct{}{}
	}
	e.mu.Unlock()
	if first {
		m.Annotate().FirstTime = true
	}
}

// SetLogging turns message persistence on or off. Off (the default) marks every ingested
// message ephemeral, so nothing is written. The profile manager calls this on activation.
func (e *Engine) SetLogging(enabled bool) { e.logging.Store(enabled) }

// Join connects the channel through the adapter for its platform.
func (e *Engine) Join(ctx context.Context, ch platform.ChannelRef, mode platform.ConnMode) error {
	e.mu.Lock()
	a, ok := e.adapters[ch.Platform]
	if ok {
		e.joined[channelKey(ch)] = ch
	}
	e.mu.Unlock()
	if !ok {
		return fmt.Errorf("engine: no adapter registered for platform %q", ch.Platform)
	}
	if err := a.Join(ctx, ch, mode); err != nil {
		e.mu.Lock()
		delete(e.joined, channelKey(ch))
		e.mu.Unlock()
		return err
	}
	return nil
}

// Leave parts the channel through its platform's adapter.
func (e *Engine) Leave(ch platform.ChannelRef) error {
	e.mu.Lock()
	a, ok := e.adapters[ch.Platform]
	delete(e.joined, channelKey(ch))
	delete(e.seen, channelKey(ch)) // release the first-time-chatter set (re-join resets it)
	e.mu.Unlock()
	if !ok {
		return fmt.Errorf("engine: no adapter registered for platform %q", ch.Platform)
	}
	return a.Leave(ch)
}

// ChannelStatus is a joined channel and the current health of its adapter.
type ChannelStatus struct {
	Channel platform.ChannelRef
	Health  platform.HealthStatus
}

// Channels lists every joined channel with its adapter's current health.
func (e *Engine) Channels() []ChannelStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]ChannelStatus, 0, len(e.joined))
	for _, ch := range e.joined {
		st := platform.HealthStatus{State: platform.HealthOK}
		if a, ok := e.adapters[ch.Platform]; ok {
			st = a.Health()
		}
		out = append(out, ChannelStatus{Channel: ch, Health: st})
	}
	return out
}

// Close shuts every adapter (which closes its event channel) and waits for the forwarders to
// drain, so no further events are submitted after Close returns. Idempotent.
func (e *Engine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	adapters := make([]platform.Adapter, 0, len(e.adapters))
	for _, a := range e.adapters {
		adapters = append(adapters, a)
	}
	e.mu.Unlock()

	for _, a := range adapters {
		_ = a.Close()
	}
	e.wg.Wait()
	return nil
}

func channelKey(ch platform.ChannelRef) string { return ch.Key() }

func idKey(ch platform.ChannelRef, platformMsgID string) string {
	return channelKey(ch) + "|" + platformMsgID
}
