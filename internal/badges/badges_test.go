package badges

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

type fakeProvider struct {
	m   map[string]string
	err error
}

func (f fakeProvider) Name() string { return "fake" }
func (f fakeProvider) Fetch(context.Context, platform.ChannelRef) (map[string]string, error) {
	return f.m, f.err
}

var ch = platform.ChannelRef{Platform: platform.Twitch, ID: "42", Slug: "shroud"}

func TestStage_StampsResolvedArtwork(t *testing.T) {
	r := NewResolver(fakeProvider{m: map[string]string{"moderator/1": "https://cdn/mod.png"}})
	r.Refresh(context.Background(), ch)

	msg := &platform.UnifiedMessage{
		Channel: ch,
		Author:  platform.Author{Badges: []platform.Badge{{Set: "moderator", Version: "1"}, {Set: "subscriber", Version: "12"}}},
	}
	if err := NewStage(r).Annotate(context.Background(), msg); err != nil {
		t.Fatalf("annotate: %v", err)
	}
	if got := msg.Author.Badges[0].URL; got != "https://cdn/mod.png" {
		t.Fatalf("moderator url = %q", got)
	}
	if msg.Author.Badges[1].URL != "" {
		t.Fatalf("unresolved badge should keep an empty URL, got %q", msg.Author.Badges[1].URL)
	}
}

func TestStage_NoBadgesOrNoSnapshotIsNoop(t *testing.T) {
	stage := NewStage(NewResolver())
	msg := &platform.UnifiedMessage{Channel: ch, Author: platform.Author{Badges: []platform.Badge{{Set: "vip", Version: "1"}}}}
	if err := stage.Annotate(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if msg.Author.Badges[0].URL != "" {
		t.Fatal("no snapshot should leave URLs empty")
	}
}

func TestResolver_SkipsErroringProvider(t *testing.T) {
	r := NewResolver(fakeProvider{err: http.ErrHandlerTimeout}, fakeProvider{m: map[string]string{"vip/1": "u"}})
	set := r.Refresh(context.Background(), ch)
	if _, ok := set.Lookup("vip", "1"); !ok {
		t.Fatal("a healthy provider must still resolve when another errors")
	}
}

func TestTwitchProvider_ParsesDisplayDocs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/global/display":
			_, _ = w.Write([]byte(`{"badge_sets":{"moderator":{"versions":{"1":{"image_url_2x":"https://cdn/mod2x"}}}}}`))
		case r.URL.Path == "/channels/42/display":
			_, _ = w.Write([]byte(`{"badge_sets":{"subscriber":{"versions":{"12":{"image_url_2x":"https://cdn/sub12"}}}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	got, err := NewTwitch(srv.Client(), srv.URL).Fetch(context.Background(), ch)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got["moderator/1"] != "https://cdn/mod2x" || got["subscriber/12"] != "https://cdn/sub12" {
		t.Fatalf("merged badges = %v", got)
	}
}

func TestTwitchProvider_IgnoresNonTwitch(t *testing.T) {
	got, err := NewTwitch(nil, "http://unused").Fetch(context.Background(), platform.ChannelRef{Platform: platform.Kick, Slug: "x"})
	if err != nil || got != nil {
		t.Fatalf("kick channel should be a no-op, got %v / %v", got, err)
	}
}
