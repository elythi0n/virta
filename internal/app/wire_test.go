package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/elythi0n/virta/internal/api"
	"github.com/elythi0n/virta/internal/app"
	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/config"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/secrets"
)

func tempConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	return config.Config{
		Addr:          "127.0.0.1:0",
		DataDir:       filepath.Join(dir, "data"),
		CacheDir:      filepath.Join(dir, "cache"),
		RuntimeDir:    filepath.Join(dir, "run"),
		DBPath:        filepath.Join(dir, "data", "virta.db"),
		StorageDriver: config.StorageSQLite,
	}
}

func TestSelectStore_DriverSelection(t *testing.T) {
	clk := clock.System{}
	gen := id.NewULID(clk)

	st, err := app.SelectStore(config.Config{StorageDriver: config.StorageSQLite, DBPath: filepath.Join(t.TempDir(), "x.db")}, clk, gen)
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	_ = st.Close()

	for _, drv := range []string{config.StoragePostgres, config.StorageMySQL, "nonsense"} {
		if _, err := app.SelectStore(config.Config{StorageDriver: drv}, clk, gen); err == nil {
			t.Errorf("SelectStore(%q) returned nil error, want a clear failure", drv)
		}
	}
}

func TestDaemon_AssemblesAndServes(t *testing.T) {
	cfg := tempConfig(t)
	d, err := app.NewDaemon(cfg)
	if err != nil {
		t.Fatalf("NewDaemon: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The daemon advertises itself via the discovery file; a frontend would read this.
	disc, err := api.ReadDiscovery(cfg.RuntimeDir)
	if err != nil {
		t.Fatalf("ReadDiscovery: %v", err)
	}
	resp, err := http.Get("http://" + disc.Addr + "/v1/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want 200", resp.StatusCode)
	}

	// Submitting an event through the assembled pipeline must not panic (no subscribers).
	d.Submit(platform.MessageEvent{Message: platform.UnifiedMessage{ID: "smoke", Type: platform.TypeChat}})
	if d.Store() == nil || d.Vault() == nil || d.Pipeline() == nil {
		t.Fatal("assembled components missing")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := api.ReadDiscovery(cfg.RuntimeDir); err == nil {
		t.Error("discovery file present after shutdown")
	}
}

// SelectVault must always return a working vault. On headless CI that's the file vault; on a
// machine with a credential store it's the keychain. Either way it round-trips.
func TestSelectVault_ReturnsWorkingVault(t *testing.T) {
	v, err := app.SelectVault(t.TempDir())
	if err != nil {
		t.Fatalf("SelectVault: %v", err)
	}
	switch v.Backend() {
	case secrets.BackendKeychain, secrets.BackendFileVault:
		// expected
	default:
		t.Fatalf("unexpected backend %q", v.Backend())
	}

	ctx := context.Background()
	key := secrets.APITokenKey("wire-test")
	t.Cleanup(func() { _ = v.Delete(ctx, key) })
	if err := v.Set(ctx, key, "secret"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := v.Get(ctx, key)
	if err != nil || got != "secret" {
		t.Fatalf("Get = %q, %v; want secret", got, err)
	}
}

// diagnosticsClientCount reads the number of connected stream clients from the daemon's
// diagnostics endpoint — a deterministic way to wait until a just-dialed client is registered.
func diagnosticsClientCount(t *testing.T, disc api.Discovery) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "http://"+disc.Addr+"/v1/diagnostics", nil)
	req.Header.Set("Authorization", "Bearer "+disc.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Clients int `json:"clients"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return body.Clients
}

// The Phase-0 capstone: an adapter's events flow through the assembled daemon's pipeline and
// reach a WebSocket client — proving every founding piece is wired together.
func TestDaemon_AdapterEventReachesStreamClient(t *testing.T) {
	cfg := tempConfig(t)
	d, err := app.NewDaemon(cfg)
	if err != nil {
		t.Fatalf("NewDaemon: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = d.Close(ctx)
	})

	disc, err := api.ReadDiscovery(cfg.RuntimeDir)
	if err != nil {
		t.Fatalf("ReadDiscovery: %v", err)
	}

	// A platform adapter feeding into the daemon's pipeline.
	adapter := platform.NewFakeAdapter(platform.Twitch, platform.Capabilities{ReadAnonymous: true})
	t.Cleanup(func() { _ = adapter.Close() })
	d.Pipeline().Attach(adapter.Events())

	// Connect a stream client and wait until the daemon reports it as connected.
	dctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(dctx, "ws://"+disc.Addr+"/v1/stream?token="+disc.Token, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	deadline := time.Now().Add(3 * time.Second)
	for diagnosticsClientCount(t, disc) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("client never registered")
		}
		time.Sleep(2 * time.Millisecond)
	}

	// The adapter emits a chat message; it should arrive at the client, end to end.
	adapter.EmitMessage(platform.UnifiedMessage{
		ID: "e2e-1", Type: platform.TypeChat,
		Channel:  platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"},
		Segments: []platform.Segment{{Kind: platform.SegText, Text: "end to end"}},
	})

	rctx, rcancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer rcancel()
	_, data, err := conn.Read(rctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var we struct {
		Type    string `json:"type"`
		Message *struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &we); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if we.Type != "message" || we.Message == nil || we.Message.ID != "e2e-1" {
		t.Fatalf("client received %s, want message e2e-1", data)
	}
}
