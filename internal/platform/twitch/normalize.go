package twitch

import (
	"regexp"
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

	segs := buildSegments(text, parseEmotes(m.tags["emotes"]))
	if bits, _ := strconv.Atoi(m.tags["bits"]); bits > 0 {
		segs = splitCheers(segs, bits)
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
		Segments: segs,
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
	// Twitch's authoritative first-message flag (a viewer's first message in this broadcast);
	// the engine's session heuristic covers platforms without such a tag.
	if m.tags["first-msg"] == "1" {
		msg.Annotate().FirstTime = true
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

// cheerToken matches a cheermote: a name immediately followed by a positive bit amount, e.g.
// "Cheer100" or "Kappa50".
var cheerToken = regexp.MustCompile(`^([A-Za-z]+)([1-9][0-9]*)$`)

// splitCheers turns cheermote tokens in the text segments into cheer segments, so the IRC path
// produces the same cheer/CheerBits output the EventSub path does. IRC doesn't mark which words
// are cheermotes, so a token is only treated as a cheer when the amounts across all candidates
// add up to the message's authoritative bits total — otherwise the text is left as-is rather
// than guessed at. The bits<=0 case (no cheer) returns the segments unchanged.
func splitCheers(segs []platform.Segment, bits int) []platform.Segment {
	sum := 0
	for _, s := range segs {
		if s.Kind != platform.SegText {
			continue
		}
		for _, tok := range strings.Fields(s.Text) {
			if m := cheerToken.FindStringSubmatch(tok); m != nil {
				n, _ := strconv.Atoi(m[2])
				sum += n
			}
		}
	}
	if sum != bits {
		return segs
	}
	out := make([]platform.Segment, 0, len(segs))
	for _, s := range segs {
		if s.Kind != platform.SegText {
			out = append(out, s)
			continue
		}
		out = append(out, splitTextCheers(s.Text)...)
	}
	return out
}

// splitTextCheers breaks one text run into alternating text and cheer segments, preserving all
// surrounding whitespace so the concatenation of segment texts still reconstructs the original.
func splitTextCheers(text string) []platform.Segment {
	var out []platform.Segment
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			out = append(out, platform.Segment{Kind: platform.SegText, Text: buf.String()})
			buf.Reset()
		}
	}
	for i := 0; i < len(text); {
		if text[i] == ' ' {
			j := i
			for j < len(text) && text[j] == ' ' {
				j++
			}
			buf.WriteString(text[i:j])
			i = j
			continue
		}
		j := i
		for j < len(text) && text[j] != ' ' {
			j++
		}
		word := text[i:j]
		if m := cheerToken.FindStringSubmatch(word); m != nil {
			n, _ := strconv.Atoi(m[2])
			flush()
			out = append(out, platform.Segment{Kind: platform.SegCheer, Text: word, CheerBits: n})
		} else {
			buf.WriteString(word)
		}
		i = j
	}
	flush()
	return out
}
