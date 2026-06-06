package kick

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// apiBaseURL is Kick's official public API.
const apiBaseURL = "https://api.kick.com/public/v1"

// maxTimeout is the longest timeout Kick's ban endpoint accepts (7 days, expressed in minutes).
const maxTimeout = 10080 * time.Minute

// RateLimitError reports a 429 from the API. RetryAfter is the server's suggested wait
// (zero when the response carried no hint). Kick does not document its limits, so the rate
// governor reacts to these adaptively rather than assuming a fixed budget.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("kick: rate limited, retry after %s", e.RetryAfter)
	}
	return "kick: rate limited"
}

// RateLimited exposes the suggested wait; the rate governor detects this method structurally
// (no import in either direction) to tighten the channel's budget.
func (e *RateLimitError) RateLimited() time.Duration { return e.RetryAfter }

// APIClient calls Kick's official API on behalf of an authenticated account: sending chat and
// moderation. The HTTP client and base URL are injectable so request shaping and error
// surfacing are tested offline; live behavior is tracked separately.
type APIClient struct {
	http *http.Client
	base string
}

// NewAPIClient builds a client over the official API.
func NewAPIClient(hc *http.Client) *APIClient {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &APIClient{http: hc, base: apiBaseURL}
}

// SetBaseURL overrides the API base (tests point it at a local server).
func (c *APIClient) SetBaseURL(u string) { c.base = u }

