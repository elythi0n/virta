package api

import (
	"context"
	"log/slog"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

func TestLogRing_WrapsAndOrders(t *testing.T) {
	r := newLogRing(3)
	for i := range 5 {
		r.add(LogLine{Message: string(rune('a' + i))})
	}
	got := r.snapshot()
	if len(got) != 3 {
		t.Fatalf("snapshot len = %d, want 3 (capacity)", len(got))
	}
	// Oldest-first, keeping the 3 most recent (c, d, e).
	if got[0].Message != "c" || got[2].Message != "e" {
		t.Errorf("snapshot order = %q..%q, want c..e", got[0].Message, got[2].Message)
	}
}

func TestLogRing_PartialSnapshot(t *testing.T) {
	r := newLogRing(10)
	r.add(LogLine{Message: "x"})
	r.add(LogLine{Message: "y"})
	got := r.snapshot()
	if len(got) != 2 || got[0].Message != "x" || got[1].Message != "y" {
		t.Errorf("partial snapshot = %+v", got)
	}
}

func TestRingHandler_CapturesThroughSlog(t *testing.T) {
	ring := newLogRing(50)
	log := slog.New(newRingHandler(slog.DiscardHandler, ring))
	if !log.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("handler reports Info disabled")
	}
	log.Info("hello", "k", "v")
	log.With("scope", "test").WithGroup("g").Warn("careful")

	got := ring.snapshot()
	if len(got) != 2 {
		t.Fatalf("captured %d records, want 2", len(got))
	}
	if got[0].Message != "hello" || got[0].Attrs["k"] != "v" {
		t.Errorf("first record = %+v", got[0])
	}
	if got[1].Level != slog.LevelWarn.String() {
		t.Errorf("second level = %q", got[1].Level)
	}
}

func TestToWire_AllEventKinds(t *testing.T) {
	ch := platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}
	cases := []struct {
		name              string
		ev                platform.Event
		wantType, wantKey string
		wantAll           bool
	}{
		{"message", platform.MessageEvent{Message: platform.UnifiedMessage{ID: "m1", Channel: ch}}, "message", "twitch:forsen", false},
		{"deleted", platform.MessageDeletedEvent{Channel: ch, PlatformMessageID: "p1"}, "message_deleted", "twitch:forsen", false},
		{"clear", platform.ChannelClearEvent{Channel: ch}, "channel_clear", "twitch:forsen", false},
		{"channel health", platform.HealthEvent{Channel: &ch, Status: platform.HealthStatus{State: platform.HealthOK}}, "state", "twitch:forsen", false},
		{"adapter health", platform.HealthEvent{Status: platform.HealthStatus{State: platform.HealthDegraded}}, "state", "", true},
		{"chat settings", platform.ChatSettingsEvent{Channel: ch, Settings: platform.ChatSettings{SlowSeconds: 30}}, "chat_settings", "twitch:forsen", false},
		{"stats", platform.StatsEvent{Channel: ch, Stats: platform.StatsSnapshot{MessagesPerSec: 1.5, UniqueChatters: 4}}, "stats", "twitch:forsen", false},
		{"profile changed", platform.ProfileChangedEvent{ProfileID: "p1", Name: "main"}, "profile_changed", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			we, key, all := toWire(c.ev)
			if we.Type != c.wantType || key != c.wantKey || all != c.wantAll {
				t.Errorf("toWire = (%q,%q,%v), want (%q,%q,%v)", we.Type, key, all, c.wantType, c.wantKey, c.wantAll)
			}
		})
	}
}

func TestHubName(t *testing.T) {
	if newHub().Name() != "wsclients" {
		t.Error("hub name")
	}
}
