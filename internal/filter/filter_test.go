package filter

import (
	"context"
	"strings"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

// Tests use innocuous placeholder terms ("badword", "höpö") rather than real profanity — the
// matcher doesn't care what the words are, and the repo stays free of slurs.

func msg(slug, login string, mtype platform.MessageType, text string) *platform.UnifiedMessage {
	return &platform.UnifiedMessage{
		Platform: platform.Twitch,
		Channel:  platform.ChannelRef{Platform: platform.Twitch, Slug: slug},
		Type:     mtype,
		Author:   platform.Author{Login: login, DisplayName: login},
		Segments: []platform.Segment{{Kind: platform.SegText, Text: text}},
	}
}

func run(t *testing.T, rules []Rule, m *platform.UnifiedMessage) {
	t.Helper()
	rs, err := Compile(rules)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := NewStage(rs).Annotate(context.Background(), m); err != nil {
		t.Fatalf("annotate: %v", err)
	}
}

func TestHide_ByAuthor(t *testing.T) {
	m := msg("forsen", "nightbot", platform.TypeChat, "!commands")
	run(t, []Rule{{ID: "bots", Action: ActionHide, Match: Match{Authors: []string{"NightBot"}}}}, m)
	if m.Annotations == nil || !m.Annotations.Hidden {
		t.Errorf("message from nightbot not hidden: %+v", m.Annotations)
	}
}

func TestHide_ByTypeScoped(t *testing.T) {
	rules := []Rule{{ID: "nofollows", Action: ActionHide, Match: Match{Types: []platform.MessageType{platform.TypeFollow}}}}
	chat := msg("forsen", "a", platform.TypeChat, "hi")
	run(t, rules, chat)
	if chat.Annotations != nil && chat.Annotations.Hidden {
		t.Error("chat wrongly hidden by a follow rule")
	}
	follow := msg("forsen", "a", platform.TypeFollow, "")
	run(t, rules, follow)
	if follow.Annotations == nil || !follow.Annotations.Hidden {
		t.Error("follow not hidden")
	}
}

func TestHighlight_ByKeyword(t *testing.T) {
	m := msg("forsen", "viewer", platform.TypeChat, "hey @me look here")
	run(t, []Rule{{ID: "mentions", Action: ActionHighlight, Match: Match{Keywords: []string{"@me"}}}}, m)
	if m.Annotations == nil || m.Annotations.Highlight != "mentions" {
		t.Errorf("not highlighted by rule id: %+v", m.Annotations)
	}
}

func TestMask_AndRevealContract(t *testing.T) {
	m := msg("forsen", "viewer", platform.TypeChat, "you absolute BADWORD gamer")
	run(t, []Rule{ProfanityRule("profanity", "badword")}, m)

	if m.Annotations == nil || !m.Annotations.Masked {
		t.Fatal("message not flagged masked")
	}
	// TTS/webhooks/logging path (PlainText) must be masked, not the original word.
	if got := m.PlainText(); got != "you absolute ∗∗∗ gamer" {
		t.Errorf("PlainText = %q, want the term masked", got)
	}
	// The original is preserved only on the masked segment for local click-to-reveal.
	var revealed string
	for _, s := range m.Segments {
		if s.Kind == platform.SegMasked {
			revealed = s.Reveal
		}
	}
	if revealed != "BADWORD" {
		t.Errorf("reveal = %q, want original BADWORD", revealed)
	}
}

func TestMask_UnicodeWholeWord(t *testing.T) {
	// Non-ASCII term, with a superstring nearby that must NOT match (whole-word only).
	m := msg("forsen", "viewer", platform.TypeChat, "voi höpö että höpöttää")
	run(t, []Rule{ProfanityRule("p", "höpö")}, m)
	if got := m.PlainText(); got != "voi ∗∗∗ että höpöttää" {
		t.Errorf("PlainText = %q; 'höpö' should mask, 'höpöttää' (superstring) should not", got)
	}
}

func TestParseList(t *testing.T) {
	terms := ParseList(strings.NewReader("# a comment\nBadWord\n\n  spaced  \n"))
	if len(terms) != 2 || terms[0] != "badword" || terms[1] != "spaced" {
		t.Errorf("ParseList = %v, want [badword spaced] (lowered, trimmed, comments/blanks skipped)", terms)
	}
}

func TestMask_CustomTermViaRegex(t *testing.T) {
	m := msg("forsen", "viewer", platform.TypeChat, "buy followers at scamsite dot com")
	run(t, []Rule{{ID: "links", Action: ActionMask, Match: Match{Regexes: []string{`scam\w+`}}}}, m)
	if got := m.PlainText(); !strings.Contains(got, "∗∗∗") || strings.Contains(got, "scamsite") {
		t.Errorf("PlainText = %q, want scamsite masked", got)
	}
}

func TestMask_LeavesNonTextSegments(t *testing.T) {
	m := &platform.UnifiedMessage{
		Platform: platform.Twitch,
		Channel:  platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"},
		Segments: []platform.Segment{
			{Kind: platform.SegText, Text: "badword "},
			{Kind: platform.SegEmote, Text: "Kappa"},
		},
	}
	run(t, []Rule{ProfanityRule("p", "badword")}, m)
	lastEmote := m.Segments[len(m.Segments)-1]
	if lastEmote.Kind != platform.SegEmote || lastEmote.Text != "Kappa" {
		t.Errorf("emote segment lost: %+v", m.Segments)
	}
}

func TestOrderedEvaluation_HideAndMaskCompose(t *testing.T) {
	m := msg("forsen", "troll", platform.TypeChat, "badword take")
	rules := []Rule{
		ProfanityRule("p", "badword"),
		{ID: "trolls", Action: ActionHide, Match: Match{Authors: []string{"troll"}}},
	}
	run(t, rules, m)
	if m.Annotations == nil || !m.Annotations.Masked || !m.Annotations.Hidden {
		t.Errorf("both mask and hide should apply: %+v", m.Annotations)
	}
}

func TestScope_PlatformConstrainsMask(t *testing.T) {
	rules := []Rule{{ID: "p", Action: ActionMask, Match: Match{
		Platforms: []platform.Platform{platform.Kick},
		Keywords:  []string{"badword"},
	}}}
	m := msg("forsen", "viewer", platform.TypeChat, "badword")
	run(t, rules, m)
	if m.Annotations != nil && m.Annotations.Masked {
		t.Error("twitch message masked by a kick-scoped rule")
	}
}

func TestScope_ChannelAndTypeMisses(t *testing.T) {
	rules := []Rule{{ID: "h", Action: ActionHighlight, Match: Match{
		Channels: []string{"otherchannel"},
		Types:    []platform.MessageType{platform.TypeChat},
		Keywords: []string{"hi"},
	}}}
	m := msg("forsen", "v", platform.TypeChat, "hi there")
	run(t, rules, m)
	if m.Annotations != nil && m.Annotations.Highlight != "" {
		t.Error("highlighted despite a channel mismatch")
	}
}

func TestMask_MergesOverlappingSpans(t *testing.T) {
	m := msg("forsen", "v", platform.TypeChat, "what a badshow")
	run(t, []Rule{{ID: "p", Action: ActionMask, Match: Match{
		Keywords: []string{"badshow"},
		Regexes:  []string{"bad"},
	}}}, m)
	masked := 0
	for _, s := range m.Segments {
		if s.Kind == platform.SegMasked {
			masked++
		}
	}
	if masked != 1 {
		t.Errorf("overlapping spans produced %d masked segments, want 1 merged", masked)
	}
	if got := m.PlainText(); got != "what a ∗∗∗" {
		t.Errorf("PlainText = %q", got)
	}
}

func TestHighlight_ByRegex(t *testing.T) {
	m := msg("forsen", "v", platform.TypeChat, "order #1234 ready")
	run(t, []Rule{{ID: "orders", Action: ActionHighlight, Match: Match{Regexes: []string{`#\d+`}}}}, m)
	if m.Annotations == nil || m.Annotations.Highlight != "orders" {
		t.Errorf("regex highlight did not fire: %+v", m.Annotations)
	}
}

func TestStageName_AndHotSwap(t *testing.T) {
	s := NewStage(nil)
	if s.Name() != "filter" {
		t.Errorf("name = %q", s.Name())
	}
	rs, _ := Compile([]Rule{ProfanityRule("p", "badword")})
	s.SetRuleset(rs)
	m := msg("forsen", "v", platform.TypeChat, "badword")
	_ = s.Annotate(context.Background(), m)
	if m.Annotations == nil || !m.Annotations.Masked {
		t.Error("hot-swapped ruleset not applied")
	}
	s.SetRuleset(nil)
	m2 := msg("forsen", "v", platform.TypeChat, "badword")
	_ = s.Annotate(context.Background(), m2)
	if m2.Annotations != nil {
		t.Error("nil swap should disable masking")
	}
}

func TestCompile_BadRegexErrors(t *testing.T) {
	if _, err := Compile([]Rule{{ID: "x", Action: ActionMask, Match: Match{Regexes: []string{"("}}}}); err == nil {
		t.Error("bad regex compiled without error")
	}
}

func TestEmptyRuleset_Noop(t *testing.T) {
	m := msg("forsen", "viewer", platform.TypeChat, "anything happens")
	if err := NewStage(nil).Annotate(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if m.Annotations != nil {
		t.Errorf("empty ruleset annotated: %+v", m.Annotations)
	}
}

func BenchmarkFilterStage(b *testing.B) {
	rs, _ := Compile([]Rule{
		ProfanityRule("p", "badword", "anotherbad", "höpö"),
		{ID: "hi", Action: ActionHighlight, Match: Match{Keywords: []string{"@me"}}},
		{ID: "bots", Action: ActionHide, Match: Match{Authors: []string{"nightbot"}}},
	})
	stage := NewStage(rs)
	text := "this is a normal twitch chat message with several words and nothing flagged at all here"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := msg("forsen", "viewer", platform.TypeChat, text)
		_ = stage.Annotate(context.Background(), m)
	}
}
