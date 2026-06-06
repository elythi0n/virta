package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// helixSendURL is Twitch's send-chat endpoint.
const helixSendURL = "https://api.twitch.tv/helix/chat/messages"

// helixUsersURL is Twitch's users endpoint, used to resolve a login to its numeric id.
const helixUsersURL = "https://api.twitch.tv/helix/users"

// HelixClient sends chat over Twitch's Helix API on behalf of an authenticated account. The
// HTTP client and URL are injectable so the request shaping and drop-reason handling are tested
// offline; live sends are tracked in live-debt.
type HelixClient struct {
	clientID func() string // read on each call so a runtime-configured id takes effect
	http     *http.Client
	sendURL  string
	usersURL string
}

// NewHelixClient builds a Helix client reading its app client id from clientID on each call.
func NewHelixClient(clientID func() string, hc *http.Client) *HelixClient {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &HelixClient{clientID: clientID, http: hc, sendURL: helixSendURL, usersURL: helixUsersURL}
}

// UserID resolves a login to its numeric Twitch user id (needed as the broadcaster id for sends).
func (c *HelixClient) UserID(ctx context.Context, accessToken, login string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.usersURL+"?login="+url.QueryEscape(login), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Client-Id", c.clientID())
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("twitch: users: status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("twitch: decode users response: %w", err)
	}
	if len(out.Data) == 0 || out.Data[0].ID == "" {
		return "", fmt.Errorf("twitch: no user found for login %q", login)
	}
	return out.Data[0].ID, nil
}

// SetUsersURL overrides the users endpoint (tests point it at a local server).
func (c *HelixClient) SetUsersURL(u string) { c.usersURL = u }

// SendChat posts a message as senderID into broadcasterID's chat, optionally as a reply. It
// returns the new message id, or an error — including a dropped message (Twitch accepted the
// request but refused the message, e.g. duplicate or AutoMod), whose reason is surfaced rather
// than swallowed.
func (c *HelixClient) SendChat(ctx context.Context, accessToken, broadcasterID, senderID, message, replyParentID string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"broadcaster_id":          broadcasterID,
		"sender_id":               senderID,
		"message":                 message,
		"reply_parent_message_id": replyParentID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.sendURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Client-Id", c.clientID())
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("twitch: send: status %d: %s", resp.StatusCode, string(raw))
	}

	var out struct {
		Data []struct {
			MessageID  string `json:"message_id"`
			IsSent     bool   `json:"is_sent"`
			DropReason *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"drop_reason"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("twitch: decode send response: %w", err)
	}
	if len(out.Data) == 0 {
		return "", fmt.Errorf("twitch: send: empty response")
	}
	d := out.Data[0]
	if !d.IsSent {
		reason := "rejected"
		if d.DropReason != nil {
			reason = d.DropReason.Code
			if d.DropReason.Message != "" {
				reason = d.DropReason.Message
			}
		}
		return "", fmt.Errorf("twitch: message not sent: %s", reason)
	}
	return d.MessageID, nil
}

// SetSendURL overrides the endpoint (tests point it at a local server).
func (c *HelixClient) SetSendURL(u string) { c.sendURL = u }
