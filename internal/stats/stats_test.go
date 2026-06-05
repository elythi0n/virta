package stats

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
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

func (r *recSubmitter) stats() []platform.StatsEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []platform.StatsEvent
	for _, ev := range r.evs {
		if se, ok := ev.(platform.StatsEvent); ok {
			out = append(out, se)
		}
	}
	return out
}

func chatMsg(slug, author, text string, emotes ...string) platform.MessageEvent {
	segs := []platform.Segment{{Kind: platform.SegText, Text: text}}
	for _, e := range emotes {
		segs = append(segs, platform.Segment{Kind: platform.SegEmote, Text: e})
	}
	return platform.MessageEvent{Message: platform.UnifiedMessage{
		Platform: platform.Twitch,
		Channel:  platform.ChannelRef{Platform: platform.Twitch, Slug: slug},
		Type:     platform.TypeChat,
		Author:   platform.Author{ID: author, Login: author},
		Segments: segs,
	}}
}

func TestAggregator_CountsAndEmits(t *testing.T) {
	clk := clock.NewFake(time.Unix(1000, 0))
	rec := &recSubmitter{}
	a := New(clk, time.Hour) // long tick; we drive emit() directly
	a.Start(rec)
	t.Cleanup(func() { _ = a.Close() })

	_ = a.Consume(context.Background(), chatMsg("forsen", "alice", "hi", "Kappa"))
	_ = a.Consume(context.Background(), chatMsg("forsen", "bob", "yo", "Kappa", "PogU"))
	_ = a.Consume(context.Background(), chatMsg("forsen", "alice", "again"))
	a.emit()

	evs := rec.stats()
	if len(evs) != 1 {
		t.Fatalf("emitted %d stats events, want 1", len(evs))
	}
	s := evs[0].Stats
	if evs[0].Channel.Slug != "forsen" {
		t.Errorf("channel = %+v", evs[0].Channel)
	}
	if s.UniqueChatters != 2 {
		t.Errorf("unique chatters = %d, want 2", s.UniqueChatters)
	}
	if want := 3.0 / float64(windowSeconds); s.MessagesPerSec != want {
		t.Errorf("msg/s = %v, want %v", s.MessagesPerSec, want)
	}
	if len(s.TopEmotes) == 0 || s.TopEmotes[0].Name != "Kappa" || s.TopEmotes[0].Count != 2 {
		t.Errorf("top emotes = %+v, want Kappa leading with 2", s.TopEmotes)
	}
}

func TestAggregator_IgnoresNonMessageEvents(t *testing.T) {
	clk := clock.NewFake(time.Unix(1000, 0))
	rec := &recSubmitter{}
	a := New(clk, time.Hour)
	a.Start(rec)
	t.Cleanup(func() { _ = a.Close() })

	// Feeding back a StatsEvent (the loop case) must not be counted — no feedback loop.
	_ = a.Consume(context.Background(), platform.StatsEvent{Channel: platform.ChannelRef{Slug: "forsen"}})
	_ = a.Consume(context.Background(), platform.HealthEvent{})
	a.emit()
	if len(rec.stats()) != 0 {
		t.Errorf("emitted stats for non-message events: %+v", rec.stats())
	}
}

func TestAggregator_WindowExpiry(t *testing.T) {
	clk := clock.NewFake(time.Unix(1000, 0))
	rec := &recSubmitter{}
	a := New(clk, time.Hour)
	a.Start(rec)
	t.Cleanup(func() { _ = a.Close() })

	_ = a.Consume(context.Background(), chatMsg("forsen", "alice", "hi"))
	clk.Advance(windowSeconds * time.Second) // message now falls outside the window
	a.emit()
	if len(rec.stats()) != 0 {
		t.Errorf("channel still active after the window expired: %+v", rec.stats())
	}
}

func TestTopEmotes_OrderAndCap(t *testing.T) {
	counts := map[string]int{"a": 5, "b": 5, "c": 9, "d": 1, "e": 2, "f": 3}
	top := topEmotes(counts, 3)
	if len(top) != 3 {
		t.Fatalf("len = %d, want 3", len(top))
	}
	// Highest count first; equal counts broken by name ("a" before "b").
	if top[0].Name != "c" || top[1].Name != "a" || top[2].Name != "b" {
		t.Errorf("order = %+v", top)
	}
}

func TestAggregator_CloseIdempotent(t *testing.T) {
	a := New(clock.NewFake(time.Unix(0, 0)), time.Hour)
	a.Start(&recSubmitter{})
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("second close: %v", err)
	}
}

func TestAggregator_Name(t *testing.T) {
	if New(clock.NewFake(time.Unix(0, 0)), 0).Name() != "stats" {
		t.Error("name")
	}
}
