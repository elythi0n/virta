package badges

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// Twitch's public GraphQL gateway resolves badge artwork anonymously: the stable web Client-ID
// below is what the site itself ships, so no user token or numeric broadcaster id is needed. The
// legacy badges.twitch.tv CDN is being retired; GraphQL also lets us fetch a channel's own badges
// by login (slug), which we have at join time, instead of requiring a resolved broadcaster id.
const (
	defaultTwitchGQL = "https://gql.twitch.tv/gql"
	twitchClientID   = "kimne78kx3ncx6brgo4mv6wki5h1ko"
)

// One round trip resolves both the global badge set and this channel's own badges. Images are
// requested at DOUBLE (2x) to stay crisp at the row's 18px without overfetching.
const badgesQuery = `query Badges($login: String!) {
  badges { setID version imageURL(size: DOUBLE) }
  user(login: $login) { broadcastBadges { setID version imageURL(size: DOUBLE) } }
}`

func httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// gqlBadge is one badge in a GraphQL badges list (global or per-channel).
type gqlBadge struct {
	SetID    string `json:"setID"`
	Version  string `json:"version"`
	ImageURL string `json:"imageURL"`
}

type gqlResponse struct {
	Data struct {
		Badges []gqlBadge `json:"badges"`
		User   *struct {
			BroadcastBadges []gqlBadge `json:"broadcastBadges"`
		} `json:"user"`
	} `json:"data"`
}

type twitch struct {
	c        *http.Client
	endpoint string
}

// NewTwitch builds the Twitch badge provider. Pass a nil client for the default and "" for the
// default GraphQL endpoint (override both in tests).
func NewTwitch(c *http.Client, endpoint string) Provider {
	if endpoint == "" {
		endpoint = defaultTwitchGQL
	}
	return &twitch{c: httpClient(c), endpoint: endpoint}
}

func (p *twitch) Name() string { return "twitch" }

func (p *twitch) Fetch(ctx context.Context, ch platform.ChannelRef) (map[string]string, error) {
	if ch.Platform != platform.Twitch || ch.Slug == "" {
		return nil, nil
	}
	body, err := json.Marshal(map[string]any{
		"query":     badgesQuery,
		"variables": map[string]string{"login": ch.Slug},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", twitchClientID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("badges: POST %s: %d", p.endpoint, resp.StatusCode)
	}
	var doc gqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}
	out := map[string]string{}
	collect(out, doc.Data.Badges) // globals first
	if doc.Data.User != nil {
		collect(out, doc.Data.User.BroadcastBadges) // channel-specific wins on conflict
	}
	return out, nil
}

func collect(out map[string]string, badges []gqlBadge) {
	for _, b := range badges {
		if b.SetID != "" && b.ImageURL != "" {
			out[setKey(b.SetID, b.Version)] = b.ImageURL
		}
	}
}

