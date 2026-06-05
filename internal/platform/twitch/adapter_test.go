package twitch

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/platformtest"
)

var errFakeClosed = errors.New("fake transport closed")

// fakeTransport is an in-memory transport: feed it IRC lines for the adapter to read, and
// inspect the lines the adapter wrote.
type fakeTransport struct {
	mu        sync.Mutex
	written   []string
	lines     chan string
	closeOnce sync.Once
}

func newFakeTransport() *fakeTransport { return &fakeTransport{lines: make(chan string, 64)} }

func (f *fakeTransport) WriteLine(_ context.Context, line string) error {
	f.mu.Lock()
	f.written = append(f.written, line)
	f.mu.Unlock()
	return nil
}

func (f *fakeTransport) ReadLine(ctx context.Context) (string, error) {
	select {
	case l, ok := <-f.lines:
		if !ok {
			return "", errFakeClosed
		}
		return l, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (f *fakeTransport) Close() error {
	f.closeOnce.Do(func() { close(f.lines) })
	return nil
}

func (f *fakeTransport) feed(line string) { f.lines <- line }

func (f *fakeTransport) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.written...)
}

func dialFake(ft *fakeTransport) DialFunc {
	return func(context.Context) (transport, error) { return ft, nil }
}

func waitForWrite(t *testing.T, ft *fakeTransport, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if slices.Contains(ft.writes(), want) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("never wrote %q; wrote %v", want, ft.writes())
}

func TestAdapter_HandshakeAndJoin(t *testing.T) {
	ft := newFakeTransport()
	a := New(Options{Nick: "justinfan999", Dial: dialFake(ft)})
	t.Cleanup(func() { _ = a.Close() })

	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "Forsen"}, platform.ModeAnonymous); err != nil {
		t.Fatalf("Join: %v", err)
	}
	w := ft.writes()
	for _, want := range []string{capRequest, "NICK justinfan999", "JOIN #forsen"} {
		if !slices.Contains(w, want) {
			t.Errorf("handshake missing %q; wrote %v", want, w)
		}
	}
}

func TestAdapter_EmitsMessage(t *testing.T) {
	ft := newFakeTransport()
	a := New(Options{Dial: dialFake(ft)})
	t.Cleanup(func() { _ = a.Close() })
	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "forsen"}, platform.ModeAnonymous); err != nil {
		t.Fatal(err)
	}

	ft.feed(`@display-name=Alice;id=x1 :alice!alice@alice.tmi.twitch.tv PRIVMSG #forsen :hello world`)

	select {
	case ev := <-a.Events():
		me, ok := ev.(platform.MessageEvent)
		if !ok {
			t.Fatalf("event = %T, want MessageEvent", ev)
		}
		if me.Message.PlainText() != "hello world" || me.Message.Author.DisplayName != "Alice" {
			t.Errorf("message = %+v", me.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message event")
	}
}

func TestAdapter_RespondsToPing(t *testing.T) {
	ft := newFakeTransport()
	a := New(Options{Dial: dialFake(ft)})
	t.Cleanup(func() { _ = a.Close() })
	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "forsen"}, platform.ModeAnonymous); err != nil {
		t.Fatal(err)
	}
	ft.feed("PING :tmi.twitch.tv")
	waitForWrite(t, ft, "PONG :tmi.twitch.tv")
}

func TestAdapter_LeaveSendsPart(t *testing.T) {
	ft := newFakeTransport()
	a := New(Options{Dial: dialFake(ft)})
	t.Cleanup(func() { _ = a.Close() })
	_ = a.Join(context.Background(), platform.ChannelRef{Slug: "forsen"}, platform.ModeAnonymous)
	if err := a.Leave(platform.ChannelRef{Slug: "Forsen"}); err != nil {
		t.Fatal(err)
	}
	waitForWrite(t, ft, "PART #forsen")
}

func TestAdapter_SendAndModerateUnsupported(t *testing.T) {
	a := New(Options{Dial: dialFake(newFakeTransport())})
	t.Cleanup(func() { _ = a.Close() })
	ch := platform.ChannelRef{Slug: "forsen"}
	if err := a.Send(context.Background(), ch, "hi", platform.SendOpts{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Send = %v, want ErrUnsupported", err)
	}
	if err := a.Moderate(context.Background(), platform.ModAction{Type: platform.ModBan, Channel: ch}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Moderate = %v, want ErrUnsupported", err)
	}
}

// The adapter must satisfy the universal read-only adapter contract.
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
	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "x"}, platform.ModeAnonymous); err == nil {
		t.Fatal("Join with failing dial returned nil error")
	}
	if a.Health().State != platform.HealthDown {
		t.Errorf("health = %v, want down after dial failure", a.Health().State)
	}
}
