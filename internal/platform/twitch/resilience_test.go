package twitch

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

// chaosServer hands out a fresh fakeTransport on every dial and remembers each one, so a
// test can drive the "current" connection, kill it, and then inspect the replacement the
// reconnect supervisor dialed.
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

// waitFor polls until cond is true or the deadline passes.
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

// fastBackoff keeps reconnect delays sub-millisecond so resilience tests run quickly.
func fastBackoff() Options {
	return Options{BackoffBase: time.Millisecond, BackoffMax: 2 * time.Millisecond}
}

func privmsgLine(id, text string) string {
	return "@id=" + id + " :u!u@u.tmi.twitch.tv PRIVMSG #forsen :" + text
}

// collectIDs drains MessageEvents from the adapter in the background, recording their ids,
// until the returned stop function is called.
func collectIDs(a *Adapter) (ids func() []string, stop func()) {
	var mu sync.Mutex
	var got []string
	done := make(chan struct{})
	var once sync.Once
	go func() {
		for {
			select {
			case <-done:
				return
			case ev, ok := <-a.Events():
				if !ok {
					return
				}
				if me, isMsg := ev.(platform.MessageEvent); isMsg {
					mu.Lock()
					got = append(got, me.Message.PlatformMessageID)
					mu.Unlock()
				}
			}
		}
	}()
	return func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), got...)
		}, func() {
			once.Do(func() { close(done) })
		}
}

func TestAdapter_ReconnectsAfterDrop(t *testing.T) {
	cs := &chaosServer{}
	opts := fastBackoff()
	opts.Dial = cs.dial
	a := New(opts)
	t.Cleanup(func() { _ = a.Close() })

	ids, stop := collectIDs(a)
	t.Cleanup(stop)

	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "forsen"}, platform.ModeAnonymous); err != nil {
		t.Fatalf("Join: %v", err)
	}
	c1 := cs.conn(0)
	waitForWrite(t, c1, "JOIN #forsen")

	c1.feed(privmsgLine("m1", "before the drop"))
	waitFor(t, "m1 delivered", func() bool { return slices.Contains(ids(), "m1") })

	// Kill the connection mid-stream; the supervisor must dial a replacement and rejoin.
	_ = c1.Close()
	waitFor(t, "reconnect dialed", func() bool { return cs.count() >= 2 })
	c2 := cs.conn(1)
	waitForWrite(t, c2, "JOIN #forsen") // membership restored without a caller re-joining

	c2.feed(privmsgLine("m2", "after the drop"))
	waitFor(t, "m2 delivered", func() bool { return slices.Contains(ids(), "m2") })

	// Health must have returned to OK after the reconnect.
	waitFor(t, "health recovered", func() bool { return a.Health().State == platform.HealthOK })

	// No message delivered twice, and nothing beyond the deliberate gap was lost.
	got := ids()
	if seen := map[string]int{}; func() bool {
		for _, id := range got {
			seen[id]++
			if seen[id] > 1 {
				return true
			}
		}
		return false
	}() {
		t.Errorf("duplicate delivery: %v", got)
	}
	if !slices.Contains(got, "m1") || !slices.Contains(got, "m2") {
		t.Errorf("ids = %v, want both m1 and m2", got)
	}
}

func TestAdapter_EscalatesToDownThenRecovers(t *testing.T) {
	cs := &chaosServer{}
	var mu sync.Mutex
	failNextDials := 0 // set after the first connect to force reconnect failures

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

	opts := fastBackoff()
	opts.Dial = dial
	a := New(opts)
	t.Cleanup(func() { _ = a.Close() })

	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "forsen"}, platform.ModeAnonymous); err != nil {
		t.Fatalf("Join: %v", err)
	}
	c1 := cs.conn(0)
	waitForWrite(t, c1, "JOIN #forsen")

	// Make the next several reconnect dials fail, then drop the live connection.
	mu.Lock()
	failNextDials = downAfterAttempts + 1
	mu.Unlock()
	_ = c1.Close()

	waitFor(t, "escalation to down", func() bool { return a.Health().State == platform.HealthDown })
	// Once dials are allowed again, the shard recovers on its own.
	waitFor(t, "recovery to ok", func() bool { return a.Health().State == platform.HealthOK })
}

func TestAdapter_ShardsAcrossConnections(t *testing.T) {
	cs := &chaosServer{}
	opts := fastBackoff()
	opts.Dial = cs.dial
	opts.ChannelsPerConn = 2
	a := New(opts)
	t.Cleanup(func() { _ = a.Close() })

	for _, slug := range []string{"a", "b", "c"} {
		if err := a.Join(context.Background(), platform.ChannelRef{Slug: slug}, platform.ModeAnonymous); err != nil {
			t.Fatalf("Join %s: %v", slug, err)
		}
	}
	// Two channels per connection → three channels need two connections.
	if got := cs.count(); got != 2 {
		t.Errorf("opened %d connections for 3 channels at cap 2, want 2", got)
	}

	// The third channel lives on the second connection.
	waitForWrite(t, cs.conn(1), "JOIN #c")
	// Re-joining an existing channel must not open another connection.
	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "a"}, platform.ModeAnonymous); err != nil {
		t.Fatal(err)
	}
	if got := cs.count(); got != 2 {
		t.Errorf("re-join opened a new connection: %d", got)
	}
}

func TestBackoff_GrowsCapsAndJitters(t *testing.T) {
	clk := clock.NewFake(time.Unix(0, 0))
	b := backoff{base: 100 * time.Millisecond, max: time.Second}

	// At a clock instant with zero nanosecond remainder, delay is exactly half the
	// (capped, exponential) interval — the fixed floor of equal jitter.
	for _, tc := range []struct {
		attempt int
		want    time.Duration
	}{
		{1, 50 * time.Millisecond},  // base/2
		{2, 100 * time.Millisecond}, // base*2/2
		{3, 200 * time.Millisecond}, // base*4/2
		{8, 500 * time.Millisecond}, // capped at max/2
	} {
		if got := b.delay(tc.attempt, clk); got != tc.want {
			t.Errorf("delay(attempt=%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}

	// A non-zero nanosecond remainder adds jitter on top of the fixed half, bounded by it.
	clk.Set(time.Unix(0, 500)) // remainder 500/1000 → +half/2
	if got := b.delay(1, clk); got != 50*time.Millisecond+25*time.Millisecond {
		t.Errorf("jittered delay = %v, want 75ms", got)
	}
}
