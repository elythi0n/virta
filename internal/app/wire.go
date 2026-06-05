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
	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/config"
	"github.com/elythi0n/virta/internal/emotes"
	"github.com/elythi0n/virta/internal/engine"
	"github.com/elythi0n/virta/internal/filter"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/pipeline"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/kick"
	"github.com/elythi0n/virta/internal/platform/twitch"
	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/secrets/filevault"
	"github.com/elythi0n/virta/internal/secrets/keychain"
	"github.com/elythi0n/virta/internal/store"
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
	case config.StoragePostgres, config.StorageMySQL:
		return nil, fmt.Errorf("storage backend %q is not implemented yet (default is %q)", cfg.StorageDriver, config.StorageSQLite)
	default:
		return nil, fmt.Errorf("unknown storage backend %q", cfg.StorageDriver)
	}
}

// Daemon is the assembled engine: storage, the secret vault, the message pipeline, and the
// local API, wired together and ready to run. It owns the lifecycle of everything it builds.
type Daemon struct {
	cfg    config.Config
	log    *slog.Logger
	store  store.Store
	vault  secrets.Vault
	runner *pipeline.Runner
	engine *engine.Engine
	api    *api.Server
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
	runner := pipeline.NewRunner(pipeline.Options{
		Clock:  clk,
		Stages: []pipeline.Stage{filterStage, emotes.NewStage(emoteResolver)},
		Sinks:  []pipeline.Sink{srv.Sink()},
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
	srv.SetChannels(channelControl{eng: eng, emotes: emoteResolver})

	return &Daemon{cfg: cfg, log: log, store: st, vault: vault, runner: runner, engine: eng, api: srv}, nil
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
// API's string boundary to platform types and back.
type channelControl struct {
	eng    *engine.Engine
	emotes *emotes.Resolver
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
	return c.eng.Leave(platform.ChannelRef{Platform: p, Slug: slug})
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
	_ = d.engine.Close()
	_ = d.runner.Close()
	storeErr := d.store.Close()
	if apiErr != nil {
		return apiErr
	}
	return storeErr
}
