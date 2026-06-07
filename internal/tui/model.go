// Package tui is the Virta terminal frontend (ADR-026). It connects to a running virtad daemon
// (or starts one), renders the live feed as a scrollable list, and lets the user type and send.
// Palettes are derived from the same design-system tokens as the web and desktop apps.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Message is one chat line in the feed.
type Message struct {
	Platform string
	Channel  string
	Author   string
	Body     string
	SentAt   time.Time
	Type     string // "chat" | "sub" | "raid" | ...
}

// Config holds TUI startup parameters.
type Config struct {
	Addr     string // daemon address, e.g. "127.0.0.1:50432"
	Token    string
	Channels []string // "" = all
	Theme    string   // "dark" (default) | "light"
}

// Palette maps design-system color roles to truecolor values. The web and desktop apps use CSS
// tokens from the same source; the TUI maps the dark-theme tokens to lipgloss colors.
type Palette struct {
	BG0, BG1, BG2               lipgloss.Color
	Text0, Text1, Text2         lipgloss.Color
	Accent, OK, Warn, Danger    lipgloss.Color
	PlatTwitch, PlatKick, PlatX lipgloss.Color
}

var DarkPalette = Palette{
	BG0: "#0E0F12", BG1: "#15171C", BG2: "#1C1F26",
	Text0: "#E8EAF0", Text1: "#9AA1AE", Text2: "#5C6370",
	Accent: "#5B8CFF", OK: "#3FB950", Warn: "#D29922", Danger: "#F85149",
	PlatTwitch: "#9146FF", PlatKick: "#53FC18", PlatX: "#E7E9EA",
}

var LightPalette = Palette{
	BG0: "#FAFBFC", BG1: "#F1F3F5", BG2: "#E7EAED",
	Text0: "#1A1D23", Text1: "#5C6370", Text2: "#7C8492",
	Accent: "#3B6FE0", OK: "#1A7F37", Warn: "#9A6700", Danger: "#CF222E",
	PlatTwitch: "#9146FF", PlatKick: "#3FA329", PlatX: "#0F1419",
}

// styles is the per-palette lipgloss style set.
type styles struct {
	pal    Palette
	row    lipgloss.Style
	author lipgloss.Style
	meta   lipgloss.Style
	input  lipgloss.Style
	status lipgloss.Style
	event  lipgloss.Style
	rail   func(platform string) lipgloss.Style
}

func newStyles(pal Palette) *styles {
	s := &styles{pal: pal}
	s.row = lipgloss.NewStyle().Foreground(pal.Text0)
	s.author = lipgloss.NewStyle().Bold(true).Foreground(pal.Text0)
	s.meta = lipgloss.NewStyle().Foreground(pal.Text2)
	s.input = lipgloss.NewStyle().Foreground(pal.Text0).BorderStyle(lipgloss.NormalBorder()).BorderTop(true).BorderForeground(pal.Accent)
	s.status = lipgloss.NewStyle().Foreground(pal.Text2).Italic(true)
	s.event = lipgloss.NewStyle().Foreground(pal.Accent).Bold(true)
	s.rail = func(platform string) lipgloss.Style {
		color := pal.Text2
		switch platform {
		case "twitch":
			color = pal.PlatTwitch
		case "kick":
			color = pal.PlatKick
		case "x":
			color = pal.PlatX
		}
		return lipgloss.NewStyle().Foreground(color)
	}
	return s
}

// Model is the Bubble Tea model.
type Model struct {
	cfg      Config
	pal      Palette
	styles   *styles
	viewport viewport.Model
	input    textinput.Model
	messages []Message
	status   string
	width    int
	height   int
	quit     chan struct{}
	incoming chan Message
	state    string // "connecting" | "connected" | "offline"
}

// New builds a Model ready to start.
func New(cfg Config) *Model {
	pal := DarkPalette
	if cfg.Theme == "light" {
		pal = LightPalette
	}
	ti := textinput.New()
	ti.Placeholder = "Type a message and press Enter…"
	ti.Focus()
	vp := viewport.New(80, 20)
	return &Model{
		cfg:      cfg,
		pal:      pal,
		styles:   newStyles(pal),
		viewport: vp,
		input:    ti,
		status:   "connecting",
		quit:     make(chan struct{}),
		incoming: make(chan Message, 256),
		state:    "connecting",
	}
}

// connectMsg is sent when the WS connection changes state.
type connectMsg struct{ state string }

// incomingMsg wraps a Message for the tea.Msg channel.
type incomingMsg Message

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.connect(),
		m.awaitMessage(),
	)
}

