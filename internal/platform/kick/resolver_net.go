package kick

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// chromeUA is sent on resolver requests so the lookups look like a normal browser.
const chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// httpFetcher is a generic GET-and-extract chatroom fetcher. The HTTP client is injected so
// the direct lookup can be given a Chrome-fingerprinting (uTLS) client — stock net/http is
// fingerprinted by Cloudflare and 403s on the v2 endpoint, which is exactly the block the
// resolver's fallback chain and circuit breaker handle.
type httpFetcher struct {
	client  *http.Client
	url     func(slug string) string
	extract func([]byte) (string, error)
}

func (f *httpFetcher) Fetch(ctx context.Context, slug string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url(slug), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", chromeUA)
	req.Header.Set("Accept", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return "", err
		}
		return f.extract(body)
	case http.StatusForbidden, http.StatusServiceUnavailable, http.StatusTooManyRequests:
		return "", errBlocked
	case http.StatusNotFound:
		return "", errNotFound
	default:
		return "", fmt.Errorf("kick resolve: unexpected status %d", resp.StatusCode)
	}
}

func defaultClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// NewDirectFetcher resolves via the undocumented kick.com/api/v2/channels/{slug} endpoint.
// Give it a uTLS-backed client in production; behind Cloudflare a stock client will be blocked
// (errBlocked), which the resolver handles by falling back and tripping its breaker.
func NewDirectFetcher(client *http.Client) ChatroomFetcher {
	return &httpFetcher{
		client:  defaultClient(client),
		url:     func(slug string) string { return "https://kick.com/api/v2/channels/" + slug },
		extract: extractChatroomID,
	}
}

// NewOfficialFetcher resolves via the official api.kick.com public channels endpoint, used as
// the fallback when the direct lookup is blocked.
func NewOfficialFetcher(client *http.Client) ChatroomFetcher {
	return &httpFetcher{
		client:  defaultClient(client),
		url:     func(slug string) string { return "https://api.kick.com/public/v1/channels?slug=" + slug },
		extract: extractOfficialChatroomID,
	}
}

// extractChatroomID pulls chatroom.id from the v2 channel payload.
func extractChatroomID(body []byte) (string, error) {
	var v struct {
		Chatroom struct {
			ID json.Number `json:"id"`
		} `json:"chatroom"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", err
	}
	if v.Chatroom.ID == "" {
		return "", errNotFound
	}
	return v.Chatroom.ID.String(), nil
}

// extractOfficialChatroomID pulls the chatroom id from the official API's channel list shape.
// The exact field layout is unverified (docs 04 ⚠), so the decode is tolerant: a missing id
// is treated as "not found" rather than an error.
func extractOfficialChatroomID(body []byte) (string, error) {
	var v struct {
		Data []struct {
			ChatroomID json.Number `json:"chatroom_id"`
			Chatroom   struct {
				ID json.Number `json:"id"`
			} `json:"chatroom"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", err
	}
	if len(v.Data) == 0 {
		return "", errNotFound
	}
	if id := v.Data[0].ChatroomID; id != "" {
		return id.String(), nil
	}
	if id := v.Data[0].Chatroom.ID; id != "" {
		return id.String(), nil
	}
	return "", errNotFound
}
