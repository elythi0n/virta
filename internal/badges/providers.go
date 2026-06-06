package badges

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// defaultTwitchBase is Twitch's tokenless badge CDN (no Helix auth needed): global badge artwork
// plus per-channel sets (sub tiers, bits) keyed by broadcaster id.
const defaultTwitchBase = "https://badges.twitch.tv/v1/badges"

func httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func getJSON(ctx context.Context, c *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("badges: GET %s: %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// badgeDisplay is the shape of Twitch's badge display documents (global and per-channel).
type badgeDisplay struct {
	BadgeSets map[string]struct {
		Versions map[string]struct {
			Image1x string `json:"image_url_1x"`
			Image2x string `json:"image_url_2x"`
			Image4x string `json:"image_url_4x"`
			Title   string `json:"title"`
		} `json:"versions"`
	} `json:"badge_sets"`
}

type twitch struct {
	c    *http.Client
	base string
}

// NewTwitch builds the Twitch badge provider. Pass a nil client for the default and "" for the
// default CDN base (override both in tests).
func NewTwitch(c *http.Client, base string) Provider {
	if base == "" {
		base = defaultTwitchBase
	}
	return &twitch{c: httpClient(c), base: base}
}

func (p *twitch) Name() string { return "twitch" }

func (p *twitch) Fetch(ctx context.Context, ch platform.ChannelRef) (map[string]string, error) {
	if ch.Platform != platform.Twitch {
		return nil, nil
	}
	out := map[string]string{}
	if err := p.load(ctx, p.base+"/global/display?language=en", out); err != nil {
		return nil, err
	}
	// Channel-specific badges (sub tiers, bits) need the broadcaster id; merge over the globals.
	if ch.ID != "" {
		_ = p.load(ctx, p.base+"/channels/"+ch.ID+"/display?language=en", out)
	}
	return out, nil
}

func (p *twitch) load(ctx context.Context, url string, out map[string]string) error {
	var doc badgeDisplay
	if err := getJSON(ctx, p.c, url, &doc); err != nil {
		return err
	}
	for set, s := range doc.BadgeSets {
		for version, v := range s.Versions {
			if img := pickImage(v.Image2x, v.Image1x, v.Image4x); img != "" {
				out[setKey(set, version)] = img
			}
		}
	}
	return nil
}

func pickImage(candidates ...string) string {
	for _, c := range candidates {
		if c != "" {
			return c
		}
	}
	return ""
}
