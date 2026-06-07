package scrollback

import (
	"context"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

func ch(slug string) platform.ChannelRef {
	return platform.ChannelRef{Platform: platform.Twitch, Slug: slug}
}

func feed(r *Ring, id, slug, author, body string) {
	_ = r.Consume(context.Background(), platform.MessageEvent{Message: platform.UnifiedMessage{
		ID:       id,
		Channel:  ch(slug),
		Type:     platform.TypeChat,
		Author:   platform.Author{DisplayName: author},
		Segments: []platform.Segment{{Kind: platform.SegText, Text: body}},
		SentAt:   time.Unix(0, 0),
	}})
}

func ids(ms []Msg) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func TestRing_HistoryNewestFirst(t *testing.T) {
	r := New()
	feed(r, "m1", "forsen", "a", "hello")
	feed(r, "m2", "forsen", "b", "world")
	feed(r, "m3", "xqc", "c", "other")

	got := r.History(ch("forsen").Key(), "", 10)
	if g := ids(got); len(g) != 2 || g[0] != "m2" || g[1] != "m1" {
		t.Fatalf("history = %v, want [m2 m1]", ids(got))
	}
}

func TestRing_HistoryCursor(t *testing.T) {
	r := New()
	feed(r, "m1", "forsen", "a", "x")
	feed(r, "m2", "forsen", "b", "y")
	feed(r, "m3", "forsen", "c", "z")
	got := r.History(ch("forsen").Key(), "m3", 10)
	if g := ids(got); len(g) != 2 || g[0] != "m2" {
		t.Errorf("before m3 = %v, want [m2 m1]", g)
	}
}

func TestRing_HistoryPerChannelCap(t *testing.T) {
	r := New()
	for i := 0; i < maxPerChannel+50; i++ {
		feed(r, string(rune('a'+i%26))+string(rune('0'+i/26)), "forsen", "a", "x")
	}
	if got := len(r.History(ch("forsen").Key(), "", 10000)); got != maxPerChannel {
		t.Errorf("retained %d, want cap %d", got, maxPerChannel)
	}
}

func TestRing_SearchAcrossChannelsAndFilters(t *testing.T) {
	r := New()
	feed(r, "m1", "forsen", "Alice", "hello world")
	feed(r, "m2", "forsen", "Bob", "goodbye world")
	feed(r, "m3", "xqc", "Alice", "hello there")

	all := r.Search("", "hello", "", "", 10)
	if g := ids(all); len(g) != 2 || g[0] != "m3" || g[1] != "m1" {
		t.Fatalf("search hello = %v, want [m3 m1]", g)
	}
	inForsen := r.Search(ch("forsen").Key(), "hello", "", "", 10)
	if g := ids(inForsen); len(g) != 1 || g[0] != "m1" {
		t.Errorf("search hello in forsen = %v, want [m1]", g)
	}
	byBob := r.Search("", "world", "bob", "", 10)
	if g := ids(byBob); len(g) != 1 || g[0] != "m2" {
		t.Errorf("search world by Bob = %v, want [m2]", g)
	}
	if g := r.Search("", "  ", "", "", 10); g != nil {
		t.Errorf("empty text search = %v, want nil", g)
	}
}

func TestRing_MarkDeleted(t *testing.T) {
	r := New()
	feed(r, "m1", "forsen", "a", "oops")
	_ = r.Consume(context.Background(), platform.MessageDeletedEvent{Channel: ch("forsen"), MessageID: "m1"})
	got := r.History(ch("forsen").Key(), "", 10)
	if len(got) != 1 || !got[0].Deleted {
		t.Errorf("message not marked deleted: %+v", got)
	}
}

func TestRing_IgnoresOtherEvents(t *testing.T) {
	r := New()
	_ = r.Consume(context.Background(), platform.StatsEvent{Channel: ch("forsen")})
	if got := r.History(ch("forsen").Key(), "", 10); len(got) != 0 {
		t.Errorf("non-message events should not populate the ring: %v", ids(got))
	}
}
