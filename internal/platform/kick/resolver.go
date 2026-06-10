package kick

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

// Resolving a Kick slug to its chatroom id is the fragile part of the integration: the direct
// endpoint sits behind Cloudflare and can start refusing requests. The Resolver isolates that
// risk — it caches resolved ids forever, walks a fallback chain when the direct lookup is
// blocked, and trips a circuit breaker so a blocked endpoint doesn't get hammered. Already
// resolved (cached) channels are never affected by a block.

// errBlocked marks a fetch refused by anti-bot protection (e.g. Cloudflare 403); errNotFound
// marks a slug the platform doesn't know. Fetchers return these so the resolver can pick the
// right reason code and decide whether to trip the breaker.
var (
	errBlocked  = errors.New("kick: chatroom lookup blocked")
	errNotFound = errors.New("kick: channel not found")
)

// ChatroomFetcher looks up a slug's chatroom id over the network.
type ChatroomFetcher interface {
	Fetch(ctx context.Context, slug string) (chatroomID string, err error)
}

// ChatroomCache persists resolved ids (chatroom ids are stable, so a hit is permanent).
type ChatroomCache interface {
	Get(ctx context.Context, slug string) (chatroomID string, ok bool)
	Put(ctx context.Context, slug, chatroomID string) error
}

// ResolveError carries a machine reason code so the UI can explain a failed join in its own
// words without parsing strings.
type ResolveError struct {
	Reason platform.ReasonCode
	Slug   string
	err    error
}

func (e *ResolveError) Error() string {
	return fmt.Sprintf("kick: resolve %q: %s", e.Slug, e.Reason)
}
func (e *ResolveError) Unwrap() error { return e.err }

// Breaker defaults: trip after a few consecutive direct failures, stay open for a cooldown.
const (
	breakerThreshold = 3
	breakerCooldown  = 2 * time.Minute
)

// Resolver maps a Kick slug to its chatroom id. The zero value is not usable; build with
// NewResolver. It is safe for concurrent use.
type Resolver struct {
	cache    ChatroomCache
	direct   ChatroomFetcher // primary (uTLS); may get blocked
	fallback ChatroomFetcher // optional secondary (official API probe)
	clk      clock.Clock

	mu        sync.Mutex
	failures  int
	openUntil time.Time
}

// NewResolver builds a resolver. fallback may be nil (then a blocked direct lookup goes
// straight to a resolver-blocked error, and the UI offers the manual assist).
func NewResolver(cache ChatroomCache, direct, fallback ChatroomFetcher, clk clock.Clock) *Resolver {
	return &Resolver{cache: cache, direct: direct, fallback: fallback, clk: clk}
}

// Resolve returns the chatroom id for slug: cache first (never re-fetched on success), then
// the direct lookup (unless the breaker is open), then the fallback. A success is cached.
func (r *Resolver) Resolve(ctx context.Context, slug string) (string, error) {
	if id, ok := r.cache.Get(ctx, slug); ok {
		return id, nil
	}

	blocked := false
	if r.breakerOpen() {
		blocked = true // skip the direct call entirely while the breaker is open
	} else if r.direct != nil {
		id, err := r.direct.Fetch(ctx, slug)
		switch {
		case err == nil:
			r.onSuccess()
			_ = r.cache.Put(ctx, slug, id)
			return id, nil
		case errors.Is(err, errBlocked):
			blocked = true
			r.onFailure()
		case errors.Is(err, errNotFound):
			return "", &ResolveError{Reason: platform.ReasonChannelNotFound, Slug: slug, err: err}
		default:
			r.onFailure()
		}
	}

	if r.fallback != nil {
		if id, err := r.fallback.Fetch(ctx, slug); err == nil {
			r.onSuccess() // resolution is working again; let the breaker recover
			_ = r.cache.Put(ctx, slug, id)
			return id, nil
		} else if errors.Is(err, errNotFound) {
			return "", &ResolveError{Reason: platform.ReasonChannelNotFound, Slug: slug, err: err}
		}
	}

	reason := platform.ReasonChannelNotFound
	if blocked {
		reason = platform.ReasonResolverBlocked // the UI offers "open Kick once / paste the id"
	}
	return "", &ResolveError{Reason: reason, Slug: slug}
}

func (r *Resolver) breakerOpen() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.clk.Now().Before(r.openUntil)
}

func (r *Resolver) onFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures++
	if r.failures >= breakerThreshold {
		r.openUntil = r.clk.Now().Add(breakerCooldown)
	}
}

func (r *Resolver) onSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures = 0
	r.openUntil = time.Time{}
}
