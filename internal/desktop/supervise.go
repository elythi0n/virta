// Package desktop holds the host-side logic for the desktop shell: finding a running daemon or
// starting one, so the shell can attach to whichever virtad is already up (a TUI may have started
// one) or launch its own. It is deliberately free of any GUI-toolkit dependency so it builds and
// is tested as part of the core module; the Wails shell (a separate module) wires it to a window.
package desktop

import (
	"context"
	"net/http"
	"time"

	"github.com/elythi0n/virta/internal/api"
)

// healthTimeout bounds a single liveness probe so attaching to a stale discovery file fails fast.
const healthTimeout = 750 * time.Millisecond

// Attach returns the discovery info of a daemon that is already running and reachable, or ok=false
// if there is none. It reads the discovery file the daemon writes and confirms the listener
// answers, so a stale file (daemon crashed without cleanup) does not look like a live daemon.
func Attach(ctx context.Context, runtimeDir string) (api.Discovery, bool) {
	d, err := api.ReadDiscovery(runtimeDir)
	if err != nil {
		return api.Discovery{}, false
	}
	if !healthy(ctx, d.Addr) {
		return api.Discovery{}, false
	}
	return d, true
}

// Ensure attaches to a running daemon, or calls spawn to start one and waits for it to advertise
// itself, returning the discovery info either way. spawn may be nil to only attach.
func Ensure(ctx context.Context, runtimeDir string, spawn func() error) (api.Discovery, error) {
	if d, ok := Attach(ctx, runtimeDir); ok {
		return d, nil
	}
	if spawn != nil {
		if err := spawn(); err != nil {
			return api.Discovery{}, err
		}
	}
	return waitForDaemon(ctx, runtimeDir)
}

// waitForDaemon polls until a daemon is reachable or ctx is done. The daemon writes its discovery
// file only after the listener is bound, so a successful Attach means it is ready for clients.
func waitForDaemon(ctx context.Context, runtimeDir string) (api.Discovery, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if d, ok := Attach(ctx, runtimeDir); ok {
			return d, nil
		}
		select {
		case <-ctx.Done():
			return api.Discovery{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

// healthy reports whether the daemon's unauthenticated health endpoint answers OK. Health needs no
// token, so this is a pure liveness check.
func healthy(ctx context.Context, addr string) bool {
	hctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(hctx, http.MethodGet, "http://"+addr+"/v1/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
