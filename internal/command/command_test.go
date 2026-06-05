package command

import (
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

var ch = platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}

func modCaps() platform.Capabilities  { return platform.Capabilities{Send: true, Moderation: true} }
func sendCaps() platform.Capabilities { return platform.Capabilities{Send: true} }

func TestParse_PlainSend(t *testing.T) {
	p := Parse("hello world", ch, sendCaps())
	if p.Kind != KindSend || p.Text != "hello world" || p.IsAction {
		t.Errorf("plain = %+v", p)
	}
}

func TestParse_EscapedSlash(t *testing.T) {
	p := Parse("//ban looks like a command", ch, sendCaps())
	if p.Kind != KindSend || p.Text != "/ban looks like a command" {
		t.Errorf("escaped = %+v", p)
	}
}

func TestParse_MeIsActionSend(t *testing.T) {
	p := Parse("/me waves", ch, sendCaps())
	if p.Kind != KindSend || !p.IsAction || p.Text != "waves" {
		t.Errorf("/me = %+v", p)
	}
}

func TestParse_TimeoutRoutesToTypedAction(t *testing.T) {
	p := Parse("/timeout baduser 600 being rude", ch, modCaps())
	if p.Kind != KindMod {
		t.Fatalf("/timeout kind = %v", p.Kind)
	}
	a := p.Action
	if a.Type != platform.ModTimeout || a.TargetUserID != "baduser" || a.Duration != 600*time.Second || a.Reason != "being rude" {
		t.Errorf("timeout action = %+v", a)
	}
	if a.Channel.Slug != "forsen" {
		t.Errorf("action channel = %+v", a.Channel)
	}
}

func TestParse_TimeoutDefaultsDuration(t *testing.T) {
	p := Parse("/timeout baduser", ch, modCaps())
	if p.Action.Duration != defaultTimeout {
		t.Errorf("default timeout = %v, want %v", p.Action.Duration, defaultTimeout)
	}
}

func TestParse_UnknownCommandNotSent(t *testing.T) {
	p := Parse("/foo bar", ch, modCaps())
	if p.Kind != KindHint || p.Hint == "" {
		t.Errorf("/foo = %+v, want a not-sent hint", p)
	}
}

func TestParse_ModCommandWithoutCapabilityIsHint(t *testing.T) {
	// A viewer (no moderation) typing /ban must get a hint, never a sent action.
	p := Parse("/ban someone", ch, sendCaps())
	if p.Kind != KindHint {
		t.Errorf("/ban without mod = %+v, want hint", p)
	}
}

func TestParse_Toggles(t *testing.T) {
	cases := map[string]struct {
		typ platform.ModActionType
		on  bool
	}{
		"/emoteonly":     {platform.ModSetEmoteOnly, true},
		"/emoteonlyoff":  {platform.ModSetEmoteOnly, false},
		"/uniquechat":    {platform.ModSetUniqueChat, true},
		"/uniquechatoff": {platform.ModSetUniqueChat, false},
	}
	for in, want := range cases {
		p := Parse(in, ch, modCaps())
		if p.Kind != KindMod || p.Action.Type != want.typ || p.Action.Enabled != want.on {
			t.Errorf("%s = %+v, want %v/%v", in, p.Action, want.typ, want.on)
		}
	}
}

func TestParse_SlowAndFollowers(t *testing.T) {
	if p := Parse("/slow 5", ch, modCaps()); p.Action.Type != platform.ModSetSlow || p.Action.Duration != 5*time.Second || !p.Action.Enabled {
		t.Errorf("/slow 5 = %+v", p.Action)
	}
	if p := Parse("/slowoff", ch, modCaps()); p.Action.Type != platform.ModSetSlow || p.Action.Enabled {
		t.Errorf("/slowoff = %+v", p.Action)
	}
	if p := Parse("/followers 10", ch, modCaps()); p.Action.Type != platform.ModSetFollowers || p.Action.Duration != 10*time.Minute {
		t.Errorf("/followers 10 = %+v", p.Action)
	}
}

func TestParse_BadArgsAreHints(t *testing.T) {
	for _, in := range []string{"/ban", "/timeout", "/delete", "/slow notnum"} {
		if p := Parse(in, ch, modCaps()); p.Kind != KindHint {
			t.Errorf("%q = %+v, want a usage hint (not sent)", in, p)
		}
	}
}

func TestParse_Help(t *testing.T) {
	if Parse("/help", ch, sendCaps()).Kind != KindHelp {
		t.Error("/help not recognized")
	}
}
