package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// helixEventSubURL is the Helix endpoint managing EventSub subscriptions.
const helixEventSubURL = "https://api.twitch.tv/helix/eventsub/subscriptions"

// errESForbidden marks a subscription the token isn't authorized for (e.g. an automod topic on
// a channel where the account isn't a moderator). Callers degrade per-channel instead of
// failing the whole session.
var errESForbidden = errors.New("twitch: eventsub subscription forbidden")

// esSubRequest describes one EventSub subscription to create over the websocket transport.
type esSubRequest struct {
	Type      string
	Version   string
	Condition map[string]string
	SessionID string
}

// CreateEventSubSubscription creates an EventSub subscription bound to a WebSocket session and
// returns its id. A 403 maps to errESForbidden; a 409 (subscription already exists for this
// session) is treated as success with the existing id when Twitch returns it, since the desired
// state already holds.
func (c *HelixClient) CreateEventSubSubscription(ctx context.Context, token string, sub esSubRequest) (string, error) {
	body := map[string]any{
		"type":      sub.Type,
		"version":   sub.Version,
		"condition": sub.Condition,
		"transport": map[string]string{"method": "websocket", "session_id": sub.SessionID},
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.eventSubURL, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", c.clientID())
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch {
	case resp.StatusCode == http.StatusForbidden:
		return "", fmt.Errorf("%w: %s %s: %s", errESForbidden, sub.Type, sub.Condition["broadcaster_user_id"], string(raw))
	case resp.StatusCode == http.StatusConflict:
		// Already subscribed (e.g. a retried create after a dropped response). Twitch's 409 body
		// doesn't include the id; return empty — the caller tracks it as "active, id unknown".
		return "", nil
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return "", fmt.Errorf("twitch: eventsub create %s: status %d: %s", sub.Type, resp.StatusCode, string(raw))
	}

	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Data) == 0 {
		return "", fmt.Errorf("twitch: eventsub create %s: unexpected response: %s", sub.Type, string(raw))
	}
	return out.Data[0].ID, nil
}

// DeleteEventSubSubscription removes a subscription by id. A 404 is success — the desired state
// (subscription gone) already holds, e.g. Twitch dropped it when its WebSocket closed.
func (c *HelixClient) DeleteEventSubSubscription(ctx context.Context, token, id string) error {
	u := c.eventSubURL + "?id=" + url.QueryEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", c.clientID())
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("twitch: eventsub delete: status %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// SetEventSubURL overrides the subscriptions endpoint for tests.
func (c *HelixClient) SetEventSubURL(u string) { c.eventSubURL = u }
