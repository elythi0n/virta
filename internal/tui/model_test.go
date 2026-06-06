package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_PaletteTokens(t *testing.T) {
	for _, pal := range []Palette{DarkPalette, LightPalette} {
		for _, c := range []string{string(pal.BG0), string(pal.Text0), string(pal.Accent)} {
			if !strings.HasPrefix(c, "#") || len(c) != 7 {
				t.Errorf("palette color %q is not a 7-char hex", c)
			}
		}
	}
}

func TestModel_InitialState(t *testing.T) {
	m := New(Config{Addr: "127.0.0.1:0", Token: "test", Theme: "dark"})
	if m.state != "connecting" {
		t.Errorf("initial state = %q, want connecting", m.state)
	}
}

func TestModel_AddMessageAndRender(t *testing.T) {
	m := New(Config{Addr: "127.0.0.1:0", Token: "test", Theme: "dark"})
	m.width, m.height = 80, 24
	m.viewport.Width = 80
	m.viewport.Height = 20

	m.messages = append(m.messages, Message{
		Platform: "twitch", Channel: "forsen", Author: "bob",
		Body: "hello world", Type: "chat", SentAt: time.Now(),
	})
	m.refreshViewport()
	view := m.View()
	if !strings.Contains(view, "bob") || !strings.Contains(view, "hello world") {
		t.Errorf("view does not contain expected message content, got:\n%s", view)
	}
}

func TestModel_EventRendering(t *testing.T) {
	m := New(Config{Addr: "127.0.0.1:0", Token: "test", Theme: "light"})
	m.width, m.height = 80, 24
	m.viewport.Width = 80
	m.viewport.Height = 20

	m.messages = append(m.messages, Message{
		Platform: "twitch", Author: "raider", Body: "raided with 100!", Type: "raid",
	})
	m.refreshViewport()
	view := m.View()
	if !strings.Contains(view, "RAID") || !strings.Contains(view, "raider") {
		t.Errorf("raid event not rendered correctly, got:\n%s", view)
	}
}

func TestModel_QuitKey(t *testing.T) {
	m := New(Config{Addr: "127.0.0.1:0", Token: "test"})
	m.width, m.height = 80, 24
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("expected a quit command on Ctrl-C")
	}
	_ = updated
}

func TestModel_RenderRow_StatusLine(t *testing.T) {
	m := New(Config{Theme: "dark"})
	line := m.renderRow(Message{Type: "status", Body: "test status"})
	if !strings.Contains(line, "test status") {
		t.Errorf("status row = %q, does not contain body", line)
	}
}
