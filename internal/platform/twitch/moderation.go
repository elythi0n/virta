package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// modQuery builds the broadcaster_id + moderator_id pair every Helix moderation call carries: the
// channel being moderated and the account performing the action.
func modQuery(broadcasterID, moderatorID string) url.Values {
	v := url.Values{}
	v.Set("broadcaster_id", broadcasterID)
	v.Set("moderator_id", moderatorID)
	return v
}

// do performs an authenticated Helix request with an optional JSON body, treating any non-2xx as
// an error and surfacing the response body. It is the shared shape behind every moderation call.
func (c *HelixClient) do(ctx context.Context, token, method, fullURL, label string, body any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", c.clientID())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("twitch: %s: status %d: %s", label, resp.StatusCode, string(raw))
	}
	return nil
}

// Ban permanently bans (durationSeconds == 0) or times out (durationSeconds > 0) targetUserID in
// broadcasterID's chat, acting as moderatorID. reason is optional.
func (c *HelixClient) Ban(ctx context.Context, token, broadcasterID, moderatorID, targetUserID string, durationSeconds int, reason string) error {
	data := map[string]any{"user_id": targetUserID}
	if durationSeconds > 0 {
		data["duration"] = durationSeconds
	}
	if reason != "" {
		data["reason"] = reason
	}
	body := map[string]any{"data": data}
	return c.do(ctx, token, http.MethodPost, c.bansURL+"?"+modQuery(broadcasterID, moderatorID).Encode(), "ban", body)
}

// Unban lifts a ban or timeout on targetUserID.
func (c *HelixClient) Unban(ctx context.Context, token, broadcasterID, moderatorID, targetUserID string) error {
	q := modQuery(broadcasterID, moderatorID)
	q.Set("user_id", targetUserID)
	return c.do(ctx, token, http.MethodDelete, c.bansURL+"?"+q.Encode(), "unban", nil)
}

// DeleteMessage removes a single message by its platform id.
func (c *HelixClient) DeleteMessage(ctx context.Context, token, broadcasterID, moderatorID, messageID string) error {
	q := modQuery(broadcasterID, moderatorID)
	q.Set("message_id", messageID)
	return c.do(ctx, token, http.MethodDelete, c.chatModURL+"?"+q.Encode(), "delete message", nil)
}

// ClearChat removes every message in the channel.
func (c *HelixClient) ClearChat(ctx context.Context, token, broadcasterID, moderatorID string) error {
	return c.do(ctx, token, http.MethodDelete, c.chatModURL+"?"+modQuery(broadcasterID, moderatorID).Encode(), "clear chat", nil)
}

// UpdateChatSettings patches the channel's chat-mode settings (slow/followers/emote/unique). Only
// the fields in patch are changed; Twitch leaves the rest untouched.
func (c *HelixClient) UpdateChatSettings(ctx context.Context, token, broadcasterID, moderatorID string, patch map[string]any) error {
	return c.do(ctx, token, http.MethodPatch, c.chatSettingsURL+"?"+modQuery(broadcasterID, moderatorID).Encode(), "chat settings", patch)
}

// ManageHeldMessage approves (allow) or denies a message AutoMod is holding. moderatorID is the
// acting account; msgID is the held message's id.
func (c *HelixClient) ManageHeldMessage(ctx context.Context, token, moderatorID, msgID string, allow bool) error {
	action := "DENY"
	if allow {
		action = "ALLOW"
	}
	body := map[string]string{"user_id": moderatorID, "msg_id": msgID, "action": action}
	return c.do(ctx, token, http.MethodPost, c.automodURL, "automod", body)
}

// SetModerationURLs overrides the moderation endpoints (tests point them at a local server).
func (c *HelixClient) SetModerationURLs(bans, chatMod, chatSettings, automod string) {
	c.bansURL, c.chatModURL, c.chatSettingsURL, c.automodURL = bans, chatMod, chatSettings, automod
}
