// Package youtube implements the platform.Adapter contract for YouTube Live Chat. It reads
// chat anonymously by polling the public InnerTube API (the youtube.com web client's own
// backend — unofficial, like Kick's Pusher feed), normalizing each chat item into a
// UnifiedMessage. Sending and moderation would need the official Data API and arrive later;
// an anonymous adapter is read-only.
//
// YouTube chat is per-broadcast, not per-channel, so each joined channel runs its own worker
// state machine: resolve slug → current live videoId → chat continuation token → poll; when
// the broadcast ends the worker degrades the channel and slowly re-resolves, waiting for the
// next broadcast, then resumes. There is no shared socket to multiplex — polling is naturally
// per-channel.
package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

const (
	defaultWebBase = "https://www.youtube.com"
	defaultAPIBase = "https://www.youtube.com"

	// Poll-error backoff (transient InnerTube failures inside an active broadcast).
	defaultBackoffBase = time.Second
	defaultBackoffMax  = 30 * time.Second
	// After this many consecutive poll failures the worker gives up on the continuation and
	// re-resolves the broadcast from scratch (the token may simply have expired).
	maxPollFailures = 5

	// Server-suggested poll intervals are clamped to this window so a hostile/garbled
	// timeoutMs can neither hammer the endpoint nor stall the feed.
	defaultPollFloor = time.Second
	defaultPollCeil  = 10 * time.Second

	// Waiting-for-live cadence: a channel that isn't broadcasting is re-checked on a slow,
	// jittered backoff so an idle channel costs ~one page fetch a minute.
	defaultResolveRetryMin = 30 * time.Second
	defaultResolveRetryMax = 2 * time.Minute
)

// Options configure an anonymous YouTube adapter. Zero values select sensible defaults; the
// bases and client exist for test injection.
type Options struct {
	Clock      clock.Clock
	WebBase    string       // base for the /@slug/live page ("" → youtube.com)
	APIBase    string       // base for the InnerTube endpoints ("" → youtube.com)
	HTTPClient *http.Client // nil → a default client with a timeout

	BackoffBase time.Duration // poll-error backoff start
	BackoffMax  time.Duration // poll-error backoff ceiling

	PollFloor time.Duration // minimum sleep between polls
	PollCeil  time.Duration // maximum sleep between polls

	ResolveRetryMin time.Duration // waiting-for-live backoff start
	ResolveRetryMax time.Duration // waiting-for-live backoff ceiling
}

