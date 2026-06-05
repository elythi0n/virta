package ratelimit

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
)

// defaultQuiet is how long a channel must send without being rate-limited before its budget
// steps back toward the seed.
const defaultQuiet = time.Minute

// rateLimited is implemented by platform errors that report a server-side rate limit (the
// suggested wait may be zero). Matched structurally so the platform packages and this one
// stay independent.
type rateLimited interface {
	RateLimited() time.Duration
}

// Adaptive wraps a Governor for platforms whose limits are undocumented: each channel starts
// at a conservative seed, halves its burst every time the platform answers 429 (honoring any
// suggested wait), and doubles back toward the seed after a quiet stretch of successful
// sends. Channels never stall — a tightened bucket still refills, just slower.
type Adaptive struct {
	g     *Governor
	clk   clock.Clock
	def   Limit
	quiet time.Duration

	mu    sync.Mutex
	seeds map[string]Limit // key-prefix → starting limit
	per   map[string]*adaptiveState
}

// adaptiveState is one channel's adaptive position.
type adaptiveState struct {
	base     Limit     // what recovery restores toward
	cur      Limit     // current (possibly tightened) limit
	lastHit  time.Time // most recent 429
	lastStep time.Time // most recent tighten or recovery step
}

// NewAdaptive builds an adaptive layer over g. def must be the same default limit g was built
// with — it is the baseline for keys no seed prefix matches.
func NewAdaptive(g *Governor, clk clock.Clock, def Limit) *Adaptive {
	return &Adaptive{g: g, clk: clk, def: def, quiet: defaultQuiet, seeds: map[string]Limit{}, per: map[string]*adaptiveState{}}
}

// SetSeed sets the starting limit for keys with the given prefix (e.g. a platform's "kick:"
// keys begin conservative). Longest matching prefix wins. Call before traffic flows.
func (a *Adaptive) SetSeed(prefix string, l Limit) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seeds[prefix] = l
}

// SetQuiet overrides the recovery quiet period (tests shorten it).
func (a *Adaptive) SetQuiet(d time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.quiet = d
}

// Limit reports key's current limit (for display and tests).
func (a *Adaptive) Limit(key string) Limit {
	a.mu.Lock()
	defer a.mu.Unlock()
	if st, ok := a.per[key]; ok {
		return st.cur
	}
	return a.baseFor(key)
}

// Submit queues a send through the underlying governor, observing its result: a rate-limit
// answer tightens the channel's budget, sustained success recovers it. Satisfies the same
// contract as Governor.Submit, so callers swap in transparently.
func (a *Adaptive) Submit(key string, send func() error) <-chan error {
	a.ensure(key)
	return a.g.Submit(key, func() error {
		err := send()
		if wait, ok := limitedWait(err); ok {
			a.hit(key, wait)
		} else if err == nil {
			a.maybeRecover(key)
		}
		return err
	})
}

// Drain delegates to the underlying governor (the production ticker calls one drain).
func (a *Adaptive) Drain() { a.g.Drain() }

// State delegates to the underlying governor.
func (a *Adaptive) State(key string) (queued int, nextIn time.Duration) { return a.g.State(key) }

// ensure initializes key at its seed on first sight.
func (a *Adaptive) ensure(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.per[key]; ok {
		return
	}
	base := a.baseFor(key)
	a.per[key] = &adaptiveState{base: base, cur: base}
	a.g.SetLimit(key, base)
}

// baseFor returns the seed for key (longest prefix match) or the governor default. Caller
// holds a.mu.
func (a *Adaptive) baseFor(key string) Limit {
	best, bestLen := a.def, -1
	for p, l := range a.seeds {
		if strings.HasPrefix(key, p) && len(p) > bestLen {
			best, bestLen = l, len(p)
		}
	}
	return best
}

// hit reacts to a 429: halve the burst (floor 1), then empty the bucket so nothing dispatches
// before the server's suggested wait.
func (a *Adaptive) hit(key string, wait time.Duration) {
	a.mu.Lock()
	st := a.per[key]
	now := a.clk.Now()
	if st.cur.Burst > 1 {
		st.cur.Burst /= 2
	}
	st.lastHit, st.lastStep = now, now
	cur := st.cur
	a.mu.Unlock()

	a.g.SetLimit(key, cur)
	a.g.Penalize(key, wait)
}

// maybeRecover steps a tightened channel's burst back toward its seed after a quiet stretch —
// one doubling per quiet period, so recovery is as gradual as the tightening was sharp.
func (a *Adaptive) maybeRecover(key string) {
	a.mu.Lock()
	st := a.per[key]
	now := a.clk.Now()
	if st.cur.Burst >= st.base.Burst || now.Sub(st.lastStep) < a.quiet {
		a.mu.Unlock()
		return
	}
	st.cur.Burst *= 2
	if st.cur.Burst > st.base.Burst {
		st.cur.Burst = st.base.Burst
	}
	st.lastStep = now
	cur := st.cur
	a.mu.Unlock()

	a.g.SetLimit(key, cur)
}

// limitedWait extracts a rate-limit signal from a send error.
func limitedWait(err error) (time.Duration, bool) {
	var rl rateLimited
	if errors.As(err, &rl) {
		return rl.RateLimited(), true
	}
	return 0, false
}
