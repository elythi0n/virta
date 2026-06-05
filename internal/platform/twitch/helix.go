package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// helixSendURL is Twitch's send-chat endpoint.
const helixSendURL = "https://api.twitch.tv/helix/chat/messages"

// HelixClient sends chat over Twitch's Helix API on behalf of an authenticated account. The
// HTTP client and URL are injectable so the request shaping and drop-reason handling are tested
// offline; live sends are tracked in live-debt.
type HelixClient struct {
	clientID string
	http     *http.Client
	sendURL  string
}

// NewHelixClient builds a Helix client for the given app client id.
func NewHelixClient(clientID string, hc *http.Client) *HelixClient {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &HelixClient{clientID: clientID, http: hc, sendURL: helixSendURL}
}

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
	req.Header.Set("Client-Id", c.clientID)
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
