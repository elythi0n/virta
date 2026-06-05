package kick

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

// fakeFetcher returns a scripted id or error and counts calls.
type fakeFetcher struct {
	mu    sync.Mutex
	id    string
	err   error
	calls int
}

func (f *fakeFetcher) Fetch(_ context.Context, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.id, f.err
}

func (f *fakeFetcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// memCache is an in-memory ChatroomCache for tests.
type memCache struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemCache() *memCache { return &memCache{m: map[string]string{}} }
func (c *memCache) Get(_ context.Context, slug string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.m[slug]
	return id, ok
}
func (c *memCache) Put(_ context.Context, slug, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[slug] = id
	return nil
}

func reasonOf(t *testing.T, err error) platform.ReasonCode {
	t.Helper()
	var re *ResolveError
	if !errors.As(err, &re) {
		t.Fatalf("error %v is not a *ResolveError", err)
	}
	return re.Reason
}

func TestResolver_CacheHitSkipsFetch(t *testing.T) {
	cache := newMemCache()
	_ = cache.Put(context.Background(), "xqc", "777")
	direct := &fakeFetcher{id: "999"}
	r := NewResolver(cache, direct, nil, clock.NewFake(time.Unix(0, 0)))

	id, err := r.Resolve(context.Background(), "xqc")
	if err != nil || id != "777" {
		t.Fatalf("Resolve = %q, %v; want cached 777", id, err)
	}
	if direct.count() != 0 {
		t.Errorf("direct fetched %d times on a cache hit, want 0", direct.count())
	}
}

func TestResolver_DirectSuccessIsCached(t *testing.T) {
	cache := newMemCache()
	direct := &fakeFetcher{id: "123"}
	r := NewResolver(cache, direct, nil, clock.NewFake(time.Unix(0, 0)))

	id, err := r.Resolve(context.Background(), "xqc")
	if err != nil || id != "123" {
		t.Fatalf("Resolve = %q, %v; want 123", id, err)
	}
	// Second call must hit the cache, not the fetcher.
	_, _ = r.Resolve(context.Background(), "xqc")
	if direct.count() != 1 {
		t.Errorf("direct fetched %d times, want 1 (second resolve cached)", direct.count())
	}
}

func TestResolver_BlockedFallsBackToOfficial(t *testing.T) {
	cache := newMemCache()
	direct := &fakeFetcher{err: errBlocked}
	fallback := &fakeFetcher{id: "456"}
	r := NewResolver(cache, direct, fallback, clock.NewFake(time.Unix(0, 0)))

	id, err := r.Resolve(context.Background(), "xqc")
	if err != nil || id != "456" {
		t.Fatalf("Resolve = %q, %v; want fallback 456", id, err)
	}
}

func TestResolver_BlockedNoFallbackIsResolverBlocked(t *testing.T) {
	r := NewResolver(newMemCache(), &fakeFetcher{err: errBlocked}, nil, clock.NewFake(time.Unix(0, 0)))
	_, err := r.Resolve(context.Background(), "xqc")
	if reasonOf(t, err) != platform.ReasonResolverBlocked {
		t.Errorf("reason = %v, want resolver_blocked", reasonOf(t, err))
	}
}

func TestResolver_NotFoundShortCircuits(t *testing.T) {
	fallback := &fakeFetcher{id: "999"}
	r := NewResolver(newMemCache(), &fakeFetcher{err: errNotFound}, fallback, clock.NewFake(time.Unix(0, 0)))
	_, err := r.Resolve(context.Background(), "ghost")
	if reasonOf(t, err) != platform.ReasonChannelNotFound {
		t.Errorf("reason = %v, want channel_not_found", reasonOf(t, err))
	}
	if fallback.count() != 0 {
		t.Errorf("fallback called %d times on a definitive not-found, want 0", fallback.count())
	}
}

func TestResolver_BreakerOpensAndSkipsDirect(t *testing.T) {
	clk := clock.NewFake(time.Unix(0, 0))
	direct := &fakeFetcher{err: errBlocked}
	r := NewResolver(newMemCache(), direct, nil, clk)

	// Trip the breaker with consecutive blocked lookups.
	for i := 0; i < breakerThreshold; i++ {
		_, _ = r.Resolve(context.Background(), "xqc")
	}
	callsAtTrip := direct.count()
	// While open, the direct fetcher is skipped entirely.
	_, err := r.Resolve(context.Background(), "another")
	if direct.count() != callsAtTrip {
		t.Errorf("direct called while breaker open (%d → %d)", callsAtTrip, direct.count())
	}
	if reasonOf(t, err) != platform.ReasonResolverBlocked {
		t.Errorf("reason = %v, want resolver_blocked while open", reasonOf(t, err))
	}

	// A cached channel is unaffected by the open breaker.
	cache := newMemCache()
	_ = cache.Put(context.Background(), "cached", "111")
	r2 := NewResolver(cache, &fakeFetcher{err: errBlocked}, nil, clk)
	for i := 0; i < breakerThreshold; i++ {
		_, _ = r2.Resolve(context.Background(), "x")
	}
	if id, err := r2.Resolve(context.Background(), "cached"); err != nil || id != "111" {
		t.Errorf("cached resolve during block = %q, %v; want 111", id, err)
	}
}

func TestResolver_BreakerRecoversAfterCooldown(t *testing.T) {
	clk := clock.NewFake(time.Unix(0, 0))
	direct := &fakeFetcher{err: errBlocked}
	r := NewResolver(newMemCache(), direct, nil, clk)
	for i := 0; i < breakerThreshold; i++ {
		_, _ = r.Resolve(context.Background(), "x")
	}
	// After the cooldown the direct fetcher is tried again (and now succeeds).
	clk.Advance(breakerCooldown + time.Second)
	direct.mu.Lock()
	direct.err = nil
	direct.id = "222"
	direct.mu.Unlock()
	id, err := r.Resolve(context.Background(), "y")
	if err != nil || id != "222" {
		t.Errorf("post-cooldown resolve = %q, %v; want 222", id, err)
	}
}
