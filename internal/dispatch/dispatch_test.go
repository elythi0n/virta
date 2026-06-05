package dispatch_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/command"
	"github.com/elythi0n/virta/internal/dispatch"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/ratelimit"
)

type sendCall struct {
	ch   platform.ChannelRef
	text string
	opts platform.SendOpts
}

type fakeAdapter struct {
	mu    sync.Mutex
	caps  platform.Capabilities
	sends []sendCall
	mods  []platform.ModAction
}

func (a *fakeAdapter) Capabilities() platform.Capabilities { return a.caps }
func (a *fakeAdapter) Send(_ context.Context, ch platform.ChannelRef, text string, opts platform.SendOpts) error {
	a.mu.Lock()
	a.sends = append(a.sends, sendCall{ch, text, opts})
	a.mu.Unlock()
	return nil
}
func (a *fakeAdapter) Moderate(_ context.Context, action platform.ModAction) error {
	a.mu.Lock()
	a.mods = append(a.mods, action)
	a.mu.Unlock()
	return nil
}

var ch = platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}

func newSender(caps platform.Capabilities) (*dispatch.Sender, *fakeAdapter) {
	a := &fakeAdapter{caps: caps}
	gov := ratelimit.New(clock.NewFake(time.Unix(0, 0)), ratelimit.Limit{Burst: 100, Window: time.Second})
	s := dispatch.New(map[platform.Platform]dispatch.Adapter{platform.Twitch: a}, gov, "help text")
	return s, a
}

func TestDispatch_PlainSend(t *testing.T) {
	s, a := newSender(platform.Capabilities{Send: true})
	out, err := s.Do(context.Background(), ch, "hello there")
	if err != nil || out.Kind != command.KindSend {
		t.Fatalf("out = %+v, err %v", out, err)
	}
	if e := <-out.Sent; e != nil {
		t.Fatalf("send result: %v", e)
	}
	if len(a.sends) != 1 || a.sends[0].text != "hello there" || a.sends[0].opts.Action {
		t.Errorf("send = %+v", a.sends)
	}
}

func TestDispatch_MeActionFlag(t *testing.T) {
	s, a := newSender(platform.Capabilities{Send: true})
	out, _ := s.Do(context.Background(), ch, "/me waves")
	<-out.Sent
	if len(a.sends) != 1 || a.sends[0].text != "waves" || !a.sends[0].opts.Action {
		t.Errorf("/me send = %+v", a.sends)
	}
}

func TestDispatch_TimeoutModerates(t *testing.T) {
	s, a := newSender(platform.Capabilities{Send: true, Moderation: true})
	out, err := s.Do(context.Background(), ch, "/timeout baddie 60")
	if err != nil || out.Kind != command.KindMod {
		t.Fatalf("out = %+v, err %v", out, err)
	}
	if len(a.mods) != 1 || a.mods[0].Type != platform.ModTimeout || a.mods[0].TargetUserID != "baddie" {
		t.Errorf("moderate = %+v", a.mods)
	}
	if len(a.sends) != 0 {
		t.Error("a moderation command must not be sent as chat")
	}
}

func TestDispatch_UnknownNotSent(t *testing.T) {
	s, a := newSender(platform.Capabilities{Send: true, Moderation: true})
	out, _ := s.Do(context.Background(), ch, "/foo bar")
	if out.Kind != command.KindHint || out.Hint == "" {
		t.Errorf("unknown = %+v", out)
	}
	if len(a.sends) != 0 || len(a.mods) != 0 {
		t.Error("unknown command leaked to send/moderate")
	}
}

func TestDispatch_ModWithoutCapabilityIsHint(t *testing.T) {
	s, a := newSender(platform.Capabilities{Send: true}) // no Moderation
	out, _ := s.Do(context.Background(), ch, "/ban someone")
	if out.Kind != command.KindHint {
		t.Errorf("ban without mod = %+v", out)
	}
	if len(a.mods) != 0 {
		t.Error("ban executed without moderation capability")
	}
}

func TestDispatch_SendWithoutCapabilityIsHint(t *testing.T) {
	s, a := newSender(platform.Capabilities{}) // no Send
	out, _ := s.Do(context.Background(), ch, "hello")
	if out.Kind != command.KindHint {
		t.Errorf("send without cap = %+v", out)
	}
	if len(a.sends) != 0 {
		t.Error("sent despite no send capability")
	}
}

func TestDispatch_Help(t *testing.T) {
	s, _ := newSender(platform.Capabilities{Send: true})
	out, _ := s.Do(context.Background(), ch, "/help")
	if out.Kind != command.KindHelp || out.Hint != "help text" {
		t.Errorf("help = %+v", out)
	}
}

func TestDispatch_UnknownPlatform(t *testing.T) {
	s, _ := newSender(platform.Capabilities{Send: true})
	out, _ := s.Do(context.Background(), platform.ChannelRef{Platform: platform.Kick, Slug: "x"}, "hi")
	if out.Kind != command.KindHint {
		t.Errorf("unknown platform = %+v", out)
	}
}
