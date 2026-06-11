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
	"sync/atomic"
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
	// ESDial opens EventSub WebSockets. nil disables the authenticated EventSub read path
	// entirely (reads stay on IRC even when signed in) — which also keeps offline tests that
	// call Authenticate from ever attempting a network dial.
	ESDial ESDialFunc
}

// Adapter is a Twitch chat adapter. Anonymous it reads over IRC, distributing joined channels
// across one or more self-healing connection shards. Authenticated (with ESDial configured) it
// migrates reads to EventSub per channel — richer events plus the AutoMod held queue — falling
// back to IRC whenever a channel's EventSub subscriptions aren't live.
type Adapter struct {
	nick    string
	dial    DialFunc
	esDial  ESDialFunc
	clk     clock.Clock
	backoff backoff
	perConn int

	events chan platform.Event
	recent recentIDs // platform-message-id dedupe across the IRC/EventSub overlap

	mu       sync.Mutex
	shards   []*shard
	shardSeq uint64                // distinct per-shard jitter seed source
	health   platform.HealthStatus // floor for initial-connect failures (no shard retained)
	closed   bool
	joined   map[string]bool // desired channel set, independent of which transport carries each
	es       *esSupervisor   // nil until Authenticate starts the EventSub read path

	ctx    context.Context
	cancel context.CancelFunc

	// auth is nil when anonymous (read-only); Authenticate sets it to enable sending.
	auth atomic.Pointer[twitchAuth]
}

// TokenFunc returns a currently-valid access token for the authenticated account (refreshing as
// needed). It is injected so the adapter never imports the auth manager.
type TokenFunc func(ctx context.Context) (string, error)

// BroadcasterResolver turns a channel login into the numeric Twitch user id that Helix send and
// moderation require. Resolution is a network lookup, so it is injected; its live behavior is
// tracked separately.
type BroadcasterResolver func(ctx context.Context, login string) (string, error)

// twitchAuth holds the authenticated send path: the account's sender id, a token source, the
// Helix client, and a broadcaster-id resolver (with a small per-login cache).
type twitchAuth struct {
	senderID string
	tokens   TokenFunc
	helix    *HelixClient
	resolve  BroadcasterResolver

	mu  sync.Mutex
	bid map[string]string // login → broadcaster id, resolved once
}

func (au *twitchAuth) broadcasterID(ctx context.Context, login string) (string, error) {
	au.mu.Lock()
	id, ok := au.bid[login]
	au.mu.Unlock()
	if ok {
		return id, nil
	}
	if au.resolve == nil {
		return "", fmt.Errorf("twitch: cannot resolve broadcaster id for %q", login)
	}
	id, err := au.resolve(ctx, login)
	if err != nil {
		return "", err
	}
	au.mu.Lock()
	au.bid[login] = id
	au.mu.Unlock()
	return id, nil
}

// targetUserID returns a numeric Twitch user id for raw, which is either already a numeric id
// (used directly) or a login resolved via Helix and cached. Moderation endpoints address users by
// numeric id, while slash commands name them by login, so this absorbs the difference.
func (au *twitchAuth) targetUserID(ctx context.Context, token, raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("twitch: moderation target required")
	}
	if isAllDigits(raw) {
		return raw, nil
	}
	login := strings.ToLower(raw)
	au.mu.Lock()
	id, ok := au.bid[login]
	au.mu.Unlock()
	if ok {
		return id, nil
	}
	id, err := au.helix.UserID(ctx, token, login)
	if err != nil {
		return "", err
	}
	au.mu.Lock()
	au.bid[login] = id
	au.mu.Unlock()
	return id, nil
}