// Adapter is an anonymous, read-only YouTube Live Chat adapter over InnerTube polling.
type Adapter struct {
	client  *http.Client
	webBase string
	apiBase string

	pollBackoff    backoff // transient poll errors
	resolveBackoff backoff // waiting for the channel to go live
	pollFloor      time.Duration
	pollCeil       time.Duration

	events chan platform.Event

	mu      sync.Mutex
	workers map[string]context.CancelFunc // channel key → worker stop
	health  platform.HealthStatus
	rng     uint64
	closed  bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates an anonymous YouTube adapter. It performs no network I/O until the first Join.
func New(opts Options) *Adapter {
	clk := opts.Clock
	if clk == nil {
		clk = clock.System{}
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	webBase := opts.WebBase
	if webBase == "" {
		webBase = defaultWebBase
	}
	apiBase := opts.APIBase
	if apiBase == "" {
		apiBase = defaultAPIBase
	}
	pb := backoff{base: opts.BackoffBase, max: opts.BackoffMax}
	if pb.base <= 0 {
		pb.base = defaultBackoffBase
	}
	if pb.max <= 0 {
		pb.max = defaultBackoffMax
	}
	rb := backoff{base: opts.ResolveRetryMin, max: opts.ResolveRetryMax}
	if rb.base <= 0 {
		rb.base = defaultResolveRetryMin
	}
	if rb.max <= 0 {
		rb.max = defaultResolveRetryMax
	}
	floor := opts.PollFloor
	if floor <= 0 {
		floor = defaultPollFloor
	}
	ceil := opts.PollCeil
	if ceil <= 0 {
		ceil = defaultPollCeil
	}
	if ceil < floor {
		ceil = floor
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Adapter{
		client:         client,
		webBase:        webBase,
		apiBase:        apiBase,
		pollBackoff:    pb,
		resolveBackoff: rb,
		pollFloor:      floor,
		pollCeil:       ceil,
		events:         make(chan platform.Event, 256),
		workers:        map[string]context.CancelFunc{},
		health:         platform.HealthStatus{State: platform.HealthOK},
		rng:            uint64(clk.Now().UnixNano()) | 1,
		ctx:            ctx,
		cancel:         cancel,
	}
}

func (a *Adapter) Platform() platform.Platform { return platform.YouTube }

func (a *Adapter) Capabilities() platform.Capabilities {
	return platform.Capabilities{ReadAnonymous: true, Stability: platform.TierUnofficial}
}

// Join starts a per-channel polling worker. The first resolve happens synchronously so a
// nonexistent channel fails the join with a reason-coded error; a channel that exists but
// isn't live joins successfully and the worker waits (Degraded/not_live) for the broadcast.
// Idempotent per channel.
func (a *Adapter) Join(ctx context.Context, ch platform.ChannelRef, _ platform.ConnMode) error {
	key := ch.Key()
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return fmt.Errorf("youtube: adapter closed")
	}
	if _, ok := a.workers[key]; ok {
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()

	// Resolve outside the lock — it's network I/O. Only "channel not found" fails the join;
	// "not live" and transient errors hand off to the worker's wait loop.
	videoID, err := a.resolveLive(ctx, ch.Slug)
	if err != nil {
		var re *ResolveError
		if errors.As(err, &re) && re.Reason == platform.ReasonChannelNotFound {
			return err
		}
		videoID = ""
	}

	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return fmt.Errorf("youtube: adapter closed")
	}
	if _, ok := a.workers[key]; ok {
		a.mu.Unlock()
		return nil // another Join won the race
	}
	wctx, cancel := context.WithCancel(a.ctx)
	a.workers[key] = cancel
	a.wg.Add(1)
	a.mu.Unlock()

	go a.runWorker(wctx, ch, videoID)
	return nil
}

// Leave stops the channel's worker. Leaving an unknown channel is a no-op.
func (a *Adapter) Leave(ch platform.ChannelRef) error {
	key := ch.Key()
	a.mu.Lock()
	cancel, ok := a.workers[key]
	if ok {
		delete(a.workers, key)
	}
	a.mu.Unlock()
	if ok {
		cancel()
	}
	return nil
}

// Send is unsupported: anonymous InnerTube reads carry no identity to post with.
func (a *Adapter) Send(context.Context, platform.ChannelRef, string, platform.SendOpts) error {
	return platform.ErrUnsupported
}

// Moderate is unsupported in anonymous mode.
func (a *Adapter) Moderate(context.Context, platform.ModAction) error {
	return platform.ErrUnsupported
}

func (a *Adapter) Events() <-chan platform.Event { return a.events }

func (a *Adapter) Health() platform.HealthStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.health
}

// Close stops every worker and closes Events. Idempotent.
func (a *Adapter) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.workers = map[string]context.CancelFunc{}
	a.mu.Unlock()

	a.cancel() // every worker context is derived from a.ctx
	a.wg.Wait()
	close(a.events)
	return nil
}

// ---- per-channel worker ----

// runWorker is one channel's lifecycle state machine: (wait for live →) fetch continuation →
// poll until the broadcast ends → degrade and wait for the next broadcast → repeat. videoID
// may arrive pre-resolved from Join ("" → start in the wait state).
func (a *Adapter) runWorker(ctx context.Context, ch platform.ChannelRef, videoID string) {
	defer a.wg.Done()
	last := platform.HealthStatus{}
	setHealth := func(h platform.HealthStatus) {
		if h.State == last.State && h.Reason == last.Reason {
			return
		}
		last = h
		a.emit(ctx, platform.HealthEvent{Channel: &ch, Status: h})
	}

	for {
		if ctx.Err() != nil {
			return
		}
		if videoID == "" {
			setHealth(platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonNotLive, Detail: "waiting for a live broadcast"})
			videoID = a.waitForLive(ctx, ch, setHealth)
			if videoID == "" {
				return // shut down while waiting
			}
		}
		cont, err := a.fetchContinuation(ctx, videoID)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Chat disabled, broadcast already over, or a transient failure: fall back to the
			// slow wait loop rather than hammering /next.
			videoID = ""
			continue
		}
		setHealth(platform.HealthStatus{State: platform.HealthOK})
		a.pollChat(ctx, ch, cont, setHealth)
		if ctx.Err() != nil {
			return
		}
		videoID = "" // broadcast ended → wait for the next one
	}
}

