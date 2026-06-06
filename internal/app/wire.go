// Package app wires the concrete implementations together. It is the single place allowed
// to import implementation packages (platform adapters, storage backends, secret vaults);
// every other package depends only on the interfaces. Keeping construction here means the
// rest of the codebase never hard-codes a choice of backend.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/api"
	kickauth "github.com/elythi0n/virta/internal/auth/kick"
	twitchauth "github.com/elythi0n/virta/internal/auth/twitch"
	"github.com/elythi0n/virta/internal/badges"
	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/config"
	"github.com/elythi0n/virta/internal/dispatch"
	"github.com/elythi0n/virta/internal/emotes"
	"github.com/elythi0n/virta/internal/engine"
	"github.com/elythi0n/virta/internal/filter"
	"github.com/elythi0n/virta/internal/held"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/logbook"
	"github.com/elythi0n/virta/internal/pipeline"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/kick"
	"github.com/elythi0n/virta/internal/platform/twitch"
	"github.com/elythi0n/virta/internal/profiles"
	"github.com/elythi0n/virta/internal/ratelimit"
	"github.com/elythi0n/virta/internal/scrollback"
	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/secrets/filevault"
	"github.com/elythi0n/virta/internal/secrets/keychain"
	"github.com/elythi0n/virta/internal/stats"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/store/postgres"
	"github.com/elythi0n/virta/internal/store/sqlite"
	"github.com/elythi0n/virta/internal/streams"
	"github.com/elythi0n/virta/internal/velocity"
	"github.com/elythi0n/virta/internal/webui"
)

// SelectVault chooses where credentials are stored: the OS credential store when one is
// available (the strong, preferred option), otherwise an encrypted file under fileVaultDir
// (the fallback for systems with no keychain). The chosen backend can be read from the
// returned vault's Backend method, which the UI surfaces so the user knows where their
// secrets live.
func SelectVault(fileVaultDir string) (secrets.Vault, error) {
	if keychain.Available() {
		return keychain.New(), nil
	}
	return filevault.New(fileVaultDir)
}

// SelectStore opens the storage backend named by cfg.StorageDriver. SQLite is the built-in
// default and needs no setup. Other engines (Postgres, MySQL) are intended to slot in here
// as additional cases backed by their own implementation of the storage interface; until
// then, asking for one returns a clear error rather than silently falling back, so the user
// knows their choice isn't available yet. Switching engines on an existing install is not a
// drop-in swap: a separate migration step copies existing data into the new backend, so the
// old history becomes available only after that copy completes.
func SelectStore(cfg config.Config, clk clock.Clock, gen id.Generator) (store.Store, error) {
	switch cfg.StorageDriver {
	case "", config.StorageSQLite:
		return sqlite.Open(cfg.DBPath, clk, gen)
	case config.StoragePostgres:
		if cfg.DBDSN == "" {
			return nil, fmt.Errorf("storage backend %q requires VIRTA_DB_DSN", cfg.StorageDriver)
		}
		return postgres.Open(cfg.DBDSN, clk, gen)
	case config.StorageMySQL:
		return nil, fmt.Errorf("storage backend %q is not implemented yet (default is %q)", cfg.StorageDriver, config.StorageSQLite)
	default:
		return nil, fmt.Errorf("unknown storage backend %q", cfg.StorageDriver)
	}
}

// Daemon is the assembled engine: storage, the secret vault, the message pipeline, and the
// local API, wired together and ready to run. It owns the lifecycle of everything it builds.
type Daemon struct {
	cfg        config.Config
	log        *slog.Logger
	store      store.Store
	vault      secrets.Vault
	runner     *pipeline.Runner
	engine     *engine.Engine
	stats      *stats.Aggregator
	logSink    *logbook.Sink
	sweeper    *logbook.Sweeper
	profiles   *profiles.Manager
	twitchAuth *twitchauth.Manager
	kickAuth   *kickauth.Manager
	api        *api.Server
}

// authControl adapts the auth managers to the API's auth controller.
type authControl struct {
	tw    *twitchauth.Manager
	kick  *kickauth.Manager
	creds *credentials
}

func (c authControl) StartTwitchDevice(ctx context.Context) (api.DeviceSession, error) {
	if c.creds.TwitchID() == "" {
		return api.DeviceSession{}, fmt.Errorf("%w: add a Twitch client id in Settings → Connections", api.ErrAuthNotConfigured)
	}
	s, err := c.tw.StartDevice(ctx)
	if err != nil {
		return api.DeviceSession{}, err
	}
	return toDeviceSession(s), nil
}

func (c authControl) TwitchDeviceStatus(id string) (api.DeviceSession, bool) {
	s, ok := c.tw.Status(id)
	if !ok {
		return api.DeviceSession{}, false
	}
	return toDeviceSession(s), true
}

func (c authControl) StartKickAuth(ctx context.Context) (api.AuthSession, error) {
	if c.creds.KickID() == "" {
		return api.AuthSession{}, fmt.Errorf("%w: add a Kick client id in Settings → Connections", api.ErrAuthNotConfigured)
	}
	s, err := c.kick.StartAuth(ctx)
	if err != nil {
		return api.AuthSession{}, err
	}
	return toKickSession(s), nil
}

