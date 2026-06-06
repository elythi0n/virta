// Package streams resolves live stream metadata (viewer count, title, category, thumbnail) per
// channel, mirroring internal/badges and internal/emotes: providers fetch off the hot path, the
// resolver caches a per-channel snapshot, and a TTL keeps it fresh on demand without blocking the
// /v1/streams handler. The platforms expose this anonymously (Twitch GraphQL, Kick's channel API),
// so no credentials are needed.
package streams

import (
	"context"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// Info is a channel's current live state.
type Info struct {
	Live         bool
	ViewerCount  int
	Title        string
	Category     string
	ThumbnailURL string
	StartedAt    time.Time
}

// Key identifies a channel's snapshot.
func Key(ch platform.ChannelRef) string { return string(ch.Platform) + ":" + ch.Slug }

// Provider fetches one channel's live info. A provider that doesn't handle the channel's platform
// returns ok=false so the resolver tries the next. Off the hot path.
type Provider interface {
	Name() string
	Fetch(ctx context.Context, ch platform.ChannelRef) (info Info, ok bool, err error)
}

// DefaultTTL is how long a snapshot is considered fresh before EnsureFresh re-fetches.
const DefaultTTL = 30 * time.Second

type slot struct {
	mu        sync.Mutex
	info      *Info
	fetchedAt time.Time
	inflight  bool
}

// Resolver caches per-channel Info and refreshes it lazily on a TTL.
type Resolver struct {
	providers []Provider
	ttl       time.Duration
	now       func() time.Time

	mu    sync.Mutex
	slots map[string]*slot
}

// NewResolver builds a resolver from providers in precedence order (first that handles the
// platform wins).
func NewResolver(providers ...Provider) *Resolver {
	return &Resolver{providers: providers, ttl: DefaultTTL, now: time.Now, slots: map[string]*slot{}}
}

func (r *Resolver) slotFor(key string) *slot {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.slots[key]
	if s == nil {
		s = &slot{}
		r.slots[key] = s
	}
	return s
}

// Snapshot returns the last resolved info for a channel, or nil if nothing has resolved yet.
func (r *Resolver) Snapshot(key string) *Info {
	s := r.slotFor(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info
}

// Refresh fetches the channel synchronously and stores the result (an offline channel is a valid
// result and is stored). A fetch error leaves the previous snapshot untouched.
func (r *Resolver) Refresh(ctx context.Context, ch platform.ChannelRef) error {
	for _, p := range r.providers {
		info, ok, err := p.Fetch(ctx, ch)
		if !ok {
			continue
		}
		if err != nil {
			return err
		}
		s := r.slotFor(Key(ch))
		s.mu.Lock()
		stored := info
		s.info = &stored
		s.fetchedAt = r.now()
		s.mu.Unlock()
		return nil
	}
	return nil
}

// EnsureFresh refreshes in the background when the snapshot is older than the TTL and no fetch is
// already in flight, so the request path (the /v1/streams handler, polled by the UI) never blocks.
func (r *Resolver) EnsureFresh(ch platform.ChannelRef) {
	s := r.slotFor(Key(ch))
	s.mu.Lock()
	stale := s.info == nil || r.now().Sub(s.fetchedAt) >= r.ttl
	if !stale || s.inflight {
		s.mu.Unlock()
		return
	}
	s.inflight = true
	s.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = r.Refresh(ctx, ch)
		s.mu.Lock()
		s.inflight = false
		s.mu.Unlock()
	}()
}
