package streams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

const (
	defaultTwitchGQL = "https://gql.twitch.tv/gql"
	twitchClientID   = "kimne78kx3ncx6brgo4mv6wki5h1ko"
	defaultKickBase  = "https://kick.com/api/v2/channels"
	// A browser User-Agent so Kick's edge serves the channel JSON to an anonymous client.
	chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

func httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// --- Twitch (anonymous GraphQL, same gateway/Client-ID as badges) ---

const twitchStreamQuery = `query StreamInfo($login: String!) {
  user(login: $login) {
    stream { viewersCount createdAt previewImageURL(width: 320, height: 180) game { displayName } }
    broadcastSettings { title }
  }
}`

type twitchResp struct {
	Data struct {
		User *struct {
			Stream *struct {
				ViewersCount    int    `json:"viewersCount"`
				CreatedAt       string `json:"createdAt"`
				PreviewImageURL string `json:"previewImageURL"`
				Game            *struct {
					DisplayName string `json:"displayName"`
				} `json:"game"`
			} `json:"stream"`
			BroadcastSettings *struct {
				Title string `json:"title"`
			} `json:"broadcastSettings"`
		} `json:"user"`
	} `json:"data"`
}

type twitchProvider struct {
	c        *http.Client
	endpoint string
}

// NewTwitch builds the Twitch stream-info provider. Pass a nil client for the default and "" for
// the default GraphQL endpoint (override both in tests).
func NewTwitch(c *http.Client, endpoint string) Provider {
	if endpoint == "" {
		endpoint = defaultTwitchGQL
	}
	return &twitchProvider{c: httpClient(c), endpoint: endpoint}
}

func (p *twitchProvider) Name() string { return "twitch" }

func (p *twitchProvider) Fetch(ctx context.Context, ch platform.ChannelRef) (Info, bool, error) {
	if ch.Platform != platform.Twitch || ch.Slug == "" {
		return Info{}, false, nil
	}
	body, err := json.Marshal(map[string]any{
		"query":     twitchStreamQuery,
		"variables": map[string]string{"login": ch.Slug},
	})
	if err != nil {
		return Info{}, true, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return Info{}, true, err
	}
	req.Header.Set("Client-ID", twitchClientID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.c.Do(req)
	if err != nil {
		return Info{}, true, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Info{}, true, fmt.Errorf("streams: twitch POST %s: %d", p.endpoint, resp.StatusCode)
	}
	var doc twitchResp
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return Info{}, true, err
	}
	u := doc.Data.User
	if u == nil || u.Stream == nil {
		return Info{Live: false}, true, nil // user not found or offline
	}
	info := Info{
		Live:         true,
		ViewerCount:  u.Stream.ViewersCount,
		ThumbnailURL: u.Stream.PreviewImageURL,
		StartedAt:    parseTime(u.Stream.CreatedAt),
	}
	if u.Stream.Game != nil {
		info.Category = u.Stream.Game.DisplayName
	}
	if u.BroadcastSettings != nil {
		info.Title = u.BroadcastSettings.Title
	}
	return info, true, nil
}

// --- Kick (anonymous channel JSON) ---

type kickResp struct {
	Livestream *struct {
		ViewerCount  int             `json:"viewer_count"`
		SessionTitle string          `json:"session_title"`
		CreatedAt    string          `json:"created_at"`
		Thumbnail    json.RawMessage `json:"thumbnail"` // shape varies: {url}|{src}|string
		Categories   []struct {
			Name string `json:"name"`
		} `json:"categories"`
	} `json:"livestream"`
}

// thumbURL pulls the image URL out of Kick's thumbnail field, which has appeared as an object with
// a "url" or "src", or as a bare string.
func thumbURL(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var obj struct {
		URL string `json:"url"`
		Src string `json:"src"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		if obj.URL != "" {
			return obj.URL
		}
		if obj.Src != "" {
			return obj.Src
		}
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

type kickProvider struct {
	c    *http.Client
	base string
}

// NewKick builds the Kick stream-info provider. Pass a nil client for the default and "" for the
// default channel API base (override both in tests).
func NewKick(c *http.Client, base string) Provider {
	if base == "" {
		base = defaultKickBase
	}
	return &kickProvider{c: httpClient(c), base: base}
}

func (p *kickProvider) Name() string { return "kick" }

func (p *kickProvider) Fetch(ctx context.Context, ch platform.ChannelRef) (Info, bool, error) {
	if ch.Platform != platform.Kick || ch.Slug == "" {
		return Info{}, false, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/"+ch.Slug, nil)
	if err != nil {
		return Info{}, true, err
	}
	req.Header.Set("User-Agent", chromeUA)
	req.Header.Set("Accept", "application/json")
	resp, err := p.c.Do(req)
	if err != nil {
		return Info{}, true, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Info{}, true, fmt.Errorf("streams: kick GET %s: %d", ch.Slug, resp.StatusCode)
	}
	var doc kickResp
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return Info{}, true, err
	}
	if doc.Livestream == nil {
		return Info{Live: false}, true, nil
	}
	info := Info{
		Live:         true,
		ViewerCount:  doc.Livestream.ViewerCount,
		Title:        doc.Livestream.SessionTitle,
		ThumbnailURL: thumbURL(doc.Livestream.Thumbnail),
		StartedAt:    parseTime(doc.Livestream.CreatedAt),
	}
	if len(doc.Livestream.Categories) > 0 {
		info.Category = doc.Livestream.Categories[0].Name
	}
	return info, true, nil
}

// parseTime accepts RFC3339 (Twitch) and Kick's "2006-01-02 15:04:05" form; an unparseable value
// yields the zero time (StartedAt is then simply omitted on the wire).
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
