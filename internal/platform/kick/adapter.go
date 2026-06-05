// Package kick implements the platform.Adapter contract for Kick chat. It reads chat
// anonymously over Kick's public Pusher WebSocket (the official API delivers chat only by
// webhook, which a local app can't receive), normalizing each ChatMessageEvent into a
// UnifiedMessage. Sending and moderation use Kick's official API and arrive later; an
// anonymous adapter is read-only.
//
// One connection carries every subscribed chatroom (Pusher multiplexes channels), and a
// supervisor reconnects on drop and re-subscribes — so the merged feed survives socket churn
// with only a health blip.
package kick

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

// defaultAppKey is Kick's public Pusher key. It can rotate, so it is a remotely-updatable
// default rather than a hard constant — overridable via Options.AppKey.
const defaultAppKey = "32cbd69e4b950bf97679"

const (
	defaultBackoffBase = 500 * time.Millisecond
	defaultBackoffMax  = 30 * time.Second
	downAfterAttempts  = 5
)

// transport is a message-oriented connection to Pusher: it reads and writes whole JSON
// frames. The real implementation runs over a WebSocket; tests inject a fake.
type transport interface {
	Write(ctx context.Context, b []byte) error
	Read(ctx context.Context) ([]byte, error)
	Close() error
}

// DialFunc opens a transport, injected so the real dialer can be swapped for a fake in tests.
type DialFunc func(ctx context.Context) (transport, error)

// Options configure an anonymous Kick adapter. Zero values select sensible defaults.
type Options struct {
	AppKey      string
	Dial        DialFunc
	Clock       clock.Clock
	BackoffBase time.Duration
	BackoffMax  time.Duration
	// Resolver turns a slug into a subscribable chatroom id when a join doesn't already carry
	// one. Optional: without it, a join must supply ChannelRef.ID directly.
	Resolver *Resolver
}

