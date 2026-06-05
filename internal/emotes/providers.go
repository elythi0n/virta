package emotes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

const providerUA = "virta/emotes"

func client(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// getJSON performs a GET and decodes JSON. A non-200 is an error so the caller can treat that
// source as absent without failing the whole resolution.
func getJSON(ctx context.Context, c *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", providerUA)
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("emotes: %s: status %d", url, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v)
}

// ---- 7TV (supports both Twitch and Kick identities) ----

type sevenTV struct {
	c    *http.Client
	base string
}

// New7TV builds the 7TV provider. base defaults to the production host.
func New7TV(c *http.Client, base string) Provider {
	if base == "" {
		base = "https://7tv.io"
	}
	return &sevenTV{c: client(c), base: base}
}

func (p *sevenTV) Name() string { return "7tv" }

type sevenTVEmote struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Data struct {
		Animated bool `json:"animated"`
	} `json:"data"`
}

func (e sevenTVEmote) ref() platform.EmoteRef {
	return platform.EmoteRef{
		Provider:    platform.Emote7TV,
		ID:          e.ID,
		Name:        e.Name,
		URLTemplate: "https://cdn.7tv.app/emote/" + e.ID + "/{size}",
		Animated:    e.Data.Animated,
	}
}

func (p *sevenTV) Fetch(ctx context.Context, ch platform.ChannelRef) ([]platform.EmoteRef, error) {
	var out []platform.EmoteRef
	// Channel set first so it wins over global on a within-provider name collision.
	var user struct {
		EmoteSet struct {
			Emotes []sevenTVEmote `json:"emotes"`
		} `json:"emote_set"`
	}
	if ch.ID != "" {
		if err := getJSON(ctx, p.c, p.base+"/v3/users/"+string(ch.Platform)+"/"+ch.ID, &user); err == nil {
			for _, e := range user.EmoteSet.Emotes {
				out = append(out, e.ref())
			}
		}
	}
	var global struct {
		Emotes []sevenTVEmote `json:"emotes"`
	}
	if err := getJSON(ctx, p.c, p.base+"/v3/emote-sets/global", &global); err == nil {
		for _, e := range global.Emotes {
			out = append(out, e.ref())
		}
	}
	return out, nil
}

// ---- BetterTTV (Twitch only) ----

type bttv struct {
	c    *http.Client
	base string
}

// NewBTTV builds the BetterTTV provider. base defaults to the production host.
func NewBTTV(c *http.Client, base string) Provider {
	if base == "" {
		base = "https://api.betterttv.net"
	}
	return &bttv{c: client(c), base: base}
}

func (p *bttv) Name() string { return "bttv" }

type bttvEmote struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	ImageType string `json:"imageType"`
	Animated  bool   `json:"animated"`
}

func (e bttvEmote) ref() platform.EmoteRef {
	return platform.EmoteRef{
		Provider:    platform.EmoteBTTV,
		ID:          e.ID,
		Name:        e.Code,
		URLTemplate: "https://cdn.betterttv.net/emote/" + e.ID + "/{size}",
		Animated:    e.Animated || e.ImageType == "gif",
	}
}

func (p *bttv) Fetch(ctx context.Context, ch platform.ChannelRef) ([]platform.EmoteRef, error) {
	if ch.Platform != platform.Twitch {
		return nil, nil // BTTV keys by Twitch id only
	}
	var out []platform.EmoteRef
	if ch.ID != "" {
		var user struct {
			ChannelEmotes []bttvEmote `json:"channelEmotes"`
			SharedEmotes  []bttvEmote `json:"sharedEmotes"`
		}
		if err := getJSON(ctx, p.c, p.base+"/3/cached/users/twitch/"+ch.ID, &user); err == nil {
			for _, e := range append(user.ChannelEmotes, user.SharedEmotes...) {
				out = append(out, e.ref())
			}
		}
	}
	var global []bttvEmote
	if err := getJSON(ctx, p.c, p.base+"/3/cached/emotes/global", &global); err == nil {
		for _, e := range global {
			out = append(out, e.ref())
		}
	}
	return out, nil
}

// ---- FrankerFaceZ (Twitch only) ----

type ffz struct {
	c    *http.Client
	base string
}

// NewFFZ builds the FrankerFaceZ provider. base defaults to the production host.
func NewFFZ(c *http.Client, base string) Provider {
	if base == "" {
		base = "https://api.frankerfacez.com"
	}
	return &ffz{c: client(c), base: base}
}

func (p *ffz) Name() string { return "ffz" }

type ffzEmote struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Animated any    `json:"animated"` // present (object/non-null) when animated
}

func (e ffzEmote) ref() platform.EmoteRef {
	id := fmt.Sprintf("%d", e.ID)
	return platform.EmoteRef{
		Provider:    platform.EmoteFFZ,
		ID:          id,
		Name:        e.Name,
		URLTemplate: "https://cdn.frankerfacez.com/emote/" + id + "/{size}",
		Animated:    e.Animated != nil,
	}
}

type ffzSets struct {
	Sets map[string]struct {
		Emoticons []ffzEmote `json:"emoticons"`
	} `json:"sets"`
}

func (s ffzSets) refs() []platform.EmoteRef {
	var out []platform.EmoteRef
	for _, set := range s.Sets {
		for _, e := range set.Emoticons {
			out = append(out, e.ref())
		}
	}
	return out
}

func (p *ffz) Fetch(ctx context.Context, ch platform.ChannelRef) ([]platform.EmoteRef, error) {
	if ch.Platform != platform.Twitch {
		return nil, nil
	}
	var out []platform.EmoteRef
	if ch.ID != "" {
		var room ffzSets
		if err := getJSON(ctx, p.c, p.base+"/v1/room/id/"+ch.ID, &room); err == nil {
			out = append(out, room.refs()...)
		}
	}
	var global ffzSets
	if err := getJSON(ctx, p.c, p.base+"/v1/set/global", &global); err == nil {
		out = append(out, global.refs()...)
	}
	return out, nil
}
