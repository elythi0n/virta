package twitch

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

func TestSubBudget_FillsConnectionsThenCaps(t *testing.T) {
	var b subBudget
	// 3 connections × 300 subs = 900 total before the cap.
	for i := 0; i < maxConnsPerUser*maxSubsPerConn; i++ {
		if _, ok := b.add(); !ok {
			t.Fatalf("add %d unexpectedly failed", i)
		}
	}
	if _, ok := b.add(); ok {
		t.Error("add past the cap succeeded")
	}
	if b.total() != maxConnsPerUser*maxSubsPerConn {
		t.Errorf("total = %d, want %d", b.total(), maxConnsPerUser*maxSubsPerConn)
	}
	// The first sub lands on connection 0; the 301st opens connection 1.
	var b2 subBudget
	c0, _ := b2.add()
	for i := 1; i < maxSubsPerConn; i++ {
		b2.add()
	}
	c1, _ := b2.add()
	if c0 != 0 || c1 != 1 {
		t.Errorf("connection indices = %d, %d, want 0, 1", c0, c1)
	}
	// Removing frees a slot for reuse.
	b2.remove(0)
	if got, ok := b2.add(); !ok || got != 0 {
		t.Errorf("after remove, add = %d, %v; want slot on conn 0", got, ok)
	}
}

// esFakeConn feeds scripted frames to the session and records writes. Close is idempotent —
// the supervisor closes conns it was handed, and tests close them too to simulate drops.
type esFakeConn struct {
	frames    chan []byte
	closeOnce sync.Once
}

func newESConn() *esFakeConn { return &esFakeConn{frames: make(chan []byte, 16)} }
func (c *esFakeConn) Read(ctx context.Context) ([]byte, error) {
	select {
	case b, ok := <-c.frames:
		if !ok {
			return nil, errors.New("closed")
		}
		return b, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *esFakeConn) Write(context.Context, []byte) error { return nil }
func (c *esFakeConn) Close() error {
	c.closeOnce.Do(func() { close(c.frames) })
	return nil
}

func TestSession_WelcomeNotificationReconnect(t *testing.T) {
	conn := newESConn()
	var welcomeID string
	var events []platform.Event
	s := &session{
		conn:      conn,
		emit:      func(e platform.Event) { events = append(events, e) },
		onWelcome: func(id string) { welcomeID = id },
	}

	conn.frames <- []byte(`{"metadata":{"message_type":"session_welcome"},"payload":{"session":{"id":"sess-9","keepalive_timeout_seconds":10}}}`)
	conn.frames <- []byte(`{"metadata":{"message_type":"session_keepalive"},"payload":{}}`)
	conn.frames <- []byte(`{"metadata":{"message_type":"notification","subscription_type":"channel.chat.message"},"payload":{"event":{"broadcaster_user_login":"forsen","chatter_user_login":"a","chatter_user_name":"A","message_id":"m","message_type":"text","message":{"text":"hi","fragments":[{"type":"text","text":"hi"}]}}}}`)
	conn.frames <- []byte(`{"metadata":{"message_type":"session_reconnect"},"payload":{"session":{"reconnect_url":"wss://new.example/ws"}}}`)

	reconnect, err := s.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if reconnect != "wss://new.example/ws" {
		t.Errorf("reconnect url = %q", reconnect)
	}
	if welcomeID != "sess-9" {
		t.Errorf("welcome id = %q", welcomeID)
	}
	if len(events) != 1 {
		t.Fatalf("emitted %d events, want 1", len(events))
	}
	if me, ok := events[0].(platform.MessageEvent); !ok || me.Message.PlainText() != "hi" {
		t.Errorf("event = %#v", events[0])
	}
}

func TestSession_ReadErrorReturns(t *testing.T) {
	conn := newESConn()
	_ = conn.Close() // reads now error
	s := &session{conn: conn, emit: func(platform.Event) {}}
	if _, err := s.run(context.Background()); err == nil {
		t.Error("run did not return the read error")
	}
}

func TestSession_IgnoresJunkFrames(t *testing.T) {
	conn := newESConn()
	var events []platform.Event
	s := &session{conn: conn, emit: func(e platform.Event) { events = append(events, e) }}
	conn.frames <- []byte(`not json`)
	conn.frames <- []byte(`{"metadata":{"message_type":"session_reconnect"},"payload":{"session":{"reconnect_url":"wss://x/ws"}}}`)
	if _, err := s.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("junk produced events: %v", events)
	}
}
