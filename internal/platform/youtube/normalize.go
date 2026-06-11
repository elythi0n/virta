package youtube

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/segment"
)

// ---- get_live_chat wire shapes (only the fields we read) ----

// liveChatResponse is one get_live_chat poll. A nil ContinuationContents means the broadcast
// ended.
type liveChatResponse struct {
	ContinuationContents *struct {
		LiveChatContinuation struct {
			Continuations []continuationWrapper `json:"continuations"`
			Actions       []chatAction          `json:"actions"`
		} `json:"liveChatContinuation"`
	} `json:"continuationContents"`
}

// continuationWrapper holds whichever continuation variant the server chose; all carry the same
// {continuation, timeoutMs} payload.
type continuationWrapper struct {
	TimedContinuationData        *continuationData `json:"timedContinuationData"`
	InvalidationContinuationData *continuationData `json:"invalidationContinuationData"`
	ReloadContinuationData       *continuationData `json:"reloadContinuationData"`
}

type continuationData struct {
	Continuation string `json:"continuation"`
	TimeoutMs    int    `json:"timeoutMs"`
}

// data returns the populated variant, or nil.
func (w continuationWrapper) data() *continuationData {
	switch {
	case w.TimedContinuationData != nil:
		return w.TimedContinuationData
	case w.InvalidationContinuationData != nil:
		return w.InvalidationContinuationData
	case w.ReloadContinuationData != nil:
		return w.ReloadContinuationData
	default:
		return nil
	}
}

// chatAction is one entry of liveChatContinuation.actions.
type chatAction struct {
	AddChatItemAction *struct {
		Item chatItem `json:"item"`
	} `json:"addChatItemAction"`
	MarkChatItemAsDeletedAction *struct {
		TargetItemID string `json:"targetItemId"`
	} `json:"markChatItemAsDeletedAction"`
	MarkChatItemsByAuthorAsDeletedAction *struct {
		ExternalChannelID string `json:"externalChannelId"`
	} `json:"markChatItemsByAuthorAsDeletedAction"`
}

// chatItem is the renderer union inside addChatItemAction; exactly one is set for the kinds we
// surface (everything else — tickers, placeholders, view-mode changes — decodes to all-nil and
// is skipped).
type chatItem struct {
	Text       *itemRenderer `json:"liveChatTextMessageRenderer"`
	Paid       *itemRenderer `json:"liveChatPaidMessageRenderer"`
	Membership *itemRenderer `json:"liveChatMembershipItemRenderer"`
}

// itemRenderer is the superset of the text/paid/membership renderer fields we read; absent
// fields decode to zero values.
type itemRenderer struct {
	ID                      string        `json:"id"`
	TimestampUsec           string        `json:"timestampUsec"`
	AuthorName              simpleText    `json:"authorName"`
	AuthorExternalChannelID string        `json:"authorExternalChannelId"`
	AuthorBadges            []authorBadge `json:"authorBadges"`
	Message                 *runsText     `json:"message"`
	HeaderSubtext           *runsText     `json:"headerSubtext"`      // membership: "New member" / "Member for N months"
	PurchaseAmountText      simpleText    `json:"purchaseAmountText"` // paid: "€5.00"
}

type simpleText struct {
	SimpleText string `json:"simpleText"`
}

type runsText struct {
	Runs []run `json:"runs"`
}

// run is one piece of a message: plain text or an emoji with inline artwork.
type run struct {
	Text  string    `json:"text"`
	Emoji *emojiRun `json:"emoji"`
}

type emojiRun struct {
	EmojiID       string     `json:"emojiId"`
	Shortcuts     []string   `json:"shortcuts"`
	Image         thumbnails `json:"image"`
	IsCustomEmoji bool       `json:"isCustomEmoji"`
}

type authorBadge struct {
	LiveChatAuthorBadgeRenderer struct {
		Tooltip string `json:"tooltip"`
		Icon    *struct {
			IconType string `json:"iconType"` // OWNER | MODERATOR | VERIFIED
		} `json:"icon"`
		CustomThumbnail *thumbnails `json:"customThumbnail"` // member badges carry artwork instead of an icon
	} `json:"liveChatAuthorBadgeRenderer"`
}

