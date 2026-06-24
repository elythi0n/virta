package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// Discovery endpoints. Channel search resolves a free-text query to channel listings (live or
// offline); top streams returns the currently-live channels sorted by viewer count. Both are
// paginated via Helix's "after" cursor (an opaque string the caller passes back to keep going).
const (
	helixSearchChannelsURL = "https://api.twitch.tv/helix/search/channels"
	helixStreamsURL        = "https://api.twitch.tv/helix/streams"
)

// ChannelSearchResult mirrors a /helix/search/channels row. Live channels carry game/title;
// offline ones leave them empty. Thumbnail and started_at help Discovery rank/render results.
type ChannelSearchResult struct {
	ID              string `json:"id"`
	BroadcasterLogin string `json:"broadcaster_login"`
	DisplayName     string `json:"display_name"`
	IsLive          bool   `json:"is_live"`
	GameID          string `json:"game_id"`
	GameName        string `json:"game_name"`
	Title           string `json:"title"`
	ThumbnailURL    string `json:"thumbnail_url"`
	StartedAt       string `json:"started_at"`
	Tags            []string `json:"tags"`
}

// Stream mirrors a /helix/streams row — every entry is live by definition (the endpoint only
// returns live broadcasts). ViewerCount lets the UI sort/badge; ThumbnailURL renders the preview.
type Stream struct {
	ID           string   `json:"id"`
	UserID       string   `json:"user_id"`
	UserLogin    string   `json:"user_login"`
	UserName     string   `json:"user_name"`
	GameID       string   `json:"game_id"`
	GameName     string   `json:"game_name"`
	Title        string   `json:"title"`
	ViewerCount  int      `json:"viewer_count"`
	StartedAt    string   `json:"started_at"`
	Language     string   `json:"language"`
	ThumbnailURL string   `json:"thumbnail_url"`
	Tags         []string `json:"tags"`
}

// StreamsPage carries one page of streams plus the cursor for the next page (empty when the API
// reports no further results).
type StreamsPage struct {
	Streams []Stream `json:"streams"`
	Cursor  string   `json:"cursor,omitempty"`
}

// ChannelSearchPage is one page of channel-search results. Helix doesn't paginate search/channels
// with a cursor (it returns up to `first`), but we wrap in a struct so the API shape is stable
// if we later layer client-side or proxied pagination on top.
type ChannelSearchPage struct {
	Results []ChannelSearchResult `json:"results"`
}

// SearchChannels resolves a free-text query to channel listings matching name. Helix returns up
// to `first` results (capped at 100). When liveOnly is true the API filters server-side.
func (c *HelixClient) SearchChannels(ctx context.Context, token, query string, first int, liveOnly bool) (ChannelSearchPage, error) {
	if query == "" {
		return ChannelSearchPage{}, nil
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("first", strconv.Itoa(clampPage(first, 20, 100)))
	if liveOnly {
		q.Set("live_only", "true")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.searchChannelsURL+"?"+q.Encode(), nil)
	if err != nil {
		return ChannelSearchPage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", c.clientID())
	resp, err := c.http.Do(req)
	if err != nil {
		return ChannelSearchPage{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return ChannelSearchPage{}, fmt.Errorf("twitch: search channels: status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data []ChannelSearchResult `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return ChannelSearchPage{}, fmt.Errorf("twitch: decode search-channels response: %w", err)
	}
	return ChannelSearchPage{Results: out.Data}, nil
}

// GetTopStreams returns the currently-live channels in viewer-count order. Pagination uses the
// opaque cursor Helix supplies in the previous response (empty string for the first page).
// gameID optionally narrows to one category; empty means "all games".
func (c *HelixClient) GetTopStreams(ctx context.Context, token string, first int, after, gameID string) (StreamsPage, error) {
	q := url.Values{}
	q.Set("first", strconv.Itoa(clampPage(first, 20, 100)))
	if after != "" {
		q.Set("after", after)
	}
	if gameID != "" {
		q.Set("game_id", gameID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.streamsURL+"?"+q.Encode(), nil)
	if err != nil {
		return StreamsPage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", c.clientID())
	resp, err := c.http.Do(req)
	if err != nil {
		return StreamsPage{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return StreamsPage{}, fmt.Errorf("twitch: top streams: status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data       []Stream `json:"data"`
		Pagination struct {
			Cursor string `json:"cursor"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return StreamsPage{}, fmt.Errorf("twitch: decode top-streams response: %w", err)
	}
	return StreamsPage{Streams: out.Data, Cursor: out.Pagination.Cursor}, nil
}

// SetDiscoveryURLs lets tests redirect both discovery endpoints to a local server.
func (c *HelixClient) SetDiscoveryURLs(searchChannels, streams string) {
	c.searchChannelsURL, c.streamsURL = searchChannels, streams
}

// clampPage holds page size to the Helix-supported range, defaulting non-positive sizes.
func clampPage(n, def, max int) int {
	if n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}
