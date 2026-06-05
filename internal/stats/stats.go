// Package stats aggregates the live event stream into per-channel activity metrics — rolling
// messages/sec, unique chatters, and an emote leaderboard — and emits them as periodic
// StatsEvents. It is a pipeline Sink (it sees every event) and feeds its derived events back
// through the pipeline, so they reach WS clients on the same path as everything else.
//
// Frontends combine per-channel snapshots into the cross-platform totals shown in the stats
// panel; the core stays per-channel and simple.
package stats

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

const (
	windowSeconds = 60 // rolling window for all metrics
	topEmoteCount = 5
	defaultTick   = time.Second
)

// Submitter is the pipeline entry point stats feeds its derived events into.
type Submitter interface {
	Submit(ev platform.Event)
}

// bucket holds one second of a channel's activity.
type bucket struct {
	sec     int64
	count   int
	authors map[string]struct{}
	emotes  map[string]int
}

func (b *bucket) reset(sec int64) {
	b.sec = sec
	b.count = 0
	b.authors = make(map[string]struct{})
	b.emotes = make(map[string]int)
}

// channelStats is a ring of per-second buckets for one channel.
type channelStats struct {
	ref     platform.ChannelRef
	buckets [windowSeconds]bucket
}

func (cs *channelStats) record(now int64, author string, emoteNames []string) {
	b := &cs.buckets[now%windowSeconds]
	if b.sec != now || b.authors == nil {
		b.reset(now)
	}
	b.count++
	if author != "" {
		b.authors[author] = struct{}{}
	}
	for _, n := range emoteNames {
		b.emotes[n]++
	}
}

func (cs *channelStats) snapshot(now int64) platform.StatsSnapshot {
	total := 0
	authors := make(map[string]struct{})
	emotes := make(map[string]int)
	for i := range cs.buckets {
		b := &cs.buckets[i]
		if b.authors == nil || now-b.sec >= windowSeconds || b.sec > now {
			continue // empty or outside the window
		}
		total += b.count
		for a := range b.authors {
			authors[a] = struct{}{}
		}
		for n, c := range b.emotes {
			emotes[n] += c
		}
	}
	return platform.StatsSnapshot{
		WindowSeconds:  windowSeconds,
		MessagesPerSec: float64(total) / float64(windowSeconds),
		UniqueChatters: len(authors),
		TopEmotes:      topEmotes(emotes, topEmoteCount),
	}
}

func (cs *channelStats) active(now int64) bool {
	for i := range cs.buckets {
		if b := &cs.buckets[i]; b.authors != nil && now-b.sec < windowSeconds && b.sec <= now {
			return true
		}
	}
	return false
}

// Aggregator is the stats Sink. Construct with New, then Start once the pipeline exists.
type Aggregator struct {
	clk      clock.Clock
	interval time.Duration

	mu       sync.Mutex
	channels map[string]*channelStats

	out       Submitter
	quit      chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
	closeOnce sync.Once
}

// New builds an aggregator. interval <= 0 emits once per second.
func New(clk clock.Clock, interval time.Duration) *Aggregator {
	if interval <= 0 {
		interval = defaultTick
	}
	return &Aggregator{
		clk:      clk,
		interval: interval,
		channels: map[string]*channelStats{},
		quit:     make(chan struct{}),
	}
}

func (a *Aggregator) Name() string { return "stats" }

// Consume tallies chat messages; every other event (including the StatsEvents this aggregator
// emits) is ignored, so there's no feedback loop.
func (a *Aggregator) Consume(_ context.Context, ev platform.Event) error {
	me, ok := ev.(platform.MessageEvent)
	if !ok {
		return nil
	}
	m := me.Message
	author := m.Author.ID
	if author == "" {
		author = m.Author.Login
	}
	var emoteNames []string
	for _, seg := range m.Segments {
		if seg.Kind == platform.SegEmote {
			emoteNames = append(emoteNames, seg.Text)
		}
	}
	key := channelKey(m.Channel)
	now := a.clk.Now().Unix()

	a.mu.Lock()
	cs := a.channels[key]
	if cs == nil {
		cs = &channelStats{ref: m.Channel}
		a.channels[key] = cs
	}
	cs.record(now, author, emoteNames)
	a.mu.Unlock()
	return nil
}

// Start begins the periodic emit loop, feeding StatsEvents into out. Idempotent.
func (a *Aggregator) Start(out Submitter) {
	a.startOnce.Do(func() {
		a.out = out
		a.wg.Add(1)
		go a.loop()
	})
}

func (a *Aggregator) loop() {
	defer a.wg.Done()
	t := time.NewTicker(a.interval)
	defer t.Stop()
	for {
		select {
		case <-a.quit:
			return
		case <-t.C:
			a.emit()
		}
	}
}

// emit snapshots every active channel and submits a StatsEvent for each.
func (a *Aggregator) emit() {
	now := a.clk.Now().Unix()
	a.mu.Lock()
	events := make([]platform.StatsEvent, 0, len(a.channels))
	for _, cs := range a.channels {
		if !cs.active(now) {
			continue
		}
		events = append(events, platform.StatsEvent{Channel: cs.ref, Stats: cs.snapshot(now)})
	}
	a.mu.Unlock()
	for _, ev := range events {
		a.out.Submit(ev)
	}
}

// Close stops the emit loop. Idempotent; satisfies pipeline.Sink.
func (a *Aggregator) Close() error {
	a.closeOnce.Do(func() {
		close(a.quit)
		a.wg.Wait()
	})
	return nil
}

func channelKey(ch platform.ChannelRef) string { return string(ch.Platform) + ":" + ch.Slug }

// topEmotes returns the n most-used emotes, ties broken by name for stable output.
func topEmotes(counts map[string]int, n int) []platform.EmoteCount {
	if len(counts) == 0 {
		return nil
	}
	out := make([]platform.EmoteCount, 0, len(counts))
	for name, c := range counts {
		out = append(out, platform.EmoteCount{Name: name, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
