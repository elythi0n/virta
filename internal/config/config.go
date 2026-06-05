// Package config resolves runtime configuration and the on-disk locations virta uses.
// Precedence is flags > environment (VIRTA_*) > built-in defaults. Paths follow each OS's
// conventions (via the standard library) so data lands where users expect it.
package config

import (
	"os"
	"path/filepath"
)

// Config is the resolved runtime configuration.
type Config struct {
	// Addr is the address the local API listens on. The default binds loopback on an
	// ephemeral port, so the daemon is reachable only from this machine and never clashes.
	Addr string
	// DataDir holds the database and the encrypted-file vault (when no OS keychain exists).
	DataDir string
	// CacheDir holds disposable caches (emote images, etc.).
	CacheDir string
	// RuntimeDir holds the discovery file that tells local frontends how to reach the daemon.
	RuntimeDir string
	// DBPath is the SQLite database file (used when StorageDriver is SQLite).
	DBPath string
	// StorageDriver selects the storage backend. SQLite is the zero-config default; other
	// engines are opt-in and chosen here (or, at runtime, in the Storage settings). Changing
	// it on an existing install requires a data migration, not an instant swap.
	StorageDriver string
}

const appName = "virta"

// Storage backend identifiers. Only SQLite is implemented today; the others are recognized
// so the UI and config can offer them and report "not available yet" rather than failing
// obscurely.
const (
	StorageSQLite   = "sqlite"
	StoragePostgres = "postgres"
	StorageMySQL    = "mysql"
)

// Load resolves configuration from defaults, then the environment. (Flag overrides are
// applied by the caller after Load, since flag sets belong to the command.)
func Load() (Config, error) {
	dataDir, err := baseDir(os.UserConfigDir, "VIRTA_DATA_DIR")
	if err != nil {
		return Config{}, err
	}
	cacheDir, err := baseDir(os.UserCacheDir, "VIRTA_CACHE_DIR")
	if err != nil {
		return Config{}, err
	}

	c := Config{
		Addr:          envOr("VIRTA_ADDR", "127.0.0.1:0"),
		DataDir:       dataDir,
		CacheDir:      cacheDir,
		RuntimeDir:    runtimeDir(dataDir),
		DBPath:        filepath.Join(dataDir, appName+".db"),
		StorageDriver: envOr("VIRTA_STORAGE", StorageSQLite),
	}
	return c, nil
}

// baseDir returns an app-specific directory under the given OS base (config or cache),
// overridable by an environment variable. It does not create the directory.
func baseDir(osBase func() (string, error), envKey string) (string, error) {
	if v := os.Getenv(envKey); v != "" {
		return v, nil
	}
	base, err := osBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName), nil
}

// runtimeDir picks a per-user runtime location for the discovery file. On Linux it honors
// XDG_RUNTIME_DIR (a tmpfs that's cleared on logout); elsewhere it falls back under the
// data directory.
func runtimeDir(dataDir string) string {
	if v := os.Getenv("VIRTA_RUNTIME_DIR"); v != "" {
		return v
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, appName)
	}
	return filepath.Join(dataDir, "run")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// EnsureDirs creates the data, cache, and runtime directories with owner-only permissions.
func (c Config) EnsureDirs() error {
	for _, d := range []string{c.DataDir, c.CacheDir, c.RuntimeDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}
