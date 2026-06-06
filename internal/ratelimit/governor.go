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
	sending    bool // a batch for this channel is in flight; keeps its sends ordered
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

// Penalize empties key's bucket so the next send dispatches no sooner than wait from now
// (or one refill interval, whichever is longer). Used when the platform signals we're over
// its limit: queued sends stay queued and pace out, nothing is dropped.
func (g *Governor) Penalize(key string, wait time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	cs := g.state(key)
	cs.refill(g.clk.Now())
	floor := 1 - wait.Seconds()*cs.limit.refillPerSec()
	if floor > 0 {
		floor = 0
	}
	if cs.tokens > floor {
		cs.tokens = floor
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
	// Dispatch only this channel: a slow send here must not stall sends waiting on other
	// channels (those are driven by their own Submit or the production Drain ticker).
	g.drainKey(key)
	return res
}

// Drain refills every channel's bucket and dispatches its eligible queued sends. Channels are
// drained concurrently so a slow send on one never delays another, while each channel's own
// sends stay ordered; it returns once the dispatched sends complete. Production calls this on a
// ticker; tests call it after advancing the clock.
func (g *Governor) Drain() {
	g.mu.Lock()
	keys := make([]string, 0, len(g.channels))
	for key := range g.channels {
		keys = append(keys, key)
	}
	g.mu.Unlock()

	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			g.drainKey(key)
		}(key)
	}
	wg.Wait()
}

// drainKey dispatches as many of one channel's queued sends as its tokens allow, in order. The
// sends run outside g.mu (they may do network I/O); the sending flag ensures only one batch per
// channel is in flight at a time, so concurrent callers can't reorder a channel's sends.
func (g *Governor) drainKey(key string) {
	now := g.clk.Now()
	g.mu.Lock()
	cs, ok := g.channels[key]
	if !ok || cs.sending {
		g.mu.Unlock()
		return
	}
	cs.refill(now)
	var ready []*entry
	for cs.tokens >= 1 && len(cs.queue) > 0 {
		e := cs.queue[0]
		cs.queue = cs.queue[1:]
		cs.tokens--
		ready = append(ready, e)
	}
	if len(ready) == 0 {
		g.mu.Unlock()
		return
	}
	cs.sending = true
	g.mu.Unlock()

	for _, e := range ready {
		e.result <- e.send()
	}

	g.mu.Lock()
	cs.sending = false
	g.mu.Unlock()
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
