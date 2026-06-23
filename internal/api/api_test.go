package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/userctx"
)

func start(t *testing.T) *Server {
	t.Helper()
	s, err := New(Config{Addr: "127.0.0.1:0", RuntimeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Close(ctx)
	})
	return s
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func TestHealth_NoAuth(t *testing.T) {
	s := start(t)
	resp, err := http.Get("http://" + s.Addr() + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("health body = %v", body)
	}
}

func TestDiagnostics_RequiresAuth(t *testing.T) {
	s := start(t)
	base := "http://" + s.Addr() + "/v1/diagnostics"

	resp, err := http.Get(base)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", resp.StatusCode)
	}

	resp2, err := http.Get(base + "?token=" + s.Token())
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("with-token status = %d, want 200", resp2.StatusCode)
	}
}

func TestDiscoveryFile_LifecycleAndContents(t *testing.T) {
	s, err := New(Config{Addr: "127.0.0.1:0", RuntimeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	dir := s.runtimeDir
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}

	d, err := ReadDiscovery(dir)
	if err != nil {
		t.Fatalf("ReadDiscovery: %v", err)
	}
	if d.Addr != s.Addr() || d.Token != s.Token() {
		t.Errorf("discovery = %+v, want addr=%s token=%s", d, s.Addr(), s.Token())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := ReadDiscovery(dir); err == nil {
		t.Error("discovery file still present after Close")
	}
}

func dialStream(t *testing.T, s *Server, withToken bool) (*websocket.Conn, error) {
	t.Helper()
	url := "ws://" + s.Addr() + "/v1/stream"
	if withToken {
		url += "?token=" + s.Token()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, url, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return conn, err
}

func TestStream_RequiresAuth(t *testing.T) {
	s := start(t)
	conn, err := dialStream(t, s, false)
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
		t.Fatal("stream dial without token succeeded, want rejection")
	}
}

func TestStream_DeliversMessages(t *testing.T) {
	s := start(t)
	conn, err := dialStream(t, s, true)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	waitFor(t, "client registered", func() bool { return s.hub.clientCount() == 1 })

	want := platform.UnifiedMessage{
		ID: "m1", Type: platform.TypeChat,
		Channel:  platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"},
		Segments: []platform.Segment{{Kind: platform.SegText, Text: "hi"}},
	}
	_ = s.Sink().Consume(context.Background(), platform.MessageEvent{Message: want})

	rctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := conn.Read(rctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var we WireEvent
	if err := json.Unmarshal(data, &we); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if we.Type != "message" || we.SchemaVersion != schemaVersion || we.Message == nil || we.Message.ID != "m1" {
		t.Fatalf("envelope = %+v", we)
	}
}

func TestServer_LoggerPresent(t *testing.T) {
	s := start(t)
	if s.Logger() == nil {
		t.Error("Logger() is nil")
	}
}

func TestAuth_BearerHeader(t *testing.T) {
	s := start(t)
	req, _ := http.NewRequest(http.MethodGet, "http://"+s.Addr()+"/v1/diagnostics", nil)
	req.Header.Set("Authorization", "Bearer "+s.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bearer-header auth status = %d, want 200", resp.StatusCode)
	}
}

func TestStream_SubscribeThenReceive(t *testing.T) {
	s := start(t)
	conn, err := dialStream(t, s, true)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
	waitFor(t, "client registered", func() bool { return s.hub.clientCount() == 1 })

	// Send a subscribe control message (exercises the read-loop subscription path).
	sub, _ := json.Marshal(subscribeMessage{Action: "subscribe", Channels: []string{"twitch:forsen"}})
	wctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Write(wctx, websocket.MessageText, sub); err != nil {
		t.Fatalf("write subscribe: %v", err)
	}

	_ = s.Sink().Consume(context.Background(), platform.MessageEvent{Message: platform.UnifiedMessage{
		ID: "x", Channel: platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"},
	}})
	rctx, cancelr := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelr()
	_, data, err := conn.Read(rctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var we WireEvent
	_ = json.Unmarshal(data, &we)
	if we.Message == nil || we.Message.ID != "x" {
		t.Fatalf("expected subscribed message, got %+v", we)
	}
}

// stubChannels is a per-user joined-channels lister so scopedSubscription's guard can be tested
// without spinning up an engine. Returns whatever the test seeded for the requested user.
type stubChannels struct {
	by map[string][]ChannelInfo
}

func (s stubChannels) Join(context.Context, string, string, string) error   { return nil }
func (s stubChannels) Leave(context.Context, string, string) error          { return nil }
func (s stubChannels) Capabilities() map[string]Capabilities                { return nil }
func (s stubChannels) Streams(context.Context) []StreamInfo                 { return nil }
func (s stubChannels) Emotes(context.Context) []EmoteInfo                   { return nil }
func (s stubChannels) List(ctx context.Context) []ChannelInfo {
	return s.by[userctx.FromContext(ctx)]
}

// TestServer_ScopedSubscription confirms the WebSocket subscription guard:
//   1. A subscribe request for a channel the user has NOT joined is silently dropped.
//   2. An empty channel list defaults to "all of the requesting user's joined channels".
//   3. In single-user mode (empty user_id), the user's joined set is the entire joined set —
//      old behavior is preserved for desktop installs.
func TestServer_ScopedSubscription(t *testing.T) {
	srv, err := New(Config{Addr: "127.0.0.1:0", RuntimeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.SetChannels(stubChannels{by: map[string][]ChannelInfo{
		"user-A": {{Platform: "twitch", Slug: "shroud"}, {Platform: "kick", Slug: "xqc"}},
		"user-B": {{Platform: "twitch", Slug: "forsen"}},
		"":       {{Platform: "twitch", Slug: "shroud"}}, // single-user mode
	}})

	ctxA := userctx.WithUser(context.Background(), "user-A")
	ctxB := userctx.WithUser(context.Background(), "user-B")
	ctxLocal := context.Background()

	// 1. User A requesting user B's channel: dropped (not in A's joined set).
	subA := srv.scopedSubscription(ctxA, []string{"twitch:forsen"})
	if _, ok := subA.channels["twitch:forsen"]; ok {
		t.Errorf("scopedSubscription leaked twitch:forsen to user A who never joined it")
	}
	// User A asking for their own channel: kept.
	subA = srv.scopedSubscription(ctxA, []string{"twitch:shroud"})
	if _, ok := subA.channels["twitch:shroud"]; !ok {
		t.Errorf("scopedSubscription dropped user A's own twitch:shroud")
	}

	// 2. User B with empty request: gets every channel they own (forsen), nothing else.
	subB := srv.scopedSubscription(ctxB, nil)
	if _, ok := subB.channels["twitch:forsen"]; !ok {
		t.Errorf("scopedSubscription empty-list should default to all of B's joined channels")
	}
	if _, ok := subB.channels["twitch:shroud"]; ok {
		t.Errorf("user B's empty-list subscription must not include user A's shroud")
	}

	// 3. Single-user mode: empty list returns the (single) user's joined channels — unchanged.
	subLocal := srv.scopedSubscription(ctxLocal, nil)
	if _, ok := subLocal.channels["twitch:shroud"]; !ok {
		t.Errorf("single-user empty-list subscription dropped its own channel")
	}
	if len(subLocal.channels) != 1 {
		t.Errorf("single-user subscription = %v, want exactly 1 channel", subLocal.channels)
	}
}

func TestHub_FiltersBySubscription(t *testing.T) {
	h := newHub()
	all := newClient()
	only := newClient()
	only.setSubscription(toSubscription([]string{"kick:xqc"}))
	h.register(all)
	h.register(only)

	mk := func(slug string) platform.MessageEvent {
		return platform.MessageEvent{Message: platform.UnifiedMessage{
			ID: slug, Channel: platform.ChannelRef{Platform: platform.Kick, Slug: slug},
		}}
	}
	_ = h.Consume(context.Background(), mk("forsen"))
	_ = h.Consume(context.Background(), mk("xqc"))

	if got := drain(all.send); got != 2 {
		t.Errorf("all-subscriber received %d, want 2", got)
	}
	if got := drain(only.send); got != 1 {
		t.Errorf("filtered subscriber received %d, want 1 (only kick:xqc)", got)
	}
}

func TestHub_AdapterHealthBroadcastsToAll(t *testing.T) {
	h := newHub()
	c := newClient()
	c.setSubscription(toSubscription([]string{"kick:xqc"})) // narrow filter
	h.register(c)
	// Adapter-wide health (no channel) must reach even a narrowly-subscribed client.
	_ = h.Consume(context.Background(), platform.HealthEvent{Status: platform.HealthStatus{State: platform.HealthDegraded}})
	if got := drain(c.send); got != 1 {
		t.Errorf("received %d adapter-health events, want 1", got)
	}
}

func TestHub_SlowClientDropsOldest(t *testing.T) {
	h := newHub()
	c := newClient()
	h.register(c)
	// Push more than the buffer holds; the hub must keep accepting (drop-oldest), never block.
	for i := range clientBuffer + 50 {
		_ = h.Consume(context.Background(), platform.MessageEvent{Message: platform.UnifiedMessage{ID: string(rune(i))}})
	}
	if got := len(c.send); got > clientBuffer {
		t.Errorf("buffer overran: %d > %d", got, clientBuffer)
	}
}

func TestHub_CloseStopsDelivery(t *testing.T) {
	h := newHub()
	c := newClient()
	h.register(c)
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if h.register(newClient()) {
		t.Error("register succeeded after Close")
	}
	// Consume after Close must not panic (no clients remain).
	_ = h.Consume(context.Background(), platform.MessageEvent{})
}

func drain(ch chan []byte) int {
	n := 0
	for {
		select {
		case <-ch:
			n++
		default:
			return n
		}
	}
}
