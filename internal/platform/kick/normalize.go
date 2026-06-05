package kick

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/segment"
)

// emoteURLTemplate is Kick's emote CDN path. Widely used, not formally documented — kept as a
// template string so a remote-config update can change it without a code change. Kick serves a
// single "fullsize" image rather than per-size variants, so there is no {size} placeholder.
const emoteURLTemplate = "https://files.kick.com/emotes/%s/fullsize"

func emoteURL(id string) string { return strings.Replace(emoteURLTemplate, "%s", id, 1) }

// chatMessage is the App\Events\ChatMessageEvent payload (the fields we use).
type chatMessage struct {
	ID        string     `json:"id"`
	Content   string     `json:"content"`
	CreatedAt string     `json:"created_at"`
	Sender    chatSender `json:"sender"`
}

type chatSender struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Slug     string `json:"slug"`
	Identity struct {
		Color  string      `json:"color"`
		Badges []kickBadge `json:"badges"`
	} `json:"identity"`
}

type kickBadge struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Count int    `json:"count"`
}

// normalizeChatMessage converts a ChatMessageEvent payload into a UnifiedMessage for channel
// ch (the adapter supplies the channel, since the payload carries only the chatroom id). The
// author comes from sender/identity; emotes are parsed inline from the content text.
func normalizeChatMessage(data []byte, ch platform.ChannelRef) (platform.UnifiedMessage, error) {
	var m chatMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return platform.UnifiedMessage{}, err
	}
	return platform.UnifiedMessage{
		PlatformMessageID: m.ID,
		Platform:          platform.Kick,
		Channel:           ch,
		Type:              platform.TypeChat,
		Author: platform.Author{
			ID:          strconv.FormatInt(m.Sender.ID, 10),
			Login:       m.Sender.Slug,
			DisplayName: m.Sender.Username,
			Color:       m.Sender.Identity.Color,
			Badges:      parseBadges(m.Sender.Identity.Badges),
		},
		Segments: parseContent(m.Content),
		SentAt:   parseSentTS(m.CreatedAt),
	}, nil
}

// parseBadges maps Kick identity badges to the unified badge model. The subscriber badge's
// month count becomes the version (matching the Twitch convention); the human label is kept
// as the title.
func parseBadges(badges []kickBadge) []platform.Badge {
	if len(badges) == 0 {
		return nil
	}
	out := make([]platform.Badge, 0, len(badges))
	for _, b := range badges {
		badge := platform.Badge{Set: b.Type, Title: b.Text}
		if b.Count > 0 {
			badge.Version = strconv.Itoa(b.Count)
		}
		out = append(out, badge)
	}
	return out
}

// parseContent splits Kick message text into segments, turning inline [emote:id:name] tokens
// into emote segments and passing the surrounding text through the shared splitter (which
// preserves exact spacing and finds mentions/links).
func parseContent(content string) []platform.Segment {
	var segs []platform.Segment
	rest := content
	for {
		open := strings.Index(rest, "[emote:")
		if open < 0 {
			return append(segs, segment.Text(rest)...)
		}
		if open > 0 {
			segs = append(segs, segment.Text(rest[:open])...)
		}
		token := rest[open:]
		end := strings.IndexByte(token, ']')
		if end < 0 {
			// Unterminated token: treat the remainder as plain text.
			return append(segs, segment.Text(rest[open:])...)
		}
		inner := token[len("[emote:"):end] // "id:name"
		if colon := strings.IndexByte(inner, ':'); colon >= 0 {
			id, name := inner[:colon], inner[colon+1:]
			segs = append(segs, platform.Segment{
				Kind: platform.SegEmote,
				Text: name,
				Emote: &platform.EmoteRef{
					Provider:    platform.EmoteKick,
					ID:          id,
					Name:        name,
					URLTemplate: emoteURL(id),
				},
			})
		} else {
			// Malformed token (no name): keep it as literal text.
			segs = append(segs, segment.Text(token[:end+1])...)
		}
		rest = rest[open+end+1:]
	}
}

// parseSentTS parses Kick's created_at timestamp, tolerating both RFC3339 and a Unix-seconds
// form; an unparseable value yields the zero time rather than an error.
func parseSentTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	if secs, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(secs, 0).UTC()
	}
	return time.Time{}
}
