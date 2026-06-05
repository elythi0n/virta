package ratelimit

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
)

// recorder collects the order sends fire in.
type recorder struct {
	mu   sync.Mutex
	sent []string
}

func (r *recorder) fn(id string) func() error {
	return func() error {
		r.mu.Lock()
		r.sent = append(r.sent, id)
		r.mu.Unlock()
		return nil
	}
}

func (r *recorder) order() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sent...)
}

func TestGovernor_BurstPacedInOrderWithCountdown(t *testing.T) {
	clk := clock.NewFake(time.Unix(0, 0))
	// 2 tokens, refilled over 2s → 1 token/sec.
	g := New(clk, Limit{Burst: 2, Window: 2 * time.Second})
	rec := &recorder{}

	results := make([]<-chan error, 5)
	for i := 0; i < 5; i++ {
		results[i] = g.Submit("twitch:forsen", rec.fn(string(rune('a'+i))))
	}
	// Burst of 2 sent immediately; 3 queued with a countdown to the next token.
	if got := rec.order(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("immediate sends = %v, want [a b]", got)
	}
	queued, nextIn := g.State("twitch:forsen")
	if queued != 3 {
		t.Errorf("queued = %d, want 3", queued)
	}
	if nextIn <= 0 || nextIn > time.Second {
		t.Errorf("nextIn = %v, want ~1s", nextIn)
	}

	// Advance 1s → one more token → "c" sends, in order.
	clk.Advance(time.Second)
	g.Drain()
	if got := rec.order(); len(got) != 3 || got[2] != "c" {
		t.Errorf("after 1s = %v, want a,b,c", got)
	}

	// Advance enough to drain the rest.
	clk.Advance(10 * time.Second)
	g.Drain()
	if got := rec.order(); len(got) != 5 || got[3] != "d" || got[4] != "e" {
		t.Errorf("final order = %v, want a..e", got)
	}
	// Every submit got a result — nothing dropped silently.
	for i, res := range results {
		select {
		case err := <-res:
			if err != nil {
				t.Errorf("send %d error: %v", i, err)
			}
		default:
			t.Errorf("send %d never reported a result", i)
		}
	}
}

func TestGovernor_ResultPropagatesError(t *testing.T) {
	clk := clock.NewFake(time.Unix(0, 0))
	g := New(clk, Limit{Burst: 5, Window: time.Second})
	boom := errors.New("boom")
	res := g.Submit("k", func() error { return boom })
	if err := <-res; !errors.Is(err, boom) {
		t.Errorf("result = %v, want boom", err)
	}
}

func TestGovernor_PerChannelIndependent(t *testing.T) {
	clk := clock.NewFake(time.Unix(0, 0))
	g := New(clk, Limit{Burst: 1, Window: time.Second})
	rec := &recorder{}
	// Each channel has its own bucket, so both first sends go through immediately.
	g.Submit("twitch:a", rec.fn("a"))
	g.Submit("twitch:b", rec.fn("b"))
	if len(rec.order()) != 2 {
		t.Errorf("independent channels both should send: %v", rec.order())
	}
	// A second send on channel a is queued (its bucket is empty), b unaffected.
	g.Submit("twitch:a", rec.fn("a2"))
	if q, _ := g.State("twitch:a"); q != 1 {
		t.Errorf("channel a queued = %d, want 1", q)
	}
}

func TestGovernor_SetLimitUpgrade(t *testing.T) {
	clk := clock.NewFake(time.Unix(0, 0))
	g := New(clk, Limit{Burst: 1, Window: 30 * time.Second}) // user bucket
	rec := &recorder{}
	g.Submit("k", rec.fn("1")) // uses the 1 token
	g.Submit("k", rec.fn("2")) // queued

	// Upgrade to a moderator bucket: more burst → the queued one can go after a refill window.
	g.SetLimit("k", Limit{Burst: 100, Window: 30 * time.Second})
	clk.Advance(time.Second)
	g.Drain()
	if got := rec.order(); len(got) != 2 {
		t.Errorf("after upgrade+refill, sent = %v, want both", got)
	}
}

func TestGovernor_StateUnknownKey(t *testing.T) {
	g := New(clock.NewFake(time.Unix(0, 0)), Limit{Burst: 1, Window: time.Second})
	if q, n := g.State("nope"); q != 0 || n != 0 {
		t.Errorf("unknown key state = %d, %v", q, n)
	}
}