type thumbnails struct {
	Thumbnails []thumbnail `json:"thumbnails"`
}

type thumbnail struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// largest returns the URL of the biggest thumbnail (the list is usually ordered small→large,
// but pick by area to be safe). "" when there is none.
func (t thumbnails) largest() string {
	best, bestArea := "", -1
	for _, th := range t.Thumbnails {
		if area := th.Width * th.Height; area > bestArea && th.URL != "" {
			best, bestArea = th.URL, area
		}
	}
	return best
}

// ---- normalization ----

// eventsFromAction turns one chat action into the platform events it represents (usually zero
// or one). The channel is supplied by the worker; YouTube payloads don't repeat it.
func eventsFromAction(act chatAction, ch platform.ChannelRef) []platform.Event {
	switch {
	case act.AddChatItemAction != nil:
		if msg, ok := normalizeItem(act.AddChatItemAction.Item, ch); ok {
			return []platform.Event{platform.MessageEvent{Message: msg}}
		}
	case act.MarkChatItemAsDeletedAction != nil:
		return []platform.Event{platform.MessageDeletedEvent{
			Channel:           ch,
			PlatformMessageID: act.MarkChatItemAsDeletedAction.TargetItemID,
		}}
	case act.MarkChatItemsByAuthorAsDeletedAction != nil:
		// A per-author wipe (ban/timeout) maps to the per-user chat clear, like Twitch CLEARCHAT.
		return []platform.Event{platform.ChannelClearEvent{
			Channel:      ch,
			TargetUserID: act.MarkChatItemsByAuthorAsDeletedAction.ExternalChannelID,
		}}
	}
	return nil
}

// normalizeItem converts an added chat item into a UnifiedMessage. ok=false for renderer kinds
// we don't surface.
func normalizeItem(item chatItem, ch platform.ChannelRef) (platform.UnifiedMessage, bool) {
	switch {
	case item.Text != nil:
		r := item.Text
		return unified(r, ch, platform.TypeChat, runsSegments(r.Message)), true
	case item.Paid != nil:
		return normalizePaid(item.Paid, ch), true
	case item.Membership != nil:
		return normalizeMembership(item.Membership, ch), true
	default:
		return platform.UnifiedMessage{}, false
	}
}

// normalizePaid maps a Super Chat to an announcement (the closest unified type: a highlighted
// message with platform-decorated framing), with the amount leading the body so the magnitude
// survives into PlainText for logging/TTS — the same way Twitch sub/raid magnitudes ride in the
// visible text rather than a dedicated field.
func normalizePaid(r *itemRenderer, ch platform.ChannelRef) platform.UnifiedMessage {
	header := "Super Chat " + r.PurchaseAmountText.SimpleText
	body := runsSegments(r.Message)
	segs := make([]platform.Segment, 0, len(body)+1)
	if len(body) > 0 {
		segs = append(segs, platform.Segment{Kind: platform.SegText, Text: header + " — "})
		segs = append(segs, body...)
	} else {
		segs = append(segs, platform.Segment{Kind: platform.SegText, Text: header})
	}
	return unified(r, ch, platform.TypeAnnouncement, segs)
}

// monthsPattern finds a month count in membership header text ("Member for 6 months").
var monthsPattern = regexp.MustCompile(`(?i)(\d+)\s+month`)

// normalizeMembership maps a membership item to the sub family: a join ("New member") becomes
// a sub, a milestone ("Member for N months") a resub — the visible text keeps the months, the
// Twitch convention for magnitudes.
func normalizeMembership(r *itemRenderer, ch platform.ChannelRef) platform.UnifiedMessage {
	header := plainRuns(r.HeaderSubtext)
	msgType := platform.TypeSub
	if monthsPattern.MatchString(header) {
		msgType = platform.TypeResub
	}
	segs := runsSegments(r.HeaderSubtext)
	if body := runsSegments(r.Message); len(body) > 0 {
		if len(segs) > 0 {
			segs = append(segs, platform.Segment{Kind: platform.SegText, Text: " — "})
		}
		segs = append(segs, body...)
	}
	return unified(r, ch, msgType, segs)
}

