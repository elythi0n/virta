package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elythi0n/virta/internal/config"
)

func TestLoad_EnvOverrides(t *testing.T) {
	data := t.TempDir()
	cache := t.TempDir()
	run := t.TempDir()
	t.Setenv("VIRTA_ADDR", "127.0.0.1:9999")
	t.Setenv("VIRTA_STORAGE", "postgres")
	t.Setenv("VIRTA_TOKEN", "fixed-server-token")
	t.Setenv("VIRTA_DATA_DIR", data)
	t.Setenv("VIRTA_CACHE_DIR", cache)
	t.Setenv("VIRTA_RUNTIME_DIR", run)

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Addr != "127.0.0.1:9999" {
		t.Errorf("Addr = %q", c.Addr)
	}
	if c.StorageDriver != "postgres" {
		t.Errorf("StorageDriver = %q", c.StorageDriver)
	}
	if c.Token != "fixed-server-token" {
		t.Errorf("Token = %q", c.Token)
	}
	if c.DataDir != data || c.CacheDir != cache || c.RuntimeDir != run {
		t.Errorf("dirs = %q %q %q", c.DataDir, c.CacheDir, c.RuntimeDir)
	}
	if c.DBPath != filepath.Join(data, "virta.db") {
		t.Errorf("DBPath = %q", c.DBPath)
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("VIRTA_DATA_DIR", t.TempDir())
	t.Setenv("VIRTA_CACHE_DIR", t.TempDir())
	t.Setenv("VIRTA_RUNTIME_DIR", t.TempDir())
	// No VIRTA_ADDR / VIRTA_STORAGE: defaults apply.
	t.Setenv("VIRTA_ADDR", "")
	t.Setenv("VIRTA_STORAGE", "")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.HasPrefix(c.Addr, "127.0.0.1:") {
		t.Errorf("default Addr = %q, want loopback", c.Addr)
	}
	if c.StorageDriver != config.StorageSQLite {
		t.Errorf("default StorageDriver = %q, want %q", c.StorageDriver, config.StorageSQLite)
	}
}

func TestLoad_RuntimeDirHonorsXDG(t *testing.T) {
	t.Setenv("VIRTA_DATA_DIR", t.TempDir())
	t.Setenv("VIRTA_CACHE_DIR", t.TempDir())
	t.Setenv("VIRTA_RUNTIME_DIR", "")
	xdg := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", xdg)

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.RuntimeDir != filepath.Join(xdg, "virta") {
		t.Errorf("RuntimeDir = %q, want under XDG", c.RuntimeDir)
	}
}

func TestLoad_UsesOSDirsWhenEnvUnset(t *testing.T) {
	t.Setenv("VIRTA_DATA_DIR", "")
	t.Setenv("VIRTA_CACHE_DIR", "")
	t.Setenv("VIRTA_RUNTIME_DIR", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Falls back to the OS config/cache dirs, namespaced under the app, with the runtime
	// dir nested under the data dir.
	if filepath.Base(c.DataDir) != "virta" {
		t.Errorf("DataDir = %q, want it to end in virta", c.DataDir)
	}
	if c.RuntimeDir != filepath.Join(c.DataDir, "run") {
		t.Errorf("RuntimeDir = %q, want %q", c.RuntimeDir, filepath.Join(c.DataDir, "run"))
	}
}

func TestEnsureDirs(t *testing.T) {
	base := t.TempDir()
	c := config.Config{
		DataDir:    filepath.Join(base, "data"),
		CacheDir:   filepath.Join(base, "cache"),
		RuntimeDir: filepath.Join(base, "run"),
	}
	if err := c.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	for _, d := range []string{c.DataDir, c.CacheDir, c.RuntimeDir} {
		info, err := os.Stat(d)
		if err != nil {
			t.Fatalf("stat %s: %v", d, err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}
