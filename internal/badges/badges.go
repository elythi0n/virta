// Package badges resolves author badge artwork (mod, subscriber, broadcaster, …) per channel,
// mirroring internal/emotes: providers fetch a channel's badge set off the hot path, the resolver
// publishes it as an atomic snapshot, and a pure pipeline stage stamps the artwork URL onto each
// message's badges. The wire Badge carries only set+version; this fills in the image.
package badges

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/elythi0n/virta/internal/platform"
)

// Key identifies a channel's badge snapshot.
func Key(ch platform.ChannelRef) string { return string(ch.Platform) + ":" + ch.Slug }

// setKey is how a badge's identity maps into a Set: "<set>/<version>".
func setKey(set, version string) string { return set + "/" + version }

// Set is an immutable resolved badge map for one channel (set/version → image URL).
type Set struct{ byKey map[string]string }

// Lookup returns the artwork URL for a badge set+version, if resolved.
func (s *Set) Lookup(set, version string) (string, bool) {
	url, ok := s.byKey[setKey(set, version)]
	return url, ok
}

func (s *Set) Len() int { return len(s.byKey) }

// Provider fetches a channel's badge artwork (global merged with channel-specific) as a map keyed
// "<set>/<version>" → image URL. Off the hot path; a provider that doesn't apply returns an empty
// map, nil.
type Provider interface {
	Name() string
	Fetch(ctx context.Context, ch platform.ChannelRef) (map[string]string, error)
}

// Resolver merges providers into a per-channel Set and publishes it as an atomic snapshot.
type Resolver struct {
	providers []Provider

	mu   sync.Mutex
	sets map[string]*atomic.Pointer[Set]
}

// NewResolver builds a resolver from providers in precedence order (highest first; later
// providers' keys win on conflict — channel-specific over global is handled within a provider).
func NewResolver(providers ...Provider) *Resolver {
	return &Resolver{providers: providers, sets: map[string]*atomic.Pointer[Set]{}}
}

// Snapshot returns the current badge set for a channel (never nil; empty before any refresh).
func (r *Resolver) Snapshot(channelKey string) *Set {
	if s := r.slot(channelKey).Load(); s != nil {
		return s
	}
	return &Set{}
}

// Refresh fetches every provider for the channel, merges them, and publishes the snapshot. A
// provider that errors is skipped so one dead source never blanks the others.
func (r *Resolver) Refresh(ctx context.Context, ch platform.ChannelRef) *Set {
	merged := map[string]string{}
	for _, p := range r.providers {
		m, err := p.Fetch(ctx, ch)
		if err != nil {
			continue
		}
		for k, v := range m {
			merged[k] = v
		}
	}
	set := &Set{byKey: merged}
	r.slot(Key(ch)).Store(set)
	return set
}

func (r *Resolver) slot(channelKey string) *atomic.Pointer[Set] {
	r.mu.Lock()
	defer r.mu.Unlock()
	ptr, ok := r.sets[channelKey]
	if !ok {
		ptr = &atomic.Pointer[Set]{}
		r.sets[channelKey] = ptr
	}
	return ptr
}

// SetSource is the read side the Stage depends on (the Resolver satisfies it).
type SetSource interface {
	Snapshot(channelKey string) *Set
}

// Refresher triggers an asynchronous badge fetch for a channel (the Resolver satisfies it).
type Refresher interface {
	Refresh(ctx context.Context, ch platform.ChannelRef) *Set
}

// Stage stamps resolved artwork onto each message's badges using the channel's snapshot.
// When the snapshot is empty (channel not yet fetched), it fires a one-shot background refresh
// so subsequent messages pick up the artwork. Lock-free on the hot path.
type Stage struct {
	src       SetSource
	refresher Refresher
	inflight  sync.Map // channelKey → struct{}; prevents concurrent fetches for the same channel
}

// NewStage builds the badge-resolution stage over a snapshot source. Pass a non-nil Refresher
// to enable lazy fetching when a channel's badge set has not been loaded yet.
func NewStage(src SetSource, refresher Refresher) *Stage {
	return &Stage{src: src, refresher: refresher}
}

func (s *Stage) Name() string { return "badges" }

func (s *Stage) Annotate(_ context.Context, msg *platform.UnifiedMessage) error {
	if len(msg.Author.Badges) == 0 {
		return nil
	}
	key := Key(msg.Channel)
	set := s.src.Snapshot(key)
	if set.Len() == 0 {
		// Trigger a one-shot background fetch so future messages carry image URLs.
		if s.refresher != nil {
			if _, loaded := s.inflight.LoadOrStore(key, struct{}{}); !loaded {
				ch := msg.Channel
				go func() {
					defer s.inflight.Delete(key)
					s.refresher.Refresh(context.Background(), ch)
				}()
			}
		}
		return nil
	}
	for i := range msg.Author.Badges {
		b := &msg.Author.Badges[i]
		if url, ok := set.Lookup(b.Set, b.Version); ok {
			b.URL = url
		}
	}
	return nil
}
