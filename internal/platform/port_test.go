package platform_test

import (
	"context"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/platformtest"
)

// The fake must satisfy the universal Adapter contract — the same suite real adapters run.
func TestFakeAdapter_Contract(t *testing.T) {
	platformtest.RunAdapterContract(t, func(t *testing.T) platform.Adapter {
		return platform.NewFakeAdapter(platform.Twitch, platform.Capabilities{
			ReadAnonymous: true,
			Stability:     platform.TierOfficial,
		})
	})
}

func TestFakeAdapter_SendWhenCapable(t *testing.T) {
	a := platform.NewFakeAdapter(platform.Kick, platform.Capabilities{ReadAuthed: true, Send: true})
	t.Cleanup(func() { _ = a.Close() })

	ch := platform.ChannelRef{Platform: platform.Kick, Slug: "xqc"}
	if err := a.Send(context.Background(), ch, "hello", platform.SendOpts{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	sends := a.Sends()
	if len(sends) != 1 || sends[0].Text != "hello" || sends[0].Channel.Slug != "xqc" {
		t.Fatalf("recorded sends = %+v, want one send of 'hello' to xqc", sends)
	}
}

func TestFakeAdapter_EmitFlowsToEvents(t *testing.T) {
	a := platform.NewFakeAdapter(platform.Twitch, platform.Capabilities{ReadAnonymous: true})
	t.Cleanup(func() { _ = a.Close() })

	want := platform.UnifiedMessage{
		Platform: platform.Twitch,
		Channel:  platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"},
		Type:     platform.TypeChat,
		Segments: []platform.Segment{{Kind: platform.SegText, Text: "OMEGALUL"}},
	}
	a.EmitMessage(want)

	select {
	case ev := <-a.Events():
		me, ok := ev.(platform.MessageEvent)
		if !ok {
			t.Fatalf("event type = %T, want MessageEvent", ev)
		}
		if me.Message.Segments[0].Text != "OMEGALUL" {
			t.Errorf("got %q", me.Message.Segments[0].Text)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestFakeAdapter_CapabilitiesAreDynamic(t *testing.T) {
	a := platform.NewFakeAdapter(platform.Kick, platform.Capabilities{ReadAnonymous: true})
	t.Cleanup(func() { _ = a.Close() })
	if a.Capabilities().Send {
		t.Fatal("expected Send=false initially")
	}
	a.SetCapabilities(platform.Capabilities{ReadAuthed: true, Send: true})
	if !a.Capabilities().Send {
		t.Fatal("expected Send=true after sign-in simulation")
	}
}

func TestFakeAdapter_JoinLeaveModerateHealth(t *testing.T) {
	a := platform.NewFakeAdapter(platform.Twitch, platform.Capabilities{
		ReadAuthed: true, Send: true, Moderation: true, Stability: platform.TierOfficial,
	})
	t.Cleanup(func() { _ = a.Close() })
	ctx := context.Background()
	ch := platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}

	// Join records mode; Joined reflects it; Leave removes it.
	if err := a.Join(ctx, ch, platform.ModeAuthenticated); err != nil {
		t.Fatalf("Join: %v", err)
	}
	if mode, ok := a.Joined("forsen"); !ok || mode != platform.ModeAuthenticated {
		t.Errorf("Joined = (%v,%v), want (authenticated,true)", mode, ok)
	}
	if err := a.Leave(ch); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if _, ok := a.Joined("forsen"); ok {
		t.Error("still joined after Leave")
	}

	// Moderate records when capable.
	act := platform.ModAction{Type: platform.ModTimeout, Channel: ch, TargetUserID: "u9", Duration: 10 * time.Second}
	if err := a.Moderate(ctx, act); err != nil {
		t.Fatalf("Moderate: %v", err)
	}
	if mods := a.Mods(); len(mods) != 1 || mods[0].Type != platform.ModTimeout {
		t.Errorf("Mods = %+v", mods)
	}

	// SetHealth updates Health() and emits a HealthEvent.
	a.SetHealth(platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonReconnecting})
	if h := a.Health(); h.State != platform.HealthDegraded || h.Reason != platform.ReasonReconnecting {
		t.Errorf("Health = %+v", h)
	}
	select {
	case ev := <-a.Events():
		he, ok := ev.(platform.HealthEvent)
		if !ok || he.Status.State != platform.HealthDegraded {
			t.Errorf("event = %#v, want degraded HealthEvent", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no HealthEvent emitted")
	}
}

func TestFakeAdapter_DeletionAndClearEvents(t *testing.T) {
	a := platform.NewFakeAdapter(platform.Kick, platform.Capabilities{ReadAnonymous: true})
	t.Cleanup(func() { _ = a.Close() })
	ch := platform.ChannelRef{Platform: platform.Kick, Slug: "xqc"}

	a.Emit(platform.MessageDeletedEvent{Channel: ch, PlatformMessageID: "p1"})
	a.Emit(platform.ChannelClearEvent{Channel: ch, TargetUserID: "u1"})

	got := make([]platform.Event, 0, 2)
	for range 2 {
		select {
		case ev := <-a.Events():
			got = append(got, ev)
		case <-time.After(time.Second):
			t.Fatal("missing event")
		}
	}
	if _, ok := got[0].(platform.MessageDeletedEvent); !ok {
		t.Errorf("first = %T, want MessageDeletedEvent", got[0])
	}
	if _, ok := got[1].(platform.ChannelClearEvent); !ok {
		t.Errorf("second = %T, want ChannelClearEvent", got[1])
	}
}

func TestUnifiedMessage_PlainText(t *testing.T) {
	tests := []struct {
		name string
		segs []platform.Segment
		want string
	}{
		{"empty", nil, ""},
		{"single", []platform.Segment{{Kind: platform.SegText, Text: "hi"}}, "hi"},
		{
			// Segments concatenate verbatim — each carries its own surrounding whitespace,
			// so the leading text run keeps its trailing space before the emote.
			"text+emote",
			[]platform.Segment{
				{Kind: platform.SegText, Text: "nice "},
				{Kind: platform.SegEmote, Text: "PogChamp"},
			},
			"nice PogChamp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := platform.UnifiedMessage{Segments: tt.segs}
			if got := m.PlainText(); got != tt.want {
				t.Errorf("PlainText() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestChannelRef_Key pins the canonical routing key: platform unchanged, slug lower-cased, so a
// channel referenced with any casing resolves to a single key.
func TestChannelRef_Key(t *testing.T) {
	cases := map[string]struct {
		ref  platform.ChannelRef
		want string
	}{
		"mixed-case twitch": {platform.ChannelRef{Platform: platform.Twitch, Slug: "Shroud"}, "twitch:shroud"},
		"lower twitch":      {platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}, "twitch:forsen"},
		"mixed-case kick":   {platform.ChannelRef{Platform: platform.Kick, Slug: "xQc"}, "kick:xqc"},
	}
	for name, tc := range cases {
		if got := tc.ref.Key(); got != tc.want {
			t.Errorf("%s: Key() = %q, want %q", name, got, tc.want)
		}
	}
}

// Compile-time proof that all event types are sealed into the Event interface.
var (
	_ platform.Event = platform.MessageEvent{}
	_ platform.Event = platform.MessageDeletedEvent{}
	_ platform.Event = platform.ChannelClearEvent{}
	_ platform.Event = platform.HealthEvent{}
)
