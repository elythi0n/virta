// Package velocity is the pipeline stage that keeps a flooded channel readable. When a channel's
// chat rate climbs past a threshold, it marks a proportion of ordinary messages with the Sampled
// annotation — a display-only hint that calm-mode frontends may thin the on-screen stream down to
// roughly the threshold rate. It never drops or hides a message: every sink (the logger, webhooks)
// still receives the full stream, so the annotation only changes what a UI chooses to draw.
//
// Priority lanes are never sampled: a channel's broadcaster, moderators, VIPs, and subscribers, a
// chatter's first message, and every non-chat event (subs, raids, announcements) always show, so
// the messages a moderator most needs to see survive any flood.
//
// The stage is stateful (it tracks each channel's recent rate), which a pure stage cannot do. That
// is safe here because the pipeline runs all stages on a single dispatcher goroutine, so the
// per-channel state below is only ever touched by one goroutine; the threshold, which a settings
// control may change from elsewhere, is the one field guarded for concurrency (an atomic).
package velocity

import (
	"context"
	"sync/atomic"

	"github.com/elythi0n/virta/internal/platform"
)

// windowSeconds is the rolling window the rate estimate covers. A few seconds smooths per-second
// jitter while still reacting to a surge within a second or two.
const windowSeconds = 3

// DefaultThreshold is the chat rate (messages per second) above which sampling begins.
const DefaultThreshold = 25

// channelRate is one channel's rolling per-second message counts plus the proportional keep
// accumulator. Touched only by the dispatcher goroutine, so it needs no lock.
type channelRate struct {
	buckets  [windowSeconds]int64 // per-second counts, indexed by second%windowSeconds
	secs     [windowSeconds]int64 // the unix second each bucket currently holds
	firstSec int64                // first second observed, so warmup divides by elapsed not the full window
	keep     float64              // proportional-keep accumulator for non-priority messages
}

// record tallies a message at unix second `now` and returns the channel's current rate (msgs/sec)
// over the rolling window, dividing by elapsed seconds during warmup so a fresh channel is not
// reported as artificially slow.
func (c *channelRate) record(now int64) float64 {
	if c.firstSec == 0 {
		c.firstSec = now
	}
	idx := now % windowSeconds
	if c.secs[idx] != now {
		c.secs[idx] = now
		c.buckets[idx] = 0
	}
	c.buckets[idx]++

	var total int64
	for i := 0; i < windowSeconds; i++ {
		if now-c.secs[i] < windowSeconds {
			total += c.buckets[i]
		}
	}
	span := now - c.firstSec + 1
	if span > windowSeconds {
		span = windowSeconds
	}
	if span < 1 {
		span = 1
	}
	return float64(total) / float64(span)
}

// Stage marks overload messages as sampled. Add it to the pipeline after the badge stage so
// subscriber/mod badges (which drive priority lanes) are already resolved on the message.
type Stage struct {
	threshold atomic.Int64
	channels  map[string]*channelRate
}

// NewStage builds a velocity stage. A threshold of zero (or negative) disables sampling entirely;
// pass DefaultThreshold for the standard behavior.
func NewStage(threshold int) *Stage {
	s := &Stage{channels: map[string]*channelRate{}}
	s.threshold.Store(int64(threshold))
	return s
}

// Name identifies the stage in diagnostics.
func (s *Stage) Name() string { return "velocity" }

// SetThreshold changes the messages-per-second trigger at runtime (e.g. from a settings change).
// Zero or negative disables sampling. Safe to call from another goroutine.
func (s *Stage) SetThreshold(n int) { s.threshold.Store(int64(n)) }

// Annotate records the message toward its channel's rate and, when the channel is over threshold,
// marks ordinary messages as sampled in proportion to the overload so roughly `threshold`
// non-priority messages per second stay unmarked. Priority messages are recorded but never marked.
func (s *Stage) Annotate(_ context.Context, msg *platform.UnifiedMessage) error {
	// Non-chat events (subs, raids, announcements) are inherently priority and don't count toward
	// the chat flood, so they pass through untouched.
	if msg.Type != platform.TypeChat {
		return nil
	}
	threshold := s.threshold.Load()
	key := msg.Channel.Key()
	c := s.channels[key]
	if c == nil {
		c = &channelRate{}
		s.channels[key] = c
	}
	rate := c.record(msg.ReceivedAt.Unix())

	if threshold <= 0 || rate <= float64(threshold) {
		c.keep = 0 // under threshold: keep everything, reset for the next surge
		return nil
	}
	if isPriority(msg) {
		return nil
	}
	// Proportional keep: admit threshold/rate of non-priority messages, sample the rest. The
	// accumulator carries the fraction across messages so the kept share is exact over time.
	c.keep += float64(threshold) / rate
	if c.keep >= 1 {
		c.keep -= 1
		return nil
	}
	msg.Annotate().Sampled = true
	return nil
}

// isPriority reports whether a message belongs to a never-sampled lane: a first-time chatter, or an
// author whose badges mark them as broadcaster/moderator/VIP/subscriber/founder.
func isPriority(m *platform.UnifiedMessage) bool {
	if m.Annotations != nil && m.Annotations.FirstTime {
		return true
	}
	for _, b := range m.Author.Badges {
		switch b.Set {
		case "broadcaster", "moderator", "vip", "subscriber", "founder":
			return true
		}
	}
	return false
}
