package streams

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

const ytLivePage = `<html><head>
<meta name="title" content="Big launch stream &amp; Q/A">
</head><body><script>
var ytInitialPlayerResponse = {"videoDetails":{"videoId":"dQw4w9WgXcQ","isLive":true},"microformat":{"liveBroadcastDetails":{"isLiveNow":true}}};
var ytInitialData = {"viewCount":{"videoViewCountRenderer":{"viewCount":{"runs":[{"text":"1,234"},{"text":" watching now"}]},"isLive":true}}};
</script></body></html>`

func TestYouTubeProvider_ParsesLivePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/@somecreator/live" {
			t.Errorf("path = %q", got)
		}
		_, _ = w.Write([]byte(ytLivePage))
	}))
	defer srv.Close()

	info, ok, err := NewYouTube(srv.Client(), srv.URL).Fetch(context.Background(), platform.ChannelRef{Platform: platform.YouTube, Slug: "somecreator"})
	if err != nil || !ok {
		t.Fatalf("fetch: ok=%v err=%v", ok, err)
	}
	if !info.Live || info.Title != "Big launch stream & Q/A" || info.ViewerCount != 1234 {
		t.Fatalf("parsed = %+v", info)
	}
	if info.ThumbnailURL != "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault_live.jpg" {
		t.Errorf("thumbnail = %q", info.ThumbnailURL)
	}
}

func TestYouTubeProvider_OfflineWhenNoLiveMarkers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>channel home, nothing live</body></html>`))
	}))
	defer srv.Close()

	info, ok, err := NewYouTube(srv.Client(), srv.URL).Fetch(context.Background(), platform.ChannelRef{Platform: platform.YouTube, Slug: "somecreator"})
	if err != nil || !ok || info.Live {
		t.Fatalf("expected offline, got live=%v ok=%v err=%v", info.Live, ok, err)
	}
}

func TestYouTubeProvider_NotFoundIsOffline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer srv.Close()

	info, ok, err := NewYouTube(srv.Client(), srv.URL).Fetch(context.Background(), platform.ChannelRef{Platform: platform.YouTube, Slug: "ghost"})
	if err != nil || !ok || info.Live {
		t.Fatalf("expected offline for 404, got live=%v ok=%v err=%v", info.Live, ok, err)
	}
}

func TestYouTubeProvider_IgnoresOtherPlatforms(t *testing.T) {
	_, ok, err := NewYouTube(nil, "http://unused").Fetch(context.Background(), platform.ChannelRef{Platform: platform.Twitch, Slug: "x"})
	if ok || err != nil {
		t.Fatalf("twitch channel should be a no-op, got ok=%v err=%v", ok, err)
	}
}
