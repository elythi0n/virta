// Package ratelimit governs outbound sends: per-channel token buckets with an ordered queue,
// so a burst is paced to the platform's limit and delivered in order, never silently dropped.
// Queue depth and the countdown to the next send are observable (the UI shows them). The
// governor is clock-driven via Drain rather than self-timing, so it's deterministic under a
// fake clock in tests and paced by a ticker in production.
package ratelimit

import (
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
)

// Limit is a token-bucket policy: Burst tokens, refilled to full over Window.
type Limit struct {
	Burst  int
	Window time.Duration
}

func (l Limit) refillPerSec() float64 {
	if l.Window <= 0 || l.Burst <= 0 {
		return 0
	}
	return float64(l.Burst) / l.Window.Seconds()
}

// entry is one queued send and the channel its result is delivered on.
type entry struct {
	send   func() error
	result chan error
}

// channelState is one channel's bucket + FIFO queue.
type channelState struct {
	limit      Limit
	tokens     float64
	lastRefill time.Time
	queue      []*entry
}

// Governor paces sends per channel key (e.g. "twitch:forsen").
type Governor struct {
	clk      clock.Clock
	def      Limit
	mu       sync.Mutex
	channels map[string]*channelState
}

// New builds a governor with a default per-channel limit.
func New(clk clock.Clock, def Limit) *Governor {
	return &Governor{clk: clk, def: def, channels: map[string]*channelState{}}
}

// SetLimit overrides a channel's limit (e.g. upgrading a user bucket to a moderator bucket).
// Existing tokens are clamped to the new burst.
func (g *Governor) SetLimit(key string, l Limit) {
	g.mu.Lock()
	defer g.mu.Unlock()
	cs := g.state(key)
	cs.limit = l
	if cs.tokens > float64(l.Burst) {
		cs.tokens = float64(l.Burst)
	}
}

// Submit queues a send for key, returning a channel that receives its result (nil on success)
// once the governor dispatches it — nothing is ever dropped silently. Call Drain (or run a
// drain loop) to make progress.
func (g *Governor) Submit(key string, send func() error) <-chan error {
	res := make(chan error, 1)
	g.mu.Lock()
	cs := g.state(key)
	cs.queue = append(cs.queue, &entry{send: send, result: res})
	g.mu.Unlock()
	g.Drain()
	return res
}

// Drain refills every channel's bucket and dispatches as many queued sends as tokens allow, in
// order. The actual send runs outside the lock (it may do network I/O). Production calls this
// on a ticker; tests call it after advancing the clock.
func (g *Governor) Drain() {
	now := g.clk.Now()
	g.mu.Lock()
	var ready []*entry
	for _, cs := range g.channels {
		cs.refill(now)
		for cs.tokens >= 1 && len(cs.queue) > 0 {
			e := cs.queue[0]
			cs.queue = cs.queue[1:]
			cs.tokens--
			ready = append(ready, e)
		}
	}
	g.mu.Unlock()

	for _, e := range ready {
		e.result <- e.send()
	}
}

// State reports a channel's queue depth and the time until its next send is permitted (0 if a
// token is available now).
func (g *Governor) State(key string) (queued int, nextIn time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	cs, ok := g.channels[key]
	if !ok {
		return 0, 0
	}
	cs.refill(g.clk.Now())
	if cs.tokens >= 1 || len(cs.queue) == 0 {
		return len(cs.queue), 0
	}
	rps := cs.limit.refillPerSec()
	if rps <= 0 {
		return len(cs.queue), 0
	}
	need := 1 - cs.tokens
	return len(cs.queue), time.Duration(need / rps * float64(time.Second))
}

// state returns (creating) the per-channel state under g.mu.
func (g *Governor) state(key string) *channelState {
	cs, ok := g.channels[key]
	if !ok {
		cs = &channelState{limit: g.def, tokens: float64(g.def.Burst), lastRefill: g.clk.Now()}
		g.channels[key] = cs
	}
	return cs
}

func (cs *channelState) refill(now time.Time) {
	if cs.lastRefill.IsZero() {
		cs.lastRefill = now
		return
	}
	elapsed := now.Sub(cs.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	cs.tokens += elapsed * cs.limit.refillPerSec()
	if max := float64(cs.limit.Burst); cs.tokens > max {
		cs.tokens = max
	}
	cs.lastRefill = now
}
