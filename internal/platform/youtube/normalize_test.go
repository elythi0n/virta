package youtube

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/platformtest"
)

// fixtureChannel is the channel the worker would have supplied for these payloads.
var fixtureChannel = platform.ChannelRef{Platform: platform.YouTube, ID: "dQw4w9WgXcQ", Slug: "somecreator", DisplayName: "SomeCreator"}

func normalizeRaw(raw []byte) (platform.UnifiedMessage, error) {
	var act chatAction
	if err := json.Unmarshal(raw, &act); err != nil {
		return platform.UnifiedMessage{}, err
	}
	if act.AddChatItemAction == nil {
		return platform.UnifiedMessage{}, fmt.Errorf("fixture line is not an addChatItemAction")
	}
	msg, ok := normalizeItem(act.AddChatItemAction.Item, fixtureChannel)
	if !ok {
		return platform.UnifiedMessage{}, fmt.Errorf("item not surfaced as a message")
	}
	return msg, nil
}

func loadFixtureLines(t *testing.T, path string) [][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()
	var lines [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, []byte(line))
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	return lines
}

func TestNormalize_ChatActionsGolden(t *testing.T) {
	lines := loadFixtureLines(t, "testdata/chat_actions.txt")
	msgs := platformtest.Replay(t, lines, normalizeRaw)
	platformtest.AssertGolden(t, "chat_actions.golden.json", msgs)
}

func TestNormalize_TextMessageSegmentsAndBadges(t *testing.T) {
	lines := loadFixtureLines(t, "testdata/chat_actions.txt")
	m, err := normalizeRaw(lines[0])
	if err != nil {
		t.Fatal(err)
	}
	if m.Type != platform.TypeChat {
		t.Errorf("type = %q, want chat", m.Type)
	}
	if want := "hello @streamer check :hand-wave: and 😀 https://example.com/clip"; m.PlainText() != want {
		t.Errorf("PlainText = %q, want %q", m.PlainText(), want)
	}
	if m.PlatformMessageID != "msg-text-1" || m.Author.ID != "UCfan12345" || m.Author.DisplayName != "StreamFan" || m.Author.Login != "streamfan" {
		t.Errorf("envelope/author = %+v %+v", m.PlatformMessageID, m.Author)
	}
	if m.SentAt.UnixMicro() != 1717599600000000 {
		t.Errorf("SentAt = %v, want the timestampUsec instant", m.SentAt)
	}

	var mention, link bool
	var emote *platform.Segment
	for i := range m.Segments {
		switch m.Segments[i].Kind {
		case platform.SegMention:
			mention = m.Segments[i].Text == "@streamer"
		case platform.SegLink:
			link = m.Segments[i].Text == "https://example.com/clip"
		case platform.SegEmote:
			emote = &m.Segments[i]
		}
	}
	if !mention || !link {
		t.Errorf("mention/link segments missing: %+v", m.Segments)
	}
	if emote == nil || emote.Emote == nil {
		t.Fatalf("no emote segment: %+v", m.Segments)
	}
	if emote.Emote.Provider != platform.EmoteYouTube || emote.Emote.ID != "UCabc123/wave-emoji-id" || emote.Emote.Name != ":hand-wave:" {
		t.Errorf("emote ref = %+v", emote.Emote)
	}
	if emote.Emote.URLTemplate != "https://yt3.ggpht.com/wave=w48-h48" {
		t.Errorf("emote artwork = %q, want the largest inline thumbnail", emote.Emote.URLTemplate)
	}

	if len(m.Author.Badges) != 2 {
		t.Fatalf("badges = %+v", m.Author.Badges)
	}
	if b := m.Author.Badges[0]; b.Set != "moderator" || b.Title != "Moderator" {
		t.Errorf("badge[0] = %+v, want moderator", b)
	}
	if b := m.Author.Badges[1]; b.Set != "subscriber" || b.URL != "https://yt3.ggpht.com/member-badge=s32" {
		t.Errorf("badge[1] = %+v, want subscriber with the largest custom thumbnail", b)
	}
}

