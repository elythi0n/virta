package main

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/elythi0n/virta/internal/api"
	"github.com/elythi0n/virta/internal/config"
	"github.com/elythi0n/virta/internal/desktop"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// startupTimeout bounds how long we wait for a freshly launched daemon to come up before the UI
// loads anyway (it will show a disconnected state and retry).
const startupTimeout = 15 * time.Second

// App is the shell's lifecycle owner: it finds or launches the daemon and tells the web UI how to
// reach it.
type App struct {
	ctx         context.Context
	discovery   api.Discovery
	integration IntegrationReport
	daemon      *daemonProcess
}

func newApp() *App { return &App{} }

// startup attaches to a running daemon or launches the embedded one, then records its address and
// token so the UI can open an authenticated connection.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.integration = resolveIntegration(runtime.GOOS, currentSession())
	cfg, err := config.Load()
	if err != nil {
		return // Discovery() returns an empty address; the UI shows "not connected"
	}
	startCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if d, err := desktop.Ensure(startCtx, cfg.RuntimeDir, a.launchDaemon); err == nil {
		a.discovery = d
	}
}

func (a *App) shutdown(_ context.Context) {
	a.daemon.stop()
}

// WindowMinimise minimises the window. Bound to the frontend via Wails bindings.
func (a *App) WindowMinimise() { wailsruntime.WindowMinimise(a.ctx) }

// WindowToggleMaximise toggles between maximised and restored. Bound to the frontend.
func (a *App) WindowToggleMaximise() { wailsruntime.WindowToggleMaximise(a.ctx) }

// WindowClose quits the application cleanly.
func (a *App) WindowClose() { wailsruntime.Quit(a.ctx) }

// assetHandler serves requests the embedded UI doesn't satisfy. It exposes the daemon address and
// token at /__discovery so the in-webview SPA can read them with a same-origin fetch, rather than
// reading the owner-only discovery file (which a webview sandbox cannot) or coupling to Wails'
// generated JS bindings. A daemon-served browser build can expose the same path.
func (a *App) assetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__discovery" {
			// Return 503 until the daemon is ready so frontends know to retry.
			// An empty token means startup is still in progress.
			if a.discovery.Token == "" {
				http.Error(w, "daemon not ready", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(a.discovery)
			return
		}
		if r.URL.Path == "/__integration" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(a.integration)
			return
		}
		http.NotFound(w, r)
	})
}
