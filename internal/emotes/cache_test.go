package emotes

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

type cacheEntry struct {
	refs      []platform.EmoteRef
	fetchedAt time.Time
}

type memSetCache struct {
	mu sync.Mutex
	m  map[string]cacheEntry
}

func newMemSetCache() *memSetCache { return &memSetCache{m: map[string]cacheEntry{}} }

func (c *memSetCache) Get(_ context.Context, key string) ([]platform.EmoteRef, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	return e.refs, e.fetchedAt, ok
}

func (c *memSetCache) Put(_ context.Context, key string, refs []platform.EmoteRef) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = cacheEntry{refs: refs, fetchedAt: time.Now()}
	return nil
}

// set seeds the cache with a specific age for staleness tests.
func (c *memSetCache) set(key string, refs []platform.EmoteRef, fetchedAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = cacheEntry{refs: refs, fetchedAt: fetchedAt}
}

type countingProvider struct {
	mu    sync.Mutex
	name  string
	refs  []platform.EmoteRef
	err   error
	calls int
}

func (p *countingProvider) Name() string { return p.name }
func (p *countingProvider) Fetch(context.Context, platform.ChannelRef) ([]platform.EmoteRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.refs, p.err
}
func (p *countingProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestCached_FreshHitSkipsFetch(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	cache := newMemSetCache()
	cache.set("7tv:twitch:1", []platform.EmoteRef{emote(platform.Emote7TV, "PogU")}, clk.Now().Add(-time.Hour))
	inner := &countingProvider{name: "7tv"}
	p := Cached(inner, cache, clk, DefaultTTL)

	refs, err := p.Fetch(context.Background(), testChannel)
	if err != nil || len(refs) != 1 || refs[0].Name != "PogU" {
		t.Fatalf("fresh hit = %v, %v", refs, err)
	}
	if inner.count() != 0 {
		t.Errorf("inner fetched %d times on a fresh hit, want 0", inner.count())
	}
}

func TestCached_StaleRefetches(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	cache := newMemSetCache()
	cache.set("7tv:twitch:1", []platform.EmoteRef{emote(platform.Emote7TV, "old")}, clk.Now().Add(-25*time.Hour))
	inner := &countingProvider{name: "7tv", refs: []platform.EmoteRef{emote(platform.Emote7TV, "new")}}
	p := Cached(inner, cache, clk, DefaultTTL)

	refs, _ := p.Fetch(context.Background(), testChannel)
	if len(refs) != 1 || refs[0].Name != "new" {
		t.Errorf("stale refetch = %v, want fresh 'new'", refs)
	}
	if inner.count() != 1 {
		t.Errorf("inner fetched %d times, want 1 (stale)", inner.count())
	}
	// Refetched data is written back fresh, so a second call is a hit.
	_, _ = p.Fetch(context.Background(), testChannel)
	if inner.count() != 1 {
		t.Errorf("second fetch hit network (%d), want cached", inner.count())
	}
}

func TestCached_ServesStaleOnFetchError(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	cache := newMemSetCache()
	cache.set("7tv:twitch:1", []platform.EmoteRef{emote(platform.Emote7TV, "cached")}, clk.Now().Add(-48*time.Hour))
	inner := &countingProvider{name: "7tv", err: errors.New("offline")}
	p := Cached(inner, cache, clk, DefaultTTL)

	refs, err := p.Fetch(context.Background(), testChannel)
	if err != nil {
		t.Fatalf("offline with cache should not error: %v", err)
	}
	if len(refs) != 1 || refs[0].Name != "cached" {
		t.Errorf("offline served %v, want stale cached set", refs)
	}
}

func TestCached_MissWithErrorReturnsError(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	inner := &countingProvider{name: "7tv", err: errors.New("offline")}
	p := Cached(inner, newMemSetCache(), clk, DefaultTTL)
	if _, err := p.Fetch(context.Background(), testChannel); err == nil {
		t.Error("cold miss + fetch error should surface the error")
	}
}

func TestCached_MissFetchesAndStores(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	cache := newMemSetCache()
	inner := &countingProvider{name: "7tv", refs: []platform.EmoteRef{emote(platform.Emote7TV, "x")}}
	p := Cached(inner, cache, clk, DefaultTTL)

	_, _ = p.Fetch(context.Background(), testChannel)
	if _, _, ok := cache.Get(context.Background(), "7tv:twitch:1"); !ok {
		t.Error("fetched set was not written to the cache")
	}
}
