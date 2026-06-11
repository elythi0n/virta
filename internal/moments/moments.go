// Package moments watches the live event stream for chat-activity spikes and bookmarks each
// one as a persisted "moment": when a channel's message rate jumps well above its slow-moving
// baseline the detector opens a moment, tracks the peak rate, samples a few spike messages as
// an excerpt, and when the rate subsides it closes the moment, persists it, and emits a
// MomentEvent. It is a pipeline Sink (it sees every event) and feeds its derived events back
// through the pipeline, so they reach WS clients on the same path as everything else — the
// same shape as the stats aggregator.
package moments

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/store"
)

const (
	// windowSeconds is the rolling window the spike rate is computed over. Shorter than the
	// stats window so a spike registers within seconds rather than being averaged away.
	windowSeconds = 10
	// excerptSize caps how many sample messages a moment carries.
	excerptSize = 8
	// maxBodyLen truncates excerpt bodies so a moment row stays small.
	maxBodyLen = 140
	// baselineAlpha is the EWMA weight for the slow baseline, applied once per second. The
	// baseline is only updated outside a spike, so a spike never inflates its own threshold.
	baselineAlpha = 0.02
	// minSpikeRate is the floor (msgs/sec) a channel must reach to open a moment, so a quiet
	// channel's tiny baseline doesn't make every third message a "spike".
	minSpikeRate = 5.0
	// spikeFactor is how far above the baseline the rate must climb to open a moment.
	spikeFactor = 3.0
	// closeQuietSeconds is how many consecutive below-threshold seconds close a moment.
	closeQuietSeconds = 5
	// cooldownSeconds suppresses a new moment on a channel right after one closed.
	cooldownSeconds = 30
	// minSpikeDuration discards rate blips shorter than this.
	minSpikeDuration = 3 * time.Second
	// idleTimeout drops a channel's state once it has been silent this long.
	idleTimeout = 5 * time.Minute
	// defaultTick is the state-machine evaluation interval.
	defaultTick = time.Second
)

// Submitter is the pipeline entry point the detector feeds its derived events into.
type Submitter interface {
	Submit(ev platform.Event)
}

// bucket holds one second of a channel's message count.
type bucket struct {
	sec   int64
	count int
}

// channelState is one channel's spike state machine: a ring of per-second counts, the slow
// baseline, the recent-message ring for excerpts, and the open moment (if any).
type channelState struct {
	ref      platform.ChannelRef
	buckets  [windowSeconds]bucket
	lastSeen int64 // unix sec of the last message; idle channels are pruned

	baseline float64

	recent []platform.MomentMessage // last excerptSize messages, oldest first

	inSpike       bool
	moment        platform.Moment // accumulating while inSpike
	quietSecs     int             // consecutive below-threshold seconds while inSpike
	cooldownUntil int64           // unix sec before which no new moment may open
}

func (cs *channelState) record(now int64) {
	b := &cs.buckets[now%windowSeconds]
	if b.sec != now {
		b.sec = now
		b.count = 0
	}
	b.count++
	cs.lastSeen = now
}

// rate is the channel's messages/sec over the rolling window ending at now.
func (cs *channelState) rate(now int64) float64 {
	total := 0
	for i := range cs.buckets {
		if b := &cs.buckets[i]; now-b.sec < windowSeconds && b.sec <= now {
			total += b.count
		}
	}
	return float64(total) / float64(windowSeconds)
}

// remember pushes one message into the bounded recent ring (oldest dropped).
func (cs *channelState) remember(m platform.MomentMessage) {
	cs.recent = append(cs.recent, m)
	if len(cs.recent) > excerptSize {
		cs.recent = cs.recent[1:]
	}
}

// snapshotRecent copies the current recent ring.
func (cs *channelState) snapshotRecent() []platform.MomentMessage {
	return append([]platform.MomentMessage{}, cs.recent...)
}

// refreshExcerpt blends the open moment's excerpt with the current ring: the onset half is
// kept, the rest is refilled with the newest distinct messages, so the excerpt reflects the
// peak rather than just the trigger. Capped at excerptSize, deduped by message identity.
func (cs *channelState) refreshExcerpt() {
	head := cs.moment.Excerpt
	if half := excerptSize / 2; len(head) > half {
		head = head[:half]
	}
	out := append([]platform.MomentMessage{}, head...)
	seen := make(map[platform.MomentMessage]struct{}, excerptSize)
	for _, m := range out {
		seen[m] = struct{}{}
	}
	// Newest distinct ring messages fill the remaining slots; pick from the end so the
	// freshest survive the cap, then append in chronological order.
	var fresh []platform.MomentMessage
	for i := len(cs.recent) - 1; i >= 0 && len(out)+len(fresh) < excerptSize; i-- {
		m := cs.recent[i]
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		fresh = append(fresh, m)
	}
	for i := len(fresh) - 1; i >= 0; i-- {
		out = append(out, fresh[i])
	}
	cs.moment.Excerpt = out
}

// Detector is the moments Sink. Construct with New, then Start once the pipeline exists.
type Detector struct {
	clk  clock.Clock
	repo store.MomentRepo
	gen  id.Generator
	log  *slog.Logger

	mu       sync.Mutex
	channels map[string]*channelState

	out       Submitter
	quit      chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
	closeOnce sync.Once
}

