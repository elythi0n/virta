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
	// DBDSN is the connection string for a server database backend (used when StorageDriver is
	// Postgres), e.g. "postgres://user:pass@host:5432/virta?sslmode=disable".
	DBDSN string
	// StorageDriver selects the storage backend. SQLite is the zero-config default; other
	// engines are opt-in and chosen here (or, at runtime, in the Storage settings). Changing
	// it on an existing install requires a data migration, not an instant swap.
	StorageDriver string
	// TwitchClientID is the registered public OAuth client id used for the Device Code Grant
	// (a public client — no secret). Empty disables Twitch sign-in (anonymous read still works).
	TwitchClientID string
	// KickClientID / KickClientSecret configure Kick OAuth (2.1 + PKCE). The secret may be
	// empty (pure PKCE); whether Kick requires it is an open question. Empty client
	// id disables Kick sign-in (anonymous read still works).
	KickClientID     string
	KickClientSecret string
	// Token fixes the API bearer token instead of generating a random one. Left empty for
	// the desktop case (a random token is written to the discovery file for local frontends);
	// set it for a server deployment, where remote clients can't read the discovery file and
	// need a known token.
	Token string
	// Hosted enables multi-user mode (VIRTA_HOSTED=1): user registration/login, per-user
	// workspaces, and Postgres as the expected backend. False in local/desktop mode.
	Hosted bool
	// LoggingEnabled force-enables message logging on startup without requiring the user
	// to toggle it in Settings. Useful for server deployments where logging should always
	// be on. When true, the daemon enables logging on the active profile at start time;
	// the user can still disable it from Settings if desired.
	LoggingEnabled bool
	// MCPRelayURL is the public base URL at which the Virta MCP server is reachable by
	// external AI clients (Claude Desktop, Cursor, etc.). Empty in local/desktop mode.
	// Set to e.g. "https://virta.example.com" in hosted deployments so the AI can tell
	// users the endpoint they need to connect their AI client to the MCP server.
	MCPRelayURL string
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
		Addr:             envOr("VIRTA_ADDR", "127.0.0.1:0"),
		DataDir:          dataDir,
		CacheDir:         cacheDir,
		RuntimeDir:       runtimeDir(dataDir),
		DBPath:           filepath.Join(dataDir, appName+".db"),
		DBDSN:            os.Getenv("VIRTA_DB_DSN"),
		StorageDriver:    envOr("VIRTA_STORAGE", StorageSQLite),
		TwitchClientID:   os.Getenv("VIRTA_TWITCH_CLIENT_ID"),
		KickClientID:     os.Getenv("VIRTA_KICK_CLIENT_ID"),
		KickClientSecret: os.Getenv("VIRTA_KICK_CLIENT_SECRET"),
		Token:            os.Getenv("VIRTA_TOKEN"),
		Hosted:           os.Getenv("VIRTA_HOSTED") == "1" || os.Getenv("VIRTA_HOSTED") == "true",
		LoggingEnabled:   os.Getenv("VIRTA_LOGGING_ENABLED") == "1" || os.Getenv("VIRTA_LOGGING_ENABLED") == "true",
		MCPRelayURL:      os.Getenv("VIRTA_MCP_RELAY_URL"),
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
