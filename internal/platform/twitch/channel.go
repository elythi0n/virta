package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ChannelInfoPatch is the set of broadcaster fields Helix lets us patch. Empty fields are
// omitted from the request body so a partial update only touches what the caller specified.
type ChannelInfoPatch struct {
	Title    string   `json:"title,omitempty"`
	GameID   string   `json:"game_id,omitempty"`
	Language string   `json:"broadcaster_language,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// Category is a Helix game/category entry returned by the search endpoint.
type Category struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BoxArtURL string `json:"box_art_url,omitempty"`
}

// ChannelInfo is the broadcaster's currently-set stream info returned by GET /helix/channels.
// Title/Category/Tags mirror what the broadcaster sees in the Twitch Creator Dashboard.
type ChannelInfo struct {
	BroadcasterID    string   `json:"broadcaster_id"`
	BroadcasterLogin string   `json:"broadcaster_login"`
	Title            string   `json:"title"`
	GameID           string   `json:"game_id"`
	GameName         string   `json:"game_name"`
	Language         string   `json:"broadcaster_language"`
	Tags             []string `json:"tags"`
}

// GetChannelInfo reads the broadcaster's currently-set stream metadata. Returns an empty
// ChannelInfo (no error) when the broadcaster has no row — a fresh account that's never set a
// title — rather than failing the caller's pre-fill.
func (c *HelixClient) GetChannelInfo(ctx context.Context, token, broadcasterID string) (ChannelInfo, error) {
	if broadcasterID == "" {
		return ChannelInfo{}, fmt.Errorf("twitch: get channel: empty broadcaster id")
	}
	q := url.Values{}
	q.Set("broadcaster_id", broadcasterID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.channelsURL+"?"+q.Encode(), nil)
	if err != nil {
		return ChannelInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", c.clientID())
	resp, err := c.http.Do(req)
	if err != nil {
		return ChannelInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return ChannelInfo{}, fmt.Errorf("twitch: get channel: status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data []ChannelInfo `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return ChannelInfo{}, fmt.Errorf("twitch: decode channel response: %w", err)
	}
	if len(out.Data) == 0 {
		return ChannelInfo{}, nil
	}
	return out.Data[0], nil
}

// UpdateChannelInfo patches the broadcaster's stream title, category, language, and tags. Only
// the non-empty fields in patch are sent; Twitch leaves the rest untouched. The token must carry
// channel:manage:broadcast.
func (c *HelixClient) UpdateChannelInfo(ctx context.Context, token, broadcasterID string, patch ChannelInfoPatch) error {
	if broadcasterID == "" {
		return fmt.Errorf("twitch: update channel: empty broadcaster id")
	}
	q := url.Values{}
	q.Set("broadcaster_id", broadcasterID)
	return c.do(ctx, token, http.MethodPatch, c.channelsURL+"?"+q.Encode(), "update channel", patch)
}

// SearchCategories returns games/categories matching query, ordered by Helix's relevance
// scoring. The caller picks one and feeds its id back into UpdateChannelInfo as GameID.
func (c *HelixClient) SearchCategories(ctx context.Context, token, query string) ([]Category, error) {
	if query == "" {
		return nil, nil
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("first", "10")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.searchCategoriesURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", c.clientID())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: search categories: status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data []Category `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("twitch: decode search response: %w", err)
	}
	return out.Data, nil
}

// SetBroadcasterURLs overrides the channels and search-categories endpoints (tests point them at
// a local server).
func (c *HelixClient) SetBroadcasterURLs(channels, searchCategories string) {
	c.channelsURL, c.searchCategoriesURL = channels, searchCategories
}
