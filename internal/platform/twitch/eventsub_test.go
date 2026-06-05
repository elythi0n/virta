package twitch

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

// TestEventSub_GoldenEqualToIRC is the 3.2 exit criterion: the same chat message delivered via
// EventSub normalizes to the same UnifiedMessage as via IRC (ignoring transport-only fields:
// the platform message id and the broadcaster display name EventSub carries but IRC doesn't).
func TestEventSub_GoldenEqualToIRC(t *testing.T) {
	// IRC PRIVMSG with a Kappa emote at rune range 6-10.
	ircLine := `@badges=moderator/1;color=#FF0000;display-name=Alice;id=m1;user-id=2;emotes=25:6-10 ` +
		`:alice!alice@alice.tmi.twitch.tv PRIVMSG #forsen :hello Kappa world`
	m, ok := parseLine(ircLine)
	if !ok {
		t.Fatal("parse IRC line")
	}
	irc := normalizePrivmsg(m)

	// The equivalent EventSub channel.chat.message event.
	es, err := normalizeEventSubMessage([]byte(`{
		"broadcaster_user_login":"forsen","broadcaster_user_name":"Forsen",
		"chatter_user_id":"2","chatter_user_login":"alice","chatter_user_name":"Alice",
		"message_id":"m1","color":"#FF0000","message_type":"text",
		"badges":[{"set_id":"moderator","id":"1","info":""}],
		"message":{"text":"hello Kappa world","fragments":[
			{"type":"text","text":"hello "},
			{"type":"emote","text":"Kappa","emote":{"id":"25"}},
			{"type":"text","text":" world"}
		]}
	}`))
	if err != nil {
		t.Fatal(err)
	}

	if irc.Type != es.Type {
		t.Errorf("type: irc %q vs es %q", irc.Type, es.Type)
	}
	if irc.Channel.Slug != es.Channel.Slug {
		t.Errorf("channel: irc %q vs es %q", irc.Channel.Slug, es.Channel.Slug)
	}
	if !reflect.DeepEqual(irc.Author, es.Author) {
		t.Errorf("author mismatch:\n irc=%+v\n  es=%+v", irc.Author, es.Author)
	}
	if !reflect.DeepEqual(irc.Segments, es.Segments) {
		t.Errorf("segments mismatch:\n irc=%+v\n  es=%+v", irc.Segments, es.Segments)
	}
}