func (c authControl) KickAuthStatus(id string) (api.AuthSession, bool) {
	s, ok := c.kick.Status(id)
	if !ok {
		return api.AuthSession{}, false
	}
	return toKickSession(s), true
}

func toKickSession(s kickauth.Session) api.AuthSession {
	return api.AuthSession{
		ID:           s.ID,
		AuthorizeURL: s.AuthorizeURL,
		State:        string(s.State),
		Login:        s.Login,
		Error:        s.Error,
	}
}

func toDeviceSession(s twitchauth.Session) api.DeviceSession {
	return api.DeviceSession{
		ID:              s.ID,
		UserCode:        s.UserCode,
		VerificationURI: s.VerificationURI,
		ExpiresIn:       s.ExpiresIn,
		Interval:        s.Interval,
		State:           string(s.State),
		Login:           s.Login,
		Error:           s.Error,
	}
}

// loggingControl fans a profile's logging policy out to the engine (ephemeral flagging), the
// logbook sink (write gating), and the sweeper (retention). It satisfies profiles.LoggingSetter.
type loggingControl struct {
	eng     *engine.Engine
	sink    *logbook.Sink
	sweeper *logbook.Sweeper
}

func (c loggingControl) SetLogging(enabled bool, retention string) {
	c.eng.SetLogging(enabled)
	c.sink.SetEnabled(enabled)
	c.sweeper.SetRetention(retention)
}