// isAllDigits reports whether s is non-empty and entirely ASCII digits — a numeric user id rather
// than a login.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Authenticate switches the adapter to authenticated mode for senderID, enabling Send. tokens and
// helix are required; resolve turns a channel login into its broadcaster id. When the adapter was
// built with ESDial it also starts the EventSub supervisor, which migrates reads channel by
// channel from IRC to EventSub (and back on failure). Idempotent; call Deauthenticate to revert
// to anonymous read-only.
func (a *Adapter) Authenticate(senderID string, tokens TokenFunc, helix *HelixClient, resolve BroadcasterResolver) {
	if tokens == nil || helix == nil {
		return
	}
	au := &twitchAuth{senderID: senderID, tokens: tokens, helix: helix, resolve: resolve, bid: map[string]string{}}
	a.auth.Store(au)

	if a.esDial == nil {
		return
	}
	a.mu.Lock()
	if a.closed || a.es != nil {
		a.mu.Unlock()
		return
	}
	es := newESSupervisor(a.ctx, a.esDial, helix, tokens, senderID,
		func(ctx context.Context, login string) (string, error) { return au.broadcasterID(ctx, login) },
		a.emit, a.onESState, a.clk, a.backoff)
	a.es = es
	slugs := make([]string, 0, len(a.joined))
	for slug := range a.joined {
		slugs = append(slugs, slug)
	}
	a.mu.Unlock()
	for _, slug := range slugs {
		es.join(slug)
	}
}

// Deauthenticate drops the authenticated send path (e.g. on sign-out), reverting to read-only.
// Channels reading over EventSub move back to IRC first, so the feed survives the sign-out.
func (a *Adapter) Deauthenticate() {
	a.mu.Lock()
	es := a.es
	a.es = nil
	var rejoin []string
	if es != nil {
		for slug := range a.joined {
			if es.channelUp(slug) {
				rejoin = append(rejoin, slug)
			}
		}
	}
	a.mu.Unlock()
	if es != nil {
		es.close()
	}
	for _, slug := range rejoin {
		_ = a.ircJoin(a.ctx, slug)
	}
	a.auth.Store(nil)
}

// ResolveID resolves a channel login to its numeric broadcaster id using the authenticated
// Helix resolver. Returns an error if the adapter is not authenticated or resolution fails.
// Used by the emote stage to populate ChannelRef.ID for third-party emote lookups.
func (a *Adapter) ResolveID(ctx context.Context, slug string) (string, error) {
	au := a.auth.Load()
	if au == nil {
		return "", fmt.Errorf("twitch: not authenticated")
	}
	return au.broadcasterID(ctx, strings.ToLower(slug))
}

// onESState is the supervisor's migration hook: a channel whose EventSub reads came up leaves
// IRC; one whose EventSub reads went down rejoins IRC. Either direction is a transport swap —
// the channel stays joined throughout.
func (a *Adapter) onESState(slug string, up bool) {
	a.mu.Lock()
	stillJoined := a.joined[slug]
	a.mu.Unlock()
	if !stillJoined {
		return
	}
	if up {
		a.ircLeave(slug)
	} else {
		_ = a.ircJoin(a.ctx, slug)
	}
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
		esDial:  opts.ESDial,
		clk:     clk,
		backoff: bo,
		perConn: perConn,
		events:  make(chan platform.Event, 256),
		health:  platform.HealthStatus{State: platform.HealthOK},
		joined:  map[string]bool{},
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (a *Adapter) Platform() platform.Platform { return platform.Twitch }

func (a *Adapter) Capabilities() platform.Capabilities {
	c := platform.Capabilities{ReadAnonymous: true, Stability: platform.TierOfficial}
	if a.auth.Load() != nil {
		c.ReadAuthed = true
		c.Send = true
		c.Replies = true
		c.Moderation = true
		c.HeldQueue = true
	}
	return c
}

// Join registers the channel and connects its reads: IRC immediately (so the feed flows with no
// wait), plus an EventSub subscription when authenticated — the supervisor's up-callback then
// retires the channel's IRC membership.
func (a *Adapter) Join(ctx context.Context, ch platform.ChannelRef, _ platform.ConnMode) error {
	slug := strings.ToLower(ch.Slug)
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return fmt.Errorf("twitch: adapter closed")
	}
	a.joined[slug] = true
	es := a.es
	a.mu.Unlock()
	if err := a.ircJoin(ctx, slug); err != nil {
		return err
	}
	if es != nil {
		es.join(slug)
	}
	return nil
}