// waitForLive re-resolves the channel on a slow, jittered backoff until it is live again.
// Returns "" only on shutdown.
func (a *Adapter) waitForLive(ctx context.Context, ch platform.ChannelRef, setHealth func(platform.HealthStatus)) string {
	for attempt := 1; ; attempt++ {
		if !sleepCtx(ctx, a.resolveBackoff.delay(attempt, a.nextRand())) {
			return ""
		}
		videoID, err := a.resolveLive(ctx, ch.Slug)
		if err == nil {
			return videoID
		}
		if ctx.Err() != nil {
			return ""
		}
		var re *ResolveError
		switch {
		case errors.As(err, &re) && re.Reason == platform.ReasonChannelNotFound:
			// The channel page vanished mid-run (rename/termination). Keep the slow retry —
			// renames sometimes resolve — but surface the stronger reason.
			setHealth(platform.HealthStatus{State: platform.HealthDown, Reason: platform.ReasonChannelNotFound})
		case errors.As(err, &re): // not live — the expected wait state
			setHealth(platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonNotLive, Detail: "waiting for a live broadcast"})
		default: // network trouble
			setHealth(platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonUpstreamDown, Detail: err.Error()})
		}
	}
}

// pollChat polls get_live_chat with the server-suggested cadence until the broadcast ends, the
// continuation chain breaks, or the worker stops. Transient errors retry with backoff; after
// maxPollFailures the caller re-resolves from scratch.
func (a *Adapter) pollChat(ctx context.Context, ch platform.ChannelRef, cont string, setHealth func(platform.HealthStatus)) {
	failures := 0
	for {
		resp, err := a.fetchChat(ctx, cont)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			failures++
			if failures >= maxPollFailures {
				return // give up on this continuation; re-resolve the broadcast
			}
			setHealth(platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonReconnecting, Detail: err.Error()})
			if !sleepCtx(ctx, a.pollBackoff.delay(failures, a.nextRand())) {
				return
			}
			continue
		}
		if failures > 0 {
			failures = 0
			setHealth(platform.HealthStatus{State: platform.HealthOK})
		}
		if resp.ContinuationContents == nil {
			return // the stream ended
		}
		lc := resp.ContinuationContents.LiveChatContinuation
		for _, act := range lc.Actions {
			for _, ev := range eventsFromAction(act, ch) {
				a.emit(ctx, ev)
			}
		}
		next, timeout := nextContinuation(lc.Continuations)
		if next == "" {
			return // no way to continue; treat like a stream end and re-resolve
		}
		cont = next
		if !sleepCtx(ctx, a.clampPoll(timeout)) {
			return
		}
	}
}

// nextContinuation picks the new token and suggested wait from a poll response.
func nextContinuation(conts []continuationWrapper) (string, time.Duration) {
	for _, w := range conts {
		if d := w.data(); d != nil && d.Continuation != "" {
			return d.Continuation, time.Duration(d.TimeoutMs) * time.Millisecond
		}
	}
	return "", 0
}

// clampPoll bounds the server-suggested sleep to the configured window.
func (a *Adapter) clampPoll(d time.Duration) time.Duration {
	if d < a.pollFloor {
		return a.pollFloor
	}
	if d > a.pollCeil {
		return a.pollCeil
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// nextRand advances a splitmix64 generator for backoff jitter; guarded by mu because every
// worker draws from it.
func (a *Adapter) nextRand() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rng += 0x9e3779b97f4a7c15
	z := a.rng
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// emit delivers an event unless the worker (or adapter) is shutting down. Blocking on the
// adapter context — not the worker's — would risk a send on the closed channel after Close;
// the worker ctx is derived from a.ctx, so it covers both.
func (a *Adapter) emit(ctx context.Context, ev platform.Event) {
	select {
	case <-ctx.Done():
	case a.events <- ev:
	}
}

var _ platform.Adapter = (*Adapter)(nil)
