package twitch

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/platformtest"
)

// normalizeRaw is the Normalizer used for the fixture suite: parse a raw IRC line and, for a
// PRIVMSG, produce its UnifiedMessage.
func normalizeRaw(raw []byte) (platform.UnifiedMessage, error) {
	m, ok := parseLine(string(raw))
	if !ok {
		return platform.UnifiedMessage{}, errors.New("unparseable line")
	}
	if m.command != "PRIVMSG" {
		return platform.UnifiedMessage{}, errors.New("not a PRIVMSG: " + m.command)
	}
	return normalizePrivmsg(m), nil
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
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, []byte(line))
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	return lines
}

// The fixture suite pins normalization of recorded PRIVMSG lines against golden output.
func TestNormalize_PrivmsgGolden(t *testing.T) {
	lines := loadFixtureLines(t, "testdata/privmsg.txt")
	msgs := platformtest.Replay(t, lines, normalizeRaw)
	platformtest.AssertGolden(t, "privmsg.golden.json", msgs)
}

func TestNormalize_ActionStripsWrapper(t *testing.T) {
	m, _ := parseLine("@display-name=Mod :mod!m@m PRIVMSG #c :" + actionPrefix + "waves" + actionSuffix)
	um := normalizePrivmsg(m)
	if um.Type != platform.TypeAction {
		t.Errorf("type = %q, want action", um.Type)
	}
	if um.PlainText() != "waves" {
		t.Errorf("text = %q, want 'waves' (CTCP wrapper stripped)", um.PlainText())
	}
}

func TestNormalize_EmoteRuneIndexing(t *testing.T) {
	// "héy Kappa ok": 'é' is one rune; the Kappa emote at runes 4-8 must land on "Kappa",
	// which only holds if indexing is by rune, not byte.
	m, _ := parseLine("@emotes=25:4-8 :u!u@u PRIVMSG #c :héy Kappa ok")
	um := normalizePrivmsg(m)
	var emote *platform.Segment
	for i := range um.Segments {
		if um.Segments[i].Kind == platform.SegEmote {
			emote = &um.Segments[i]
		}
	}
	if emote == nil || emote.Text != "Kappa" {
		t.Fatalf("emote segment = %+v, want text 'Kappa'", emote)
	}
	if emote.Emote == nil || emote.Emote.Provider != platform.EmoteTwitch || emote.Emote.ID != "25" {
		t.Errorf("emote ref = %+v", emote.Emote)
	}
}

func TestNormalize_Reply(t *testing.T) {
	m, _ := parseLine(`@reply-parent-msg-id=p1;reply-parent-user-login=alice;reply-parent-msg-body=hi\sthere :b!b@b PRIVMSG #c :yes`)
	um := normalizePrivmsg(m)
	if um.ReplyTo == nil || um.ReplyTo.PlatformMessageID != "p1" || um.ReplyTo.AuthorLogin != "alice" || um.ReplyTo.TextSnippet != "hi there" {
		t.Errorf("replyTo = %+v", um.ReplyTo)
	}
}

func TestParseEmotes_MultiRange(t *testing.T) {
	spans := parseEmotes("25:0-4,12-16/1902:6-10")
	if len(spans) != 3 {
		t.Fatalf("spans = %d, want 3", len(spans))
	}
	// sorted by start: 0,6,12
	if spans[0].start != 0 || spans[1].start != 6 || spans[2].start != 12 {
		t.Errorf("starts = %d,%d,%d", spans[0].start, spans[1].start, spans[2].start)
	}
	if spans[0].id != "25" || spans[1].id != "1902" {
		t.Errorf("ids = %s,%s", spans[0].id, spans[1].id)
	}
}

func TestParseBadges(t *testing.T) {
	b := parseBadges("broadcaster/1,subscriber/12")
	if len(b) != 2 || b[0].Set != "broadcaster" || b[0].Version != "1" || b[1].Set != "subscriber" || b[1].Version != "12" {
		t.Errorf("badges = %+v", b)
	}
	if parseBadges("") != nil {
		t.Error("empty badges should be nil")
	}
}
