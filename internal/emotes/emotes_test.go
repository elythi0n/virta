package emotes

import (
	"context"
	"errors"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

type fakeProvider struct {
	name   string
	emotes []platform.EmoteRef
	err    error
}

func (f fakeProvider) Name() string { return f.name }
func (f fakeProvider) Fetch(context.Context, platform.ChannelRef) ([]platform.EmoteRef, error) {
	return f.emotes, f.err
}

func emote(provider platform.EmoteProvider, name string) platform.EmoteRef {
	return platform.EmoteRef{Provider: provider, ID: name + "-id", Name: name}
}

func textMsg(ch platform.ChannelRef, text string) *platform.UnifiedMessage {
	return &platform.UnifiedMessage{Channel: ch, Segments: []platform.Segment{{Kind: platform.SegText, Text: text}}}
}

var testChannel = platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen", ID: "1"}

func TestStage_ResolvesWordEmotes(t *testing.T) {
	r := NewResolver(fakeProvider{name: "7tv", emotes: []platform.EmoteRef{emote(platform.Emote7TV, "PogU")}})
	r.Refresh(context.Background(), testChannel)
	stage := NewStage(r)

	msg := textMsg(testChannel, "hey PogU nice play")
	if err := stage.Annotate(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	// Spacing is preserved: the pieces still concatenate to the original.
	if msg.PlainText() != "hey PogU nice play" {
		t.Errorf("PlainText = %q, want original text", msg.PlainText())
	}
	var found *platform.Segment
	for i := range msg.Segments {
		if msg.Segments[i].Kind == platform.SegEmote {
			found = &msg.Segments[i]
		}
	}
	if found == nil {
		t.Fatal("PogU was not resolved to an emote segment")
	}
	if found.Text != "PogU" || found.Emote == nil || found.Emote.Provider != platform.Emote7TV {
		t.Errorf("emote segment = %+v", found)
	}
}

func TestStage_LeavesNonTextSegments(t *testing.T) {
	r := NewResolver(fakeProvider{name: "7tv", emotes: []platform.EmoteRef{emote(platform.Emote7TV, "name")}})
	r.Refresh(context.Background(), testChannel)
	stage := NewStage(r)

	// A mention segment whose text happens to match an emote name must not be reclassified.
	msg := &platform.UnifiedMessage{Channel: testChannel, Segments: []platform.Segment{
		{Kind: platform.SegMention, Text: "name"},
	}}
	_ = stage.Annotate(context.Background(), msg)
	if msg.Segments[0].Kind != platform.SegMention {
		t.Errorf("mention was rewritten to %v", msg.Segments[0].Kind)
	}
}

func TestStage_NoSnapshotIsNoop(t *testing.T) {
	r := NewResolver(fakeProvider{name: "7tv", emotes: []platform.EmoteRef{emote(platform.Emote7TV, "PogU")}})
	// No Refresh for this channel → empty snapshot → message unchanged.
	stage := NewStage(r)
	msg := textMsg(testChannel, "hey PogU")
	_ = stage.Annotate(context.Background(), msg)
	if len(msg.Segments) != 1 || msg.Segments[0].Kind != platform.SegText {
		t.Errorf("segments mutated without a snapshot: %+v", msg.Segments)
	}
}

func TestResolver_PrecedenceFirstProviderWins(t *testing.T) {
	// 7TV and BTTV both define "LUL"; 7TV is listed first, so it wins.
	r := NewResolver(
		fakeProvider{name: "7tv", emotes: []platform.EmoteRef{emote(platform.Emote7TV, "LUL")}},
		fakeProvider{name: "bttv", emotes: []platform.EmoteRef{emote(platform.EmoteBTTV, "LUL")}},
	)
	set := r.Refresh(context.Background(), testChannel)
	got, ok := set.Lookup("LUL")
	if !ok || got.Provider != platform.Emote7TV {
		t.Errorf("LUL resolved to %v, want 7tv (higher precedence)", got.Provider)
	}
}

func TestResolver_SkipsFailingProvider(t *testing.T) {
	r := NewResolver(
		fakeProvider{name: "7tv", err: errors.New("down")},
		fakeProvider{name: "bttv", emotes: []platform.EmoteRef{emote(platform.EmoteBTTV, "catKISS")}},
	)
	set := r.Refresh(context.Background(), testChannel)
	if _, ok := set.Lookup("catKISS"); !ok {
		t.Error("a failing provider blanked a working one")
	}
}

func TestResolver_SnapshotNeverNil(t *testing.T) {
	r := NewResolver()
	if s := r.Snapshot("twitch:unknown"); s == nil || s.Len() != 0 {
		t.Errorf("snapshot for unknown channel = %v, want empty non-nil", s)
	}
}

func TestApplyEmotes_PreservesMultipleSpaces(t *testing.T) {
	set := merge([]platform.EmoteRef{emote(platform.Emote7TV, "Kappa")})
	segs := applyEmotes("a  Kappa   b", set)
	var got string
	for _, s := range segs {
		got += s.Text
	}
	if got != "a  Kappa   b" {
		t.Errorf("reassembled = %q, want exact spacing preserved", got)
	}
}
