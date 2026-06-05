package twitch

import "testing"

func TestParseLine_FullPrivmsg(t *testing.T) {
	line := `@badges=broadcaster/1;color=#FF0000;display-name=Alice;id=abc-123;user-id=99 :alice!alice@alice.tmi.twitch.tv PRIVMSG #forsen :hello world`
	m, ok := parseLine(line)
	if !ok {
		t.Fatal("parse failed")
	}
	if m.command != "PRIVMSG" {
		t.Errorf("command = %q", m.command)
	}
	if m.nick() != "alice" {
		t.Errorf("nick = %q", m.nick())
	}
	if m.tags["display-name"] != "Alice" || m.tags["id"] != "abc-123" || m.tags["color"] != "#FF0000" {
		t.Errorf("tags = %v", m.tags)
	}
	if len(m.params) != 1 || m.params[0] != "#forsen" {
		t.Errorf("params = %v", m.params)
	}
	if m.trailing() != "hello world" {
		t.Errorf("trailing = %q", m.trailing())
	}
}

func TestParseLine_NoTags(t *testing.T) {
	m, ok := parseLine(":tmi.twitch.tv PING")
	if !ok || m.command != "PING" {
		t.Fatalf("ok=%v command=%q", ok, m.command)
	}
}

func TestParseLine_PingWithTrailing(t *testing.T) {
	m, ok := parseLine("PING :tmi.twitch.tv")
	if !ok || m.command != "PING" || m.trailing() != "tmi.twitch.tv" {
		t.Fatalf("got ok=%v command=%q trailing=%q", ok, m.command, m.trailing())
	}
}

func TestParseLine_Empty(t *testing.T) {
	if _, ok := parseLine("\r\n"); ok {
		t.Error("empty line parsed as ok")
	}
}

func TestParseLine_TrailingWithColons(t *testing.T) {
	m, _ := parseLine(":a!a@a PRIVMSG #c :time is 12:30 http://x")
	if m.trailing() != "time is 12:30 http://x" {
		t.Errorf("trailing = %q", m.trailing())
	}
}

func TestUnescapeTagValue(t *testing.T) {
	tests := map[string]string{
		`plain`:       `plain`,
		`a\sb`:        `a b`,
		`semi\:colon`: `semi;colon`,
		`back\\slash`: `back\slash`,
		`line\nbreak`: "line\nbreak",
		`trailing\`:   `trailing`,
		`\s\s`:        `  `,
	}
	for in, want := range tests {
		if got := unescapeTagValue(in); got != want {
			t.Errorf("unescape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTags_EmptyAndValueless(t *testing.T) {
	tags := parseTags("badge-info=;badges=;flagonly")
	if v, ok := tags["badge-info"]; !ok || v != "" {
		t.Errorf("badge-info = %q,%v", v, ok)
	}
	if v, ok := tags["flagonly"]; !ok || v != "" {
		t.Errorf("flagonly = %q,%v", v, ok)
	}
}
