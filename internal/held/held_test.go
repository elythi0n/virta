package held

import (
	"context"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

func ch(slug string) platform.ChannelRef {
	return platform.ChannelRef{Platform: platform.Twitch, Slug: slug}
}

func hold(q *Queue, id, slug, author, text string) {
	q.Consume(context.Background(), platform.MessageHeldEvent{Held: platform.HeldMessage{
		ID:      id,
		Channel: ch(slug),
		Author:  platform.Author{Login: author},
		Text:    text,
		HeldAt:  time.Unix(0, 0),
	}})
}

func TestQueue_AddListOrder(t *testing.T) {
	q := New()
	hold(q, "a", "forsen", "bob", "first")
	hold(q, "b", "xqc", "eve", "second")
	hold(q, "c", "forsen", "amy", "third")

	got := q.List()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	wantIDs := []string{"a", "b", "c"} // oldest first, regardless of channel
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Errorf("List[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
}

func TestQueue_ResolveRemoves(t *testing.T) {
	q := New()
	hold(q, "a", "forsen", "bob", "hi")
	hold(q, "b", "forsen", "eve", "yo")

	q.Consume(context.Background(), platform.HeldResolvedEvent{Channel: ch("forsen"), ID: "a", Approved: true})

	got := q.List()
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("after resolve, List = %+v, want only b", got)
	}
	if _, ok := q.Get("a"); ok {
		t.Error("Get(a) still found after resolve")
	}
}

func TestQueue_GetFindsAcrossChannels(t *testing.T) {
	q := New()
	hold(q, "a", "forsen", "bob", "hi")
	hold(q, "b", "xqc", "eve", "yo")

	m, ok := q.Get("b")
	if !ok {
		t.Fatal("Get(b) not found")
	}
	if m.Channel.Slug != "xqc" || m.Author.Login != "eve" {
		t.Errorf("Get(b) = %+v, want xqc/eve", m)
	}
}

func TestQueue_ReheldKeepsPosition(t *testing.T) {
	q := New()
	hold(q, "a", "forsen", "bob", "hi")
	hold(q, "b", "forsen", "eve", "yo")
	hold(q, "a", "forsen", "bob", "hi again") // duplicate id

	got := q.List()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (no duplicate)", len(got))
	}
	if got[0].ID != "a" || got[0].Text != "hi" {
		t.Errorf("first entry = %+v, want original a/hi kept", got[0])
	}
}

func TestQueue_EmptyIDIgnored(t *testing.T) {
	q := New()
	hold(q, "", "forsen", "bob", "hi")
	if len(q.List()) != 0 {
		t.Error("empty-id held message should be ignored")
	}
}

func TestQueue_PerChannelCap(t *testing.T) {
	q := New()
	for i := 0; i < maxPerChannel+50; i++ {
		hold(q, string(rune('a'+i%26))+string(rune('0'+i/26)), "forsen", "bob", "x")
	}
	if got := len(q.List()); got != maxPerChannel {
		t.Errorf("queue length = %d, want cap %d", got, maxPerChannel)
	}
}

func TestQueue_OtherEventsIgnored(t *testing.T) {
	q := New()
	q.Consume(context.Background(), platform.StatsEvent{Channel: ch("forsen")})
	q.Consume(context.Background(), platform.ChannelClearEvent{Channel: ch("forsen")})
	if len(q.List()) != 0 {
		t.Error("non-held events should not populate the queue")
	}
}