// NewDaemon builds every component from cfg. It does not start serving; call Start.
func NewDaemon(cfg config.Config) (*Daemon, error) {
	if err := cfg.EnsureDirs(); err != nil {
		return nil, fmt.Errorf("prepare directories: %w", err)
	}

	srv, err := api.New(api.Config{
		Addr:       cfg.Addr,
		Token:      cfg.Token, // fixed token for server deployments; empty → random (desktop)
		RuntimeDir: cfg.RuntimeDir,
		Logger:     slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
	})
	if err != nil {
		return nil, fmt.Errorf("build api: %w", err)
	}
	srv.SetWebUI(webui.Handler()) // serve the embedded SPA itself, if one was built in
	log := srv.Logger()

	clk := clock.System{}
	gen := id.NewULID(clk)

	st, err := SelectStore(cfg, clk, gen)
	if err != nil {
		return nil, err
	}

	vault, err := SelectVault(cfg.DataDir)
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("open vault: %w", err)
	}
	log.Info("secrets backend selected", "backend", vault.Backend())

	// Third-party emote overlays (7TV/BTTV/FFZ), resolved per channel off the hot path and
	// applied as a pure pipeline stage. Each provider is wrapped with a disk-backed set cache
	// (24h TTL, serves stale on a network blip), so a warm start skips the network and an
	// offline start still has emotes. Snapshots populate once a channel's platform user id is
	// available (Twitch via Helix in M3; Kick from the channel payload) — until then the stage
	// is a no-op, never blocking the feed.
	emoteCache := emoteSetCache{repo: st.Emotes(), clk: clk}
	emoteResolver := emotes.NewResolver(
		emotes.Cached(emotes.New7TV(nil, ""), emoteCache, clk, emotes.DefaultTTL),
		emotes.Cached(emotes.NewBTTV(nil, ""), emoteCache, clk, emotes.DefaultTTL),
		emotes.Cached(emotes.NewFFZ(nil, ""), emoteCache, clk, emotes.DefaultTTL),
	)

	// Badge artwork (mod/sub/broadcaster icons) is resolved per channel from Twitch's tokenless
	// badge CDN and stamped onto each message's badges; channels without resolved artwork fall
	// back to the frontend's text chips.
	badgeResolver := badges.NewResolver(badges.NewTwitch(nil, ""))

	// Live stream metadata (viewer count, title, thumbnail) per channel, resolved anonymously
	// (Twitch GraphQL, Kick's channel API) and served over GET /v1/streams for the streams rail.
	streamResolver := streams.NewResolver(streams.NewTwitch(nil, ""), streams.NewKick(nil, ""))

	// The pipeline annotates each message — filter rules (hide/highlight/mask) first, then
	// emote overlays — and fans events out to the API hub (and, later, the logger and other
	// sinks). The filter stage starts with an empty ruleset (no surprise masking); profiles and
	// settings populate and hot-swap it once they land.
	filterStage := filter.NewStage(nil)
	// The stats aggregator is a sink that tallies activity and feeds periodic StatsEvents back
	// through the pipeline (started after the runner exists, so it has something to submit to).
	statsAgg := stats.New(clk, 0)
	// The logbook sink persists chat when logging is enabled (opt-in, default off). It only
	// writes non-ephemeral messages, so logging-off persists nothing (ADR-014).
	logSink := logbook.NewSink(st.Messages(), clk, log)
	sweeper := logbook.NewSweeper(logSink, clk)
	// The held queue is a sink that tracks AutoMod-held messages for the moderation pane; it
	// clears an entry when a HeldResolvedEvent flows past (platform-driven or after an approve/deny).
	heldQueue := held.New()
	// The velocity stage marks overload messages as sampled so calm-mode UIs can thin a flooded
	// channel; it runs after badges so subscriber/mod badges (priority lanes) are resolved. It only
	// annotates, so the logger and other sinks still receive every message.
	velocityStage := velocity.NewStage(velocity.DefaultThreshold)
	// The scrollback ring retains a bounded per-channel tail in memory, so history/search work even
	// when persistent logging is off (session-scoped); it's unused for reads when logging is on.
	scrollbackRing := scrollback.New()
	runner := pipeline.NewRunner(pipeline.Options{
		Clock:  clk,
		Stages: []pipeline.Stage{filterStage, emotes.NewStage(emoteResolver), badges.NewStage(badgeResolver), velocityStage},
		Sinks:  []pipeline.Sink{srv.Sink(), statsAgg, logSink, heldQueue, scrollbackRing},
		Logger: log,
	})

	// The engine sits between adapters and the pipeline: it mints message ULIDs, resolves
	// deletions, and routes channel join/leave to the right adapter. Register the read-only
	// anonymous Twitch adapter (no credentials needed); more platforms register here later.
	eng := engine.New(runner, gen)
	twitchAdapter := twitch.New(twitch.Options{Clock: clk})
	eng.Register(twitchAdapter)
	// Kick needs a slug→chatroom-id resolver, cached forever in the channels table. The direct
	// lookup uses a stock client today (a uTLS Chrome-fingerprint upgrade is tracked); when it's
	// blocked the resolver falls back to the official API and trips its breaker.
	kickResolver := kick.NewResolver(
		kickChatroomCache{repo: st.Channels(), clk: clk},
		kick.NewDirectFetcher(nil),
		kick.NewOfficialFetcher(nil),
		clk,
	)
	kickAdapter := kick.New(kick.Options{Clock: clk, Resolver: kickResolver})
	eng.Register(kickAdapter)

	// The profile manager owns the active workspace: it drives the engine, filter stage, and
	// logging policy on activation and persists live channel changes. Profiles activate at Start.
	logCtl := loggingControl{eng: eng, sink: logSink, sweeper: sweeper}
	mgr := profiles.New(st.Profiles(), eng, filterStage, logCtl, runner, clk)
	srv.SetChannels(channelControl{eng: eng, emotes: emoteResolver, badges: badgeResolver, streams: streamResolver, profiles: mgr})
	srv.SetProfiles(profileControl{mgr: mgr, repo: st.Profiles()})
	srv.SetFilters(filterControl{mgr: mgr})
	srv.SetConnections(connectionsControl{mgr: mgr})
	srv.SetAccounts(accountsControl{
		repo:  st.Accounts(),
		vault: vault,
		deauth: map[platform.Platform]func(){
			platform.Twitch: twitchAdapter.Deauthenticate,
			platform.Kick:   kickAdapter.Deauthenticate,
		},
	})

	// Outbound sends are paced per channel and cross-posted through the typed-action layer. Kick
	// is seeded conservatively (its limits are undocumented and adapt on 429); Twitch starts at
	// its standard rate. A message to several channels reaches every signed-in one and reports a
	// signed-out one as excluded rather than failing the whole send.
	limit := ratelimit.Limit{Burst: 20, Window: 30 * time.Second}
	gov := ratelimit.NewAdaptive(ratelimit.New(clk, limit), clk, limit)
	gov.SetSeed("kick:", limit)
	sender := dispatch.New(map[platform.Platform]dispatch.Adapter{
		platform.Twitch: twitchAdapter,
		platform.Kick:   kickAdapter,
	}, gov, sendHelpText)
	srv.SetSend(sendControl{sender: sender})
	srv.SetHeld(heldControl{queue: heldQueue, sender: sender, emitter: runner})
	srv.SetHistory(historyControl{store: st, ring: scrollbackRing, loggingOn: logSink.Enabled})

	// OAuth app credentials are read through providers so they can be set at runtime via the UI
	// (stored in the vault), seeded from the env vars on first run.
	creds := newCredentials(vault)
	creds.seed(context.Background(), cfg.TwitchClientID, cfg.KickClientID, cfg.KickClientSecret)

	// Twitch sign-in via Device Code Grant. Tokens live in the vault; the account row in the
	// store. Sign-in returns a clear error until a client id is configured (env or UI).
	twitchAuth := twitchauth.NewManager(twitchauth.NewClient(creds.TwitchID, nil, clk), vault, st.Accounts(), gen, clk)
	kickAuth := kickauth.NewManager(kickauth.NewClient(creds.KickID, creds.KickSecret, nil, clk), vault, st.Accounts(), gen, clk)
	srv.SetAuth(authControl{tw: twitchAuth, kick: kickAuth, creds: creds})
	srv.SetAuthConfig(authConfigControl{creds: creds})

	// Attach an authenticated account to its platform adapter: bind a token source to the
	// account's vault ref and a broadcaster-id resolver, then flip the adapter to authenticated
	// (enabling send/moderation). Tokens never leave the auth manager; the adapter only gets a
	// closure. This runs both when an account signs in (the hooks below) and at startup for
	// accounts already stored, so a signed-in account survives a restart.
	helix := twitch.NewHelixClient(creds.TwitchID, nil)
	kickAPI := kick.NewAPIClient(nil)
	authTwitch := func(acc store.Account) {
		ref := acc.SecretRef
		tokens := func(ctx context.Context) (string, error) { return twitchAuth.AccessToken(ctx, ref) }
		resolve := func(ctx context.Context, login string) (string, error) {
			tok, err := tokens(ctx)
			if err != nil {
				return "", err
			}
			return helix.UserID(ctx, tok, login)
		}
		twitchAdapter.Authenticate(acc.PlatformUID, tokens, helix, resolve)
	}
	authKick := func(acc store.Account) {
		ref := acc.SecretRef
		tokens := func(ctx context.Context) (string, error) { return kickAuth.AccessToken(ctx, ref) }
		resolve := func(ctx context.Context, slug string) (string, error) {
			tok, err := tokens(ctx)
			if err != nil {
				return "", err
			}
			return kickAPI.BroadcasterID(ctx, tok, slug)
		}
		kickAdapter.Authenticate(tokens, kickAPI, resolve)
	}
	twitchAuth.SetOnAuthorized(authTwitch)
	kickAuth.SetOnAuthorized(authKick)
	if accs, err := st.Accounts().List(context.Background()); err == nil {
		for _, acc := range accs {
			switch acc.Platform {
			case platform.Twitch:
				authTwitch(acc)
			case platform.Kick:
				authKick(acc)
			}
		}
	}

	return &Daemon{cfg: cfg, log: log, store: st, vault: vault, runner: runner, engine: eng, stats: statsAgg, logSink: logSink, sweeper: sweeper, profiles: mgr, twitchAuth: twitchAuth, kickAuth: kickAuth, api: srv}, nil
}