// SendChat posts a message into broadcasterUserID's chat as the authenticated user, optionally
// as a reply. It returns the new message id, or an error — including a message the API
// accepted but did not send, which is surfaced rather than swallowed.
func (c *APIClient) SendChat(ctx context.Context, accessToken, broadcasterUserID, content, replyToMessageID string) (string, error) {
	bid, err := numericID(broadcasterUserID)
	if err != nil {
		return "", fmt.Errorf("kick: send: %w", err)
	}
	body := map[string]any{
		"type":                "user",
		"broadcaster_user_id": bid,
		"content":             content,
	}
	if replyToMessageID != "" {
		body["reply_to_message_id"] = replyToMessageID
	}
	raw, err := c.do(ctx, http.MethodPost, c.base+"/chat", accessToken, body)
	if err != nil {
		return "", err
	}

	// The id and confirmation may sit at the top level or under "data"; accept both.
	var out struct {
		IsSent    bool   `json:"is_sent"`
		MessageID string `json:"message_id"`
		Data      struct {
			IsSent    bool   `json:"is_sent"`
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("kick: decode send response: %w", err)
	}
	sent, id := out.IsSent, out.MessageID
	if out.Data.MessageID != "" || out.Data.IsSent {
		sent, id = out.Data.IsSent, out.Data.MessageID
	}
	if !sent {
		return "", errors.New("kick: message not sent")
	}
	return id, nil
}

// BroadcasterID resolves a channel slug to its numeric broadcaster user id, which send and
// moderation require. The public API's exact channel shape is unverified, so decoding is
// tolerant; live behavior is tracked separately.
func (c *APIClient) BroadcasterID(ctx context.Context, accessToken, slug string) (string, error) {
	raw, err := c.do(ctx, http.MethodGet, c.base+"/channels?slug="+url.QueryEscape(slug), accessToken, nil)
	if err != nil {
		return "", err
	}
	var body struct {
		Data []struct {
			BroadcasterUserID json.Number `json:"broadcaster_user_id"`
			UserID            json.Number `json:"user_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", fmt.Errorf("kick: decode channel: %w", err)
	}
	if len(body.Data) == 0 {
		return "", fmt.Errorf("kick: no channel for slug %q", slug)
	}
	id := body.Data[0].BroadcasterUserID.String()
	if id == "" || id == "0" {
		id = body.Data[0].UserID.String()
	}
	if id == "" || id == "0" {
		return "", fmt.Errorf("kick: no broadcaster id for slug %q", slug)
	}
	return id, nil
}

// DeleteMessage removes a chat message by its platform id.
func (c *APIClient) DeleteMessage(ctx context.Context, accessToken, messageID string) error {
	_, err := c.do(ctx, http.MethodDelete, c.base+"/chat/"+messageID, accessToken, nil)
	return err
}

// Ban bans (duration zero) or times out (duration > 0) userID in broadcasterUserID's channel.
// Kick takes timeouts in whole minutes up to 7 days; shorter durations round up to a minute,
// longer ones are refused rather than silently shortened.
func (c *APIClient) Ban(ctx context.Context, accessToken, broadcasterUserID, userID string, duration time.Duration, reason string) error {
	bid, err := numericID(broadcasterUserID)
	if err != nil {
		return fmt.Errorf("kick: ban: %w", err)
	}
	uid, err := numericID(userID)
	if err != nil {
		return fmt.Errorf("kick: ban: %w", err)
	}
	body := map[string]any{
		"broadcaster_user_id": bid,
		"user_id":             uid,
	}
	if duration > 0 {
		if duration > maxTimeout {
			return fmt.Errorf("kick: timeout longer than the 7-day maximum: %s", duration)
		}
		mins := int((duration + time.Minute - 1) / time.Minute)
		body["duration"] = mins
	}
	if reason != "" {
		body["reason"] = reason
	}
	_, err = c.do(ctx, http.MethodPost, c.base+"/moderation/bans", accessToken, body)
	return err
}

// Unban lifts a ban or timeout on userID in broadcasterUserID's channel.
func (c *APIClient) Unban(ctx context.Context, accessToken, broadcasterUserID, userID string) error {
	bid, err := numericID(broadcasterUserID)
	if err != nil {
		return fmt.Errorf("kick: unban: %w", err)
	}
	uid, err := numericID(userID)
	if err != nil {
		return fmt.Errorf("kick: unban: %w", err)
	}
	body := map[string]any{
		"broadcaster_user_id": bid,
		"user_id":             uid,
	}
	_, err = c.do(ctx, http.MethodDelete, c.base+"/moderation/bans", accessToken, body)
	return err
}

// Moderate executes a typed moderation action against the official API. Kick has no
// chat-settings endpoints (slow mode and the like), so only bans, timeouts, and message
// deletion are supported; anything else reports unsupported rather than guessing.
func (c *APIClient) Moderate(ctx context.Context, accessToken, broadcasterUserID string, action platform.ModAction) error {
	switch action.Type {
	case platform.ModBan:
		return c.Ban(ctx, accessToken, broadcasterUserID, action.TargetUserID, 0, action.Reason)
	case platform.ModTimeout:
		d := action.Duration
		if d <= 0 {
			d = time.Minute
		}
		return c.Ban(ctx, accessToken, broadcasterUserID, action.TargetUserID, d, action.Reason)
	case platform.ModUnban, platform.ModUntimeout:
		return c.Unban(ctx, accessToken, broadcasterUserID, action.TargetUserID)
	case platform.ModDeleteMessage:
		return c.DeleteMessage(ctx, accessToken, action.TargetMessageID)
	default:
		return platform.ErrUnsupported
	}
}

// do issues one authenticated JSON request and returns the response body. Non-2xx statuses
// become errors; a 429 becomes a RateLimitError carrying any Retry-After hint.
func (c *APIClient) do(ctx context.Context, method, url, accessToken string, body map[string]any) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusTooManyRequests {
		var wait time.Duration
		if s := resp.Header.Get("Retry-After"); s != "" {
			if secs, err := strconv.Atoi(s); err == nil && secs > 0 {
				wait = time.Duration(secs) * time.Second
			}
		}
		return nil, &RateLimitError{RetryAfter: wait}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("kick: %s %s: status %d: %s", method, url, resp.StatusCode, string(raw))
	}
	return raw, nil
}

// numericID parses a platform user id, which Kick's API takes as a JSON number.
func numericID(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("user id %q is not a positive number", s)
	}
	return n, nil
}
