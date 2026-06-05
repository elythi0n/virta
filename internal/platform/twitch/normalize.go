package twitch

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/segment"
)

// emoteURLTemplate is Twitch's emote CDN URL with a {size} placeholder the frontend fills in
// (1.0, 2.0, or 3.0). The "%s" is the emote id.
const emoteURLTemplate = "https://static-cdn.jtvnw.net/emoticons/v2/%s/default/dark/{size}"

// actionPrefix and actionSuffix wrap a "/me" message in the IRC CTCP ACTION convention.
const (
	actionPrefix = "\x01ACTION "
	actionSuffix = "\x01"
)

// normalizePrivmsg converts a parsed PRIVMSG into a UnifiedMessage. ReceivedAt is left zero
// for the engine to stamp on arrival; SentAt comes from Twitch's tmi-sent-ts tag.
func normalizePrivmsg(m ircMessage) platform.UnifiedMessage {
	text := m.trailing()
	isAction := false
	if strings.HasPrefix(text, actionPrefix) && strings.HasSuffix(text, actionSuffix) && len(text) >= len(actionPrefix)+len(actionSuffix) {
		text = text[len(actionPrefix) : len(text)-len(actionSuffix)]
		isAction = true
	}

	login := m.nick()
	display := m.tags["display-name"]
	if display == "" {
		display = login
	}

	msg := platform.UnifiedMessage{
		PlatformMessageID: m.tags["id"],
		Platform:          platform.Twitch,
		Channel:           platform.ChannelRef{Platform: platform.Twitch, Slug: channelSlug(m)},
		Type:              platform.TypeChat,
		Author: platform.Author{
			ID:          m.tags["user-id"],
			Login:       login,
			DisplayName: display,
			Color:       m.tags["color"],
			Badges:      parseBadges(m.tags["badges"]),
		},
		Segments: buildSegments(text, parseEmotes(m.tags["emotes"])),
		SentAt:   parseSentTS(m.tags["tmi-sent-ts"]),
	}
	if isAction {
		msg.Type = platform.TypeAction
	}
	if id := m.tags["reply-parent-msg-id"]; id != "" {
		msg.ReplyTo = &platform.MessageRef{
			PlatformMessageID: id,
			AuthorLogin:       m.tags["reply-parent-user-login"],
			TextSnippet:       m.tags["reply-parent-msg-body"],
		}
	}
	return msg
}

// channelSlug extracts the channel name from the first parameter ("#forsen" → "forsen").
func channelSlug(m ircMessage) string {
	if len(m.params) == 0 {
		return ""
	}
	return strings.TrimPrefix(m.params[0], "#")
}

func parseSentTS(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	ms, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

// parseBadges turns the badges tag ("broadcaster/1,subscriber/12") into Badge values.
func parseBadges(v string) []platform.Badge {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	badges := make([]platform.Badge, 0, len(parts))
	for _, p := range parts {
		set, version, _ := strings.Cut(p, "/")
		if set == "" {
			continue
		}
		badges = append(badges, platform.Badge{Set: set, Version: version})
	}
	return badges
}

// emoteSpan is one emote occurrence located by inclusive rune indices into the message text.
type emoteSpan struct {
	id    string
	start int
	end   int // inclusive
}

// parseEmotes parses the emotes tag ("25:0-4,12-16/1902:6-10") into spans sorted by start.
func parseEmotes(v string) []emoteSpan {
	if v == "" {
		return nil
	}
	var spans []emoteSpan
	for _, group := range strings.Split(v, "/") {
		id, ranges, ok := strings.Cut(group, ":")
		if !ok {
			continue
		}
		for _, r := range strings.Split(ranges, ",") {
			lo, hi, ok := strings.Cut(r, "-")
			if !ok {
				continue
			}
			start, err1 := strconv.Atoi(lo)
			end, err2 := strconv.Atoi(hi)
			if err1 != nil || err2 != nil {
				continue
			}
			spans = append(spans, emoteSpan{id: id, start: start, end: end})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	return spans
}

// buildSegments splits text into ordered segments: emote ranges become emote segments, and
// the plain runs between them are segmented into text/mention/link. Emote indices are rune
// offsets (Twitch counts code points, not bytes), so the text is indexed as runes.
func buildSegments(text string, emotes []emoteSpan) []platform.Segment {
	if len(emotes) == 0 {
		return segment.Text(text)
	}
	runes := []rune(text)
	var segs []platform.Segment
	cursor := 0
	for _, sp := range emotes {
		if sp.start < 0 || sp.end >= len(runes) || sp.start > sp.end || sp.start < cursor {
			continue // out-of-range or overlapping span; skip defensively
		}
		if sp.start > cursor {
			segs = append(segs, segment.Text(string(runes[cursor:sp.start]))...)
		}
		name := string(runes[sp.start : sp.end+1])
		segs = append(segs, platform.Segment{
			Kind: platform.SegEmote,
			Text: name,
			Emote: &platform.EmoteRef{
				Provider:    platform.EmoteTwitch,
				ID:          sp.id,
				Name:        name,
				URLTemplate: emoteURL(sp.id),
			},
		})
		cursor = sp.end + 1
	}
	if cursor < len(runes) {
		segs = append(segs, segment.Text(string(runes[cursor:]))...)
	}
	return segs
}

func emoteURL(id string) string {
	return strings.Replace(emoteURLTemplate, "%s", id, 1)
}