// unified assembles the common envelope around the per-kind segments.
func unified(r *itemRenderer, ch platform.ChannelRef, t platform.MessageType, segs []platform.Segment) platform.UnifiedMessage {
	return platform.UnifiedMessage{
		PlatformMessageID: r.ID,
		Platform:          platform.YouTube,
		Channel:           ch,
		Type:              t,
		Author: platform.Author{
			ID: r.AuthorExternalChannelID,
			// YouTube has no separate login handle; the lowercased display name keeps
			// author-based filters and mention matching case-insensitive like other platforms.
			Login:       strings.ToLower(r.AuthorName.SimpleText),
			DisplayName: r.AuthorName.SimpleText,
			Badges:      parseBadges(r.AuthorBadges),
		},
		Segments: segs,
		SentAt:   parseUsec(r.TimestampUsec),
	}
}

// parseBadges maps YouTube author badges to the unified model: the icon kinds translate to the
// cross-platform sets (OWNER→broadcaster, MODERATOR→moderator, VERIFIED→verified) and a member
// badge — YouTube's subscription equivalent — becomes a subscriber badge carrying its artwork.
func parseBadges(badges []authorBadge) []platform.Badge {
	if len(badges) == 0 {
		return nil
	}
	out := make([]platform.Badge, 0, len(badges))
	for _, b := range badges {
		r := b.LiveChatAuthorBadgeRenderer
		switch {
		case r.Icon != nil:
			set := ""
			switch r.Icon.IconType {
			case "OWNER":
				set = "broadcaster"
			case "MODERATOR":
				set = "moderator"
			case "VERIFIED":
				set = "verified"
			}
			if set != "" {
				out = append(out, platform.Badge{Set: set, Title: r.Tooltip})
			}
		case r.CustomThumbnail != nil:
			out = append(out, platform.Badge{Set: "subscriber", Title: r.Tooltip, URL: r.CustomThumbnail.largest()})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// runsSegments converts message runs into segments: text runs go through the shared tokenizer
// (mentions/links), custom emoji runs become complete emote segments — YouTube delivers the
// artwork inline, so no separate emote-resolution pass is needed — and standard unicode emoji
// stay as text (every frontend renders unicode natively).
func runsSegments(rt *runsText) []platform.Segment {
	if rt == nil || len(rt.Runs) == 0 {
		return nil
	}
	var segs []platform.Segment
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			segs = append(segs, segment.Text(buf.String())...)
			buf.Reset()
		}
	}
	for _, r := range rt.Runs {
		switch {
		case r.Emoji == nil:
			buf.WriteString(r.Text)
		case r.Emoji.IsCustomEmoji && r.Emoji.Image.largest() != "":
			flush()
			name := emojiName(r.Emoji)
			segs = append(segs, platform.Segment{
				Kind: platform.SegEmote,
				Text: name,
				Emote: &platform.EmoteRef{
					Provider:    platform.EmoteYouTube,
					ID:          r.Emoji.EmojiID,
					Name:        name,
					URLTemplate: r.Emoji.Image.largest(),
				},
			})
		default:
			// Standard emoji: emojiId is the literal unicode character, which every
			// frontend renders natively; fall back to the shortcut when it's absent.
			if r.Emoji.EmojiID != "" {
				buf.WriteString(r.Emoji.EmojiID)
			} else {
				buf.WriteString(emojiName(r.Emoji))
			}
		}
	}
	flush()
	return segs
}

// emojiName picks the human-readable handle for an emoji: the first shortcut (":hand-wave:")
// when present, else the raw emojiId (the unicode char for standard emoji).
func emojiName(e *emojiRun) string {
	if len(e.Shortcuts) > 0 && e.Shortcuts[0] != "" {
		return e.Shortcuts[0]
	}
	return e.EmojiID
}

// plainRuns flattens runs to plain text (emoji as their names) for type-detection heuristics.
func plainRuns(rt *runsText) string {
	if rt == nil {
		return ""
	}
	var b strings.Builder
	for _, r := range rt.Runs {
		if r.Emoji != nil {
			b.WriteString(emojiName(r.Emoji))
		} else {
			b.WriteString(r.Text)
		}
	}
	return b.String()
}

// parseUsec converts timestampUsec (a decimal string of microseconds) to a UTC time; an
// unparseable value yields the zero time rather than an error.
func parseUsec(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	usec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMicro(usec).UTC()
}
