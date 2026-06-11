package moments

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/store"
)

type recSubmitter struct {
	mu  sync.Mutex
	evs []platform.Event
}

func (r *recSubmitter) Submit(ev platform.Event) {
	r.mu.Lock()
	r.evs = append(r.evs, ev)
	r.mu.Unlock()
}

func (r *recSubmitter) moments() []platform.MomentEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []platform.MomentEvent
	for _, ev := range r.evs {
		if me, ok := ev.(platform.MomentEvent); ok {
			out = append(out, me)
		}
	}
	return out
}

func chatMsg(slug, author, text string) platform.MessageEvent {
	return platform.MessageEvent{Message: platform.UnifiedMessage{
		Platform: platform.Twitch,
		Channel:  platform.ChannelRef{Platform: platform.Twitch, Slug: slug},
		Type:     platform.TypeChat,
		Author:   platform.Author{Login: author, DisplayName: author},
		Segments: []platform.Segment{{Kind: platform.SegText, Text: text}},
	}}
}

// newDetector builds a detector wired to a fake clock, the in-memory repo, and a recording
// submitter, without starting the real ticker goroutine — tests drive tick() directly so the
// state machine advances deterministically with the fake clock.
func newDetector(t *testing.T) (*Detector, *clock.Fake, store.MomentRepo, *recSubmitter) {
	t.Helper()
	clk := clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC))
	repo := store.NewMemory(clk).Moments()
	d := New(clk, repo, id.NewFake("mom"), nil)
	rec := &recSubmitter{}
	d.out = rec
	return d, clk, repo, rec
}

// second feeds n messages into one channel, evaluates the tick, and advances one second.
func second(d *Detector, clk *clock.Fake, n int, seq *int) {
	for i := 0; i < n; i++ {
		*seq++
		_ = d.Consume(context.Background(), chatMsg("forsen", fmt.Sprintf("user%d", *seq%5), fmt.Sprintf("msg %d", *seq)))
	}
	d.tick()
	clk.Advance(time.Second)
}

func TestDetector_SpikePersistsMomentAndCooldownSuppressesNext(t *testing.T) {
	d, clk, repo, rec := newDetector(t)
	ctx := context.Background()
	seq := 0

	// Baseline trickle: 1 msg/s for 30s must never trigger.
	for i := 0; i < 30; i++ {
		second(d, clk, 1, &seq)
	}
	if got, _ := repo.List(ctx, store.MomentQuery{Limit: 10}); len(got) != 0 {
		t.Fatalf("trickle opened a moment: %+v", got)
	}

	// Burst: 8 msg/s for 12s pushes the rate well past max(minSpikeRate, factor*baseline).
	for i := 0; i < 12; i++ {
		second(d, clk, 8, &seq)
	}
	// Quiet: the rate drains out of the window; after 5 consecutive sub-threshold seconds the
	// moment closes and is persisted.
	for i := 0; i < 25; i++ {
		second(d, clk, 0, &seq)
	}

	got, err := repo.List(ctx, store.MomentQuery{Channel: "twitch:forsen", Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("persisted %d moments, want 1", len(got))
	}
	m := got[0]
	if m.ID == "" {
		t.Error("moment has no id")
	}
	if m.Channel.Key() != "twitch:forsen" {
		t.Errorf("channel = %+v", m.Channel)
	}
	if m.PeakRate != 8.0 {
		t.Errorf("peak rate = %v, want 8.0", m.PeakRate)
	}
	if m.Baseline <= 0 || m.Baseline >= minSpikeRate {
		t.Errorf("baseline = %v, want a small pre-spike rate", m.Baseline)
	}
	if !m.EndedAt.After(m.StartedAt) {
		t.Errorf("ended %v not after started %v", m.EndedAt, m.StartedAt)
	}
	if m.EndedAt.Sub(m.StartedAt) < minSpikeDuration {
		t.Errorf("duration %v shorter than the minimum", m.EndedAt.Sub(m.StartedAt))
	}
	if len(m.Excerpt) == 0 || len(m.Excerpt) > excerptSize {
		t.Errorf("excerpt len = %d, want 1..%d", len(m.Excerpt), excerptSize)
	}
	for _, ex := range m.Excerpt {
		if ex.Author == "" || ex.Body == "" || ex.SentAt == 0 {
			t.Errorf("excerpt line incomplete: %+v", ex)
		}
	}
	evs := rec.moments()
	if len(evs) != 1 || evs[0].Moment.ID != m.ID {
		t.Fatalf("emitted %d MomentEvents (want 1 matching %s): %+v", len(evs), m.ID, evs)
	}

	// Cooldown: an immediate second burst must not open another moment.
	for i := 0; i < 10; i++ {
		second(d, clk, 8, &seq)
	}
	if got, _ := repo.List(ctx, store.MomentQuery{Limit: 10}); len(got) != 1 {
		t.Errorf("cooldown did not suppress a second moment: %d persisted", len(got))
	}
	if len(rec.moments()) != 1 {
		t.Errorf("cooldown did not suppress a second MomentEvent")
	}
}

func TestDetector_ExcerptBodiesTruncated(t *testing.T) {
	d, clk, _, _ := newDetector(t)
	long := strings.Repeat("a", 3*maxBodyLen)
	_ = d.Consume(context.Background(), chatMsg("forsen", "alice", long))
	clk.Advance(time.Second)

	d.mu.Lock()
	defer d.mu.Unlock()
	cs := d.channels["twitch:forsen"]
	if cs == nil || len(cs.recent) != 1 {
		t.Fatalf("recent ring = %+v", cs)
	}
	if got := len([]rune(cs.recent[0].Body)); got != maxBodyLen {
		t.Errorf("body length = %d, want %d", got, maxBodyLen)
	}
}

func TestDetector_IdleChannelsPruned(t *testing.T) {
	d, clk, _, _ := newDetector(t)
	_ = d.Consume(context.Background(), chatMsg("forsen", "alice", "hi"))
	d.tick()
	d.mu.Lock()
	if len(d.channels) != 1 {
		t.Fatalf("channels = %d, want 1", len(d.channels))
	}
	d.mu.Unlock()

	clk.Advance(idleTimeout + time.Second)
	d.tick()
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.channels) != 0 {
		t.Errorf("idle channel not pruned: %d remain", len(d.channels))
	}
}

func TestDetector_IgnoresNonMessageEvents(t *testing.T) {
	d, _, _, _ := newDetector(t)
	_ = d.Consume(context.Background(), platform.MomentEvent{}) // its own output: no feedback loop
	_ = d.Consume(context.Background(), platform.HealthEvent{})
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.channels) != 0 {
		t.Errorf("non-message events created channel state: %d", len(d.channels))
	}
}

func TestDetector_CloseIdempotent(t *testing.T) {
	d, _, _, rec := newDetector(t)
	d.Start(rec)
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Errorf("second close: %v", err)
	}
}

func TestDetector_Name(t *testing.T) {
	d, _, _, _ := newDetector(t)
	if d.Name() != "moments" {
		t.Error("name")
	}
}