// kickChatroomCache backs the Kick resolver's permanent cache with the channels table: a
// resolved chatroom id is stored in the channel's meta JSON and never re-fetched.
type kickChatroomCache struct {
	repo store.ChannelRepo
	clk  clock.Clock
}

func (c kickChatroomCache) Get(ctx context.Context, slug string) (string, bool) {
	ch, err := c.repo.GetBySlug(ctx, platform.Kick, slug)
	if err != nil {
		return "", false
	}
	var meta struct {
		ChatroomID string `json:"chatroom_id"`
	}
	if len(ch.Meta) > 0 {
		_ = json.Unmarshal(ch.Meta, &meta)
	}
	return meta.ChatroomID, meta.ChatroomID != ""
}

func (c kickChatroomCache) Put(ctx context.Context, slug, chatroomID string) error {
	meta, _ := json.Marshal(map[string]string{"chatroom_id": chatroomID})
	_, err := c.repo.Upsert(ctx, store.Channel{
		Platform:   platform.Kick,
		Slug:       slug,
		Meta:       meta,
		LastSeenAt: c.clk.Now(),
	})
	return err
}

// emoteSetCache backs the emote resolver's per-provider cache with the emote_sets table, so a
// warm start skips the network and an offline start still serves the last-known sets.
type emoteSetCache struct {
	repo store.EmoteRepo
	clk  clock.Clock
}

func (c emoteSetCache) Get(ctx context.Context, key string) ([]platform.EmoteRef, time.Time, bool) {
	s, err := c.repo.GetSet(ctx, key)
	if err != nil {
		return nil, time.Time{}, false
	}
	var refs []platform.EmoteRef
	if err := json.Unmarshal(s.Data, &refs); err != nil {
		return nil, time.Time{}, false
	}
	return refs, s.FetchedAt, true
}

func (c emoteSetCache) Put(ctx context.Context, key string, refs []platform.EmoteRef) error {
	data, err := json.Marshal(refs)
	if err != nil {
		return err
	}
	return c.repo.PutSet(ctx, store.EmoteSet{Key: key, Data: data, FetchedAt: c.clk.Now()})
}

// channelControl adapts the engine to the API's join/leave controller, translating the
// API's string boundary to platform types and back, and persisting changes to the active
// profile so a user's channel edits survive a restart.
type channelControl struct {
	eng      *engine.Engine
	emotes   *emotes.Resolver
	badges   *badges.Resolver
	streams  *streams.Resolver
	profiles *profiles.Manager
}

func (c channelControl) Join(ctx context.Context, plat, slug, mode string) error {
	p, ok := parsePlatform(plat)
	if !ok {
		return fmt.Errorf("%w: %q", api.ErrUnknownPlatform, plat)
	}
	m := platform.ConnMode(mode)
	if m == "" {
		m = c.profiles.MethodFor(p) // honor the platform's pinned connection method (else Automatic)
	}
	ref := platform.ChannelRef{Platform: p, Slug: slug}
	if err := c.eng.Join(ctx, ref, m); err != nil {
		return err
	}
	_ = c.profiles.AddChannel(ctx, ref, m) // persist to the active profile
	// Warm the channel's third-party emote set and badge artwork off the request path. Badges
	// resolve by login, so they fill in here; emotes stay a no-op until the platform user id is
	// known, but are wired so they light up the moment it is.
	go func() { _ = c.emotes.Refresh(context.Background(), ref) }()
	go c.badges.Refresh(context.Background(), ref)
	go func() { _ = c.streams.Refresh(context.Background(), ref) }()
	return nil
}

