// Package held maintains the AutoMod held-message queue: the set of messages a platform is
// holding for moderator review (Twitch automod.message.hold and equivalents), pending an approve
// or deny. It is a pipeline Sink — it watches MessageHeldEvent / HeldResolvedEvent flow past and
// keeps a bounded, per-channel ordered snapshot that the API serves to the moderation pane.
//
// The queue only tracks state; performing an approve/deny is the dispatch layer's job (one typed
// ModAction path). After a resolve succeeds, a HeldResolvedEvent flows back through the pipeline
// and clears the entry here, so the queue and every connected client converge on the same path
// as a platform-driven resolution.
package held

import (
	"context"
	"sort"
	"sync"

	"github.com/elythi0n/virta/internal/platform"
)

// maxPerChannel bounds how many held messages we retain per channel. AutoMod queues are small in
// practice; the cap keeps memory constant if a channel is flooded and a moderator steps away.
const maxPerChannel = 200

// entry is one held message plus its arrival order, so the snapshot is stable oldest-first.
type entry struct {
	msg platform.HeldMessage
	seq uint64
}

// Queue is the live held-message set. Safe for concurrent use: the pipeline drives Consume from a
// single worker, but the API reads List/Get from request goroutines.
type Queue struct {
	mu  sync.RWMutex
	seq uint64
	// keyed by channel key, then by held-message id, so a resolve is O(1) and channel caps are local.
	byChannel map[string]map[string]entry
}

// New returns an empty queue.
func New() *Queue {
	return &Queue{byChannel: map[string]map[string]entry{}}
}

// Name identifies the sink in diagnostics.
func (q *Queue) Name() string { return "held" }

// Consume records held messages and clears resolved ones; all other events are ignored.
func (q *Queue) Consume(_ context.Context, ev platform.Event) error {
	switch e := ev.(type) {
	case platform.MessageHeldEvent:
		q.add(e.Held)
	case platform.HeldResolvedEvent:
		q.remove(e.Channel.Key(), e.ID)
	}
	return nil
}

// Close satisfies pipeline.Sink; the queue holds no resources.
func (q *Queue) Close() error { return nil }

func (q *Queue) add(m platform.HeldMessage) {
	if m.ID == "" {
		return
	}
	key := m.Channel.Key()
	q.mu.Lock()
	defer q.mu.Unlock()
	set := q.byChannel[key]
	if set == nil {
		set = map[string]entry{}
		q.byChannel[key] = set
	}
	// A re-held id keeps its original position rather than jumping the queue.
	if _, ok := set[m.ID]; ok {
		return
	}
	q.seq++
	set[m.ID] = entry{msg: m, seq: q.seq}
	q.evict(set)
}

// evict drops the oldest entries in a channel set once it exceeds the cap.
func (q *Queue) evict(set map[string]entry) {
	if len(set) <= maxPerChannel {
		return
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return set[ids[i]].seq < set[ids[j]].seq })
	for _, id := range ids[:len(set)-maxPerChannel] {
		delete(set, id)
	}
}

func (q *Queue) remove(channelKey, id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	set := q.byChannel[channelKey]
	if set == nil {
		return
	}
	delete(set, id)
	if len(set) == 0 {
		delete(q.byChannel, channelKey)
	}
}

// Get returns the held message for an id (searching every channel, since the API addresses held
// messages by id alone) and whether it was found.
func (q *Queue) Get(id string) (platform.HeldMessage, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	for _, set := range q.byChannel {
		if e, ok := set[id]; ok {
			return e.msg, true
		}
	}
	return platform.HeldMessage{}, false
}

// List returns every held message across all channels, oldest first.
func (q *Queue) List() []platform.HeldMessage {
	q.mu.RLock()
	defer q.mu.RUnlock()
	all := make([]entry, 0)
	for _, set := range q.byChannel {
		for _, e := range set {
			all = append(all, e)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].seq < all[j].seq })
	out := make([]platform.HeldMessage, len(all))
	for i, e := range all {
		out[i] = e.msg
	}
	return out
}
