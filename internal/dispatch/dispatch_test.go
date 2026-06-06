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

// newCrossSender wires a Twitch and a Kick adapter with the given capabilities, for the
// cross-posting tests.
func newCrossSender(tw, kick platform.Capabilities) (*dispatch.Sender, *fakeAdapter, *fakeAdapter) {
	twa, ka := &fakeAdapter{caps: tw}, &fakeAdapter{caps: kick}
	gov := ratelimit.New(clock.NewFake(time.Unix(0, 0)), ratelimit.Limit{Burst: 100, Window: time.Second})
	s := dispatch.New(map[platform.Platform]dispatch.Adapter{platform.Twitch: twa, platform.Kick: ka}, gov, "help")
	return s, twa, ka
}

var twitchCh = platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}
var kickCh = platform.ChannelRef{Platform: platform.Kick, Slug: "xqc"}

// TestSendMany_ReachableAndExcluded is the cross-posting exit behavior: a message goes to the
// reachable platform and the signed-out one is excluded before send (reported, not errored), so
// the reachable send still happens.
func TestSendMany_ReachableAndExcluded(t *testing.T) {
	s, twa, ka := newCrossSender(
		platform.Capabilities{Send: true},          // Twitch signed in
		platform.Capabilities{ReadAnonymous: true}, // Kick signed out → no Send
	)
	results := s.SendMany(context.Background(), []platform.ChannelRef{twitchCh, kickCh}, "gg", platform.SendOpts{})

	byKey := map[string]dispatch.TargetSend{}
	for _, r := range results {
		byKey[r.Channel.Key()] = r
	}
	tw := byKey["twitch:forsen"]
	if !tw.Reachable {
		t.Fatalf("twitch should be reachable: %+v", tw)
	}
	if e := <-tw.Sent; e != nil {
		t.Errorf("twitch send result: %v", e)
	}
	k := byKey["kick:xqc"]
	if k.Reachable || k.Reason != platform.ReasonAuthRequired {
		t.Errorf("kick should be excluded with auth_required, got %+v", k)
	}
	// The reachable platform was sent to; the excluded one was not.
	if len(twa.sends) != 1 || twa.sends[0].text != "gg" {
		t.Errorf("twitch sends = %+v, want one 'gg'", twa.sends)
	}
	if len(ka.sends) != 0 {
		t.Errorf("excluded kick must not be sent to, got %+v", ka.sends)
	}
}

// TestTargets_PreSendReachability covers the pre-send chip state: each target reports whether it
// can send and why not, without sending.
func TestTargets_PreSendReachability(t *testing.T) {
	s, twa, ka := newCrossSender(
		platform.Capabilities{Send: true},
		platform.Capabilities{ReadAnonymous: true},
	)
	states := s.Targets([]platform.ChannelRef{twitchCh, kickCh})
	if len(states) != 2 || !states[0].CanSend || states[1].CanSend || states[1].Reason != platform.ReasonAuthRequired {
		t.Errorf("target states = %+v", states)
	}
	// Pure pre-send check: nothing was sent.
	if len(twa.sends) != 0 || len(ka.sends) != 0 {
		t.Error("Targets must not send")
	}
}

// TestQueueState_ReportsDepthAndCountdown: a send beyond the per-channel burst is queued, and
// QueueState surfaces the depth and a positive countdown for it.
func TestQueueState_ReportsDepthAndCountdown(t *testing.T) {
	a := &fakeAdapter{caps: platform.Capabilities{Send: true}}
	gov := ratelimit.New(clock.NewFake(time.Unix(0, 0)), ratelimit.Limit{Burst: 1, Window: time.Second})
	s := dispatch.New(map[platform.Platform]dispatch.Adapter{platform.Twitch: a}, gov, "help")

	s.SendMany(context.Background(), []platform.ChannelRef{twitchCh}, "one", platform.SendOpts{}) // uses the one token
	s.SendMany(context.Background(), []platform.ChannelRef{twitchCh}, "two", platform.SendOpts{}) // queued: no token left

	st := s.QueueState([]platform.ChannelRef{twitchCh})
	if len(st) != 1 || st[0].Queued != 1 || st[0].NextIn <= 0 {
		t.Errorf("queue state = %+v, want one queued send with a positive countdown", st)
	}
}

// TestSendMany_UnknownPlatformExcluded: a target on a platform with no adapter is excluded, not
// errored.
func TestSendMany_UnknownPlatformExcluded(t *testing.T) {
	s, _, _ := newCrossSender(platform.Capabilities{Send: true}, platform.Capabilities{Send: true})
	results := s.SendMany(context.Background(), []platform.ChannelRef{{Platform: platform.X, Slug: "z"}}, "hi", platform.SendOpts{})
	if len(results) != 1 || results[0].Reachable {
		t.Errorf("unknown-platform target should be excluded: %+v", results)
	}
}