func (c channelControl) Leave(plat, slug string) error {
	p, ok := parsePlatform(plat)
	if !ok {
		return fmt.Errorf("%w: %q", api.ErrUnknownPlatform, plat)
	}
	ref := platform.ChannelRef{Platform: p, Slug: slug}
	if err := c.eng.Leave(ref); err != nil {
		return err
	}
	_ = c.profiles.RemoveChannel(context.Background(), ref)
	return nil
}

// filterControl adapts the profile manager to the API's filter-ruleset controller, translating the
// wire's string rules to the engine's typed grammar and back.
type filterControl struct{ mgr *profiles.Manager }

func toWireRule(r filter.Rule) api.FilterRule {
	w := api.FilterRule{
		ID:       r.ID,
		Action:   string(r.Action),
		Channels: r.Match.Channels,
		Authors:  r.Match.Authors,
		Keywords: r.Match.Keywords,
		Regexes:  r.Match.Regexes,
	}
	for _, p := range r.Match.Platforms {
		w.Platforms = append(w.Platforms, string(p))
	}
	for _, t := range r.Match.Types {
		w.Types = append(w.Types, string(t))
	}
	return w
}

func fromWireRule(w api.FilterRule) filter.Rule {
	r := filter.Rule{
		ID:     w.ID,
		Action: filter.Action(w.Action),
		Match:  filter.Match{Channels: w.Channels, Authors: w.Authors, Keywords: w.Keywords, Regexes: w.Regexes},
	}
	for _, p := range w.Platforms {
		r.Match.Platforms = append(r.Match.Platforms, platform.Platform(p))
	}
	for _, t := range w.Types {
		r.Match.Types = append(r.Match.Types, platform.MessageType(t))
	}
	return r
}

func (c filterControl) Filters() []api.FilterRule {
	rules := c.mgr.Filters()
	out := make([]api.FilterRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, toWireRule(r))
	}
	return out
}

func (c filterControl) SetFilters(ctx context.Context, rules []api.FilterRule) error {
	in := make([]filter.Rule, 0, len(rules))
	for _, r := range rules {
		in = append(in, fromWireRule(r))
	}
	// Validate (a bad regex) before touching the active profile, mapping to the API's 400 path.
	if _, err := filter.Compile(in); err != nil {
		return fmt.Errorf("%w: %v", api.ErrInvalidRuleset, err)
	}
	return c.mgr.SetFilters(ctx, in)
}

// connectionsControl adapts the profile manager to the API's per-platform connection-method
// controller.
type connectionsControl struct{ mgr *profiles.Manager }

func (c connectionsControl) Methods() map[string]string {
	out := map[string]string{}
	for p, mode := range c.mgr.Methods() {
		out[string(p)] = string(mode)
	}
	return out
}

func (c connectionsControl) SetMethod(ctx context.Context, plat, method string) error {
	p, ok := parsePlatform(plat)
	if !ok {
		return fmt.Errorf("%w: %q", api.ErrUnknownPlatform, plat)
	}
	return c.mgr.SetMethod(ctx, p, platform.ConnMode(method))
}

// authConfigControl adapts the credentials holder to the API's auth-config controller: read which
// platforms have an OAuth client id, and set the id/secret (persisted to the vault).
type authConfigControl struct{ creds *credentials }

func (c authConfigControl) AuthConfig() api.AuthConfig {
	return api.AuthConfig{
		Twitch: api.PlatformAuthConfig{ClientID: c.creds.TwitchID(), Configured: c.creds.TwitchID() != ""},
		Kick: api.PlatformAuthConfig{
			ClientID:   c.creds.KickID(),
			HasSecret:  c.creds.KickSecret() != "",
			Configured: c.creds.KickID() != "",
		},
	}
}

func (c authConfigControl) SetAuthConfig(ctx context.Context, plat, clientID, clientSecret string) error {
	p, ok := parsePlatform(plat)
	if !ok {
		return fmt.Errorf("%w: %q", api.ErrUnknownPlatform, plat)
	}
	switch p {
	case platform.Twitch:
		return c.creds.SetTwitch(ctx, clientID)
	case platform.Kick:
		return c.creds.SetKick(ctx, clientID, clientSecret)
	default:
		return fmt.Errorf("%w: %q has no configurable sign-in", api.ErrUnknownPlatform, plat)
	}
}

// accountsControl adapts the account store + adapters to the API's accounts controller: it lists
// connected accounts and disconnects one (delete its keychain secret and row, then revert that
// platform's adapter to anonymous read-only).
type accountsControl struct {
	repo   store.AccountRepo
	vault  secrets.Vault
	deauth map[platform.Platform]func()
}

func (c accountsControl) Accounts() []api.AccountInfo {
	list, err := c.repo.List(context.Background())
	if err != nil {
		return nil
	}
	out := make([]api.AccountInfo, 0, len(list))
	for _, a := range list {
		out = append(out, api.AccountInfo{
			ID:          a.ID,
			Platform:    string(a.Platform),
			Login:       a.Login,
			DisplayName: a.DisplayName,
			Scopes:      a.Scopes,
		})
	}
	return out
}

