package ratelimit

import (
	"errors"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
)

// limited is a stand-in for a platform's rate-limit error.
type limited struct{ wait time.Duration }

func (l *limited) Error() string              { return "rate limited" }
func (l *limited) RateLimited() time.Duration { return l.wait }

func newAdaptive(t *testing.T) (*Adaptive, *Governor, *clock.Fake) {
	t.Helper()
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	def := Limit{Burst: 20, Window: 30 * time.Second}
	g := New(clk, def)
	a := NewAdaptive(g, clk, def)
	return a, g, clk
}

func TestAdaptive_SeedAppliedByPrefix(t *testing.T) {
	a, _, _ := newAdaptive(t)
	a.SetSeed("kick:", Limit{Burst: 5, Window: 30 * time.Second})

	<-a.Submit("kick:xqc", func() error { return nil })
	if got := a.Limit("kick:xqc"); got.Burst != 5 {
		t.Errorf("seeded burst = %d, want 5", got.Burst)
	}
	// A key outside the prefix keeps the governor default.
	<-a.Submit("twitch:forsen", func() error { return nil })
	if got := a.Limit("twitch:forsen"); got.Burst != 20 {
		t.Errorf("unseeded burst = %d, want 20", got.Burst)
	}
}

func TestAdaptive_TightensOnRateLimit(t *testing.T) {
	a, _, clk := newAdaptive(t)
	a.SetSeed("kick:", Limit{Burst: 8, Window: 30 * time.Second})

	err := <-a.Submit("kick:xqc", func() error { return &limited{} })
	var rl *limited
	if !errors.As(err, &rl) {
		t.Fatalf("submit swallowed the error: %v", err)
	}
	if got := a.Limit("kick:xqc"); got.Burst != 4 {
		t.Errorf("burst after 429 = %d, want 4", got.Burst)
	}
	// Repeated hits keep halving but never reach zero; the bucket keeps refilling between
	// them (a tightened channel is slower, never stalled).
	for i := 0; i < 5; i++ {
		clk.Advance(31 * time.Second) // ≥ one full token even at the tightest limit (1/30s)
		<-a.Submit("kick:xqc", func() error { return &limited{} })
	}
	if got := a.Limit("kick:xqc"); got.Burst != 1 {
		t.Errorf("burst after repeated 429s = %d, want 1", got.Burst)
	}
}

func TestAdaptive_HonorsRetryAfter(t *testing.T) {
	a, _, clk := newAdaptive(t)
	a.SetSeed("kick:", Limit{Burst: 8, Window: 30 * time.Second})

	<-a.Submit("kick:xqc", func() error { return &limited{wait: time.Minute} })

	// The next send stays queued until the suggested wait has passed.
	done := a.Submit("kick:xqc", func() error { return nil })
	a.Drain()
	select {
	case <-done:
		t.Fatal("send dispatched during the retry-after window")
	default:
	}
	if queued, nextIn := a.State("kick:xqc"); queued != 1 || nextIn <= 50*time.Second {
		t.Errorf("state = %d queued, next in %s; want 1 queued ≈60s", queued, nextIn)
	}

	clk.Advance(61 * time.Second)
	a.Drain()
	if err := <-done; err != nil {
		t.Fatalf("send after the window: %v", err)
	}
}

func TestAdaptive_RecoversAfterQuiet(t *testing.T) {
	a, _, clk := newAdaptive(t)
	a.SetSeed("kick:", Limit{Burst: 8, Window: 8 * time.Second}) // 1 token/s — fast refill for the test
	a.SetQuiet(10 * time.Second)

	<-a.Submit("kick:xqc", func() error { return &limited{} })
	<-func() <-chan error {
		clk.Advance(2 * time.Second)
		ch := a.Submit("kick:xqc", func() error { return &limited{} })
		a.Drain()
		return ch
	}()
	if got := a.Limit("kick:xqc"); got.Burst != 2 {
		t.Fatalf("burst after two 429s = %d, want 2", got.Burst)
	}

	// Successful sends inside the quiet period do not recover.
	clk.Advance(5 * time.Second)
	ch := a.Submit("kick:xqc", func() error { return nil })
	a.Drain()
	<-ch
	if got := a.Limit("kick:xqc"); got.Burst != 2 {
		t.Errorf("burst recovered too early: %d", got.Burst)
	}

	// After a quiet stretch, one success steps the burst up; further quiet stretches restore
	// the seed but never overshoot it.
	for want := 4; want <= 8; want *= 2 {
		clk.Advance(11 * time.Second)
		ch = a.Submit("kick:xqc", func() error { return nil })
		a.Drain()
		<-ch
		if got := a.Limit("kick:xqc"); got.Burst != want {
			t.Fatalf("burst = %d, want %d", got.Burst, want)
		}
	}
	clk.Advance(11 * time.Second)
	ch = a.Submit("kick:xqc", func() error { return nil })
	a.Drain()
	<-ch
	if got := a.Limit("kick:xqc"); got.Burst != 8 {
		t.Errorf("burst overshot the seed: %d", got.Burst)
	}
}

func TestAdaptive_OtherErrorsDoNotTighten(t *testing.T) {
	a, _, _ := newAdaptive(t)
	a.SetSeed("kick:", Limit{Burst: 8, Window: 30 * time.Second})
	<-a.Submit("kick:xqc", func() error { return errors.New("network down") })
	if got := a.Limit("kick:xqc"); got.Burst != 8 {
		t.Errorf("plain error changed the limit: burst %d", got.Burst)
	}
}

func TestGovernor_Penalize(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	g := New(clk, Limit{Burst: 10, Window: 10 * time.Second}) // 1 token/s

	// A fresh bucket is full; a 30s penalty empties it and pushes the next send out 30s.
	g.Penalize("k", 30*time.Second)
	done := g.Submit("k", func() error { return nil })
	select {
	case <-done:
		t.Fatal("send dispatched through a penalty")
	default:
	}
	if queued, nextIn := g.State("k"); queued != 1 || nextIn < 29*time.Second || nextIn > 31*time.Second {
		t.Errorf("state = %d queued, next in %s; want 1 queued ≈30s", queued, nextIn)
	}
	clk.Advance(31 * time.Second)
	g.Drain()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// A zero-wait penalty still empties the bucket: the next send waits one refill interval.
	g.Penalize("k", 0)
	done2 := g.Submit("k", func() error { return nil })
	select {
	case <-done2:
		t.Fatal("send dispatched straight through a zero-wait penalty")
	default:
	}
	if _, nextIn := g.State("k"); nextIn <= 0 {
		t.Error("zero-wait penalty left tokens in the bucket")
	}
	clk.Advance(2 * time.Second)
	g.Drain()
	if err := <-done2; err != nil {
		t.Fatal(err)
	}
}