// New builds a detector persisting closed moments to repo, minting their ids with gen.
func New(clk clock.Clock, repo store.MomentRepo, gen id.Generator, log *slog.Logger) *Detector {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Detector{
		clk:      clk,
		repo:     repo,
		gen:      gen,
		log:      log,
		channels: map[string]*channelState{},
		quit:     make(chan struct{}),
	}
}

func (d *Detector) Name() string { return "moments" }

// Consume tallies chat messages into the per-channel rate ring and excerpt ring; every other
// event (including the MomentEvents this detector emits) is ignored, so there's no feedback loop.
func (d *Detector) Consume(_ context.Context, ev platform.Event) error {
	me, ok := ev.(platform.MessageEvent)
	if !ok {
		return nil
	}
	m := me.Message
	now := d.clk.Now()

	d.mu.Lock()
	cs := d.channels[m.Channel.Key()]
	if cs == nil {
		cs = &channelState{ref: m.Channel}
		d.channels[m.Channel.Key()] = cs
	}
	cs.record(now.Unix())
	cs.remember(excerptMessage(m, now))
	d.mu.Unlock()
	return nil
}

// excerptMessage reduces a message to its excerpt line (author, truncated body, sent-at ms).
func excerptMessage(m platform.UnifiedMessage, now time.Time) platform.MomentMessage {
	author := m.Author.DisplayName
	if author == "" {
		author = m.Author.Login
	}
	if author == "" {
		author = m.Author.ID
	}
	body := m.PlainText()
	if runes := []rune(body); len(runes) > maxBodyLen {
		body = string(runes[:maxBodyLen])
	}
	sentAt := m.SentAt
	if sentAt.IsZero() {
		sentAt = now
	}
	return platform.MomentMessage{Author: author, Body: body, SentAt: sentAt.UnixMilli()}
}

// Start begins the periodic evaluation loop, feeding MomentEvents into out. Idempotent.
func (d *Detector) Start(out Submitter) {
	d.startOnce.Do(func() {
		d.out = out
		d.wg.Add(1)
		go d.loop()
	})
}

func (d *Detector) loop() {
	defer d.wg.Done()
	t := time.NewTicker(defaultTick)
	defer t.Stop()
	for {
		select {
		case <-d.quit:
			return
		case <-t.C:
			d.tick()
		}
	}
}

// tick advances every channel's state machine by one second: update the baseline, open a
// moment on a spike, track its peak, and close it once the rate has subsided.
func (d *Detector) tick() {
	now := d.clk.Now()
	nowSec := now.Unix()

	d.mu.Lock()
	var closed []platform.Moment
	for key, cs := range d.channels {
		if nowSec-cs.lastSeen > int64(idleTimeout/time.Second) {
			delete(d.channels, key) // idle channel: drop its state entirely
			continue
		}
		rate := cs.rate(nowSec)
		if cs.inSpike {
			if m, ok := cs.advanceSpike(now, rate); ok {
				closed = append(closed, m)
			}
			continue
		}
		// The baseline learns only outside a spike, so a spike can't inflate its own threshold.
		cs.baseline = cs.baseline*(1-baselineAlpha) + rate*baselineAlpha
		if nowSec >= cs.cooldownUntil && rate >= math.Max(minSpikeRate, spikeFactor*cs.baseline) {
			cs.inSpike = true
			cs.quietSecs = 0
			cs.moment = platform.Moment{
				ID:        d.gen.New(),
				Channel:   cs.ref,
				StartedAt: now,
				PeakRate:  rate,
				Baseline:  cs.baseline,
				Excerpt:   cs.snapshotRecent(),
			}
		}
	}
	d.mu.Unlock()

	for _, m := range closed {
		if err := d.repo.Add(context.Background(), m); err != nil {
			d.log.Error("persist moment failed", "channel", m.Channel.Key(), "err", err)
		}
		if d.out != nil {
			d.out.Submit(platform.MomentEvent{Moment: m})
		}
	}
}

// advanceSpike runs one second of an open moment: track the peak, refresh the excerpt near it,
// and close once the rate has stayed below the close threshold long enough. Returns the
// finished moment when it closes (and is long enough to keep).
func (cs *channelState) advanceSpike(now time.Time, rate float64) (platform.Moment, bool) {
	if rate > cs.moment.PeakRate {
		cs.moment.PeakRate = rate
	}
	if rate >= cs.moment.PeakRate*0.9 {
		cs.refreshExcerpt() // near the peak: make the excerpt reflect it, not just the onset
	}
	if rate >= math.Max(minSpikeRate*0.6, cs.baseline*1.5) {
		cs.quietSecs = 0
		return platform.Moment{}, false
	}
	cs.quietSecs++
	if cs.quietSecs < closeQuietSeconds {
		return platform.Moment{}, false
	}
	cs.inSpike = false
	cs.cooldownUntil = now.Unix() + cooldownSeconds
	if now.Sub(cs.moment.StartedAt) < minSpikeDuration {
		return platform.Moment{}, false // a blip, not a moment
	}
	m := cs.moment
	m.EndedAt = now
	return m, true
}

// Close stops the evaluation loop. Idempotent; satisfies pipeline.Sink.
func (d *Detector) Close() error {
	d.closeOnce.Do(func() {
		close(d.quit)
		d.wg.Wait()
	})
	return nil
}
