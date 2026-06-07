package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime"
	"strings"
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

// assetHandler serves requests the embedded UI doesn't satisfy:
//
//   - /__discovery: returns {addr:"", token:"<TOKEN>"} (addr is always empty so the
//     frontend uses same-origin relative URLs; the proxy below forwards them to the daemon).
//     Returns 503 until the daemon is ready, which the frontend retries.
//
//   - /__integration: host OS/session capabilities for the settings panel.
//
//   - /v1/*, /overlay, /popout, /overlay.html, etc.: reverse-proxied to the daemon.
//     Using a proxy instead of the daemon's direct address avoids cross-origin CORS
//     issues: the Wails webview is served from a custom scheme (wails://wails) that is a
//     different origin from http://127.0.0.1:PORT; any request carrying an Authorization
//     header would trigger a CORS preflight that the daemon does not handle.
func (a *App) assetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/__discovery":
			if a.discovery.Token == "" {
				http.Error(w, "daemon not ready", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			// Addr is intentionally empty: the frontend uses same-origin relative URLs
			// and this handler proxies them to the daemon.
			_ = json.NewEncoder(w).Encode(api.Discovery{Token: a.discovery.Token})

		case r.URL.Path == "/__integration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(a.integration)

		case strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/v1":
			a.proxyToDaemon(w, r)

		default:
			http.NotFound(w, r)
		}
	})
}

// proxyToDaemon forwards the request to the embedded daemon. If the daemon is not
// ready yet the request gets a 503. httputil.ReverseProxy handles both regular HTTP
// and WebSocket upgrades (Upgrade: websocket), so /v1/stream works too.
func (a *App) proxyToDaemon(w http.ResponseWriter, r *http.Request) {
	if a.discovery.Addr == "" {
		http.Error(w, "daemon not ready", http.StatusServiceUnavailable)
		return
	}
	target, err := url.Parse("http://" + a.discovery.Addr)
	if err != nil {
		http.Error(w, "bad daemon address", http.StatusInternalServerError)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// Strip the X-Forwarded-For / Host rewriting that would expose internals.
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
	}
	proxy.ServeHTTP(w, r)
}
