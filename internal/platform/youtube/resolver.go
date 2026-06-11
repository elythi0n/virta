package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"github.com/elythi0n/virta/internal/platform"
)

// YouTube live chat lives per-broadcast, not per-channel, so reading a channel is a two-step
// resolve that no other platform needs: slug → the channel's current live videoId (scraped from
// the /@slug/live page), then videoId → the chat continuation token (the InnerTube /next call).
// This file owns both steps and the typed errors the adapter's state machine branches on.

// chromeUA is sent on every request so the lookups look like a normal browser.
const chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// innertubeContext is the web client identity InnerTube requires on every call. With
// prettyPrint=false these endpoints need no API key.
const innertubeContext = `{"client":{"clientName":"WEB","clientVersion":"2.20240501.00.00"}}`

// Sentinel causes for a failed resolve. errNotLive is the steady state of an offline channel
// (the worker waits and retries); errNotFound and errNoChat are surfaced to the user.
var (
	errNotLive  = errors.New("youtube: channel has no live broadcast")
	errNotFound = errors.New("youtube: channel not found")
	errNoChat   = errors.New("youtube: live chat unavailable for this broadcast")
)

// ResolveError carries a machine reason code so the UI can explain a failed join in its own
// words without parsing strings.
type ResolveError struct {
	Reason platform.ReasonCode
	Slug   string
	err    error
}

func (e *ResolveError) Error() string {
	return fmt.Sprintf("youtube: resolve %q: %s", e.Slug, e.Reason)
}
func (e *ResolveError) Unwrap() error { return e.err }

// maxPageBytes bounds the /live HTML read; watch pages run 1–2 MB, the markers and videoId sit
// in the embedded player JSON well within this.
const maxPageBytes = 4 << 20

// videoIDPattern matches the canonical 11-character videoId embedded in the page JSON.
var videoIDPattern = regexp.MustCompile(`"videoId":"([A-Za-z0-9_-]{11})"`)

// resolveLive fetches the channel's /live page and extracts the current broadcast's videoId.
// Returns a ResolveError wrapping errNotFound (no such channel) or errNotLive (channel exists
// but is offline) so callers can branch without string matching.
func (a *Adapter) resolveLive(ctx context.Context, slug string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.webBase+"/@"+url.PathEscape(slug)+"/live", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", chromeUA)
	req.Header.Set("Accept-Language", "en")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		// fall through to the body scan
	case http.StatusNotFound:
		return "", &ResolveError{Reason: platform.ReasonChannelNotFound, Slug: slug, err: errNotFound}
	default:
		return "", fmt.Errorf("youtube: live page for %q: unexpected status %d", slug, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes))
	if err != nil {
		return "", err
	}
	// A live channel's page carries explicit live markers in the embedded player JSON; an
	// offline channel's /live page lacks them (it shows the channel home instead).
	if !bytes.Contains(body, []byte(`{"isLive":true}`)) && !bytes.Contains(body, []byte(`"isLiveNow":true`)) {
		return "", &ResolveError{Reason: platform.ReasonNotLive, Slug: slug, err: errNotLive}
	}
	m := videoIDPattern.FindSubmatch(body)
	if m == nil {
		return "", &ResolveError{Reason: platform.ReasonNotLive, Slug: slug, err: errNotLive}
	}
	return string(m[1]), nil
}

// innertube POSTs one InnerTube call and decodes the JSON response into out.
func (a *Adapter) innertube(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", chromeUA)
	req.Header.Set("Origin", "https://www.youtube.com")
	req.Header.Set("Referer", "https://www.youtube.com/")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("youtube: POST %s: unexpected status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// nextResponse is the slice of the InnerTube /next payload that carries the live chat's initial
// continuation token.
type nextResponse struct {
	Contents struct {
		TwoColumnWatchNextResults struct {
			ConversationBar struct {
				LiveChatRenderer *struct {
					Continuations []struct {
						ReloadContinuationData *continuationData `json:"reloadContinuationData"`
					} `json:"continuations"`
				} `json:"liveChatRenderer"`
			} `json:"conversationBar"`
		} `json:"twoColumnWatchNextResults"`
	} `json:"contents"`
}

// fetchContinuation turns a live videoId into the chat's initial continuation token. A missing
// conversationBar/liveChatRenderer means chat is disabled or the broadcast already ended.
func (a *Adapter) fetchContinuation(ctx context.Context, videoID string) (string, error) {
	payload := map[string]any{
		"context": json.RawMessage(innertubeContext),
		"videoId": videoID,
	}
	var doc nextResponse
	if err := a.innertube(ctx, "/youtubei/v1/next?prettyPrint=false", payload, &doc); err != nil {
		return "", err
	}
	lc := doc.Contents.TwoColumnWatchNextResults.ConversationBar.LiveChatRenderer
	if lc == nil {
		return "", errNoChat
	}
	for _, c := range lc.Continuations {
		if c.ReloadContinuationData != nil && c.ReloadContinuationData.Continuation != "" {
			return c.ReloadContinuationData.Continuation, nil
		}
	}
	return "", errNoChat
}

// fetchChat performs one get_live_chat poll. An absent continuationContents means the broadcast
// ended (reported via the response, not an error, so the caller can transition cleanly).
func (a *Adapter) fetchChat(ctx context.Context, continuation string) (*liveChatResponse, error) {
	payload := map[string]any{
		"context":      json.RawMessage(innertubeContext),
		"continuation": continuation,
	}
	var doc liveChatResponse
	if err := a.innertube(ctx, "/youtubei/v1/live_chat/get_live_chat?prettyPrint=false", payload, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}
