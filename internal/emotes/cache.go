package emotes

import (
	"context"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

// DefaultTTL is how long a cached provider set is served before a refetch (REST sources;
// 7TV's EventAPI will invalidate sooner once wired).
const DefaultTTL = 24 * time.Hour

// SetCache persists a provider's resolved emotes so a warm start skips the network and an
// offline start still has emotes. Keyed "provider:platform:userid" (e.g. "7tv:twitch:12345").
type SetCache interface {
	Get(ctx context.Context, key string) (refs []platform.EmoteRef, fetchedAt time.Time, ok bool)
	Put(ctx context.Context, key string, refs []platform.EmoteRef) error
}

// cachingProvider wraps a Provider with a disk-backed set cache: a fresh cache hit skips the
// fetch, a stale entry triggers a refetch, and a fetch failure falls back to whatever is
// cached (so a network blip or an offline start still serves emotes). The cache is disposable
// — a miss simply refetches.
type cachingProvider struct {
	inner Provider
	cache SetCache
	clk   clock.Clock
	ttl   time.Duration
}

// Cached wraps inner so its results are cached. ttl <= 0 uses DefaultTTL.
func Cached(inner Provider, cache SetCache, clk clock.Clock, ttl time.Duration) Provider {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &cachingProvider{inner: inner, cache: cache, clk: clk, ttl: ttl}
}

func (p *cachingProvider) Name() string { return p.inner.Name() }

func (p *cachingProvider) Fetch(ctx context.Context, ch platform.ChannelRef) ([]platform.EmoteRef, error) {
	key := p.inner.Name() + ":" + string(ch.Platform) + ":" + ch.ID
	cached, fetchedAt, ok := p.cache.Get(ctx, key)
	if ok && p.clk.Now().Sub(fetchedAt) < p.ttl {
		return cached, nil // fresh: no network
	}
	fresh, err := p.inner.Fetch(ctx, ch)
	if err != nil {
		if ok {
			return cached, nil // stale-but-present beats nothing (offline/blip resilience)
		}
		return nil, err
	}
	_ = p.cache.Put(ctx, key, fresh)
	return fresh, nil
}
