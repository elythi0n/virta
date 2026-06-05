package kick

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/platformtest"
)

var errFakeClosed = errors.New("fake transport closed")

// fakeTransport is an in-memory Pusher transport: feed it frames for the adapter to read and
// inspect the frames it wrote.
type fakeTransport struct {
	mu        sync.Mutex
	written   [][]byte
	frames    chan []byte
	closeOnce sync.Once
}

func newFakeTransport() *fakeTransport { return &fakeTransport{frames: make(chan []byte, 64)} }

func (f *fakeTransport) Write(_ context.Context, b []byte) error {
	f.mu.Lock()
	f.written = append(f.written, append([]byte(nil), b...))
	f.mu.Unlock()
	return nil
}

func (f *fakeTransport) Read(ctx context.Context) ([]byte, error) {
	select {
	case b, ok := <-f.frames:
		if !ok {
			return nil, errFakeClosed
		}
		return b, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *fakeTransport) Close() error {
	f.closeOnce.Do(func() { close(f.frames) })
	return nil
}

func (f *fakeTransport) feed(b []byte) { f.frames <- b }

func (f *fakeTransport) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.written))
	for i, b := range f.written {
		out[i] = string(b)
	}
	return out
}

func dialFake(ft *fakeTransport) DialFunc {
	return func(context.Context) (transport, error) { return ft, nil }
}

func waitForWriteContaining(t *testing.T, ft *fakeTransport, sub string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, w := range ft.writes() {
			if strings.Contains(w, sub) {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("never wrote a frame containing %q; wrote %v", sub, ft.writes())
}

func chatFrame(channel, inner string) []byte {
	b, _ := json.Marshal(map[string]any{"event": eventChatMessage, "channel": channel, "data": inner})
	return b
}

func TestAdapter_SubscribesOnJoin(t *testing.T) {
	ft := newFakeTransport()
	a := New(Options{Dial: dialFake(ft)})
	t.Cleanup(func() { _ = a.Close() })

	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "xqc", ID: "123"}, platform.ModeAnonymous); err != nil {
		t.Fatalf("Join: %v", err)
	}
	waitForWriteContaining(t, ft, `chatrooms.123.v2`)
}

func TestAdapter_ResubscribesOnConnectionEstablished(t *testing.T) {
	ft := newFakeTransport()
	a := New(Options{Dial: dialFake(ft)})
	t.Cleanup(func() { _ = a.Close() })
	_ = a.Join(context.Background(), platform.ChannelRef{Slug: "xqc", ID: "123"}, platform.ModeAnonymous)

	ft.feed([]byte(`{"event":"pusher:connection_established","data":"{\"socket_id\":\"1.1\"}"}`))
	// subscribeAll must (re)send the subscribe for the joined chatroom.
	waitForWriteContaining(t, ft, `"channel":"chatrooms.123.v2"`)
}

func TestAdapter_EmitsChatMessage(t *testing.T) {
	ft := newFakeTransport()
	a := New(Options{Dial: dialFake(ft)})
	t.Cleanup(func() { _ = a.Close() })
	_ = a.Join(context.Background(), platform.ChannelRef{Slug: "xqc", ID: "123"}, platform.ModeAnonymous)

	inner := `{"id":"c1","content":"hello [emote:5:Kappa]","sender":{"id":7,"username":"Neo","slug":"neo","identity":{"color":"#fff","badges":[]}}}`
	ft.feed(chatFrame("chatrooms.123.v2", inner))

	select {
	case ev := <-a.Events():
		me, ok := ev.(platform.MessageEvent)
		if !ok {
			t.Fatalf("event = %T, want MessageEvent", ev)
		}
		if me.Message.PlainText() != "hello Kappa" || me.Message.Author.DisplayName != "Neo" {
			t.Errorf("message = %+v", me.Message)
		}
		if me.Message.Channel.Slug != "xqc" {
			t.Errorf("channel = %+v, want the joined channel", me.Message.Channel)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message event")
	}
}

func TestAdapter_IgnoresUnsubscribedChannel(t *testing.T) {
	ft := newFakeTransport()
	a := New(Options{Dial: dialFake(ft)})
	t.Cleanup(func() { _ = a.Close() })
	_ = a.Join(context.Background(), platform.ChannelRef{Slug: "xqc", ID: "123"}, platform.ModeAnonymous)

	// A message for a chatroom we never joined must be dropped, not emitted with a zero channel.
	ft.feed(chatFrame("chatrooms.999.v2", `{"id":"x","content":"hi","sender":{"id":1,"username":"U","slug":"u","identity":{}}}`))
	// Follow with a legitimate one; the first event we receive must be the legit one.
	ft.feed(chatFrame("chatrooms.123.v2", `{"id":"y","content":"real","sender":{"id":2,"username":"V","slug":"v","identity":{}}}`))

	select {
	case ev := <-a.Events():
		if me := ev.(platform.MessageEvent); me.Message.PlatformMessageID != "y" {
			t.Errorf("received %q, want the subscribed-channel message", me.Message.PlatformMessageID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message event")
	}
}

func TestAdapter_RespondsToPing(t *testing.T) {
	ft := newFakeTransport()
	a := New(Options{Dial: dialFake(ft)})
	t.Cleanup(func() { _ = a.Close() })
	_ = a.Join(context.Background(), platform.ChannelRef{Slug: "xqc", ID: "123"}, platform.ModeAnonymous)

	ft.feed([]byte(`{"event":"pusher:ping","data":"{}"}`))
	waitForWriteContaining(t, ft, `pusher:pong`)
}

func TestAdapter_JoinWithoutChatroomID(t *testing.T) {
	a := New(Options{Dial: dialFake(newFakeTransport())})
	t.Cleanup(func() { _ = a.Close() })
	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "xqc"}, platform.ModeAnonymous); err == nil {
		t.Fatal("Join without a chatroom id returned nil error")
	}
}

func TestAdapter_SendAndModerateUnsupported(t *testing.T) {
	a := New(Options{Dial: dialFake(newFakeTransport())})
	t.Cleanup(func() { _ = a.Close() })
	ch := platform.ChannelRef{Slug: "xqc", ID: "1"}
	if err := a.Send(context.Background(), ch, "hi", platform.SendOpts{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Send = %v, want ErrUnsupported", err)
	}
	if err := a.Moderate(context.Background(), platform.ModAction{Type: platform.ModBan, Channel: ch}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Moderate = %v, want ErrUnsupported", err)
	}
}

func TestAdapter_Contract(t *testing.T) {
	platformtest.RunAdapterContract(t, func(t *testing.T) platform.Adapter {
		return New(Options{Dial: dialFake(newFakeTransport())})
	})
}

func TestAdapter_JoinDialError(t *testing.T) {
	a := New(Options{Dial: func(context.Context) (transport, error) {
		return nil, errors.New("dial boom")
	}})
	t.Cleanup(func() { _ = a.Close() })
	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "x", ID: "1"}, platform.ModeAnonymous); err == nil {
		t.Fatal("Join with failing dial returned nil error")
	}
	if a.Health().State != platform.HealthDown {
		t.Errorf("health = %v, want down after dial failure", a.Health().State)
	}
}