func (c accountsControl) Disconnect(ctx context.Context, id string) error {
	a, err := c.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if a.SecretRef != "" {
		_ = c.vault.Delete(ctx, a.SecretRef)
	}
	if err := c.repo.Delete(ctx, id); err != nil {
		return err
	}
	if fn := c.deauth[a.Platform]; fn != nil {
		fn() // revert the adapter to anonymous; capabilities drop to read-only
	}
	return nil
}

// profileControl adapts the profile manager to the API's profile controller.
type profileControl struct {
	mgr  *profiles.Manager
	repo store.ProfileRepo
}

func (c profileControl) List(ctx context.Context) ([]api.ProfileInfo, error) {
	ps, err := c.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	active := c.mgr.ActiveID()
	out := make([]api.ProfileInfo, 0, len(ps))
	for _, p := range ps {
		out = append(out, api.ProfileInfo{ID: p.ID, Name: p.Name, Active: p.ID == active, Default: p.IsDefault})
	}
	return out, nil
}

func (c profileControl) Create(ctx context.Context, name string) (api.ProfileInfo, error) {
	doc, err := profiles.NewDoc().Marshal()
	if err != nil {
		return api.ProfileInfo{}, err
	}
	p, err := c.repo.Create(ctx, name, doc)
	if err != nil {
		return api.ProfileInfo{}, err
	}
	return api.ProfileInfo{ID: p.ID, Name: p.Name, Default: p.IsDefault}, nil
}

func (c profileControl) Activate(ctx context.Context, id string) error {
	return c.mgr.Activate(ctx, id)
}

func (c channelControl) Capabilities() map[string]api.Capabilities {
	out := map[string]api.Capabilities{}
	for p, caps := range c.eng.Capabilities() {
		out[string(p)] = api.Capabilities{
			ReadAnonymous: caps.ReadAnonymous,
			ReadAuthed:    caps.ReadAuthed,
			Send:          caps.Send,
			Moderation:    caps.Moderation,
			Replies:       caps.Replies,
			HeldQueue:     caps.HeldQueue,
			Stability:     string(caps.Stability),
		}
	}
	return out
}

func (c channelControl) List() []api.ChannelInfo {
	statuses := c.eng.Channels()
	out := make([]api.ChannelInfo, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, api.ChannelInfo{
			Platform: string(s.Channel.Platform),
			Slug:     s.Channel.Slug,
			State:    string(s.Health.State),
			Reason:   string(s.Health.Reason),
		})
	}
	return out
}

// emoteSize picks a medium image size per provider for the URL template's {size} placeholder.
var emoteSize = map[platform.EmoteProvider]string{"twitch": "2.0", "kick": "2", "7tv": "2x", "bttv": "2x", "ffz": "2x"}

func emoteURL(e platform.EmoteRef) string {
	size := emoteSize[e.Provider]
	if size == "" {
		size = "2x"
	}
	return strings.ReplaceAll(e.URLTemplate, "{size}", size)
}

func (c channelControl) Emotes() []api.EmoteInfo {
	seen := map[string]struct{}{}
	var out []api.EmoteInfo
	for _, s := range c.eng.Channels() {
		for _, e := range c.emotes.Snapshot(emotes.Key(s.Channel)).Entries() {
			if _, dup := seen[e.Name]; dup {
				continue
			}
			seen[e.Name] = struct{}{}
			out = append(out, api.EmoteInfo{Code: e.Name, URL: emoteURL(e)})
		}
	}
	return out
}

func (c channelControl) Streams() []api.StreamInfo {
	statuses := c.eng.Channels()
	out := make([]api.StreamInfo, 0, len(statuses))
	for _, s := range statuses {
		ref := s.Channel
		c.streams.EnsureFresh(ref) // background re-fetch when stale; never blocks the response
		si := api.StreamInfo{Platform: string(ref.Platform), Slug: ref.Slug}
		if info := c.streams.Snapshot(streams.Key(ref)); info != nil {
			si.Live = info.Live
			si.ViewerCount = info.ViewerCount
			si.Title = info.Title
			si.Category = info.Category
			si.ThumbnailURL = info.ThumbnailURL
			if !info.StartedAt.IsZero() {
				si.StartedAt = info.StartedAt.Format(time.RFC3339)
			}
		}
		out = append(out, si)
	}
	return out
}

// sendHelpText is shown for /help: the slash commands the composer understands.
const sendHelpText = "Commands: /ban /unban /timeout /untimeout /delete /clear /slow /followers " +
	"/emoteonly /uniquechat /me /help"

// sendControl adapts the dispatch sender to the API's cross-posting controller, translating the
// API's "platform:slug" target strings to and from platform refs.
type sendControl struct {
	sender *dispatch.Sender
}

