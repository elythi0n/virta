// Package twitch implements the platform.Adapter contract for Twitch chat. It reads chat
// over Twitch's IRC interface (anonymously, with no account, using a justinfan nick) and
// normalizes each message into a UnifiedMessage. Sending and moderation require an
// authenticated connection and arrive later; an anonymous adapter is read-only.
//
// Channels are spread across several IRC connections (shards) to stay under a
// per-connection channel cap, and each shard reconnects itself on an unexpected drop —
// rejoining its channels — so the merged feed survives socket churn with only a health blip.
package twitch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

// defaultNick is the anonymous login. Twitch accepts any "justinfan" + digits as a
// read-only, password-less connection.
const defaultNick = "justinfan12345"

// capabilities requested on connect: message tags (badges, color, emotes, ids), Twitch
// commands (USERNOTICE, CLEARCHAT, …), and membership (JOIN/PART).
const capRequest = "CAP REQ :twitch.tv/tags twitch.tv/commands twitch.tv/membership"

// Connection-sharding and reconnect defaults. The channel cap keeps any one socket well
// within Twitch's join limits with headroom; the backoff bounds reconnect storms.
const (
	defaultChannelsPerConn = 20
	defaultBackoffBase     = 500 * time.Millisecond
	defaultBackoffMax      = 30 * time.Second
)

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

// Options configure an anonymous Twitch adapter. Zero values select sensible defaults.
type Options struct {
	Nick            string        // anonymous login; defaults to a justinfan nick
	Dial            DialFunc      // transport opener; defaults to the WebSocket dialer
	Clock           clock.Clock   // time source for reconnect jitter; defaults to the system clock
	ChannelsPerConn int           // max channels per connection before a new shard opens
	BackoffBase     time.Duration // first reconnect delay
	BackoffMax      time.Duration // reconnect delay ceiling
}

// Adapter is an anonymous, read-only Twitch chat adapter. It distributes joined channels
// across one or more self-healing connection shards.
type Adapter struct {
	nick    string
	dial    DialFunc
	clk     clock.Clock
	backoff backoff
	perConn int

	events chan platform.Event

	mu       sync.Mutex
	shards   []*shard
	shardSeq uint64                // distinct per-shard jitter seed source
	health   platform.HealthStatus // floor for initial-connect failures (no shard retained)
	closed   bool

	ctx    context.Context
	cancel context.CancelFunc
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
	clk := opts.Clock
	if clk == nil {
		clk = clock.System{}
	}
	perConn := opts.ChannelsPerConn
	if perConn <= 0 {
		perConn = defaultChannelsPerConn
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
		nick:    nick,
		dial:    dial,
		clk:     clk,
		backoff: bo,
		perConn: perConn,
		events:  make(chan platform.Event, 256),
		health:  platform.HealthStatus{State: platform.HealthOK},
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (a *Adapter) Platform() platform.Platform { return platform.Twitch }

func (a *Adapter) Capabilities() platform.Capabilities {
	return platform.Capabilities{
		ReadAnonymous: true,
		Stability:     platform.TierOfficial,
	}
}

// Join routes the channel to a shard with spare capacity, opening a new connection when all
// existing shards are full. Anonymous mode is the only mode this adapter supports today.
func (a *Adapter) Join(ctx context.Context, ch platform.ChannelRef, _ platform.ConnMode) error {
	slug := strings.ToLower(ch.Slug)
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return fmt.Errorf("twitch: adapter closed")
	}
	for _, sh := range a.shards {
		if sh.has(slug) {
			a.mu.Unlock()
			return nil
		}
	}
	var sh *shard
	for _, candidate := range a.shards {
		if candidate.count() < a.perConn {
			sh = candidate
			break
		}
	}
	if sh == nil {
		// Seed each shard distinctly (counter mixed with the clock) so a fleet dropped
		// together draws independent reconnect jitter.
		a.shardSeq++
		seed := uint64(a.clk.Now().UnixNano()) ^ (a.shardSeq * 0x9e3779b97f4a7c15)
		sh = newShard(a.ctx, a.nick, a.dial, a.backoff, a.emit, seed)
		if err := sh.start(ctx); err != nil {
			a.health = platform.HealthStatus{State: platform.HealthDown, Reason: platform.ReasonUpstreamDown, Detail: err.Error()}
			a.mu.Unlock()
			return err
		}
		a.health = platform.HealthStatus{State: platform.HealthOK}
		a.shards = append(a.shards, sh)
	}
	a.mu.Unlock()
	return sh.join(ctx, slug)
}

// Leave parts the channel from whichever shard holds it.
func (a *Adapter) Leave(ch platform.ChannelRef) error {
	slug := strings.ToLower(ch.Slug)
	a.mu.Lock()
	var sh *shard
	for _, candidate := range a.shards {
		if candidate.has(slug) {
			sh = candidate
			break
		}
	}
	a.mu.Unlock()
	if sh != nil {
		sh.leave(slug)
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

// Health reports the worst state across all shards (so any one connection reconnecting or
// down is visible adapter-wide), falling back to the initial-connect floor when there are
// no shards yet.
func (a *Adapter) Health() platform.HealthStatus {
	a.mu.Lock()
	shards := append([]*shard(nil), a.shards...)
	worst := a.health
	a.mu.Unlock()
	for _, sh := range shards {
		if h := sh.healthStatus(); healthRank(h.State) > healthRank(worst.State) {
			worst = h
		}
	}
	return worst
}

func healthRank(s platform.HealthState) int {
	switch s {
	case platform.HealthDown:
		return 2
	case platform.HealthDegraded:
		return 1
	default:
		return 0
	}
}

// Close shuts the adapter down and closes Events. Cancelling the context first unblocks any
// shard goroutine waiting to emit, so waiting on the shards can't deadlock against the
// event channel.
func (a *Adapter) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	shards := append([]*shard(nil), a.shards...)
	a.mu.Unlock()

	a.cancel()
	for _, sh := range shards {
		sh.close()
	}
	close(a.events)
	return nil
}

// emit sends an event unless the adapter is shutting down (avoids sending on a closed
// channel during Close).
func (a *Adapter) emit(ev platform.Event) {
	select {
	case <-a.ctx.Done():
	case a.events <- ev:
	}
}

var _ platform.Adapter = (*Adapter)(nil)
