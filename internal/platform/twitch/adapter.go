// Package twitch implements the platform.Adapter contract for Twitch chat. It reads chat
// over Twitch's IRC interface (anonymously, with no account, using a justinfan nick) and
// normalizes each message into a UnifiedMessage. Sending and moderation require an
// authenticated connection and arrive later; an anonymous adapter is read-only.
package twitch

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/elythi0n/virta/internal/platform"
)

// defaultNick is the anonymous login. Twitch accepts any "justinfan" + digits as a
// read-only, password-less connection.
const defaultNick = "justinfan12345"

// capabilities requested on connect: message tags (badges, color, emotes, ids), Twitch
// commands (USERNOTICE, CLEARCHAT, …), and membership (JOIN/PART).
const capRequest = "CAP REQ :twitch.tv/tags twitch.tv/commands twitch.tv/membership"

// transport is a line-oriented connection to Twitch IRC. The real implementation runs over
// a WebSocket; tests inject a fake so the adapter's handshake and read loop are exercised
// without a network.
type transport interface {
	WriteLine(ctx context.Context, line string) error
	ReadLine(ctx context.Context) (string, error)
	Close() error
}

// DialFunc opens a transport. It's injected so the real WebSocket dialer can be swapped for
// a fake in tests.
type DialFunc func(ctx context.Context) (transport, error)

// Options configure an anonymous Twitch adapter.
type Options struct {
	Nick string   // anonymous login; defaults to a justinfan nick
	Dial DialFunc // transport opener; defaults to the WebSocket dialer
}

// Adapter is an anonymous, read-only Twitch chat adapter. One connection serves all joined
// channels (connection sharding for very large channel counts comes later).
type Adapter struct {
	nick string
	dial DialFunc

	events chan platform.Event

	mu      sync.Mutex
	joined  map[string]struct{} // channel slugs
	conn    transport
	started bool
	health  platform.HealthStatus

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed bool
}

// New creates an anonymous Twitch adapter. It does not connect until the first Join.
func New(opts Options) *Adapter {
	nick := opts.Nick
	if nick == "" {
		nick = defaultNick
	}
	dial := opts.Dial
	if dial == nil {
		dial = dialWebSocket
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Adapter{
		nick:   nick,
		dial:   dial,
		events: make(chan platform.Event, 256),
		joined: map[string]struct{}{},
		health: platform.HealthStatus{State: platform.HealthOK},
		ctx:    ctx,
		cancel: cancel,
	}
}

func (a *Adapter) Platform() platform.Platform { return platform.Twitch }

func (a *Adapter) Capabilities() platform.Capabilities {
	return platform.Capabilities{
		ReadAnonymous: true,
		Stability:     platform.TierOfficial,
	}
}

// Join connects (on first call) and joins the channel. Anonymous mode is the only mode this
// adapter supports today.
func (a *Adapter) Join(ctx context.Context, ch platform.ChannelRef, _ platform.ConnMode) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return fmt.Errorf("twitch: adapter closed")
	}
	if !a.started {
		if err := a.connectLocked(ctx); err != nil {
			return err
		}
	}
	slug := strings.ToLower(ch.Slug)
	if _, ok := a.joined[slug]; ok {
		return nil
	}
	if err := a.conn.WriteLine(ctx, "JOIN #"+slug); err != nil {
		return fmt.Errorf("twitch: join %s: %w", slug, err)
	}
	a.joined[slug] = struct{}{}
	return nil
}

func (a *Adapter) Leave(ch platform.ChannelRef) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	slug := strings.ToLower(ch.Slug)
	if _, ok := a.joined[slug]; !ok {
		return nil
	}
	delete(a.joined, slug)
	if a.conn != nil {
		_ = a.conn.WriteLine(a.ctx, "PART #"+slug)
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
		a.wg.Wait() // let the read loop finish before closing the channel it sends on
	}
	close(a.events)
	return nil
}

// connectLocked dials, performs the anonymous handshake, and starts the read loop. Caller
// holds a.mu.
func (a *Adapter) connectLocked(ctx context.Context) error {
	conn, err := a.dial(ctx)
	if err != nil {
		a.setHealthLocked(platform.HealthStatus{State: platform.HealthDown, Reason: platform.ReasonUpstreamDown, Detail: err.Error()})
		return fmt.Errorf("twitch: dial: %w", err)
	}
	// Anonymous handshake: request capabilities, then log in with the justinfan nick.
	for _, line := range []string{capRequest, "NICK " + a.nick} {
		if err := conn.WriteLine(ctx, line); err != nil {
			_ = conn.Close()
			return fmt.Errorf("twitch: handshake: %w", err)
		}
	}
	a.conn = conn
	a.started = true
	a.setHealthLocked(platform.HealthStatus{State: platform.HealthOK})
	a.wg.Add(1)
	go a.readLoop(conn)
	return nil
}

// readLoop reads lines until the connection closes or the adapter is shut down, normalizing
// PRIVMSGs into events and answering PINGs to keep the connection alive.
func (a *Adapter) readLoop(conn transport) {
	defer a.wg.Done()
	for {
		line, err := conn.ReadLine(a.ctx)
		if err != nil {
			a.onDisconnect(err)
			return
		}
		msg, ok := parseLine(line)
		if !ok {
			continue
		}
		// PING needs a reply rather than an event, so handle it directly; everything else
		// that maps to an event is emitted.
		if msg.command == "PING" {
			_ = conn.WriteLine(a.ctx, "PONG :"+msg.trailing())
			continue
		}
		if ev, ok := eventFromLine(msg); ok {
			a.emit(ev)
		}
	}
}

// onDisconnect records a degraded state and emits a health event. Reconnection is added
// separately; for now a dropped connection simply surfaces as down.
func (a *Adapter) onDisconnect(err error) {
	a.mu.Lock()
	closed := a.closed
	a.mu.Unlock()
	if closed {
		return // expected: we're shutting down
	}
	a.setHealth(platform.HealthStatus{State: platform.HealthDown, Reason: platform.ReasonUpstreamDown, Detail: err.Error()})
	a.emit(platform.HealthEvent{Status: platform.HealthStatus{State: platform.HealthDown, Reason: platform.ReasonUpstreamDown}})
}

// emit sends an event unless the adapter is shutting down (avoids sending on a closed
// channel during Close).
func (a *Adapter) emit(ev platform.Event) {
	select {
	case <-a.ctx.Done():
	case a.events <- ev:
	}
}

func (a *Adapter) setHealth(h platform.HealthStatus) {
	a.mu.Lock()
	a.health = h
	a.mu.Unlock()
}

func (a *Adapter) setHealthLocked(h platform.HealthStatus) { a.health = h }

var _ platform.Adapter = (*Adapter)(nil)
