package platform

import (
	"context"
	"sync"
)

// FakeAdapter is a scriptable in-memory Adapter for tests across the codebase (engine,
// pipeline, api). It is a real, behaving Adapter — not a stub: it honors Capabilities
// (returning ErrUnsupported when Send/Moderation are off), records calls, and lets a test
// drive its Events() stream via Emit. Construct with NewFakeAdapter.
type FakeAdapter struct {
	platform Platform
	caps     Capabilities

	mu     sync.Mutex
	joined map[string]ConnMode // keyed by channel slug
	sends  []FakeSend
	mods   []ModAction
	health HealthStatus
	closed bool
	events chan Event
}

// FakeSend records a Send call for assertions.
type FakeSend struct {
	Channel ChannelRef
	Text    string
	Opts    SendOpts
}

// NewFakeAdapter creates a fake for the given platform with the given capabilities.
func NewFakeAdapter(p Platform, caps Capabilities) *FakeAdapter {
	return &FakeAdapter{
		platform: p,
		caps:     caps,
		joined:   make(map[string]ConnMode),
		health:   HealthStatus{State: HealthOK},
		events:   make(chan Event, 64),
	}
}

func (f *FakeAdapter) Platform() Platform         { return f.platform }
func (f *FakeAdapter) Capabilities() Capabilities { return f.caps }

// SetCapabilities updates capabilities at runtime (e.g. to simulate sign-in).
func (f *FakeAdapter) SetCapabilities(caps Capabilities) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.caps = caps
}

func (f *FakeAdapter) Join(_ context.Context, ch ChannelRef, mode ConnMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.joined[ch.Slug] = mode
	return nil
}

func (f *FakeAdapter) Leave(ch ChannelRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.joined, ch.Slug)
	return nil
}

func (f *FakeAdapter) Send(_ context.Context, ch ChannelRef, text string, opts SendOpts) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.caps.Send {
		return ErrUnsupported
	}
	f.sends = append(f.sends, FakeSend{Channel: ch, Text: text, Opts: opts})
	return nil
}

func (f *FakeAdapter) Moderate(_ context.Context, action ModAction) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.caps.Moderation {
		return ErrUnsupported
	}
	f.mods = append(f.mods, action)
	return nil
}

func (f *FakeAdapter) Events() <-chan Event { return f.events }

func (f *FakeAdapter) Health() HealthStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.health
}

func (f *FakeAdapter) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	close(f.events)
	return nil
}

// ---- test-driving helpers ----

// Emit pushes an event onto the adapter's stream (blocks if the buffer is full, like a real
// adapter under backpressure).
func (f *FakeAdapter) Emit(ev Event) { f.events <- ev }

// EmitMessage is a convenience for the common case.
func (f *FakeAdapter) EmitMessage(m UnifiedMessage) { f.events <- MessageEvent{Message: m} }

// SetHealth updates the reported health and emits a corresponding HealthEvent.
func (f *FakeAdapter) SetHealth(s HealthStatus) {
	f.mu.Lock()
	f.health = s
	f.mu.Unlock()
	f.events <- HealthEvent{Status: s}
}

// Joined reports the mode a channel was joined with, and whether it is currently joined.
func (f *FakeAdapter) Joined(slug string) (ConnMode, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.joined[slug]
	return m, ok
}

// Sends returns a copy of recorded Send calls.
func (f *FakeAdapter) Sends() []FakeSend {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeSend(nil), f.sends...)
}

// Mods returns a copy of recorded Moderate calls.
func (f *FakeAdapter) Mods() []ModAction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ModAction(nil), f.mods...)
}

var _ Adapter = (*FakeAdapter)(nil)
