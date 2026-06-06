package desktop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/api"
)

func writeDiscovery(t *testing.T, dir, addr, token string) {
	t.Helper()
	b, err := json.Marshal(api.Discovery{Addr: addr, Token: token})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "daemon.json"), b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func healthServer(t *testing.T) (addr string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestAttach_NoFile(t *testing.T) {
	if _, ok := Attach(context.Background(), t.TempDir()); ok {
		t.Fatal("no discovery file should mean no daemon")
	}
}

func TestAttach_Healthy(t *testing.T) {
	addr := healthServer(t)
	dir := t.TempDir()
	writeDiscovery(t, dir, addr, "tok-123")

	d, ok := Attach(context.Background(), dir)
	if !ok {
		t.Fatal("a reachable daemon should attach")
	}
	if d.Addr != addr || d.Token != "tok-123" {
		t.Fatalf("discovery = %+v", d)
	}
}

func TestAttach_StaleFileIsNotLive(t *testing.T) {
	dir := t.TempDir()
	writeDiscovery(t, dir, "127.0.0.1:1", "tok") // nothing listening: a stale file
	if _, ok := Attach(context.Background(), dir); ok {
		t.Fatal("a stale discovery file must not look like a live daemon")
	}
}

func TestEnsure_SpawnsThenAttaches(t *testing.T) {
	addr := healthServer(t)
	dir := t.TempDir()
	spawned := false
	spawn := func() error {
		spawned = true
		writeDiscovery(t, dir, addr, "tok") // stand in for a daemon coming up
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	d, err := Ensure(ctx, dir, spawn)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !spawned {
		t.Fatal("expected spawn to be called when no daemon was running")
	}
	if d.Addr != addr {
		t.Fatalf("discovery = %+v", d)
	}
}

func TestEnsure_AttachesWithoutSpawning(t *testing.T) {
	addr := healthServer(t)
	dir := t.TempDir()
	writeDiscovery(t, dir, addr, "tok")

	spawn := func() error {
		t.Fatal("must not spawn when a daemon is already running")
		return nil
	}
	if _, err := Ensure(context.Background(), dir, spawn); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
}