// chaosServer hands out a fresh fakeTransport per dial for the reconnect test.
type chaosServer struct {
	mu    sync.Mutex
	conns []*fakeTransport
}

func (c *chaosServer) dial(context.Context) (transport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ft := newFakeTransport()
	c.conns = append(c.conns, ft)
	return ft, nil
}

func (c *chaosServer) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.conns)
}

func (c *chaosServer) conn(i int) *fakeTransport {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conns[i]
}

func TestAdapter_ReconnectsAndResubscribes(t *testing.T) {
	cs := &chaosServer{}
	a := New(Options{Dial: cs.dial, BackoffBase: time.Millisecond, BackoffMax: 2 * time.Millisecond})
	t.Cleanup(func() { _ = a.Close() })

	_ = a.Join(context.Background(), platform.ChannelRef{Slug: "xqc", ID: "123"}, platform.ModeAnonymous)
	c1 := cs.conn(0)
	c1.feed([]byte(`{"event":"pusher:connection_established","data":"{}"}`))
	c1.feed(chatFrame("chatrooms.123.v2", `{"id":"a","content":"before","sender":{"id":1,"username":"U","slug":"u","identity":{}}}`))

	if !waitMsg(t, a, "a") {
		t.Fatal("did not receive pre-drop message")
	}

	// Drop the connection; the supervisor must redial and re-subscribe on the new socket.
	_ = c1.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cs.count() < 2 {
		time.Sleep(time.Millisecond)
	}
	if cs.count() < 2 {
		t.Fatal("no reconnect dialed")
	}
	c2 := cs.conn(1)
	c2.feed([]byte(`{"event":"pusher:connection_established","data":"{}"}`))
	waitForWriteContaining(t, c2, `chatrooms.123.v2`) // re-subscribed without a caller re-joining
	c2.feed(chatFrame("chatrooms.123.v2", `{"id":"b","content":"after","sender":{"id":1,"username":"U","slug":"u","identity":{}}}`))

	if !waitMsg(t, a, "b") {
		t.Fatal("did not receive post-reconnect message")
	}
}

func TestAdapter_LeaveUnsubscribes(t *testing.T) {
	ft := newFakeTransport()
	a := New(Options{Dial: dialFake(ft)})
	t.Cleanup(func() { _ = a.Close() })
	ch := platform.ChannelRef{Slug: "xqc", ID: "123"}
	_ = a.Join(context.Background(), ch, platform.ModeAnonymous)
	waitForWriteContaining(t, ft, `"event":"pusher:subscribe"`)

	if err := a.Leave(ch); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	waitForWriteContaining(t, ft, `pusher:unsubscribe`)
	// Leaving an unknown channel is a no-op, not an error.
	if err := a.Leave(platform.ChannelRef{ID: "404"}); err != nil {
		t.Errorf("Leave unknown = %v, want nil", err)
	}
}

func TestAdapter_EscalatesToDownThenRecovers(t *testing.T) {
	cs := &chaosServer{}
	var mu sync.Mutex
	failNextDials := 0

	dial := func(ctx context.Context) (transport, error) {
		mu.Lock()
		fail := failNextDials > 0
		if fail {
			failNextDials--
		}
		mu.Unlock()
		if fail {
			return nil, errors.New("dial refused")
		}
		return cs.dial(ctx)
	}
	a := New(Options{Dial: dial, BackoffBase: time.Millisecond, BackoffMax: 2 * time.Millisecond})
	t.Cleanup(func() { _ = a.Close() })

	_ = a.Join(context.Background(), platform.ChannelRef{Slug: "xqc", ID: "123"}, platform.ModeAnonymous)
	c1 := cs.conn(0)

	mu.Lock()
	failNextDials = downAfterAttempts + 1
	mu.Unlock()
	_ = c1.Close()

	waitFor(t, "escalation to down", func() bool { return a.Health().State == platform.HealthDown })
	waitFor(t, "recovery to ok", func() bool { return a.Health().State == platform.HealthOK })
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
	t.Fatalf("timed out waiting for %s", what)
}

func waitMsg(t *testing.T, a *Adapter, id string) bool {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-a.Events():
			if me, ok := ev.(platform.MessageEvent); ok && me.Message.PlatformMessageID == id {
				return true
			}
		case <-timeout:
			return false
		}
	}
}
