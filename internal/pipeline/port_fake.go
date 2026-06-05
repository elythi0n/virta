package pipeline

import (
	"context"
	"sync"

	"github.com/elythi0n/virta/internal/platform"
)

// RecordingSink is a Sink that captures every event it consumes — the standard sink double
// for pipeline, engine, and api tests. Safe for concurrent use.
type RecordingSink struct {
	name string

	mu     sync.Mutex
	events []platform.Event
	closed bool

	// gate, when non-nil, blocks Consume until it is closed — simulates a slow sink so
	// tests can verify it doesn't stall others. nil means "consume immediately".
	gate chan struct{}

	// entered is closed the first time Consume is called, so a test can wait until the
	// sink is genuinely blocked before continuing.
	entered   chan struct{}
	enterOnce sync.Once
}

// NewRecordingSink creates a recording sink with the given name.
func NewRecordingSink(name string) *RecordingSink { return &RecordingSink{name: name} }

// NewBlockingSink creates a recording sink whose Consume blocks until Release is called.
func NewBlockingSink(name string) *RecordingSink {
	return &RecordingSink{name: name, gate: make(chan struct{}), entered: make(chan struct{})}
}

// Entered returns a channel closed when Consume is first called. Only meaningful for a
// blocking sink; lets a test wait until the sink is actually blocked.
func (s *RecordingSink) Entered() <-chan struct{} { return s.entered }

// Release unblocks a sink created with NewBlockingSink.
func (s *RecordingSink) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gate != nil {
		close(s.gate)
		s.gate = nil
	}
}

func (s *RecordingSink) Name() string { return s.name }

func (s *RecordingSink) Consume(ctx context.Context, ev platform.Event) error {
	s.enterOnce.Do(func() {
		if s.entered != nil {
			close(s.entered)
		}
	})
	s.mu.Lock()
	gate := s.gate
	s.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	s.events = append(s.events, ev)
	s.mu.Unlock()
	return nil
}

func (s *RecordingSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// Events returns a snapshot of consumed events.
func (s *RecordingSink) Events() []platform.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]platform.Event(nil), s.events...)
}

// Messages returns just the messages consumed (filtering out non-message events).
func (s *RecordingSink) Messages() []platform.UnifiedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []platform.UnifiedMessage
	for _, ev := range s.events {
		if me, ok := ev.(platform.MessageEvent); ok {
			out = append(out, me.Message)
		}
	}
	return out
}

// Closed reports whether Close was called.
func (s *RecordingSink) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

var _ Sink = (*RecordingSink)(nil)

// TagStage is a trivial pure Stage that appends a text segment — the standard stage double.
// It proves annotations applied by stages reach the sinks (and, in ordering tests, that
// stages run in order).
type TagStage struct{ Tag string }

// NewTagStage returns a stage that appends tag as a text segment.
func NewTagStage(tag string) *TagStage { return &TagStage{Tag: tag} }

func (s *TagStage) Name() string { return "tag:" + s.Tag }

func (s *TagStage) Annotate(_ context.Context, msg *platform.UnifiedMessage) error {
	msg.Segments = append(msg.Segments, platform.Segment{Kind: platform.SegText, Text: s.Tag})
	return nil
}

var _ Stage = (*TagStage)(nil)
