// Package scrollback keeps a bounded, in-memory ring of recent messages per channel so the app has
// session history even when persistent logging is off. It is a pipeline Sink — it watches the
// event stream and retains the tail — and serves the same History/Search shape the store does, so
// the API can fall back to it when the database isn't being written (logging is opt-in).
//
// When logging is on the store holds the authoritative, durable history and this ring is unused for
// reads; when off, this is the only scrollback there is, and it vanishes on restart by design.
package scrollback

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/elythi0n/virta/internal/platform"
)

// maxPerChannel bounds the retained tail per channel. A few hundred lines covers a session's
// scrollback while keeping memory flat on a busy channel.
const maxPerChannel = 500

// Msg is the compact retained form of a message — just what History/Search render.
type Msg struct {
	ID      string
	Channel platform.ChannelRef
	Author  string
	Body    string
	SentAt  int64 // unix milliseconds
	Deleted bool
}

// Ring is the per-channel scrollback. Safe for concurrent use: the pipeline drives Consume from a
// single worker, but the API reads History/Search from request goroutines.
type Ring struct {
	mu        sync.RWMutex
	byChannel map[string][]Msg // channel key → oldest..newest
}

// New returns an empty ring.
func New() *Ring { return &Ring{byChannel: map[string][]Msg{}} }

// Name identifies the sink in diagnostics.
func (r *Ring) Name() string { return "scrollback" }

// Consume retains chat-bearing messages and marks deletions; other events are ignored.
func (r *Ring) Consume(_ context.Context, ev platform.Event) error {
	switch e := ev.(type) {
	case platform.MessageEvent:
		r.add(e.Message)
	case platform.MessageDeletedEvent:
		r.markDeleted(e.Channel.Key(), e.MessageID)
	}
	return nil
}

// Close satisfies pipeline.Sink; the ring holds no resources.
func (r *Ring) Close() error { return nil }

func (r *Ring) add(m platform.UnifiedMessage) {
	if m.ID == "" {
		return
	}
	author := m.Author.DisplayName
	if author == "" {
		author = m.Author.Login
	}
	rec := Msg{ID: m.ID, Channel: m.Channel, Author: author, Body: m.PlainText(), SentAt: m.SentAt.UnixMilli()}
	key := m.Channel.Key()
	r.mu.Lock()
	defer r.mu.Unlock()
	buf := append(r.byChannel[key], rec)
	if len(buf) > maxPerChannel {
		buf = buf[len(buf)-maxPerChannel:]
	}
	r.byChannel[key] = buf
}

func (r *Ring) markDeleted(channelKey, id string) {
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	buf := r.byChannel[channelKey]
	for i := range buf {
		if buf[i].ID == id {
			buf[i].Deleted = true
			return
		}
	}
}

// History returns a channel's retained tail, newest-first, before the given ULID cursor.
func (r *Ring) History(channelKey, before string, limit int) []Msg {
	limit = clampLimit(limit)
	r.mu.RLock()
	defer r.mu.RUnlock()
	buf := r.byChannel[channelKey]
	out := make([]Msg, 0, limit)
	for i := len(buf) - 1; i >= 0 && len(out) < limit; i-- {
		if before != "" && buf[i].ID >= before {
			continue
		}
		out = append(out, buf[i])
	}
	return out
}

// Search scans the retained messages for a case-insensitive substring of text, newest-first,
// optionally narrowed to one channel ("" = all) and/or author (matched by display name). It mirrors
// the store's full-text search closely enough for a session's worth of scrollback.
func (r *Ring) Search(channelKey, text, author, before string, limit int) []Msg {
	needle := strings.ToLower(strings.TrimSpace(text))
	if needle == "" {
		return nil
	}
	limit = clampLimit(limit)
	r.mu.RLock()
	defer r.mu.RUnlock()

	var hits []Msg
	scan := func(buf []Msg) {
		for _, m := range buf {
			if before != "" && m.ID >= before {
				continue
			}
			if author != "" && !strings.EqualFold(m.Author, author) {
				continue
			}
			if !strings.Contains(strings.ToLower(m.Body), needle) {
				continue
			}
			hits = append(hits, m)
		}
	}
	if channelKey != "" {
		scan(r.byChannel[channelKey])
	} else {
		for _, buf := range r.byChannel {
			scan(buf)
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].ID > hits[j].ID }) // newest-first
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func clampLimit(n int) int {
	if n <= 0 {
		return 100
	}
	if n > 1000 {
		return 1000
	}
	return n
}
