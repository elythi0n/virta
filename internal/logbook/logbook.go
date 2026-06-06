// Package logbook persists chat to the store when logging is enabled. It is the only writer of
// the messages table, and writing is opt-in: the engine marks messages ephemeral while logging
// is off, the Sink skips them, and the store's Append refuses them — three layers guaranteeing
// "logging off ⇒ nothing written" (ADR-014). When on, the Sink batches inserts (by size or a
// short timer) so persistence never stalls the feed, and a Sweeper enforces retention.
package logbook

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

const (
	flushEvery = 250 * time.Millisecond
	flushAt    = 200 // messages — buffer is drained synchronously at this size
	sweepEvery = 10 * time.Minute
)

// MessageStore is the subset of the store the logbook writes through.
type MessageStore interface {
	Append(ctx context.Context, msgs []platform.UnifiedMessage) error
	MarkDeleted(ctx context.Context, id string) error
	Sweep(ctx context.Context, channelID string, olderThan time.Time) (int, error)
}

// Sink is the logging pipeline sink: it batches non-ephemeral messages and writes them when
// logging is enabled. Construct with NewSink, then Start.
type Sink struct {
	store MessageStore
	clk   clock.Clock
	log   *slog.Logger

	enabled atomic.Bool
	closed  atomic.Bool

	mu       sync.Mutex
	buf      []platform.UnifiedMessage
	channels map[string]struct{} // channel ids written to, for the sweeper

	quit      chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
	closeOnce sync.Once
}

// NewSink builds the logging sink.
func NewSink(store MessageStore, clk clock.Clock, log *slog.Logger) *Sink {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Sink{
		store:    store,
		clk:      clk,
		log:      log,
		channels: map[string]struct{}{},
		quit:     make(chan struct{}),
	}
}

func (s *Sink) Name() string { return "logbook" }

// SetEnabled turns logging on or off (the profile manager calls this on activation). When off,
// Consume buffers nothing and any messages buffered while logging was on are discarded, so a
// later flush can't persist chat received before the user switched logging off.
func (s *Sink) SetEnabled(on bool) {
	s.enabled.Store(on)
	if !on {
		s.mu.Lock()
		s.buf = nil
		s.mu.Unlock()
	}
}

// Enabled reports whether persistent logging is currently on, so callers (e.g. the history API) can
// choose the durable store over the in-memory scrollback ring.
func (s *Sink) Enabled() bool { return s.enabled.Load() }

// Consume buffers a chat message for persistence (when logging is on and it isn't ephemeral)
// and applies deletions. Other events are ignored.
func (s *Sink) Consume(_ context.Context, ev platform.Event) error {
	if s.closed.Load() || !s.enabled.Load() {
		return nil
	}
	switch e := ev.(type) {
	case platform.MessageEvent:
		if e.Message.Ephemeral {
			return nil
		}
		m := e.Message
		m.Channel.ID = channelID(m.Channel) // stable opaque channel key for history grouping
		s.mu.Lock()
		s.buf = append(s.buf, m)
		s.channels[m.Channel.ID] = struct{}{}
		full := len(s.buf) >= flushAt
		s.mu.Unlock()
		if full {
			s.flush()
		}
	case platform.MessageDeletedEvent:
		if e.MessageID == "" {
			return nil // never logged / aged out of the id map
		}
		if err := s.store.MarkDeleted(context.Background(), e.MessageID); err != nil {
			s.log.Warn("logbook mark-deleted failed", "id", e.MessageID, "err", err)
		}
	}
	return nil
}

// Start launches the periodic flush loop. Idempotent.
func (s *Sink) Start() {
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.loop()
	})
}

func (s *Sink) loop() {
	defer s.wg.Done()
	t := time.NewTicker(flushEvery)
	defer t.Stop()
	for {
		select {
		case <-s.quit:
			s.flush() // final drain
			return
		case <-t.C:
			s.flush()
		}
	}
}

// flush writes the buffered batch. A failed write drops the batch (chat logging is
// best-effort) rather than blocking the feed or growing memory.
func (s *Sink) flush() {
	s.mu.Lock()
	if len(s.buf) == 0 {
		s.mu.Unlock()
		return
	}
	batch := s.buf
	s.buf = nil
	s.mu.Unlock()

	if err := s.store.Append(context.Background(), batch); err != nil {
		s.log.Warn("logbook append failed; dropping batch", "n", len(batch), "err", err)
	}
}

// loggedChannels returns the channel ids written to (for the sweeper).
func (s *Sink) loggedChannels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.channels))
	for id := range s.channels {
		out = append(out, id)
	}
	return out
}

// Close stops the flush loop after a final drain. Idempotent; satisfies pipeline.Sink.
func (s *Sink) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true) // stop accepting before the final drain
		close(s.quit)
		s.wg.Wait()
	})
	return nil
}

func channelID(ch platform.ChannelRef) string { return string(ch.Platform) + ":" + ch.Slug }
