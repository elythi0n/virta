// Package app wires the concrete implementations together. It is the single place allowed
// to import implementation packages (platform adapters, storage backends, secret vaults);
// every other package depends only on the interfaces. Keeping construction here means the
// rest of the codebase never hard-codes a choice of backend.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/elythi0n/virta/internal/api"
	kickauth "github.com/elythi0n/virta/internal/auth/kick"
	twitchauth "github.com/elythi0n/virta/internal/auth/twitch"
	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/config"
	"github.com/elythi0n/virta/internal/emotes"
	"github.com/elythi0n/virta/internal/engine"
	"github.com/elythi0n/virta/internal/filter"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/logbook"
	"github.com/elythi0n/virta/internal/pipeline"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/kick"
	"github.com/elythi0n/virta/internal/platform/twitch"
	"github.com/elythi0n/virta/internal/profiles"
	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/secrets/filevault"
	"github.com/elythi0n/virta/internal/secrets/keychain"
	"github.com/elythi0n/virta/internal/stats"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/store/postgres"
	"github.com/elythi0n/virta/internal/store/sqlite"
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
	tw               *twitchauth.Manager
	twitchConfigured bool
	kick             *kickauth.Manager
	kickConfigured   bool
}

func (c authControl) StartTwitchDevice(ctx context.Context) (api.DeviceSession, error) {
	if !c.twitchConfigured {
		return api.DeviceSession{}, fmt.Errorf("twitch sign-in is not configured (set VIRTA_TWITCH_CLIENT_ID)")
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
	if !c.kickConfigured {
		return api.AuthSession{}, fmt.Errorf("kick sign-in is not configured (set VIRTA_KICK_CLIENT_ID)")
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
	runner := pipeline.NewRunner(pipeline.Options{
		Clock:  clk,
		Stages: []pipeline.Stage{filterStage, emotes.NewStage(emoteResolver)},
		Sinks:  []pipeline.Sink{srv.Sink(), statsAgg, logSink},
		Logger: log,
	})

	// The engine sits between adapters and the pipeline: it mints message ULIDs, resolves
	// deletions, and routes channel join/leave to the right adapter. Register the read-only
	// anonymous Twitch adapter (no credentials needed); more platforms register here later.
	eng := engine.New(runner, gen)
	eng.Register(twitch.New(twitch.Options{Clock: clk}))
	// Kick needs a slug→chatroom-id resolver, cached forever in the channels table. The direct
	// lookup uses a stock client today (a uTLS Chrome-fingerprint upgrade is tracked); when it's
	// blocked the resolver falls back to the official API and trips its breaker.
	kickResolver := kick.NewResolver(
		kickChatroomCache{repo: st.Channels(), clk: clk},
		kick.NewDirectFetcher(nil),
		kick.NewOfficialFetcher(nil),
		clk,
	)
	eng.Register(kick.New(kick.Options{Clock: clk, Resolver: kickResolver}))

	// The profile manager owns the active workspace: it drives the engine, filter stage, and
	// logging policy on activation and persists live channel changes. Profiles activate at Start.
	logCtl := loggingControl{eng: eng, sink: logSink, sweeper: sweeper}
	mgr := profiles.New(st.Profiles(), eng, filterStage, logCtl, runner, clk)
	srv.SetChannels(channelControl{eng: eng, emotes: emoteResolver, profiles: mgr})
	srv.SetProfiles(profileControl{mgr: mgr, repo: st.Profiles()})

	// Twitch sign-in via Device Code Grant. Tokens live in the vault; the account row in the
	// store. Disabled (sign-in returns a clear error) when no client id is configured.
	twitchAuth := twitchauth.NewManager(twitchauth.NewClient(cfg.TwitchClientID, nil, clk), vault, st.Accounts(), gen, clk)
	kickAuth := kickauth.NewManager(kickauth.NewClient(cfg.KickClientID, cfg.KickClientSecret, nil, clk), vault, st.Accounts(), gen, clk)
	srv.SetAuth(authControl{
		tw: twitchAuth, twitchConfigured: cfg.TwitchClientID != "",
		kick: kickAuth, kickConfigured: cfg.KickClientID != "",
	})

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
	profiles *profiles.Manager
}

func (c channelControl) Join(ctx context.Context, plat, slug, mode string) error {
	p, ok := parsePlatform(plat)
	if !ok {
		return fmt.Errorf("%w: %q", api.ErrUnknownPlatform, plat)
	}
	m := platform.ConnMode(mode)
	if m == "" {
		m = platform.ModeAutomatic
	}
	ref := platform.ChannelRef{Platform: p, Slug: slug}
	if err := c.eng.Join(ctx, ref, m); err != nil {
		return err
	}
	_ = c.profiles.AddChannel(ctx, ref, m) // persist to the active profile
	// Warm the channel's third-party emote set off the request path. A no-op until the
	// platform user id is known, but wired so it lights up the moment it is.
	go func() { _ = c.emotes.Refresh(context.Background(), ref) }()
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
