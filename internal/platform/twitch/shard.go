package twitch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// downAfterAttempts is how many consecutive reconnect attempts must fail before a shard
// reports "down" rather than "degraded": long enough to ride out a transient blip without
// alarming the user, short enough that a genuinely dead upstream is reported honestly.
const downAfterAttempts = 5

// shard owns a single Twitch IRC connection and the set of channels assigned to it. After
// the initial connect, a supervisor goroutine reads messages and, on an unexpected
// disconnect, reconnects with backoff and rejoins every channel — so a dropped socket
// surfaces only as a brief health blip, never as lost membership. One adapter spreads its
// channels across several shards to stay under the per-connection channel cap.
type shard struct {
	nick    string
	dial    DialFunc
	backoff backoff
	emit    func(platform.Event) // adapter's event sink; safe to call after shutdown (drops)

	mu       sync.Mutex
	channels map[string]struct{} // joined slugs; the source of truth replayed on reconnect
	conn     transport
	health   platform.HealthStatus
	closed   bool
	rng      uint64 // backoff-jitter generator state; only touched by the supervisor goroutine

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newShard(parent context.Context, nick string, dial DialFunc, bo backoff, emit func(platform.Event), seed uint64) *shard {
	ctx, cancel := context.WithCancel(parent)
	return &shard{
		nick:     nick,
		dial:     dial,
		backoff:  bo,
		emit:     emit,
		channels: map[string]struct{}{},
		health:   platform.HealthStatus{State: platform.HealthOK},
		rng:      seed,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// nextRand advances the shard's splitmix64 generator. Each shard is seeded distinctly, so a
// fleet dropped together draws independent backoff jitter without a shared/global RNG (which
// the determinism rules forbid here). Only the supervisor goroutine calls this.
func (s *shard) nextRand() uint64 {
	s.rng += 0x9e3779b97f4a7c15
	z := s.rng
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// start performs the initial, synchronous connect so a caller's first Join surfaces a dial
// failure directly. On success it hands the connection to a supervisor goroutine that owns
// reading and all subsequent reconnection.
func (s *shard) start(ctx context.Context) error {
	conn, err := s.connect(ctx)
	if err != nil {
		s.setHealth(platform.HealthStatus{State: platform.HealthDown, Reason: platform.ReasonUpstreamDown, Detail: err.Error()}, false)
		return err
	}
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
	s.wg.Add(1)
	go s.supervise(conn)
	return nil
}

// connect dials and runs the anonymous handshake (capability request + justinfan login). It
// does not join channels; the caller (initial start) or rejoin (reconnect) does that.
func (s *shard) connect(ctx context.Context) (transport, error) {
	conn, err := s.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("twitch: dial: %w", err)
	}
	for _, line := range []string{capRequest, "NICK " + s.nick} {
		if err := conn.WriteLine(ctx, line); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("twitch: handshake: %w", err)
		}
	}
	return conn, nil
}

// join records the channel and, if currently connected, sends JOIN immediately. The slug is
// kept in the set as the source of truth, so a reconnect replays it. A failed write is not an
// error: the membership is recorded and a (re)connect will (re)join it — duplicate JOINs are
// harmless on Twitch. Recording the slug and reading conn happen in one critical section that
// pairs with reconnect's publish step, so a slug added mid-reconnect is never silently lost
// (it is either replayed by reconnect or written here against the freshly published conn).
func (s *shard) join(_ context.Context, slug string) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("twitch: adapter closed")
	}
	if _, ok := s.channels[slug]; ok {
		s.mu.Unlock()
		return nil
	}
	s.channels[slug] = struct{}{}
	conn := s.conn
	s.mu.Unlock()
	if conn != nil {
		_ = conn.WriteLine(s.ctx, "JOIN #"+slug)
	}
	return nil
}

// leave drops the channel from the set and sends PART if connected.
func (s *shard) leave(slug string) {
	s.mu.Lock()
	if _, ok := s.channels[slug]; !ok {
		s.mu.Unlock()
		return
	}
	delete(s.channels, slug)
	conn := s.conn
	s.mu.Unlock()
	if conn != nil {
		_ = conn.WriteLine(s.ctx, "PART #"+slug)
	}
}

