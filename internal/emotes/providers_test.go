package emotes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

// routeServer serves canned JSON bodies keyed by exact request path.
func routeServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func byName(refs []platform.EmoteRef) map[string]platform.EmoteRef {
	m := make(map[string]platform.EmoteRef, len(refs))
	for _, e := range refs {
		m[e.Name] = e
	}
	return m
}

func TestSevenTV_ParsesChannelAndGlobal(t *testing.T) {
	srv := routeServer(t, map[string]string{
		"/v3/users/twitch/1":    `{"emote_set":{"emotes":[{"id":"a1","name":"PogU","data":{"animated":true}}]}}`,
		"/v3/emote-sets/global": `{"emotes":[{"id":"g1","name":"Clap","data":{"animated":false}}]}`,
	})
	p := New7TV(srv.Client(), srv.URL)
	refs, err := p.Fetch(context.Background(), platform.ChannelRef{Platform: platform.Twitch, ID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	m := byName(refs)
	if e := m["PogU"]; e.Provider != platform.Emote7TV || e.ID != "a1" || !e.Animated {
		t.Errorf("PogU = %+v", e)
	}
	if e := m["Clap"]; e.URLTemplate != "https://cdn.7tv.app/emote/g1/{size}" {
		t.Errorf("Clap url = %q", e.URLTemplate)
	}
}

func TestSevenTV_SupportsKickIdentity(t *testing.T) {
	srv := routeServer(t, map[string]string{
		"/v3/users/kick/55": `{"emote_set":{"emotes":[{"id":"k1","name":"kickEmote","data":{}}]}}`,
	})
	p := New7TV(srv.Client(), srv.URL)
	refs, _ := p.Fetch(context.Background(), platform.ChannelRef{Platform: platform.Kick, ID: "55"})
	if _, ok := byName(refs)["kickEmote"]; !ok {
		t.Errorf("kick identity not resolved: %+v", refs)
	}
}

func TestBTTV_ParsesAndTwitchOnly(t *testing.T) {
	srv := routeServer(t, map[string]string{
		"/3/cached/users/twitch/1": `{"channelEmotes":[{"id":"c1","code":"chanEmote","imageType":"png"}],"sharedEmotes":[{"id":"s1","code":"sharedEmote","imageType":"gif"}]}`,
		"/3/cached/emotes/global":  `[{"id":"g1","code":"globalEmote","imageType":"png","animated":false}]`,
	})
	p := NewBTTV(srv.Client(), srv.URL)
	refs, _ := p.Fetch(context.Background(), platform.ChannelRef{Platform: platform.Twitch, ID: "1"})
	m := byName(refs)
	if e := m["sharedEmote"]; !e.Animated || e.Provider != platform.EmoteBTTV {
		t.Errorf("sharedEmote (gif) = %+v, want animated bttv", e)
	}
	if _, ok := m["globalEmote"]; !ok {
		t.Error("global emote missing")
	}
	// BTTV does not apply to Kick.
	if refs, _ := p.Fetch(context.Background(), platform.ChannelRef{Platform: platform.Kick, ID: "1"}); refs != nil {
		t.Errorf("BTTV returned emotes for a Kick channel: %+v", refs)
	}
}

func TestFFZ_ParsesRoomAndGlobal(t *testing.T) {
	srv := routeServer(t, map[string]string{
		"/v1/room/id/1":  `{"sets":{"42":{"emoticons":[{"id":100,"name":"roomEmote","animated":{"1":"x"}}]}}}`,
		"/v1/set/global": `{"sets":{"3":{"emoticons":[{"id":200,"name":"globalEmote"}]}}}`,
	})
	p := NewFFZ(srv.Client(), srv.URL)
	refs, _ := p.Fetch(context.Background(), platform.ChannelRef{Platform: platform.Twitch, ID: "1"})
	m := byName(refs)
	if e := m["roomEmote"]; e.ID != "100" || !e.Animated || e.Provider != platform.EmoteFFZ {
		t.Errorf("roomEmote = %+v", e)
	}
	if e := m["globalEmote"]; e.Animated {
		t.Errorf("globalEmote should not be animated: %+v", e)
	}
}

func TestProviders_ToleratePartialFailure(t *testing.T) {
	// Only the global route exists; the channel route 404s. The provider still returns global.
	srv := routeServer(t, map[string]string{
		"/v3/emote-sets/global": `{"emotes":[{"id":"g1","name":"OnlyGlobal","data":{}}]}`,
	})
	p := New7TV(srv.Client(), srv.URL)
	refs, err := p.Fetch(context.Background(), platform.ChannelRef{Platform: platform.Twitch, ID: "1"})
	if err != nil {
		t.Fatalf("partial failure should not error: %v", err)
	}
	if _, ok := byName(refs)["OnlyGlobal"]; !ok {
		t.Error("global emotes lost when channel route failed")
	}
}
