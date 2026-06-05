package kick

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/platformtest"
)

// fixtureChannel is the channel the adapter would have supplied for these chatroom payloads.
var fixtureChannel = platform.ChannelRef{Platform: platform.Kick, ID: "999", Slug: "xqc", DisplayName: "xQc"}

func normalizeRaw(raw []byte) (platform.UnifiedMessage, error) {
	return normalizeChatMessage(raw, fixtureChannel)
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

func TestNormalize_ChatGolden(t *testing.T) {
	lines := loadFixtureLines(t, "testdata/chat.txt")
	msgs := platformtest.Replay(t, lines, normalizeRaw)
	platformtest.AssertGolden(t, "chat.golden.json", msgs)
}

func TestNormalize_EmoteSegments(t *testing.T) {
	m, err := normalizeChatMessage([]byte(`{"id":"x","content":"a [emote:5:Kappa] b","sender":{"id":1,"username":"U","slug":"u","identity":{}}}`), fixtureChannel)
	if err != nil {
		t.Fatal(err)
	}
	// Expect: "a " (text), Kappa (emote), " b" (text) — concatenating to the original.
	if m.PlainText() != "a Kappa b" {
		t.Errorf("PlainText = %q, want %q", m.PlainText(), "a Kappa b")
	}
	var emote *platform.Segment
	for i := range m.Segments {
		if m.Segments[i].Kind == platform.SegEmote {
			emote = &m.Segments[i]
		}
	}
	if emote == nil {
		t.Fatal("no emote segment produced")
	}
	if emote.Emote == nil || emote.Emote.Provider != platform.EmoteKick || emote.Emote.ID != "5" {
		t.Errorf("emote ref = %+v", emote.Emote)
	}
	if emote.Emote.URLTemplate != "https://files.kick.com/emotes/5/fullsize" {
		t.Errorf("emote url = %q", emote.Emote.URLTemplate)
	}
}

func TestNormalize_BadgesAndAuthor(t *testing.T) {
	m, err := normalizeChatMessage([]byte(`{"id":"x","content":"hi","sender":{"id":42,"username":"Streamer","slug":"streamer","identity":{"color":"#ABCDEF","badges":[{"type":"subscriber","text":"Subscriber","count":7}]}}}`), fixtureChannel)
	if err != nil {
		t.Fatal(err)
	}
	if m.Author.ID != "42" || m.Author.Login != "streamer" || m.Author.DisplayName != "Streamer" || m.Author.Color != "#ABCDEF" {
		t.Errorf("author = %+v", m.Author)
	}
	if len(m.Author.Badges) != 1 {
		t.Fatalf("badges = %+v", m.Author.Badges)
	}
	if b := m.Author.Badges[0]; b.Set != "subscriber" || b.Version != "7" || b.Title != "Subscriber" {
		t.Errorf("badge = %+v, want subscriber/7/Subscriber", b)
	}
}

func TestParseSentTS(t *testing.T) {
	if got := parseSentTS(""); !got.IsZero() {
		t.Errorf("empty → %v, want zero", got)
	}
	if got := parseSentTS("2024-06-05T15:06:44Z"); got.IsZero() || got.Hour() != 15 {
		t.Errorf("RFC3339 → %v", got)
	}
	if got := parseSentTS("1717599600"); got.Unix() != 1717599600 {
		t.Errorf("unix seconds → %v", got)
	}
	if got := parseSentTS("not-a-time"); !got.IsZero() {
		t.Errorf("garbage → %v, want zero", got)
	}
}

func TestNormalize_MalformedEmoteKeptAsText(t *testing.T) {
	// A token without a name (no second colon) must not panic or vanish — it stays literal.
	m, err := normalizeChatMessage([]byte(`{"id":"x","content":"a [emote:broken] b","sender":{"id":1,"username":"U","slug":"u","identity":{}}}`), fixtureChannel)
	if err != nil {
		t.Fatal(err)
	}
	if m.PlainText() != "a [emote:broken] b" {
		t.Errorf("PlainText = %q, want the malformed token kept literally", m.PlainText())
	}
	for _, s := range m.Segments {
		if s.Kind == platform.SegEmote {
			t.Errorf("malformed token became an emote segment: %+v", s)
		}
	}
}
