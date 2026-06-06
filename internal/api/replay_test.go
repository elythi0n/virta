package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

func kickMsg(slug string) platform.MessageEvent {
	return platform.MessageEvent{Message: platform.UnifiedMessage{
		ID: slug, Channel: platform.ChannelRef{Platform: platform.Kick, Slug: slug},
	}}
}

// seqsFrom drains a client's send buffer and returns the seq of each queued event.
func seqsFrom(ch chan []byte) []uint64 {
	var out []uint64
	for {
		select {
		case b := <-ch:
			var we wireEvent
			_ = json.Unmarshal(b, &we)
			out = append(out, we.Seq)
		default:
			return out
		}
	}
}

func TestHub_AssignsMonotonicSeq(t *testing.T) {
	h := newHub()
	c := newClient()
	h.register(c)
	_ = h.Consume(context.Background(), kickMsg("a"))
	_ = h.Consume(context.Background(), kickMsg("b"))
	got := seqsFrom(c.send)
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("seqs = %v, want [1 2]", got)
	}
}

// TestHub_SubscriptionMatchesCanonicalSlugCase guards against a case-sensitive channel filter: a
// client subscribing with the casing the user typed must still receive messages, which arrive
// carrying the platform's canonical lower-case slug.
func TestHub_SubscriptionMatchesCanonicalSlugCase(t *testing.T) {
	h := newHub()
	c := newClient()
	h.register(c)
	c.setSubscription(toSubscription([]string{"twitch:Shroud"}))
	_ = h.Consume(context.Background(), platform.MessageEvent{Message: platform.UnifiedMessage{
		ID: "m1", Channel: platform.ChannelRef{Platform: platform.Twitch, Slug: "shroud"},
	}})
	if got := seqsFrom(c.send); len(got) != 1 {
		t.Fatalf("mixed-case subscriber received %d messages, want 1", len(got))
	}
}

func TestHub_ReplayResumesPastCursor(t *testing.T) {
	h := newHub()
	// Buffer three events with no client attached (seq 1,2,3).
	for _, slug := range []string{"a", "b", "c"} {
		_ = h.Consume(context.Background(), kickMsg(slug))
	}
	// A client resuming from cursor 1 should receive only seq 2 and 3, in order.
	c := newClient()
	h.replayTo(c, 1)
	got := seqsFrom(c.send)
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("replayed seqs = %v, want [2 3]", got)
	}
}

func TestHub_ReplayRespectsSubscription(t *testing.T) {
	h := newHub()
	_ = h.Consume(context.Background(), kickMsg("forsen")) // seq 1
	_ = h.Consume(context.Background(), kickMsg("xqc"))    // seq 2

	c := newClient()
	c.setSubscription(toSubscription([]string{"kick:xqc"}))
	h.replayTo(c, 0) // from the beginning, but filtered
	got := seqsFrom(c.send)
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("filtered replay seqs = %v, want [2] (only kick:xqc)", got)
	}
}

func TestHub_ReplayBufferIsBounded(t *testing.T) {
	h := newHub()
	// Push well past the ring capacity; only the most recent replayBuffer survive.
	total := replayBuffer + 100
	for i := 0; i < total; i++ {
		_ = h.Consume(context.Background(), kickMsg("c"))
	}
	c := newClient()
	h.replayTo(c, 0) // everything still buffered
	got := seqsFrom(c.send)
	if len(got) > replayBuffer {
		t.Errorf("replay returned %d events, want <= %d (bounded ring)", len(got), replayBuffer)
	}
	// The oldest retained event must be from the tail of the stream, not the start.
	if len(got) > 0 && got[0] <= uint64(total-replayBuffer) {
		t.Errorf("oldest replayed seq = %d, want > %d (old events evicted)", got[0], total-replayBuffer)
	}
}
