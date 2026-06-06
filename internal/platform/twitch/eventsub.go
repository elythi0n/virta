package twitch

import (
	"encoding/json"
	"time"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/segment"
)

// EventSub is Twitch's authenticated chat transport. Its events arrive pre-parsed (message
// fragments, structured badges), which is why it's the preferred path once signed in — Twitch
// did the parsing. This file turns EventSub WebSocket frames into the same UnifiedMessage /
// Event model the IRC path produces, so downstream code is transport-agnostic.

// EventSub WebSocket message types (metadata.message_type).
const (
	esWelcome      = "session_welcome"
	esKeepalive    = "session_keepalive"
	esNotification = "notification"
	esReconnect    = "session_reconnect"
	esRevocation   = "revocation"
)

// EventSub subscription types we handle (metadata.subscription_type).
const (
	subChatMessage  = "channel.chat.message"
	subChatNotif    = "channel.chat.notification"
	subChatClear    = "channel.chat.clear"
	subChatDelete   = "channel.chat.message_delete"
	subChatSettings = "channel.chat_settings.update"
)

// esEnvelope is the outer EventSub WS frame.
type esEnvelope struct {
	Metadata struct {
		MessageType      string `json:"message_type"`
		SubscriptionType string `json:"subscription_type"`
		MessageTimestamp string `json:"message_timestamp"` // RFC3339; the message's send time
	} `json:"metadata"`
	Payload json.RawMessage `json:"payload"`
}

