package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
)

// recSubmitter records every event the engine submits, for assertions.
type recSubmitter struct {
	mu  sync.Mutex
	evs []platform.Event
}

func (r *recSubmitter) Submit(ev platform.Event) {
	r.mu.Lock()
	r.evs = append(r.evs, ev)
	r.mu.Unlock()
}

func (r *recSubmitter) events() []platform.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]platform.Event(nil), r.evs...)
}

// waitForCount blocks until the submitter has recorded n events, or fails.
func (r *recSubmitter) waitForCount(t *testing.T, n int) []platform.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if evs := r.events(); len(evs) >= n {
			return evs
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("only %d events submitted, want %d", len(r.events()), n)
	return nil
}

func newTestEngine() (*Engine, *recSubmitter, *platform.FakeAdapter) {
	out := &recSubmitter{}
	eng := New(out, id.NewFake("ulid-"))
	tw := platform.NewFakeAdapter(platform.Twitch, platform.Capabilities{ReadAnonymous: true})
	eng.Register(tw)
	return eng, out, tw
}

func twMsg(platformID, body string) platform.UnifiedMessage {
	return platform.UnifiedMessage{
		Platform:          platform.Twitch,
		Channel:           platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"},
		PlatformMessageID: platformID,
		Segments:          []platform.Segment{{Kind: platform.SegText, Text: body}},
	}
}

func TestEngine_AssignsULIDToMessages(t *testing.T) {
	eng, out, tw := newTestEngine()
	t.Cleanup(func() { _ = eng.Close() })

	tw.EmitMessage(twMsg("p1", "hi"))
	evs := out.waitForCount(t, 1)
	me, ok := evs[0].(platform.MessageEvent)
	if !ok {
		t.Fatalf("event = %T, want MessageEvent", evs[0])
	}
	if me.Message.ID == "" {
		t.Error("engine did not assign a ULID to the message")
	}
}

func TestEngine_PreservesGivenID(t *testing.T) {
	eng, out, tw := newTestEngine()
	t.Cleanup(func() { _ = eng.Close() })

	m := twMsg("p1", "hi")
	m.ID = "already-set"
	tw.EmitMessage(m)
	evs := out.waitForCount(t, 1)
	if got := evs[0].(platform.MessageEvent).Message.ID; got != "already-set" {
		t.Errorf("ID = %q, want it left untouched", got)
	}
}

func TestEngine_ResolvesDeletionToULID(t *testing.T) {
	eng, out, tw := newTestEngine()
	t.Cleanup(func() { _ = eng.Close() })

	tw.EmitMessage(twMsg("p1", "hi"))
	msgs := out.waitForCount(t, 1)
	wantULID := msgs[0].(platform.MessageEvent).Message.ID

	tw.Emit(platform.MessageDeletedEvent{
		Channel:           platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"},
		PlatformMessageID: "p1",
	})
	evs := out.waitForCount(t, 2)
	del, ok := evs[1].(platform.MessageDeletedEvent)
	if !ok {
		t.Fatalf("event = %T, want MessageDeletedEvent", evs[1])
	}
	if del.MessageID != wantULID {
		t.Errorf("resolved MessageID = %q, want %q (the original message's ULID)", del.MessageID, wantULID)
	}
}

func TestEngine_DeletionOfUnknownMessageResolvesEmpty(t *testing.T) {
	eng, out, tw := newTestEngine()
	t.Cleanup(func() { _ = eng.Close() })

	tw.Emit(platform.MessageDeletedEvent{
		Channel:           platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"},
		PlatformMessageID: "never-seen",
	})
	evs := out.waitForCount(t, 1)
	if del := evs[0].(platform.MessageDeletedEvent); del.MessageID != "" {
		t.Errorf("MessageID = %q, want empty for an unknown deletion", del.MessageID)
	}
}

func TestEngine_JoinLeaveRouteByPlatform(t *testing.T) {
	out := &recSubmitter{}
	eng := New(out, id.NewFake("ulid-"))
	tw := platform.NewFakeAdapter(platform.Twitch, platform.Capabilities{ReadAnonymous: true})
	kick := platform.NewFakeAdapter(platform.Kick, platform.Capabilities{ReadAnonymous: true})
	eng.Register(tw)
	eng.Register(kick)
	t.Cleanup(func() { _ = eng.Close() })

	if err := eng.Join(context.Background(), platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}, platform.ModeAnonymous); err != nil {
		t.Fatalf("Join: %v", err)
	}
	if _, ok := tw.Joined("forsen"); !ok {
		t.Error("twitch adapter did not receive the join")
	}
	if _, ok := kick.Joined("forsen"); ok {
		t.Error("kick adapter wrongly received a twitch join")
	}

	if err := eng.Leave(platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if _, ok := tw.Joined("forsen"); ok {
		t.Error("twitch adapter still joined after Leave")
	}
}

func TestEngine_JoinUnknownPlatform(t *testing.T) {
	eng, _, _ := newTestEngine()
	t.Cleanup(func() { _ = eng.Close() })
	err := eng.Join(context.Background(), platform.ChannelRef{Platform: platform.Kick, Slug: "x"}, platform.ModeAnonymous)
	if err == nil {
		t.Fatal("Join to an unregistered platform returned nil error")
	}
}

func TestEngine_ChannelsListsJoinedWithHealth(t *testing.T) {
	eng, _, tw := newTestEngine()
	t.Cleanup(func() { _ = eng.Close() })

	_ = eng.Join(context.Background(), platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}, platform.ModeAnonymous)
	tw.SetHealth(platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonReconnecting})

	list := eng.Channels()
	if len(list) != 1 || list[0].Channel.Slug != "forsen" {
		t.Fatalf("Channels = %+v, want one (forsen)", list)
	}
	if list[0].Health.State != platform.HealthDegraded {
		t.Errorf("health = %v, want degraded", list[0].Health.State)
	}
}

func TestEngine_CloseIsIdempotent(t *testing.T) {
	eng, _, _ := newTestEngine()
	if err := eng.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := eng.Close(); err != nil {
		t.Errorf("second Close: %v, want nil", err)
	}
}