// Adapter is an anonymous, read-only Kick chat adapter over Pusher.
type Adapter struct {
	dial     DialFunc
	backoff  backoff
	resolver *Resolver

	events chan platform.Event

	mu      sync.Mutex
	subs    map[string]platform.ChannelRef // chatroom id → channel
	conn    transport
	health  platform.HealthStatus
	rng     uint64
	started bool
	closed  bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates an anonymous Kick adapter. It does not connect until the first Join.
func New(opts Options) *Adapter {
	appKey := opts.AppKey
	if appKey == "" {
		appKey = defaultAppKey
	}
	clk := opts.Clock
	if clk == nil {
		clk = clock.System{}
	}
	dial := opts.Dial
	if dial == nil {
		dial = func(ctx context.Context) (transport, error) { return dialPusher(ctx, appKey) }
	}
	bo := backoff{base: opts.BackoffBase, max: opts.BackoffMax}
	if bo.base <= 0 {
		bo.base = defaultBackoffBase
	}
	if bo.max <= 0 {
		bo.max = defaultBackoffMax
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Adapter{
		dial:     dial,
		backoff:  bo,
		resolver: opts.Resolver,
		events:   make(chan platform.Event, 256),
		subs:     map[string]platform.ChannelRef{},
		health:   platform.HealthStatus{State: platform.HealthOK},
		rng:      uint64(clk.Now().UnixNano()) | 1,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (a *Adapter) Platform() platform.Platform { return platform.Kick }

func (a *Adapter) Capabilities() platform.Capabilities {
	return platform.Capabilities{
		ReadAnonymous: true,
		Stability:     platform.TierUnofficial, // unofficial Pusher read path (docs 04)
	}
}

// Join subscribes to a channel's chatroom. When ch.ID is empty the configured resolver turns
// the slug into a chatroom id (failing with a reason code if the lookup is blocked); without
// a resolver, ch.ID must be supplied directly.
func (a *Adapter) Join(ctx context.Context, ch platform.ChannelRef, _ platform.ConnMode) error {
	if ch.ID == "" {
		if a.resolver == nil {
			return fmt.Errorf("kick: channel %q has no chatroom id to subscribe to", ch.Slug)
		}
		id, err := a.resolver.Resolve(ctx, ch.Slug)
		if err != nil {
			return err // carries a reason code (resolver_blocked / channel_not_found)
		}
		ch.ID = id
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return fmt.Errorf("kick: adapter closed")
	}
	if _, ok := a.subs[ch.ID]; ok {
		a.mu.Unlock()
		return nil
	}
	if !a.started {
		if err := a.connectLocked(ctx); err != nil {
			a.mu.Unlock()
			return err
		}
	}
	a.subs[ch.ID] = ch
	conn := a.conn
	a.mu.Unlock()

	if conn != nil {
		if frame, err := subscribeFrame(chatroomChannel(ch.ID)); err == nil {
			_ = conn.Write(a.ctx, frame)
		}
	}
	return nil
}

// Leave unsubscribes from a channel's chatroom.
func (a *Adapter) Leave(ch platform.ChannelRef) error {
	a.mu.Lock()
	if _, ok := a.subs[ch.ID]; !ok {
		a.mu.Unlock()
		return nil
	}
	delete(a.subs, ch.ID)
	conn := a.conn
	a.mu.Unlock()
	if conn != nil {
		_ = conn.Write(a.ctx, []byte(`{"event":"pusher:unsubscribe","data":{"channel":"`+chatroomChannel(ch.ID)+`"}}`))
	}
	return nil
}

// Send and Moderate are unsupported on an anonymous connection.
func (a *Adapter) Send(context.Context, platform.ChannelRef, string, platform.SendOpts) error {
	return platform.ErrUnsupported
}

func (a *Adapter) Moderate(context.Context, platform.ModAction) error {
	return platform.ErrUnsupported
}

func (a *Adapter) Events() <-chan platform.Event { return a.events }

func (a *Adapter) Health() platform.HealthStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.health
}

// Close shuts the adapter down and closes Events.
func (a *Adapter) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	conn := a.conn
	started := a.started
	a.mu.Unlock()

	a.cancel()
	if conn != nil {
		_ = conn.Close()
	}
	if started {
		a.wg.Wait()
	}
	close(a.events)
	return nil
}

// connectLocked dials and starts the supervisor. Caller holds a.mu.
func (a *Adapter) connectLocked(ctx context.Context) error {
	conn, err := a.dial(ctx)
	if err != nil {
		a.health = platform.HealthStatus{State: platform.HealthDown, Reason: platform.ReasonUpstreamDown, Detail: err.Error()}
		return fmt.Errorf("kick: dial: %w", err)
	}
	a.conn = conn
	a.started = true
	a.health = platform.HealthStatus{State: platform.HealthOK}
	a.wg.Add(1)
	go a.supervise(conn)
	return nil
}

// supervise reads from conn until it drops, then reconnects and resumes — until shutdown.
func (a *Adapter) supervise(conn transport) {
	defer a.wg.Done()
	for {
		a.readLoop(conn)
		if a.ctx.Err() != nil {
			return
		}
		next, ok := a.reconnect()
		if !ok {
			return
		}
		conn = next
	}
}

// readLoop dispatches frames until the connection closes or the adapter is shut down. The
// server sends connection_established first (and again after each reconnect), which is when we
// (re)subscribe every chatroom.
func (a *Adapter) readLoop(conn transport) {
	for {
		b, err := conn.Read(a.ctx)
		if err != nil {
			return
		}
		f, err := parseFrame(b)
		if err != nil {
			continue
		}
		switch f.Event {
		case eventPing:
			_ = conn.Write(a.ctx, pongFrame())
		case eventConnEstablished:
			a.subscribeAll(conn)
		case eventChatMessage:
			a.handleChatMessage(f)
		}
	}
}

func (a *Adapter) handleChatMessage(f frame) {
	a.mu.Lock()
	ch, ok := a.subs[chatroomIDFromChannel(f.Channel)]
	a.mu.Unlock()
	if !ok {
		return // a channel we're no longer subscribed to
	}
	payload, err := pusherPayload(f)
	if err != nil {
		return
	}
	msg, err := normalizeChatMessage(payload, ch)
	if err != nil {
		return
	}
	a.emit(platform.MessageEvent{Message: msg})
}

// subscribeAll (re)subscribes every currently joined chatroom on conn.
func (a *Adapter) subscribeAll(conn transport) {
	a.mu.Lock()
	ids := make([]string, 0, len(a.subs))
	for id := range a.subs {
		ids = append(ids, id)
	}
	a.mu.Unlock()
	for _, id := range ids {
		if frame, err := subscribeFrame(chatroomChannel(id)); err == nil {
			_ = conn.Write(a.ctx, frame)
		}
	}
}

// reconnect tears down the dead connection and redials with backoff until it succeeds or the
// adapter is closed. Re-subscription happens when the fresh connection_established arrives.
func (a *Adapter) reconnect() (transport, bool) {
	a.mu.Lock()
	old := a.conn
	a.conn = nil
	a.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}

	for attempt := 1; ; attempt++ {
		if attempt >= downAfterAttempts {
			a.setHealth(platform.HealthStatus{State: platform.HealthDown, Reason: platform.ReasonUpstreamDown})
		} else {
			a.setHealth(platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonReconnecting})
		}
		if !a.sleep(a.backoff.delay(attempt, a.nextRand())) {
			return nil, false
		}
		conn, err := a.dial(a.ctx)
		if err != nil {
			continue
		}
		a.mu.Lock()
		a.conn = conn
		a.mu.Unlock()
		a.setHealth(platform.HealthStatus{State: platform.HealthOK})
		return conn, true
	}
}

func (a *Adapter) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-a.ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// nextRand advances a splitmix64 generator for backoff jitter (only the supervisor calls it).
func (a *Adapter) nextRand() uint64 {
	a.rng += 0x9e3779b97f4a7c15
	z := a.rng
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// setHealth records the new status and emits a HealthEvent only when it actually changed.
func (a *Adapter) setHealth(h platform.HealthStatus) {
	a.mu.Lock()
	changed := a.health.State != h.State || a.health.Reason != h.Reason
	a.health = h
	a.mu.Unlock()
	if changed {
		a.emit(platform.HealthEvent{Status: h})
	}
}

func (a *Adapter) emit(ev platform.Event) {
	select {
	case <-a.ctx.Done():
	case a.events <- ev:
	}
}

var _ platform.Adapter = (*Adapter)(nil)