func (m *Model) connect() tea.Cmd {
	return func() tea.Msg {
		addr := m.cfg.Addr
		if addr == "" {
			addr = "127.0.0.1:0" // fallback — caller should discover from runtime dir
		}
		// Subscribe to the stream.
		wsURL := "ws://" + addr + "/v1/stream?token=" + url.QueryEscape(m.cfg.Token)
		conn, _, err := websocketConnect(wsURL)
		if err != nil {
			return connectMsg{state: "offline"}
		}
		// Subscribe message.
		sub, _ := json.Marshal(map[string]any{"action": "subscribe", "channels": m.cfg.Channels})
		_ = conn.Write(context.Background(), 1, sub)
		go func() {
			defer func() { _ = conn.Close(1000, "bye") }()
			for {
				select {
				case <-m.quit:
					return
				default:
				}
				_, b, err := conn.Read(context.Background())
				if err != nil {
					m.incoming <- Message{Type: "status", Body: "disconnected"}
					return
				}
				var ev map[string]any
				if json.Unmarshal(b, &ev) != nil {
					continue
				}
				if ev["type"] == "message" {
					raw, _ := json.Marshal(ev["message"])
					var msg struct {
						Platform string `json:"platform"`
						Channel  struct {
							Slug string `json:"slug"`
						} `json:"channel"`
						Author   struct{ DisplayName, Login string } `json:"author"`
						Segments []struct {
							Text string `json:"text"`
						} `json:"segments"`
						Type   string `json:"type"`
						SentAt string `json:"sent_at"`
					}
					_ = json.Unmarshal(raw, &msg)
					var body strings.Builder
					for _, seg := range msg.Segments {
						body.WriteString(seg.Text)
					}
					author := msg.Author.DisplayName
					if author == "" {
						author = msg.Author.Login
					}
					t, _ := time.Parse(time.RFC3339, msg.SentAt)
					m.incoming <- Message{
						Platform: msg.Platform,
						Channel:  msg.Channel.Slug,
						Author:   author,
						Body:     body.String(),
						SentAt:   t,
						Type:     msg.Type,
					}
				}
			}
		}()
		return connectMsg{state: "connected"}
	}
}

func (m *Model) awaitMessage() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-m.incoming:
			return incomingMsg(msg)
		case <-m.quit:
			return nil
		}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			close(m.quit)
			return m, tea.Quit
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input.Value())
			if text != "" {
				go m.send(text)
				m.input.SetValue("")
			}
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 4 // room for status bar + input
		m.input.Width = msg.Width - 2
		m.refreshViewport()
	case connectMsg:
		m.state = msg.state
		m.status = msg.state
		if msg.state == "offline" {
			m.messages = append(m.messages, Message{Type: "status", Body: "Not connected — is virtad running?"})
			m.refreshViewport()
		}
	case incomingMsg:
		m.messages = append(m.messages, Message(msg))
		if len(m.messages) > 5000 {
			m.messages = m.messages[len(m.messages)-5000:]
		}
		m.refreshViewport()
		m.viewport.GotoBottom()
		return m, m.awaitMessage()
	}
	var vpCmd, inputCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.input, inputCmd = m.input.Update(msg)
	return m, tea.Batch(vpCmd, inputCmd)
}

func (m *Model) View() string {
	status := m.styles.status.Render("● " + m.state)
	body := m.viewport.View()
	inp := m.styles.input.Render(m.input.View())
	return lipgloss.JoinVertical(lipgloss.Left, status, body, inp)
}

func (m *Model) refreshViewport() {
	var lines []string
	for _, msg := range m.messages {
		lines = append(lines, m.renderRow(msg))
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m *Model) renderRow(msg Message) string {
	if msg.Type == "status" {
		return m.styles.status.Render("  " + msg.Body)
	}
	if msg.Type != "" && msg.Type != "chat" && msg.Type != "action" {
		label := strings.ToUpper(msg.Type)
		return m.styles.event.Render("  ["+label+"] ") + m.styles.author.Render(msg.Author) + " " + m.styles.row.Render(msg.Body)
	}
	rail := m.styles.rail(msg.Platform).Render("│")
	ts := ""
	if !msg.SentAt.IsZero() {
		ts = m.styles.meta.Render(msg.SentAt.Format("15:04")) + " "
	}
	return rail + " " + ts + m.styles.author.Render(msg.Author) + m.styles.meta.Render(": ") + m.styles.row.Render(msg.Body)
}

func (m *Model) send(text string) {
	body, _ := json.Marshal(map[string]any{
		"channels": m.cfg.Channels,
		"text":     text,
	})
	addr := m.cfg.Addr
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/send", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+m.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		m.incoming <- Message{Type: "status", Body: fmt.Sprintf("send error: %v", err)}
		return
	}
	defer func() { _ = resp.Body.Close() }()
}