// ircJoin routes the channel to a shard with spare capacity, opening a new connection when all
// existing shards are full.
func (a *Adapter) ircJoin(ctx context.Context, slug string) error {
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

// Leave unregisters the channel from both transports.
func (a *Adapter) Leave(ch platform.ChannelRef) error {
	slug := strings.ToLower(ch.Slug)
	a.mu.Lock()
	delete(a.joined, slug)
	es := a.es
	a.mu.Unlock()
	a.ircLeave(slug)
	if es != nil {
		es.leave(slug)
	}
	return nil
}

// ircLeave parts the channel from whichever shard holds it, if any.
func (a *Adapter) ircLeave(slug string) {
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
}

// Send posts a message to the channel via Helix when authenticated; it is unsupported on an
// anonymous connection. A /me action is sent as Twitch's in-chat action command.
func (a *Adapter) Send(ctx context.Context, ch platform.ChannelRef, text string, opts platform.SendOpts) error {
	au := a.auth.Load()
	if au == nil {
		return platform.ErrUnsupported
	}
	bid, err := au.broadcasterID(ctx, strings.ToLower(ch.Slug))
	if err != nil {
		return err
	}
	tok, err := au.tokens(ctx)
	if err != nil {
		return err
	}
	if opts.Action {
		text = "/me " + text
	}
	_, err = au.helix.SendChat(ctx, tok, bid, au.senderID, text, opts.ReplyParentID)
	return err
}

// Moderate performs a typed moderation action via Helix when authenticated; it is unsupported on
// an anonymous connection. The acting moderator is the authenticated account (its sender id); the
// channel's broadcaster id and any target user are resolved as needed. Held approve/deny act as
// the moderator alone and need no channel.
func (a *Adapter) Moderate(ctx context.Context, action platform.ModAction) error {
	au := a.auth.Load()
	if au == nil {
		return platform.ErrUnsupported
	}
	tok, err := au.tokens(ctx)
	if err != nil {
		return err
	}
	switch action.Type {
	case platform.ModApproveHeld:
		return au.helix.ManageHeldMessage(ctx, tok, au.senderID, action.TargetMessageID, true)
	case platform.ModDenyHeld:
		return au.helix.ManageHeldMessage(ctx, tok, au.senderID, action.TargetMessageID, false)
	}

	bid, err := au.broadcasterID(ctx, strings.ToLower(action.Channel.Slug))
	if err != nil {
		return err
	}
	switch action.Type {
	case platform.ModBan:
		uid, err := au.targetUserID(ctx, tok, action.TargetUserID)
		if err != nil {
			return err
		}
		return au.helix.Ban(ctx, tok, bid, au.senderID, uid, 0, action.Reason)
	case platform.ModTimeout:
		uid, err := au.targetUserID(ctx, tok, action.TargetUserID)
		if err != nil {
			return err
		}
		return au.helix.Ban(ctx, tok, bid, au.senderID, uid, clampTimeout(action.Duration), action.Reason)
	case platform.ModUnban, platform.ModUntimeout:
		uid, err := au.targetUserID(ctx, tok, action.TargetUserID)
		if err != nil {
			return err
		}
		return au.helix.Unban(ctx, tok, bid, au.senderID, uid)
	case platform.ModDeleteMessage:
		return au.helix.DeleteMessage(ctx, tok, bid, au.senderID, action.TargetMessageID)
	case platform.ModClear:
		return au.helix.ClearChat(ctx, tok, bid, au.senderID)
	case platform.ModSetSlow, platform.ModSetFollowers, platform.ModSetEmoteOnly, platform.ModSetUniqueChat:
		return au.helix.UpdateChatSettings(ctx, tok, bid, au.senderID, chatSettingsPatch(action))
	default:
		return platform.ErrUnsupported
	}
}

// twitchMaxTimeout is Twitch's ceiling for a timeout (14 days, in seconds).
const twitchMaxTimeout = 1_209_600

// clampTimeout converts a timeout duration to seconds within Twitch's bounds, defaulting a
// non-positive duration to ten minutes.
func clampTimeout(d time.Duration) int {
	secs := int(d / time.Second)
	if secs <= 0 {
		return 600
	}
	if secs > twitchMaxTimeout {
		return twitchMaxTimeout
	}
	return secs
}

// chatSettingsPatch maps a set_* toggle to the Helix chat-settings body, including the wait time
// or follow age only when the mode is being enabled.
func chatSettingsPatch(a platform.ModAction) map[string]any {
	switch a.Type {
	case platform.ModSetSlow:
		m := map[string]any{"slow_mode": a.Enabled}
		if a.Enabled {
			m["slow_mode_wait_time"] = int(a.Duration / time.Second)
		}
		return m
	case platform.ModSetFollowers:
		m := map[string]any{"follower_mode": a.Enabled}
		if a.Enabled {
			m["follower_mode_duration"] = int(a.Duration / time.Minute)
		}
		return m
	case platform.ModSetEmoteOnly:
		return map[string]any{"emote_mode": a.Enabled}
	case platform.ModSetUniqueChat:
		return map[string]any{"unique_chat_mode": a.Enabled}
	}
	return map[string]any{}
}

func (a *Adapter) Events() <-chan platform.Event { return a.events }

// Health reports the worst state across all shards (so any one connection reconnecting or
// down is visible adapter-wide), falling back to the initial-connect floor when there are
// no shards yet.
func (a *Adapter) Health() platform.HealthStatus {
	a.mu.Lock()
	shards := append([]*shard(nil), a.shards...)
	es := a.es
	worst := a.health
	a.mu.Unlock()
	for _, sh := range shards {
		if h := sh.healthStatus(); healthRank(h.State) > healthRank(worst.State) {
			worst = h
		}
	}
	// The EventSub side never worsens past degraded here: IRC fallback keeps reads flowing, so
	// a down supervisor is a capability loss (held queue, richer events), not an outage.
	if es != nil {
		if h := es.healthStatus(); h.State != platform.HealthOK && healthRank(platform.HealthDegraded) > healthRank(worst.State) {
			worst = platform.HealthStatus{State: platform.HealthDegraded, Reason: h.Reason, Detail: h.Detail}
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
	es := a.es
	a.es = nil
	a.mu.Unlock()

	a.cancel()
	if es != nil {
		es.close()
	}
	for _, sh := range shards {
		sh.close()
	}
	close(a.events)
	return nil
}

// emit sends an event unless the adapter is shutting down (avoids sending on a closed
// channel during Close). Chat messages are deduped by platform message id: during a per-channel
// IRC→EventSub migration both transports briefly carry the same traffic, and the feed must not
// show it twice.
func (a *Adapter) emit(ev platform.Event) {
	if me, ok := ev.(platform.MessageEvent); ok && me.Message.PlatformMessageID != "" {
		if a.recent.seen(me.Message.PlatformMessageID) {
			return
		}
	}
	select {
	case <-a.ctx.Done():
	case a.events <- ev:
	}
}

// recentIDs is a fixed-size set of recently emitted platform message ids — enough capacity to
// cover the seconds-long transport overlap window without growing unbounded.
type recentIDs struct {
	mu    sync.Mutex
	set   map[string]struct{}
	order []string
	next  int
}

const recentIDCap = 4096

// seen records id and reports whether it was already present.
func (r *recentIDs) seen(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.set == nil {
		r.set = make(map[string]struct{}, recentIDCap)
		r.order = make([]string, recentIDCap)
	}
	if _, dup := r.set[id]; dup {
		return true
	}
	if old := r.order[r.next]; old != "" {
		delete(r.set, old)
	}
	r.order[r.next] = id
	r.next = (r.next + 1) % recentIDCap
	r.set[id] = struct{}{}
	return false
}

var _ platform.Adapter = (*Adapter)(nil)
