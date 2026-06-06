package streams

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

func TestTwitchProvider_ParsesLiveStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Client-ID") == "" {
			t.Errorf("expected POST with a Client-ID, got %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"data":{"user":{
			"stream":{"viewersCount":1234,"createdAt":"2026-06-06T10:00:00Z","previewImageURL":"https://cdn/thumb.jpg","game":{"displayName":"Just Chatting"}},
			"broadcastSettings":{"title":"big stream"}
		}}}`))
	}))
	defer srv.Close()

	info, ok, err := NewTwitch(srv.Client(), srv.URL).Fetch(context.Background(), platform.ChannelRef{Platform: platform.Twitch, Slug: "shroud"})
	if err != nil || !ok {
		t.Fatalf("fetch: ok=%v err=%v", ok, err)
	}
	if !info.Live || info.ViewerCount != 1234 || info.Title != "big stream" || info.Category != "Just Chatting" || info.ThumbnailURL != "https://cdn/thumb.jpg" {
		t.Fatalf("parsed = %+v", info)
	}
	if info.StartedAt.IsZero() {
		t.Fatal("expected a parsed start time")
	}
}

func TestTwitchProvider_OfflineWhenStreamNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"user":{"stream":null,"broadcastSettings":{"title":"x"}}}}`))
	}))
	defer srv.Close()

	info, ok, err := NewTwitch(srv.Client(), srv.URL).Fetch(context.Background(), platform.ChannelRef{Platform: platform.Twitch, Slug: "shroud"})
	if err != nil || !ok || info.Live {
		t.Fatalf("expected offline, got live=%v ok=%v err=%v", info.Live, ok, err)
	}
}

func TestTwitchProvider_IgnoresNonTwitch(t *testing.T) {
	_, ok, err := NewTwitch(nil, "http://unused").Fetch(context.Background(), platform.ChannelRef{Platform: platform.Kick, Slug: "x"})
	if ok || err != nil {
		t.Fatalf("kick channel should be a no-op, got ok=%v err=%v", ok, err)
	}
}

func TestKickProvider_ParsesLivestream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/trainwreck" {
			t.Errorf("path = %q", got)
		}
		_, _ = w.Write([]byte(`{"livestream":{"viewer_count":987,"session_title":"slots","created_at":"2026-06-06 09:00:00","thumbnail":{"url":"https://kick/thumb.jpg"},"categories":[{"name":"Slots"}]}}`))
	}))
	defer srv.Close()

	info, ok, err := NewKick(srv.Client(), srv.URL).Fetch(context.Background(), platform.ChannelRef{Platform: platform.Kick, Slug: "trainwreck"})
	if err != nil || !ok {
		t.Fatalf("fetch: ok=%v err=%v", ok, err)
	}
	if !info.Live || info.ViewerCount != 987 || info.Title != "slots" || info.Category != "Slots" || info.ThumbnailURL != "https://kick/thumb.jpg" {
		t.Fatalf("parsed = %+v", info)
	}
}

func TestKickProvider_ThumbnailVariants(t *testing.T) {
	cases := map[string]string{
		`{"url":"https://k/u.jpg"}`: "https://k/u.jpg",
		`{"src":"https://k/s.jpg"}`: "https://k/s.jpg",
		`"https://k/string.jpg"`:    "https://k/string.jpg",
		`null`:                      "",
	}
	for thumb, want := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"livestream":{"viewer_count":1,"thumbnail":` + thumb + `}}`))
		}))
		got, _, err := NewKick(srv.Client(), srv.URL).Fetch(context.Background(), platform.ChannelRef{Platform: platform.Kick, Slug: "c"})
		srv.Close()
		if err != nil {
			t.Fatalf("thumb %s: %v", thumb, err)
		}
		if got.ThumbnailURL != want {
			t.Errorf("thumb %s → %q, want %q", thumb, got.ThumbnailURL, want)
		}
	}
}

func TestKickProvider_OfflineWhenNoLivestream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"livestream":null}`))
	}))
	defer srv.Close()

	info, ok, err := NewKick(srv.Client(), srv.URL).Fetch(context.Background(), platform.ChannelRef{Platform: platform.Kick, Slug: "x"})
	if err != nil || !ok || info.Live {
		t.Fatalf("expected offline, got live=%v ok=%v err=%v", info.Live, ok, err)
	}
}

func TestResolver_RefreshAndSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"user":{"stream":{"viewersCount":5},"broadcastSettings":{"title":"t"}}}}`))
	}))
	defer srv.Close()

	r := NewResolver(NewTwitch(srv.Client(), srv.URL))
	ch := platform.ChannelRef{Platform: platform.Twitch, Slug: "a"}
	if got := r.Snapshot(Key(ch)); got != nil {
		t.Fatal("expected no snapshot before refresh")
	}
	if err := r.Refresh(context.Background(), ch); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	got := r.Snapshot(Key(ch))
	if got == nil || got.ViewerCount != 5 {
		t.Fatalf("snapshot = %+v", got)
	}
}