// heldControl adapts the held queue and dispatch sender to the API's hold-queue controller.
// Approve and deny look the message up by id, perform the typed moderation action, then emit a
// HeldResolvedEvent so the queue and every connected client clear the row on the same path a
// platform-driven resolution would take.
type heldControl struct {
	queue   *held.Queue
	sender  *dispatch.Sender
	emitter interface{ Submit(platform.Event) }
}

func (c heldControl) List() []api.HeldMessage {
	msgs := c.queue.List()
	out := make([]api.HeldMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, api.HeldFrom(m))
	}
	return out
}

func (c heldControl) Approve(ctx context.Context, id string) error { return c.resolve(ctx, id, true) }
func (c heldControl) Deny(ctx context.Context, id string) error    { return c.resolve(ctx, id, false) }

// historyControl adapts the message log to the API's search/scrollback controller. When persistent
// logging is on it reads the durable store (resolving "platform:slug" filters to channel ids and
// mapping rows back to channel keys); when off it falls back to the in-memory scrollback ring, so
// the feature works in the default logging-off mode (just session-scoped). loggingOn selects which.
type historyControl struct {
	store     store.Store
	ring      *scrollback.Ring
	loggingOn func() bool
}

func (c historyControl) Search(ctx context.Context, p api.SearchParams) ([]api.LoggedMessage, error) {
	if !c.loggingOn() {
		return ringToLogged(c.ring.Search(p.Channel, p.Text, p.Author, p.Before, p.Limit)), nil
	}
	q := store.SearchQuery{Text: p.Text, Author: p.Author, Before: p.Before, Limit: p.Limit}
	if p.Channel != "" {
		id, ok, err := c.channelID(ctx, p.Channel)
		if err != nil {
			return nil, err
		}
		if !ok {
			return []api.LoggedMessage{}, nil // unknown channel: nothing logged for it
		}
		q.ChannelID = id
	}
	msgs, err := c.store.Messages().Search(ctx, q)
	if err != nil {
		return nil, err
	}
	return c.toLogged(ctx, msgs)
}

func (c historyControl) History(ctx context.Context, p api.HistoryParams) ([]api.LoggedMessage, error) {
	if !c.loggingOn() {
		return ringToLogged(c.ring.History(p.Channel, p.Before, p.Limit)), nil
	}
	id, ok, err := c.channelID(ctx, p.Channel)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []api.LoggedMessage{}, nil
	}
	msgs, err := c.store.Messages().History(ctx, store.HistoryQuery{ChannelID: id, Before: p.Before, Limit: p.Limit})
	if err != nil {
		return nil, err
	}
	return c.toLogged(ctx, msgs)
}

// ringToLogged maps in-memory scrollback rows to the wire form (they already carry the channel ref).
func ringToLogged(rows []scrollback.Msg) []api.LoggedMessage {
	out := make([]api.LoggedMessage, 0, len(rows))
	for _, m := range rows {
		out = append(out, api.LoggedMessage{
			ID:       m.ID,
			Channel:  m.Channel.Key(),
			Platform: string(m.Channel.Platform),
			Author:   m.Author,
			Body:     m.Body,
			SentAtMs: m.SentAt,
			Deleted:  m.Deleted,
		})
	}
	return out
}

// channelID resolves a "platform:slug" key to the channel's stored id; ok is false when no such
// channel has been seen (so callers return an empty page rather than an error).
func (c historyControl) channelID(ctx context.Context, key string) (string, bool, error) {
	ref, err := parseTarget(key)
	if err != nil {
		return "", false, err
	}
	ch, err := c.store.Channels().GetBySlug(ctx, ref.Platform, ref.Slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return ch.ID, true, nil
}

// toLogged maps stored rows to the wire form, translating each row's channel id back to its
// "platform:slug" key via a one-shot lookup of the known channels.
func (c historyControl) toLogged(ctx context.Context, msgs []store.StoredMessage) ([]api.LoggedMessage, error) {
	if len(msgs) == 0 {
		return []api.LoggedMessage{}, nil
	}
	keyByID := map[string]string{}
	if chans, err := c.store.Channels().List(ctx); err == nil {
		for _, ch := range chans {
			keyByID[ch.ID] = platform.ChannelRef{Platform: ch.Platform, Slug: ch.Slug}.Key()
		}
	}
	out := make([]api.LoggedMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, api.LoggedMessage{
			ID:       m.ID,
			Channel:  keyByID[m.ChannelID],
			Platform: string(m.Platform),
			Author:   m.AuthorName,
			Body:     m.Body,
			SentAtMs: m.SentAt.UnixMilli(),
			Deleted:  m.Deleted,
		})
	}
	return out, nil
}

func (c heldControl) resolve(ctx context.Context, id string, approve bool) error {
	m, ok := c.queue.Get(id)
	if !ok {
		return api.ErrHeldNotFound
	}
	action := platform.ModAction{Type: platform.ModDenyHeld, Channel: m.Channel, TargetMessageID: m.ID}
	if approve {
		action.Type = platform.ModApproveHeld
	}
	if err := c.sender.Moderate(ctx, action); err != nil {
		return err
	}
	c.emitter.Submit(platform.HeldResolvedEvent{Channel: m.Channel, ID: m.ID, Approved: approve})
	return nil
}

