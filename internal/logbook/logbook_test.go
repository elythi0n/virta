package logbook

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

// fakeStore records appends/deletes/sweeps and enforces the same ephemeral choke as the real
// store, so tests exercise the full guarantee.
type fakeStore struct {
	mu       sync.Mutex
	appended []platform.UnifiedMessage
	deleted  []string
	swept    map[string]time.Time
}

func newFakeStore() *fakeStore { return &fakeStore{swept: map[string]time.Time{}} }

func (s *fakeStore) Append(_ context.Context, msgs []platform.UnifiedMessage) error {
	for _, m := range msgs {
		if m.Ephemeral {
			return platformErrEphemeral
		}
	}
	s.mu.Lock()
	s.appended = append(s.appended, msgs...)
	s.mu.Unlock()
	return nil
}

func (s *fakeStore) MarkDeleted(_ context.Context, id string) error {
	s.mu.Lock()
	s.deleted = append(s.deleted, id)
	s.mu.Unlock()
	return nil
}

func (s *fakeStore) Sweep(_ context.Context, channelID string, olderThan time.Time) (int, error) {
	s.mu.Lock()
	s.swept[channelID] = olderThan
	s.mu.Unlock()
	return 1, nil
}

func (s *fakeStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.appended)
}

func (s *fakeStore) deletes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.deleted...)
}

var platformErrEphemeral = errEphemeral{}

type errEphemeral struct{}

func (errEphemeral) Error() string { return "ephemeral" }

func chat(id, slug, text string, ephemeral bool) platform.MessageEvent {
	return platform.MessageEvent{Message: platform.UnifiedMessage{
		ID: id, Platform: platform.Twitch, Type: platform.TypeChat,
		Channel:   platform.ChannelRef{Platform: platform.Twitch, Slug: slug},
		Author:    platform.Author{ID: "u", DisplayName: "u"},
		Segments:  []platform.Segment{{Kind: platform.SegText, Text: text}},
		Ephemeral: ephemeral,
	}}
}

// TestZeroWriteWhenDisabled is the zero-write guarantee: with logging disabled, nothing is
// persisted no matter what flows through.
func TestZeroWriteWhenDisabled(t *testing.T) {
	fs := newFakeStore()
	s := NewSink(fs, clock.NewFake(time.Unix(0, 0)), nil)
	// enabled defaults to false; feed non-ephemeral messages anyway.
	for i := 0; i < 10; i++ {
		_ = s.Consume(context.Background(), chat("m", "forsen", "hi", false))
	}
	s.flush()
	if fs.count() != 0 {
		t.Errorf("persisted %d messages with logging disabled, want 0", fs.count())
	}
}

// TestDisablingLoggingDiscardsBuffered is the in-flight half of the zero-write guarantee:
// messages buffered while logging was on are non-ephemeral (so the store choke point accepts
// them), so turning logging off must discard the buffer, or a later flush would persist chat the
// user expected to stay unlogged.
func TestDisablingLoggingDiscardsBuffered(t *testing.T) {
	fs := newFakeStore()
	s := NewSink(fs, clock.NewFake(time.Unix(0, 0)), nil)
	s.SetEnabled(true)
	for i := 0; i < 3; i++ {
		_ = s.Consume(context.Background(), chat("m", "forsen", "hi", false))
	}
	s.SetEnabled(false) // user turns logging off before the batch timer fires
	s.flush()           // a flush still happens on the timer / at shutdown
	if fs.count() != 0 {
		t.Errorf("persisted %d buffered messages after logging off, want 0", fs.count())
	}
}

func TestEnabledButEphemeralStillNotWritten(t *testing.T) {
	fs := newFakeStore()
	s := NewSink(fs, clock.NewFake(time.Unix(0, 0)), nil)
	s.SetEnabled(true)
	// An ephemeral message must be skipped even when logging is enabled (defense in depth).
	_ = s.Consume(context.Background(), chat("m", "forsen", "hi", true))
	s.flush()
	if fs.count() != 0 {
		t.Errorf("persisted an ephemeral message, want 0")
	}
}

func TestLoggingOnPersistsBatched(t *testing.T) {
	fs := newFakeStore()
	s := NewSink(fs, clock.NewFake(time.Unix(0, 0)), nil)
	s.SetEnabled(true)
	for i := 0; i < 3; i++ {
		_ = s.Consume(context.Background(), chat("m", "forsen", "hi", false))
	}
	if fs.count() != 0 {
		t.Errorf("wrote before flush (%d); should batch", fs.count())
	}
	s.flush()
	if fs.count() != 3 {
		t.Errorf("persisted %d after flush, want 3", fs.count())
	}
	// channel_id is the stable platform:slug key.
	if fs.appended[0].Channel.ID != "twitch:forsen" {
		t.Errorf("channel id = %q, want twitch:forsen", fs.appended[0].Channel.ID)
	}
}