// parseESTimestamp parses an EventSub metadata timestamp, returning the zero time if absent or
// malformed (the engine still stamps ReceivedAt regardless).
func parseESTimestamp(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// esSession is the welcome/reconnect payload's session object.
type esSession struct {
	ID                   string `json:"id"`
	KeepaliveTimeoutSecs int    `json:"keepalive_timeout_seconds"`
	ReconnectURL         string `json:"reconnect_url"`
	Status               string `json:"status"`
}

func parseEnvelope(b []byte) (esEnvelope, error) {
	var e esEnvelope
	err := json.Unmarshal(b, &e)
	return e, err
}

// sessionFromPayload extracts the session object from a welcome/reconnect payload.
func sessionFromPayload(payload json.RawMessage) (esSession, error) {
	var p struct {
		Session esSession `json:"session"`
	}
	err := json.Unmarshal(payload, &p)
	return p.Session, err
}

// eventFromNotification maps a notification's event payload to a platform Event by subscription
// type. sentAt is the frame's timestamp, applied to chat messages. Returns ok=false for types we
// don't surface.
func eventFromNotification(subType string, payload json.RawMessage, sentAt time.Time) (platform.Event, bool, error) {
	var p struct {
		Event json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, false, err
	}
	switch subType {
	case subChatMessage:
		m, err := normalizeEventSubMessage(p.Event)
		m.SentAt = sentAt
		return platform.MessageEvent{Message: m}, err == nil, err
	case subChatNotif:
		m, err := normalizeEventSubNotification(p.Event)
		m.SentAt = sentAt
		return platform.MessageEvent{Message: m}, err == nil, err
	case subChatDelete:
		return deletedFromEvent(p.Event)
	case subChatClear:
		return clearFromEvent(p.Event)
	case subChatSettings:
		return settingsFromEvent(p.Event)
	default:
		return nil, false, nil
	}
}

// esMessageBody is the shared message object (chat + notification carry the same shape).
type esMessageBody struct {
	Text      string       `json:"text"`
	Fragments []esFragment `json:"fragments"`
}

type esFragment struct {
	Type  string `json:"type"` // text | emote | mention | cheermote
	Text  string `json:"text"`
	Emote *struct {
		ID string `json:"id"`
	} `json:"emote,omitempty"`
	Cheermote *struct {
		Bits int `json:"bits"`
	} `json:"cheermote,omitempty"`
}

type esBadge struct {
	SetID string `json:"set_id"`
	ID    string `json:"id"`
	Info  string `json:"info"`
}

// esChatMessage is the channel.chat.message event.
type esChatMessage struct {
	BroadcasterLogin string        `json:"broadcaster_user_login"`
	BroadcasterName  string        `json:"broadcaster_user_name"`
	ChatterID        string        `json:"chatter_user_id"`
	ChatterLogin     string        `json:"chatter_user_login"`
	ChatterName      string        `json:"chatter_user_name"`
	MessageID        string        `json:"message_id"`
	Message          esMessageBody `json:"message"`
	Color            string        `json:"color"`
	Badges           []esBadge     `json:"badges"`
	MessageType      string        `json:"message_type"`
	Reply            *struct {
		ParentMessageID   string `json:"parent_message_id"`
		ParentUserLogin   string `json:"parent_user_login"`
		ParentMessageBody string `json:"parent_message_body"`
	} `json:"reply,omitempty"`
}

// normalizeEventSubMessage converts a channel.chat.message event into a UnifiedMessage, using
// the same segment/emote building blocks as the IRC path so the two transports produce
// equivalent output.
func normalizeEventSubMessage(raw json.RawMessage) (platform.UnifiedMessage, error) {
	var e esChatMessage
	if err := json.Unmarshal(raw, &e); err != nil {
		return platform.UnifiedMessage{}, err
	}
	msg := platform.UnifiedMessage{
		PlatformMessageID: e.MessageID,
		Platform:          platform.Twitch,
		Channel:           platform.ChannelRef{Platform: platform.Twitch, Slug: e.BroadcasterLogin, DisplayName: e.BroadcasterName},
		Type:              chatType(e.MessageType),
		Author: platform.Author{
			ID:          e.ChatterID,
			Login:       e.ChatterLogin,
			DisplayName: e.ChatterName,
			Color:       e.Color,
			Badges:      badgesFromEventSub(e.Badges),
		},
		Segments: segmentsFromFragments(e.Message.Fragments),
	}
	if e.Reply != nil {
		msg.ReplyTo = &platform.MessageRef{
			PlatformMessageID: e.Reply.ParentMessageID,
			AuthorLogin:       e.Reply.ParentUserLogin,
			TextSnippet:       e.Reply.ParentMessageBody,
		}
	}
	return msg, nil
}

// chatType maps EventSub's message_type to ours (a /me action vs normal chat).
func chatType(t string) platform.MessageType {
	if t == "action" {
		return platform.TypeAction
	}
	return platform.TypeChat
}

// segmentsFromFragments builds segments from EventSub's pre-split fragments. Text fragments
// still pass through the shared splitter (to find links the same way IRC does); emote/mention/
// cheer fragments map directly — yielding output equivalent to buildSegments on the IRC path.
func segmentsFromFragments(frags []esFragment) []platform.Segment {
	var segs []platform.Segment
	for _, f := range frags {
		switch f.Type {
		case "emote":
			id := ""
			if f.Emote != nil {
				id = f.Emote.ID
			}
			segs = append(segs, platform.Segment{
				Kind:  platform.SegEmote,
				Text:  f.Text,
				Emote: &platform.EmoteRef{Provider: platform.EmoteTwitch, ID: id, Name: f.Text, URLTemplate: emoteURL(id)},
			})
		case "mention":
			segs = append(segs, platform.Segment{Kind: platform.SegMention, Text: f.Text})
		case "cheermote":
			bits := 0
			if f.Cheermote != nil {
				bits = f.Cheermote.Bits
			}
			segs = append(segs, platform.Segment{Kind: platform.SegCheer, Text: f.Text, CheerBits: bits})
		default: // "text"
			segs = append(segs, segment.Text(f.Text)...)
		}
	}
	return segs
}

// badgesFromEventSub maps structured EventSub badges to the unified model (set_id → set, id →
// version), matching what the IRC `badges` tag parser produces.
func badgesFromEventSub(badges []esBadge) []platform.Badge {
	if len(badges) == 0 {
		return nil
	}
	out := make([]platform.Badge, 0, len(badges))
	for _, b := range badges {
		out = append(out, platform.Badge{Set: b.SetID, Version: b.ID})
	}
	return out
}

// esNoticeTypes maps channel.chat.notification notice types to message types.
var esNoticeTypes = map[string]platform.MessageType{
	"sub":                platform.TypeSub,
	"resub":              platform.TypeResub,
	"sub_gift":           platform.TypeGiftSub,
	"community_sub_gift": platform.TypeGiftSub,
	"raid":               platform.TypeRaid,
	"announcement":       platform.TypeAnnouncement,
}

// normalizeEventSubNotification converts a channel.chat.notification (sub/raid/etc.) into a
// UnifiedMessage, showing the user's message fragments (the system text is in system_message).
func normalizeEventSubNotification(raw json.RawMessage) (platform.UnifiedMessage, error) {
	var e struct {
		BroadcasterLogin string        `json:"broadcaster_user_login"`
		ChatterID        string        `json:"chatter_user_id"`
		ChatterLogin     string        `json:"chatter_user_login"`
		ChatterName      string        `json:"chatter_user_name"`
		Color            string        `json:"color"`
		Badges           []esBadge     `json:"badges"`
		NoticeType       string        `json:"notice_type"`
		SystemMessage    string        `json:"system_message"`
		Message          esMessageBody `json:"message"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return platform.UnifiedMessage{}, err
	}
	typ, ok := esNoticeTypes[e.NoticeType]
	if !ok {
		typ = platform.TypeSystem
	}
	segs := segmentsFromFragments(e.Message.Fragments)
	if len(segs) == 0 {
		segs = segment.Text(e.SystemMessage)
	}
	return platform.UnifiedMessage{
		Platform: platform.Twitch,
		Channel:  platform.ChannelRef{Platform: platform.Twitch, Slug: e.BroadcasterLogin},
		Type:     typ,
		Author: platform.Author{
			ID: e.ChatterID, Login: e.ChatterLogin, DisplayName: e.ChatterName,
			Color: e.Color, Badges: badgesFromEventSub(e.Badges),
		},
		Segments: segs,
	}, nil
}

func deletedFromEvent(raw json.RawMessage) (platform.Event, bool, error) {
	var e struct {
		BroadcasterLogin string `json:"broadcaster_user_login"`
		MessageID        string `json:"message_id"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, false, err
	}
	return platform.MessageDeletedEvent{
		Channel:           platform.ChannelRef{Platform: platform.Twitch, Slug: e.BroadcasterLogin},
		PlatformMessageID: e.MessageID,
	}, true, nil
}

func clearFromEvent(raw json.RawMessage) (platform.Event, bool, error) {
	var e struct {
		BroadcasterLogin string `json:"broadcaster_user_login"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, false, err
	}
	return platform.ChannelClearEvent{
		Channel: platform.ChannelRef{Platform: platform.Twitch, Slug: e.BroadcasterLogin},
	}, true, nil
}

func settingsFromEvent(raw json.RawMessage) (platform.Event, bool, error) {
	var e struct {
		BroadcasterLogin         string `json:"broadcaster_user_login"`
		EmoteMode                bool   `json:"emote_mode"`
		FollowerMode             bool   `json:"follower_mode"`
		FollowerModeDurationMins *int   `json:"follower_mode_duration_minutes"`
		SlowMode                 bool   `json:"slow_mode"`
		SlowModeWaitTimeSecs     *int   `json:"slow_mode_wait_time_seconds"`
		SubscriberMode           bool   `json:"subscriber_mode"`
		UniqueChatMode           bool   `json:"unique_chat_mode"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, false, err
	}
	s := platform.ChatSettings{
		EmoteOnly:            e.EmoteMode,
		SubsOnly:             e.SubscriberMode,
		UniqueChat:           e.UniqueChatMode,
		FollowersOnlyMinutes: -1,
	}
	if e.FollowerMode {
		if e.FollowerModeDurationMins != nil {
			s.FollowersOnlyMinutes = *e.FollowerModeDurationMins
		} else {
			s.FollowersOnlyMinutes = 0
		}
	}
	if e.SlowMode && e.SlowModeWaitTimeSecs != nil {
		s.SlowSeconds = *e.SlowModeWaitTimeSecs
	}
	return platform.ChatSettingsEvent{
		Channel:  platform.ChannelRef{Platform: platform.Twitch, Slug: e.BroadcasterLogin},
		Settings: s,
	}, true, nil
}