// parseTarget turns a "platform:slug" target into a ChannelRef, rejecting an unknown platform so
// the API answers 400 rather than attempting a send.
func parseTarget(s string) (platform.ChannelRef, error) {
	plat, slug, ok := strings.Cut(s, ":")
	if !ok || slug == "" {
		return platform.ChannelRef{}, fmt.Errorf("%w: %q", api.ErrUnknownPlatform, s)
	}
	p, ok := parsePlatform(plat)
	if !ok {
		return platform.ChannelRef{}, fmt.Errorf("%w: %q", api.ErrUnknownPlatform, plat)
	}
	return platform.ChannelRef{Platform: p, Slug: slug}, nil
}

func parseTargets(targets []string) ([]platform.ChannelRef, error) {
	refs := make([]platform.ChannelRef, 0, len(targets))
	for _, t := range targets {
		ref, err := parseTarget(t)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (c sendControl) Preview(targets []string) ([]api.SendTarget, error) {
	refs, err := parseTargets(targets)
	if err != nil {
		return nil, err
	}
	states := c.sender.Targets(refs)
	out := make([]api.SendTarget, 0, len(states))
	for _, st := range states {
		out = append(out, api.SendTarget{Channel: st.Channel.Key(), CanSend: st.CanSend, Reason: string(st.Reason)})
	}
	return out, nil
}

func (c sendControl) Send(ctx context.Context, targets []string, text, replyTo string) ([]api.SendResult, error) {
	refs, err := parseTargets(targets)
	if err != nil {
		return nil, err
	}
	out := make([]api.SendResult, 0, len(refs))
	for _, ts := range c.sender.SendMany(ctx, refs, text, platform.SendOpts{ReplyParentID: replyTo}) {
		r := api.SendResult{Channel: ts.Channel.Key()}
		if !ts.Reachable {
			r.Status, r.Reason = api.SendExcluded, string(ts.Reason)
			out = append(out, r)
			continue
		}
		// A burst send dispatches inline, so its result is ready immediately; a paced send isn't
		// yet, so it's reported as queued and its final state surfaces over the event stream.
		select {
		case e := <-ts.Sent:
			if e != nil {
				r.Status, r.Reason = api.SendDropped, "send_failed"
			} else {
				r.Status = api.SendSent
			}
		default:
			r.Status = api.SendQueued
		}
		out = append(out, r)
	}
	return out, nil
}

func (c sendControl) Queue(targets []string) ([]api.QueueState, error) {
	refs, err := parseTargets(targets)
	if err != nil {
		return nil, err
	}
	infos := c.sender.QueueState(refs)
	out := make([]api.QueueState, 0, len(infos))
	for _, qi := range infos {
		out = append(out, api.QueueState{
			Channel:  qi.Channel.Key(),
			Queued:   qi.Queued,
			NextInMs: int(qi.NextIn / time.Millisecond),
		})
	}
	return out, nil
}

// parsePlatform validates a platform string against the known platforms, so an unknown one is
// the caller's 400 rather than a connection error.
func parsePlatform(s string) (platform.Platform, bool) {
	switch platform.Platform(s) {
	case platform.Twitch:
		return platform.Twitch, true
	case platform.Kick:
		return platform.Kick, true
	case platform.X:
		return platform.X, true
	default:
		return "", false
	}
}

// Start launches the pipeline and begins serving the local API.
func (d *Daemon) Start() error {
	d.runner.Start()
	d.stats.Start(d.runner) // feed StatsEvents back through the pipeline
	d.logSink.Start()       // periodic batch flush
	d.sweeper.Start()       // retention sweeps

	// Activate the default profile so its channels connect and its filters apply on startup.
	ctx := context.Background()
	def, err := d.profiles.EnsureDefault(ctx)
	if err != nil {
		_ = d.runner.Close()
		return fmt.Errorf("ensure default profile: %w", err)
	}
	if err := d.profiles.Activate(ctx, def.ID); err != nil {
		_ = d.runner.Close()
		return fmt.Errorf("activate default profile: %w", err)
	}

	if err := d.api.Start(); err != nil {
		_ = d.runner.Close()
		return err
	}
	return nil
}

// Submit feeds an event into the pipeline (the entry point adapters will use).
func (d *Daemon) Submit(ev platform.Event) { d.runner.Submit(ev) }

// Store, Vault, and Pipeline expose the assembled components for the parts of the daemon
// that build on them.
func (d *Daemon) Store() store.Store         { return d.store }
func (d *Daemon) Vault() secrets.Vault       { return d.vault }
func (d *Daemon) Pipeline() *pipeline.Runner { return d.runner }

// Close shuts everything down in order: stop accepting clients, close adapters so no new
// events arrive, drain the pipeline, then release storage.
func (d *Daemon) Close(ctx context.Context) error {
	apiErr := d.api.Close(ctx)
	_ = d.twitchAuth.Close()
	_ = d.kickAuth.Close()
	_ = d.engine.Close()
	_ = d.sweeper.Close()
	_ = d.runner.Close() // closes the sinks, including the logbook sink (final flush)
	storeErr := d.store.Close()
	if apiErr != nil {
		return apiErr
	}
	return storeErr
}
