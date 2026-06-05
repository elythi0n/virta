// Package app wires the concrete implementations together. It is the single place allowed
// to import implementation packages (platform adapters, storage backends, secret vaults);
// every other package depends only on the interfaces. Keeping construction here means the
// rest of the codebase never hard-codes a choice of backend.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/elythi0n/virta/internal/api"
	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/config"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/pipeline"
	"github.com/elythi0n/virta/internal/platform"
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

	// The pipeline fans events out to the API hub (and, later, the logger and other sinks).
	// No stages are wired yet — adapters and stages arrive next.
	runner := pipeline.NewRunner(pipeline.Options{
		Clock:  clk,
		Sinks:  []pipeline.Sink{srv.Sink()},
		Logger: log,
	})

	return &Daemon{cfg: cfg, log: log, store: st, vault: vault, runner: runner, api: srv}, nil
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

// Close shuts everything down in order: stop accepting clients, drain the pipeline, then
// release storage.
func (d *Daemon) Close(ctx context.Context) error {
	apiErr := d.api.Close(ctx)
	_ = d.runner.Close()
	storeErr := d.store.Close()
	if apiErr != nil {
		return apiErr
	}
	return storeErr
}
