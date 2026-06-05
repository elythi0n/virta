package logbook

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elythi0n/virta/internal/clock"
)

// Sweeper enforces retention: it periodically deletes logged messages older than the
// configured window, per channel the Sink has written to. "forever" (or an unset/count-based
// policy) disables time sweeping. Count-based retention ("N messages/channel") is a follow-up.
type Sweeper struct {
	sink *Sink
	clk  clock.Clock

	retain atomic.Int64 // retention window in seconds; 0 = keep forever (no sweep)

	interval  time.Duration
	quit      chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
	closeOnce sync.Once
}

// NewSweeper builds a retention sweeper over the sink's logged channels.
func NewSweeper(sink *Sink, clk clock.Clock) *Sweeper {
	return &Sweeper{sink: sink, clk: clk, interval: sweepEvery, quit: make(chan struct{})}
}

// SetRetention configures the policy from a profile string ("7d", "30d", "forever", …). An
// unrecognized or count-based value disables time sweeping (returns false).
func (s *Sweeper) SetRetention(policy string) bool {
	d, ok := parseRetention(policy)
	if !ok {
		s.retain.Store(0)
		return false
	}
	s.retain.Store(int64(d / time.Second))
	return true
}

// Start launches the periodic sweep loop. Idempotent.
func (s *Sweeper) Start() {
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.loop()
	})
}

func (s *Sweeper) loop() {
	defer s.wg.Done()
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-t.C:
			s.sweep()
		}
	}
}

// sweep removes messages older than the retention window across all logged channels.
func (s *Sweeper) sweep() {
	secs := s.retain.Load()
	if secs <= 0 {
		return // keep forever
	}
	cutoff := s.clk.Now().Add(-time.Duration(secs) * time.Second)
	for _, ch := range s.sink.loggedChannels() {
		if n, err := s.sink.store.Sweep(context.Background(), ch, cutoff); err != nil {
			s.sink.log.Warn("logbook sweep failed", "channel", ch, "err", err)
		} else if n > 0 {
			s.sink.log.Info("logbook swept old messages", "channel", ch, "removed", n)
		}
	}
}

// Close stops the sweep loop. Idempotent.
func (s *Sweeper) Close() error {
	s.closeOnce.Do(func() {
		close(s.quit)
		s.wg.Wait()
	})
	return nil
}

// parseRetention turns a policy string into a duration. "forever"/""/count-based → not a
// time window (ok=false). Accepts "<n>d" and "<n>h".
func parseRetention(policy string) (time.Duration, bool) {
	p := strings.TrimSpace(strings.ToLower(policy))
	if p == "" || p == "forever" {
		return 0, false
	}
	if strings.HasSuffix(p, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(p, "d")); err == nil && n > 0 {
			return time.Duration(n) * 24 * time.Hour, true
		}
	}
	if strings.HasSuffix(p, "h") {
		if n, err := strconv.Atoi(strings.TrimSuffix(p, "h")); err == nil && n > 0 {
			return time.Duration(n) * time.Hour, true
		}
	}
	return 0, false // count-based ("N") or unrecognized: no time sweep
}