func TestSizeFlushAt(t *testing.T) {
	fs := newFakeStore()
	s := NewSink(fs, clock.NewFake(time.Unix(0, 0)), nil)
	s.SetEnabled(true)
	for i := 0; i < flushAt; i++ {
		_ = s.Consume(context.Background(), chat("m", "forsen", "hi", false))
	}
	if fs.count() != flushAt {
		t.Errorf("size-triggered flush wrote %d, want %d", fs.count(), flushAt)
	}
}

func TestDeletionMarkedWhenEnabled(t *testing.T) {
	fs := newFakeStore()
	s := NewSink(fs, clock.NewFake(time.Unix(0, 0)), nil)
	del := platform.MessageDeletedEvent{MessageID: "ulid-1"}

	_ = s.Consume(context.Background(), del) // disabled → ignored
	if len(fs.deletes()) != 0 {
		t.Error("marked deleted while disabled")
	}
	s.SetEnabled(true)
	_ = s.Consume(context.Background(), del)
	_ = s.Consume(context.Background(), platform.MessageDeletedEvent{}) // empty id → ignored
	if d := fs.deletes(); len(d) != 1 || d[0] != "ulid-1" {
		t.Errorf("deletes = %v, want [ulid-1]", d)
	}
}

func TestSweeper_RetentionAndForever(t *testing.T) {
	fs := newFakeStore()
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	s := NewSink(fs, clk, nil)
	s.SetEnabled(true)
	_ = s.Consume(context.Background(), chat("m", "forsen", "hi", false))
	s.flush()

	sw := NewSweeper(s, clk)
	// "forever" → no sweep.
	if sw.SetRetention("forever") {
		t.Error("forever should not enable time sweeping")
	}
	sw.sweep()
	if len(fs.swept) != 0 {
		t.Error("swept under forever retention")
	}
	// "7d" → sweeps logged channels with the cutoff.
	if !sw.SetRetention("7d") {
		t.Fatal("7d should enable sweeping")
	}
	sw.sweep()
	cutoff, ok := fs.swept["twitch:forsen"]
	if !ok {
		t.Fatal("did not sweep the logged channel")
	}
	if want := clk.Now().Add(-7 * 24 * time.Hour); !cutoff.Equal(want) {
		t.Errorf("cutoff = %v, want %v", cutoff, want)
	}
}

func TestSink_Name(t *testing.T) {
	if NewSink(newFakeStore(), clock.NewFake(time.Unix(0, 0)), nil).Name() != "logbook" {
		t.Error("name")
	}
}

func TestSweeper_StartCloseLifecycle(t *testing.T) {
	s := NewSink(newFakeStore(), clock.NewFake(time.Unix(0, 0)), nil)
	sw := NewSweeper(s, clock.NewFake(time.Unix(0, 0)))
	sw.Start()
	if err := sw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := sw.Close(); err != nil {
		t.Errorf("second close: %v", err)
	}
}

// errStore fails Append, to exercise the drop-batch-on-error path.
type errStore struct{}

func (errStore) Append(context.Context, []platform.UnifiedMessage) error { return errEphemeral{} }
func (errStore) MarkDeleted(context.Context, string) error               { return nil }
func (errStore) Sweep(context.Context, string, time.Time) (int, error)   { return 0, nil }

func TestFlush_DropsBatchOnError(t *testing.T) {
	s := NewSink(&errStore{}, clock.NewFake(time.Unix(0, 0)), nil)
	s.SetEnabled(true)
	_ = s.Consume(context.Background(), chat("m", "forsen", "hi", false))
	s.flush() // must not panic; batch is dropped
	// A second flush has nothing buffered.
	s.flush()
}

func TestParseRetention(t *testing.T) {
	cases := map[string]struct {
		d  time.Duration
		ok bool
	}{
		"7d":      {7 * 24 * time.Hour, true},
		"48h":     {48 * time.Hour, true},
		"forever": {0, false},
		"":        {0, false},
		"100":     {0, false}, // count-based: not a time window
		"garbage": {0, false},
	}
	for in, want := range cases {
		d, ok := parseRetention(in)
		if ok != want.ok || d != want.d {
			t.Errorf("parseRetention(%q) = (%v,%v), want (%v,%v)", in, d, ok, want.d, want.ok)
		}
	}
}

func TestSink_CloseFlushesAndIsIdempotent(t *testing.T) {
	fs := newFakeStore()
	s := NewSink(fs, clock.NewFake(time.Unix(0, 0)), nil)
	s.SetEnabled(true)
	s.Start()
	_ = s.Consume(context.Background(), chat("m", "forsen", "hi", false))
	if err := s.Close(); err != nil { // final drain
		t.Fatal(err)
	}
	if fs.count() != 1 {
		t.Errorf("close did not flush the final batch: %d", fs.count())
	}
	if err := s.Close(); err != nil {
		t.Errorf("second close: %v", err)
	}
}
