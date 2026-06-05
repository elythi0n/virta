package twitch

import (
	"strconv"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/segment"
)

// eventFromLine turns a parsed IRC line into the platform event it represents, or false for
// lines that aren't surfaced as events (PING is handled separately by the adapter because it
// requires a reply). This is the single place command → event mapping lives.
func eventFromLine(m ircMessage) (platform.Event, bool) {
	switch m.command {
	case "PRIVMSG":
		return platform.MessageEvent{Message: normalizePrivmsg(m)}, true
	case "USERNOTICE":
		return platform.MessageEvent{Message: normalizeUsernotice(m)}, true
	case "CLEARMSG":
		return platform.MessageDeletedEvent{
			Channel:           chanRef(m),
			PlatformMessageID: m.tags["target-msg-id"],
		}, true
	case "CLEARCHAT":
		return platform.ChannelClearEvent{
			Channel:      chanRef(m),
			TargetUserID: clearTarget(m),
		}, true
	case "ROOMSTATE":
		return platform.ChatSettingsEvent{
			Channel:  chanRef(m),
			Settings: parseRoomState(m.tags),
		}, true
	default:
		return nil, false
	}
}

func chanRef(m ircMessage) platform.ChannelRef {
	return platform.ChannelRef{Platform: platform.Twitch, Slug: channelSlug(m)}
}

// usernoticeTypes maps Twitch's USERNOTICE msg-id to our message type.
var usernoticeTypes = map[string]platform.MessageType{
	"sub":            platform.TypeSub,
	"resub":          platform.TypeResub,
	"subgift":        platform.TypeGiftSub,
	"anonsubgift":    platform.TypeGiftSub,
	"submysterygift": platform.TypeGiftSub,
	"raid":           platform.TypeRaid,
	"announcement":   platform.TypeAnnouncement,
}

// normalizeUsernotice converts a USERNOTICE (sub, resub, gift, raid, announcement, …) into a
// UnifiedMessage. The visible text is the user's own message when they wrote one (e.g. a
// resub or announcement message, which can contain emotes), otherwise Twitch's descriptive
// system message. The author is read from the login/display-name tags (the IRC prefix is the
// server, not the user).
func normalizeUsernotice(m ircMessage) platform.UnifiedMessage {
	msgType, ok := usernoticeTypes[m.tags["msg-id"]]
	if !ok {
		msgType = platform.TypeSystem
	}

	var segs []platform.Segment
	if user := m.trailing(); user != "" {
		segs = buildSegments(user, parseEmotes(m.tags["emotes"]))
	} else {
		segs = segment.Text(m.tags["system-msg"])
	}

	login := m.tags["login"]
	display := m.tags["display-name"]
	if display == "" {
		display = login
	}
	return platform.UnifiedMessage{
		PlatformMessageID: m.tags["id"],
		Platform:          platform.Twitch,
		Channel:           chanRef(m),
		Type:              msgType,
		Author: platform.Author{
			ID:          m.tags["user-id"],
			Login:       login,
			DisplayName: display,
			Color:       m.tags["color"],
			Badges:      parseBadges(m.tags["badges"]),
		},
		Segments: segs,
		SentAt:   parseSentTS(m.tags["tmi-sent-ts"]),
	}
}

// clearTarget returns the user a CLEARCHAT applies to (by id when available, else login), or
// "" for a whole-channel clear.
func clearTarget(m ircMessage) string {
	if id := m.tags["target-user-id"]; id != "" {
		return id
	}
	return m.trailing() // the banned/timed-out user's login, empty for a full clear
}

// parseRoomState reads ROOMSTATE tags into ChatSettings. Twitch may send only the changed
// tags; absent tags fall back to "off" (followers-only off is -1).
func parseRoomState(tags map[string]string) platform.ChatSettings {
	s := platform.ChatSettings{FollowersOnlyMinutes: -1}
	if v, ok := tags["emote-only"]; ok {
		s.EmoteOnly = v == "1"
	}
	if v, ok := tags["subs-only"]; ok {
		s.SubsOnly = v == "1"
	}
	if v, ok := tags["r9k"]; ok {
		s.UniqueChat = v == "1"
	}
	if v, ok := tags["followers-only"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			s.FollowersOnlyMinutes = n
		}
	}
	if v, ok := tags["slow"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			s.SlowSeconds = n
		}
	}
	return s
}
