// Package emotes resolves third-party, word-based emote overlays (7TV, BTTV, FFZ) that chat
// platforms don't mark up themselves. Platform-native emotes arrive already segmented from the
// adapters (Twitch IRC ranges, Kick inline tokens); this package fills in the rest by matching
// plain-text words against per-channel emote sets.
//
// Fetching is done off the hot path and published as an immutable per-channel snapshot; the
// pipeline Stage only ever reads the current snapshot (a lock-free atomic load), so emote
// resolution never blocks the feed.
package emotes

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/elythi0n/virta/internal/platform"
)

// Key identifies a channel's emote snapshot. It matches the channel-key convention used
// elsewhere (platform:slug).
func Key(ch platform.ChannelRef) string { return string(ch.Platform) + ":" + ch.Slug }

// Set is a resolved, name-keyed collection of emotes for one channel. Lookups are O(1); when
// two providers define the same name, the one merged first wins (the documented precedence).
type Set struct{ byName map[string]platform.EmoteRef }

// Lookup returns the emote for an exact name match.
func (s *Set) Lookup(name string) (platform.EmoteRef, bool) {
	if s == nil {
		return platform.EmoteRef{}, false
	}
	e, ok := s.byName[name]
	return e, ok
}

// Len is the number of distinct emote names in the set.
func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return len(s.byName)
}

// Entries returns every emote in the set (unordered), for listing (e.g. composer autocomplete).
func (s *Set) Entries() []platform.EmoteRef {
	if s == nil {
		return nil
	}
	out := make([]platform.EmoteRef, 0, len(s.byName))
	for _, e := range s.byName {
		out = append(out, e)
	}
	return out
}

// merge builds a Set from provider results in precedence order — earlier groups win name
// collisions.
func merge(groups ...[]platform.EmoteRef) *Set {
	s := &Set{byName: make(map[string]platform.EmoteRef)}
	for _, g := range groups {
		for _, e := range g {
			if e.Name == "" {
				continue
			}
			if _, taken := s.byName[e.Name]; !taken {
				s.byName[e.Name] = e
			}
		}
	}
	return s
}

// Provider fetches one source's emotes for a channel (its global set merged with the
// channel-specific set). Implementations make network calls and must be called off the hot
// path. A provider that doesn't apply to the channel's platform returns an empty slice.
type Provider interface {
	Name() string
	Fetch(ctx context.Context, ch platform.ChannelRef) ([]platform.EmoteRef, error)
}

// Resolver merges providers (in precedence order) into a per-channel Set and publishes it as
// an atomic snapshot. Safe for concurrent use.
type Resolver struct {
	providers []Provider

	mu   sync.Mutex
	sets map[string]*atomic.Pointer[Set]
}

// NewResolver builds a resolver from providers given in precedence order (highest first).
func NewResolver(providers ...Provider) *Resolver {
	return &Resolver{providers: providers, sets: map[string]*atomic.Pointer[Set]{}}
}

// Snapshot returns the current emote set for a channel (never nil; an empty set before any
// refresh). This is the lock-free read the pipeline stage uses.
func (r *Resolver) Snapshot(channelKey string) *Set {
	ptr := r.slot(channelKey)
	if s := ptr.Load(); s != nil {
		return s
	}
	return &Set{}
}

// Refresh fetches every provider for the channel, merges by precedence, and publishes the new
// snapshot. A provider that errors is skipped (its emotes are simply absent) so one dead
// source never blanks the others. Returns the published set.
func (r *Resolver) Refresh(ctx context.Context, ch platform.ChannelRef) *Set {
	groups := make([][]platform.EmoteRef, 0, len(r.providers))
	for _, p := range r.providers {
		es, err := p.Fetch(ctx, ch)
		if err != nil {
			continue // skip this provider this round; others still resolve
		}
		groups = append(groups, es)
	}
	set := merge(groups...)
	r.slot(Key(ch)).Store(set)
	return set
}

// slot returns the atomic pointer for a channel key, creating it on first use.
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

// Stage is a pure pipeline stage that splits plain-text words into emote segments using the
// channel's current snapshot. It is read-only and lock-free on the hot path.
type Stage struct{ src SetSource }

// NewStage builds the emote-annotation stage over a snapshot source.
func NewStage(src SetSource) *Stage { return &Stage{src: src} }

func (s *Stage) Name() string { return "emotes" }

// Annotate rewrites the message's text segments, replacing words that match a known emote with
// emote segments. Non-text segments (existing emotes, mentions, links) pass through untouched.
func (s *Stage) Annotate(_ context.Context, msg *platform.UnifiedMessage) error {
	set := s.src.Snapshot(Key(msg.Channel))
	if set.Len() == 0 {
		return nil
	}
	// Rebuild the segment slice only once a text segment actually contains an emote. The common
	// case (a message with no custom emote) then allocates nothing and leaves Segments untouched.
	var out []platform.Segment
	changed := false
	for i, seg := range msg.Segments {
		if seg.Kind == platform.SegText {
			if repl, didChange := applyEmotes(seg.Text, set); didChange {
				if !changed {
					out = make([]platform.Segment, 0, len(msg.Segments)+2)
					out = append(out, msg.Segments[:i]...)
					changed = true
				}
				out = append(out, repl...)
				continue
			}
		}
		if changed {
			out = append(out, seg)
		}
	}
	if changed {
		msg.Segments = out
	}
	return nil
}

// containsEmote reports whether any whole space-delimited word in text matches an emote, doing
// only map lookups so the no-match path allocates nothing.
func containsEmote(text string, set *Set) bool {
	for i := 0; i < len(text); {
		if text[i] == ' ' {
			i++
			continue
		}
		j := i
		for j < len(text) && text[j] != ' ' {
			j++
		}
		if _, ok := set.Lookup(text[i:j]); ok {
			return true
		}
		i = j
	}
	return false
}

// applyEmotes splits text on spaces (preserving them exactly) and turns each whole word that
// matches an emote name into an emote segment, coalescing the rest into text segments so the
// pieces still concatenate back to the original text. It returns (nil, false) when no word
// matched, so the caller can keep the original segment without allocating.
func applyEmotes(text string, set *Set) ([]platform.Segment, bool) {
	if !containsEmote(text, set) {
		return nil, false
	}
	var segs []platform.Segment
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			segs = append(segs, platform.Segment{Kind: platform.SegText, Text: buf.String()})
			buf.Reset()
		}
	}
	for i := 0; i < len(text); {
		if text[i] == ' ' {
			buf.WriteByte(' ')
			i++
			continue
		}
		j := i
		for j < len(text) && text[j] != ' ' {
			j++
		}
		word := text[i:j]
		if e, ok := set.Lookup(word); ok {
			flush()
			emote := e
			segs = append(segs, platform.Segment{Kind: platform.SegEmote, Text: word, Emote: &emote})
		} else {
			buf.WriteString(word)
		}
		i = j
	}
	flush()
	return segs, true
}