func (s *shard) has(slug string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.channels[slug]
	return ok
}

func (s *shard) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.channels)
}

func (s *shard) healthStatus() platform.HealthStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.health
}

// supervise reads from conn until it drops, then reconnects and resumes — repeating until
// the shard is closed.
func (s *shard) supervise(conn transport) {
	defer s.wg.Done()
	for {
		s.readLoop(conn)
		if s.ctx.Err() != nil {
			return // expected: shutting down
		}
		next, ok := s.reconnect()
		if !ok {
			return
		}
		conn = next
	}
}

// readLoop normalizes lines into events and answers PINGs, returning when the connection
// closes or the shard is shut down.
func (s *shard) readLoop(conn transport) {
	for {
		line, err := conn.ReadLine(s.ctx)
		if err != nil {
			return
		}
		msg, ok := parseLine(line)
		if !ok {
			continue
		}
		// PING needs a reply rather than an event, so handle it directly; everything else
		// that maps to an event is emitted.
		if msg.command == "PING" {
			_ = conn.WriteLine(s.ctx, "PONG :"+msg.trailing())
			continue
		}
		if ev, ok := eventFromLine(msg); ok {
			s.emit(ev)
		}
	}
}

// reconnect tears down the dead connection and retries with backoff until a fresh one is
// established and all channels rejoined, or the shard is closed (returns ok=false). Health
// moves to degraded while retrying and escalates to down after sustained failure.
func (s *shard) reconnect() (transport, bool) {
	s.mu.Lock()
	old := s.conn
	s.conn = nil
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}

	for attempt := 1; ; attempt++ {
		if attempt >= downAfterAttempts {
			s.setHealth(platform.HealthStatus{State: platform.HealthDown, Reason: platform.ReasonUpstreamDown}, true)
		} else {
			s.setHealth(platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonReconnecting}, true)
		}
		if !s.sleep(s.backoff.delay(attempt, s.nextRand())) {
			return nil, false
		}
		conn, err := s.connect(s.ctx)
		if err != nil {
			continue
		}
		// Publish the connection and snapshot the channel set in one critical section: a
		// join() racing this either lands before (its slug is in the snapshot, replayed
		// below) or after (it sees the published conn and writes its own JOIN). Either way
		// the channel is joined; the worst case is a duplicate JOIN, which Twitch ignores.
		s.mu.Lock()
		s.conn = conn
		slugs := make([]string, 0, len(s.channels))
		for slug := range s.channels {
			slugs = append(slugs, slug)
		}
		s.mu.Unlock()
		if err := s.writeJoins(conn, slugs); err != nil {
			s.mu.Lock()
			if s.conn == conn {
				s.conn = nil
			}
			s.mu.Unlock()
			_ = conn.Close()
			continue
		}
		s.setHealth(platform.HealthStatus{State: platform.HealthOK}, true)
		return conn, true
	}
}

// writeJoins replays JOIN for each slug onto a fresh connection.
func (s *shard) writeJoins(conn transport, slugs []string) error {
	for _, slug := range slugs {
		if err := conn.WriteLine(s.ctx, "JOIN #"+slug); err != nil {
			return err
		}
	}
	return nil
}

// sleep waits for d or until the shard is shut down, returning false if shut down.
func (s *shard) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-s.ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// setHealth records the new status and, when emitEvent is set and the state or reason
// actually changed, emits a HealthEvent (so consumers see transitions, not every retry).
func (s *shard) setHealth(h platform.HealthStatus, emitEvent bool) {
	s.mu.Lock()
	changed := s.health.State != h.State || s.health.Reason != h.Reason
	s.health = h
	s.mu.Unlock()
	if emitEvent && changed {
		s.emit(platform.HealthEvent{Status: h})
	}
}

// close shuts the shard down: it cancels the context, closes the connection, and waits for
// the supervisor goroutine to finish so no further events are emitted.
func (s *shard) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	conn := s.conn
	s.mu.Unlock()
	s.cancel()
	if conn != nil {
		_ = conn.Close()
	}
	s.wg.Wait()
}
