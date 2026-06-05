package twitch

import (
	"errors"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/platformtest"
)

func normalizeUsernoticeRaw(raw []byte) (platform.UnifiedMessage, error) {
	m, ok := parseLine(string(raw))
	if !ok {
		return platform.UnifiedMessage{}, errors.New("unparseable line")
	}
	if m.command != "USERNOTICE" {
		return platform.UnifiedMessage{}, errors.New("not a USERNOTICE: " + m.command)
	}
	return normalizeUsernotice(m), nil
}

func TestNormalize_UsernoticeGolden(t *testing.T) {
	lines := loadFixtureLines(t, "testdata/usernotice.txt")
	msgs := platformtest.Replay(t, lines, normalizeUsernoticeRaw)
	platformtest.AssertGolden(t, "usernotice.golden.json", msgs)
}

func TestUsernotice_TypesByMsgID(t *testing.T) {
	cases := map[string]platform.MessageType{
		"sub":            platform.TypeSub,
		"resub":          platform.TypeResub,
		"subgift":        platform.TypeGiftSub,
		"submysterygift": platform.TypeGiftSub,
		"raid":           platform.TypeRaid,
		"announcement":   platform.TypeAnnouncement,
		"ritual":         platform.TypeSystem, // unmapped → system
	}
	for msgID, want := range cases {
		m, _ := parseLine("@msg-id=" + msgID + ";login=u;system-msg=hi :tmi.twitch.tv USERNOTICE #c")
		if got := normalizeUsernotice(m).Type; got != want {
			t.Errorf("msg-id %q → %q, want %q", msgID, got, want)
		}
	}
}

func TestUsernotice_UserMessagePreferredOverSystemMsg(t *testing.T) {
	// A resub with a user message shows the user's text (with emotes), not the system line.
	m, _ := parseLine(`@msg-id=resub;login=loyal;display-name=Loyal;emotes=25:0-4;system-msg=Loyal\ssubscribed. :tmi.twitch.tv USERNOTICE #c :Kappa hi`)
	um := normalizeUsernotice(m)
	if um.PlainText() != "Kappa hi" {
		t.Errorf("text = %q, want user message", um.PlainText())
	}
	if len(um.Segments) == 0 || um.Segments[0].Kind != platform.SegEmote {
		t.Errorf("expected leading emote segment, got %+v", um.Segments)
	}
}

func TestUsernotice_SystemMsgWhenNoUserMessage(t *testing.T) {
	m, _ := parseLine(`@msg-id=sub;login=n;system-msg=N\ssubscribed\swith\sPrime. :tmi.twitch.tv USERNOTICE #c`)
	um := normalizeUsernotice(m)
	if got := um.PlainText(); got != "N subscribed with Prime." {
		t.Errorf("text = %q, want system message", got)
	}
}

func TestEventFromLine_ClearMsg(t *testing.T) {
	m, _ := parseLine("@login=alice;target-msg-id=abc123 :tmi.twitch.tv CLEARMSG #forsen :bad message")
	ev, ok := eventFromLine(m)
	if !ok {
		t.Fatal("no event")
	}
	d, ok := ev.(platform.MessageDeletedEvent)
	if !ok || d.PlatformMessageID != "abc123" || d.Channel.Slug != "forsen" {
		t.Errorf("event = %#v", ev)
	}
}

func TestEventFromLine_ClearChat(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		m, _ := parseLine("@target-user-id=999;ban-duration=600 :tmi.twitch.tv CLEARCHAT #forsen :baduser")
		ev, _ := eventFromLine(m)
		c, ok := ev.(platform.ChannelClearEvent)
		if !ok || c.TargetUserID != "999" {
			t.Errorf("event = %#v, want clear of user 999", ev)
		}
	})
	t.Run("whole channel", func(t *testing.T) {
		m, _ := parseLine(":tmi.twitch.tv CLEARCHAT #forsen")
		ev, _ := eventFromLine(m)
		c, ok := ev.(platform.ChannelClearEvent)
		if !ok || c.TargetUserID != "" {
			t.Errorf("event = %#v, want whole-channel clear", ev)
		}
	})
}

func TestEventFromLine_RoomState(t *testing.T) {
	m, _ := parseLine("@emote-only=1;followers-only=10;slow=30;subs-only=0;r9k=1 :tmi.twitch.tv ROOMSTATE #forsen")
	ev, _ := eventFromLine(m)
	cs, ok := ev.(platform.ChatSettingsEvent)
	if !ok {
		t.Fatalf("event = %#v", ev)
	}
	want := platform.ChatSettings{EmoteOnly: true, UniqueChat: true, FollowersOnlyMinutes: 10, SlowSeconds: 30}
	if cs.Settings != want {
		t.Errorf("settings = %+v, want %+v", cs.Settings, want)
	}
}

func TestParseRoomState_DefaultsFollowersOnlyOff(t *testing.T) {
	// A partial ROOMSTATE with only slow set: followers-only defaults to off (-1), not 0.
	s := parseRoomState(map[string]string{"slow": "5"})
	if s.FollowersOnlyMinutes != -1 || s.SlowSeconds != 5 {
		t.Errorf("settings = %+v", s)
	}
}

func TestEventFromLine_PingIsNotAnEvent(t *testing.T) {
	m, _ := parseLine("PING :tmi.twitch.tv")
	if _, ok := eventFromLine(m); ok {
		t.Error("PING should not map to an event (handled by the adapter)")
	}
}