func TestEventSub_MentionCheerAndReply(t *testing.T) {
	es, err := normalizeEventSubMessage([]byte(`{
		"broadcaster_user_login":"forsen","chatter_user_id":"7","chatter_user_login":"bob","chatter_user_name":"Bob",
		"message_id":"x","message_type":"text","color":"",
		"reply":{"parent_message_id":"p1","parent_user_login":"alice","parent_message_body":"hi"},
		"message":{"text":"@alice cheer100 yo","fragments":[
			{"type":"mention","text":"@alice","mention":{"user_id":"2"}},
			{"type":"text","text":" "},
			{"type":"cheermote","text":"cheer100","cheermote":{"bits":100}},
			{"type":"text","text":" yo"}
		]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	kinds := []platform.SegmentKind{}
	for _, s := range es.Segments {
		kinds = append(kinds, s.Kind)
	}
	want := []platform.SegmentKind{platform.SegMention, platform.SegText, platform.SegCheer, platform.SegText}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("segment kinds = %v, want %v", kinds, want)
	}
	for _, s := range es.Segments {
		if s.Kind == platform.SegCheer && s.CheerBits != 100 {
			t.Errorf("cheer bits = %d, want 100", s.CheerBits)
		}
	}
	if es.ReplyTo == nil || es.ReplyTo.PlatformMessageID != "p1" {
		t.Errorf("reply = %+v", es.ReplyTo)
	}
}

func TestEventSub_NotificationSub(t *testing.T) {
	ev, ok, err := eventFromNotification(subChatNotif, json.RawMessage(`{"event":{
		"broadcaster_user_login":"forsen","chatter_user_login":"newsub","chatter_user_name":"NewSub",
		"notice_type":"sub","system_message":"NewSub subscribed!",
		"message":{"text":"","fragments":[]}
	}}`))
	if err != nil || !ok {
		t.Fatalf("notification: ok=%v err=%v", ok, err)
	}
	m := ev.(platform.MessageEvent).Message
	if m.Type != platform.TypeSub {
		t.Errorf("type = %q, want sub", m.Type)
	}
	// No user message → falls back to the system message text.
	if m.PlainText() != "NewSub subscribed!" {
		t.Errorf("text = %q", m.PlainText())
	}
}

func TestEventSub_DeleteClearSettings(t *testing.T) {
	del, ok, _ := eventFromNotification(subChatDelete, json.RawMessage(`{"event":{"broadcaster_user_login":"forsen","message_id":"abc"}}`))
	if d, _ := del.(platform.MessageDeletedEvent); !ok || d.PlatformMessageID != "abc" || d.Channel.Slug != "forsen" {
		t.Errorf("delete = %#v", del)
	}

	clr, ok, _ := eventFromNotification(subChatClear, json.RawMessage(`{"event":{"broadcaster_user_login":"forsen"}}`))
	if c, _ := clr.(platform.ChannelClearEvent); !ok || c.Channel.Slug != "forsen" {
		t.Errorf("clear = %#v", clr)
	}

	set, ok, _ := eventFromNotification(subChatSettings, json.RawMessage(`{"event":{
		"broadcaster_user_login":"forsen","emote_mode":true,"slow_mode":true,"slow_mode_wait_time_seconds":30,
		"follower_mode":false,"subscriber_mode":false,"unique_chat_mode":true
	}}`))
	cs, _ := set.(platform.ChatSettingsEvent)
	if !ok || !cs.Settings.EmoteOnly || cs.Settings.SlowSeconds != 30 || !cs.Settings.UniqueChat || cs.Settings.FollowersOnlyMinutes != -1 {
		t.Errorf("settings = %#v", set)
	}
}

func TestEventSub_EnvelopeAndSession(t *testing.T) {
	env, err := parseEnvelope([]byte(`{"metadata":{"message_type":"session_welcome"},"payload":{"session":{"id":"sess-1","keepalive_timeout_seconds":10,"status":"connected"}}}`))
	if err != nil || env.Metadata.MessageType != esWelcome {
		t.Fatalf("envelope = %+v, err %v", env, err)
	}
	s, err := sessionFromPayload(env.Payload)
	if err != nil || s.ID != "sess-1" || s.KeepaliveTimeoutSecs != 10 {
		t.Errorf("session = %+v, err %v", s, err)
	}

	// A notification envelope routes by subscription type.
	nenv, _ := parseEnvelope([]byte(`{"metadata":{"message_type":"notification","subscription_type":"channel.chat.clear"},"payload":{"event":{"broadcaster_user_login":"forsen"}}}`))
	if nenv.Metadata.MessageType != esNotification {
		t.Fatalf("notif envelope = %+v", nenv)
	}
	ev, ok, err := eventFromNotification(nenv.Metadata.SubscriptionType, nenv.Payload)
	if err != nil || !ok {
		t.Fatalf("dispatch: ok=%v err=%v", ok, err)
	}
	if _, isClear := ev.(platform.ChannelClearEvent); !isClear {
		t.Errorf("event = %T, want ChannelClearEvent", ev)
	}
}

func TestEventSub_UnknownSubTypeIgnored(t *testing.T) {
	_, ok, err := eventFromNotification("channel.follow", json.RawMessage(`{"event":{}}`))
	if ok || err != nil {
		t.Errorf("unknown sub type: ok=%v err=%v, want (false,nil)", ok, err)
	}
}