func TestNormalize_OwnerAndVerifiedBadges(t *testing.T) {
	lines := loadFixtureLines(t, "testdata/chat_actions.txt")
	m, err := normalizeRaw(lines[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Author.Badges) != 2 || m.Author.Badges[0].Set != "broadcaster" || m.Author.Badges[1].Set != "verified" {
		t.Errorf("badges = %+v, want broadcaster+verified", m.Author.Badges)
	}
}

func TestNormalize_PaidMessage(t *testing.T) {
	lines := loadFixtureLines(t, "testdata/chat_actions.txt")
	m, err := normalizeRaw(lines[2])
	if err != nil {
		t.Fatal(err)
	}
	if m.Type != platform.TypeAnnouncement {
		t.Errorf("super chat type = %q, want announcement", m.Type)
	}
	if want := "Super Chat €5.00 — great stream!"; m.PlainText() != want {
		t.Errorf("PlainText = %q, want %q", m.PlainText(), want)
	}

	// A super chat without a message keeps just the amount header.
	m2, err := normalizeRaw(lines[3])
	if err != nil {
		t.Fatal(err)
	}
	if want := "Super Chat $2.00"; m2.PlainText() != want {
		t.Errorf("PlainText = %q, want %q", m2.PlainText(), want)
	}
}

func TestNormalize_Membership(t *testing.T) {
	lines := loadFixtureLines(t, "testdata/chat_actions.txt")
	join, err := normalizeRaw(lines[4])
	if err != nil {
		t.Fatal(err)
	}
	if join.Type != platform.TypeSub || join.PlainText() != "New member" {
		t.Errorf("membership join = %q %q, want sub/'New member'", join.Type, join.PlainText())
	}

	milestone, err := normalizeRaw(lines[5])
	if err != nil {
		t.Fatal(err)
	}
	if milestone.Type != platform.TypeResub {
		t.Errorf("milestone type = %q, want resub", milestone.Type)
	}
	if want := "Member for 6 months — still here!"; milestone.PlainText() != want {
		t.Errorf("milestone PlainText = %q, want %q", milestone.PlainText(), want)
	}
}

func TestNormalize_DeletionActions(t *testing.T) {
	var del chatAction
	if err := json.Unmarshal([]byte(`{"markChatItemAsDeletedAction":{"targetItemId":"msg-text-1","deletedStateMessage":{"runs":[{"text":"[message deleted]"}]}}}`), &del); err != nil {
		t.Fatal(err)
	}
	evs := eventsFromAction(del, fixtureChannel)
	if len(evs) != 1 {
		t.Fatalf("events = %+v", evs)
	}
	de, ok := evs[0].(platform.MessageDeletedEvent)
	if !ok || de.PlatformMessageID != "msg-text-1" || de.Channel.Slug != "somecreator" {
		t.Errorf("deletion event = %+v", evs[0])
	}

	var wipe chatAction
	if err := json.Unmarshal([]byte(`{"markChatItemsByAuthorAsDeletedAction":{"externalChannelId":"UCbanned123"}}`), &wipe); err != nil {
		t.Fatal(err)
	}
	evs = eventsFromAction(wipe, fixtureChannel)
	if len(evs) != 1 {
		t.Fatalf("events = %+v", evs)
	}
	ce, ok := evs[0].(platform.ChannelClearEvent)
	if !ok || ce.TargetUserID != "UCbanned123" {
		t.Errorf("author wipe event = %+v", evs[0])
	}
}

func TestNormalize_UnknownRendererSkipped(t *testing.T) {
	var act chatAction
	if err := json.Unmarshal([]byte(`{"addChatItemAction":{"item":{"liveChatViewerEngagementMessageRenderer":{"id":"x"}}}}`), &act); err != nil {
		t.Fatal(err)
	}
	if evs := eventsFromAction(act, fixtureChannel); len(evs) != 0 {
		t.Errorf("unknown renderer produced events: %+v", evs)
	}
}

func TestParseUsec(t *testing.T) {
	if got := parseUsec(""); !got.IsZero() {
		t.Errorf("empty → %v, want zero", got)
	}
	if got := parseUsec("1717599600000000"); got.Unix() != 1717599600 {
		t.Errorf("usec → %v", got)
	}
	if got := parseUsec("garbage"); !got.IsZero() {
		t.Errorf("garbage → %v, want zero", got)
	}
}
