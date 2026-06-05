package platform_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/platformtest"
)

// toyNormalize parses a deliberately tiny "slug|user|text" payload into a UnifiedMessage. It
// stands in for a real adapter's normalizer so the replay + golden harness can be exercised
// before any real adapter exists; real adapters provide their own Normalizer of the same shape.
func toyNormalize(raw []byte) (platform.UnifiedMessage, error) {
	parts := strings.SplitN(string(raw), "|", 3)
	if len(parts) != 3 {
		return platform.UnifiedMessage{}, errors.New("want slug|user|text")
	}
	return platform.UnifiedMessage{
		PlatformMessageID: parts[0] + "-" + parts[1],
		Platform:          platform.Twitch,
		Channel:           platform.ChannelRef{Platform: platform.Twitch, Slug: parts[0]},
		Type:              platform.TypeChat,
		Author:            platform.Author{Login: parts[1], DisplayName: parts[1]},
		Segments:          []platform.Segment{{Kind: platform.SegText, Text: parts[2]}},
	}, nil
}

func TestReplay_NormalizesAndMatchesGolden(t *testing.T) {
	raw := [][]byte{
		[]byte("forsen|alice|hello world"),
		[]byte("forsen|bob|PogChamp"),
		[]byte("xqc|carol|gg"),
	}
	msgs := platformtest.Replay(t, raw, toyNormalize)
	if len(msgs) != 3 {
		t.Fatalf("replayed %d messages, want 3", len(msgs))
	}
	platformtest.AssertGolden(t, "toy_replay.json", msgs)
}

func TestReplay_FailsLoudlyOnBadPayload(t *testing.T) {
	// A normalizer that rejects everything must surface as a test failure, not a silent skip.
	// Replay reports via Fatalf (which calls Goexit), so run it in its own goroutine and
	// observe the recorded failure once that goroutine unwinds.
	bad := func([]byte) (platform.UnifiedMessage, error) { return platform.UnifiedMessage{}, errors.New("nope") }
	failed := make(chan bool, 1)
	go func() {
		fakeT := &testing.T{}
		defer func() { failed <- fakeT.Failed() }()
		platformtest.Replay(fakeT, [][]byte{[]byte("x")}, bad)
	}()
	if !<-failed {
		t.Error("Replay did not fail on a rejecting normalizer")
	}
}
