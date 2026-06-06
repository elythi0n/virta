package velocity

import (
	"context"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

const base = int64(1_700_000_000) // a realistic unix second, so the zero-second default never collides

func msg(slug, author string, sec int64) *platform.UnifiedMessage {
	return &platform.UnifiedMessage{
		Platform:   platform.Twitch,
		Channel:    platform.ChannelRef{Platform: platform.Twitch, Slug: slug},
		Type:       platform.TypeChat,
		Author:     platform.Author{Login: author},
		ReceivedAt: time.Unix(base+sec, 0),
	}
}

// run feeds n messages spread across one second and returns how many were marked sampled.
func runSecond(s *Stage, slug string, n int, sec int64, decorate func(int, *platform.UnifiedMessage)) int {
	sampled := 0
	for i := 0; i < n; i++ {
		m := msg(slug, "viewer", sec)
		if decorate != nil {
			decorate(i, m)
		}
		_ = s.Annotate(context.Background(), m)
		if m.Annotations != nil && m.Annotations.Sampled {
			sampled++
		}
	}
	return sampled
}

func TestStage_BelowThresholdNeverSamples(t *testing.T) {
	s := NewStage(25)
	// Warm the window at a calm rate, then a second well under threshold.
	for sec := int64(0); sec < 4; sec++ {
		if got := runSecond(s, "forsen", 10, sec, nil); got != 0 {
			t.Fatalf("sec %d: sampled %d at 10 msg/s (threshold 25)", sec, got)
		}
	}
}

func TestStage_OverloadThinsTowardThreshold(t *testing.T) {
	s := NewStage(25)
	// Warm the rolling window at 100 msg/s for a couple of seconds so the rate estimate is stable.
	runSecond(s, "forsen", 100, 0, nil)
	runSecond(s, "forsen", 100, 1, nil)
	kept := 100 - runSecond(s, "forsen", 100, 2, nil)
	// At ~100 msg/s with a 25 threshold, roughly a quarter survive. Allow slack for window warmup.
	if kept < 15 || kept > 40 {
		t.Errorf("kept %d of 100 at 100 msg/s, want ~25 (15..40)", kept)
	}
}

func TestStage_PriorityNeverSampled(t *testing.T) {
	s := NewStage(25)
	mod := func(_ int, m *platform.UnifiedMessage) {
		m.Author.Badges = []platform.Badge{{Set: "moderator"}}
	}
	for sec := int64(0); sec < 3; sec++ {
		if got := runSecond(s, "forsen", 100, sec, mod); got != 0 {
			t.Fatalf("sec %d: a moderator message was sampled (%d)", sec, got)
		}
	}
}

func TestStage_FirstTimerNeverSampled(t *testing.T) {
	s := NewStage(25)
	first := func(_ int, m *platform.UnifiedMessage) { m.Annotate().FirstTime = true }
	for sec := int64(0); sec < 3; sec++ {
		if got := runSecond(s, "forsen", 100, sec, first); got != 0 {
			t.Fatalf("sec %d: a first-time chatter was sampled (%d)", sec, got)
		}
	}
}

func TestStage_NonChatNeverSampled(t *testing.T) {
	s := NewStage(25)
	for sec := int64(0); sec < 3; sec++ {
		for i := 0; i < 100; i++ {
			m := msg("forsen", "viewer", sec)
			m.Type = platform.TypeSub
			_ = s.Annotate(context.Background(), m)
			if m.Annotations != nil && m.Annotations.Sampled {
				t.Fatalf("a non-chat event was sampled")
			}
		}
	}
}

func TestStage_ThresholdZeroDisables(t *testing.T) {
	s := NewStage(0)
	for sec := int64(0); sec < 3; sec++ {
		if got := runSecond(s, "forsen", 200, sec, nil); got != 0 {
			t.Fatalf("sec %d: sampling occurred with threshold 0 (%d)", sec, got)
		}
	}
}

func TestStage_NeverHidesOrDrops(t *testing.T) {
	s := NewStage(25)
	for sec := int64(0); sec < 3; sec++ {
		for i := 0; i < 100; i++ {
			m := msg("forsen", "viewer", sec)
			if err := s.Annotate(context.Background(), m); err != nil {
				t.Fatalf("Annotate returned error %v (would drop the message)", err)
			}
			if m.Annotations != nil && m.Annotations.Hidden {
				t.Fatalf("velocity set Hidden; it must only set Sampled")
			}
		}
	}
}

func TestStage_PerChannelIndependent(t *testing.T) {
	s := NewStage(25)
	// One channel floods; a calm channel must remain fully unsampled.
	for sec := int64(0); sec < 3; sec++ {
		runSecond(s, "busy", 100, sec, nil)
		if got := runSecond(s, "calm", 5, sec, nil); got != 0 {
			t.Fatalf("sec %d: calm channel sampled (%d) — channels not independent", sec, got)
		}
	}
}

func TestStage_SetThresholdTakesEffect(t *testing.T) {
	s := NewStage(25)
	runSecond(s, "forsen", 60, 0, nil)
	runSecond(s, "forsen", 60, 1, nil)
	before := runSecond(s, "forsen", 60, 2, nil)
	if before == 0 {
		t.Fatal("expected sampling at 60 msg/s with threshold 25")
	}
	s.SetThreshold(0) // disable
	if got := runSecond(s, "forsen", 60, 3, nil); got != 0 {
		t.Errorf("after SetThreshold(0), still sampled %d", got)
	}
}
