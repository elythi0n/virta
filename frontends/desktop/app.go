package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/api"
	"github.com/elythi0n/virta/internal/config"
	"github.com/elythi0n/virta/internal/desktop"
	"github.com/wailsapp/wails/v3/internal/assetserver/bundledassets"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// startupTimeout bounds how long we wait for a freshly launched daemon to come up before the UI
// loads anyway (it will show a disconnected state and retry).
const startupTimeout = 15 * time.Second

// App is the shell's lifecycle owner: it finds or launches the daemon and tells the web UI how to
// reach it. It is also the Wails service — its exported methods are callable from the frontend.
type App struct {
	app         *application.App
	mainWindow  *application.WebviewWindow
	discovery   api.Discovery
	integration IntegrationReport
	daemon      *daemonProcess

	// httpAddr is the address of the local HTTP server that serves the UI.
	// The webview loads from this address so that location.origin is
	// "http://localhost:PORT" and CSP frame-ancestors checks (Twitch, Kick embeds) pass.
	httpAddr string
}

func newApp(embeds fs.FS) *App {
	a := &App{}
	a.httpAddr = a.startHTTPServer(embeds)
	return a
}

func (a *App) setApp(app *application.App)                { a.app = app }
func (a *App) setMainWindow(w *application.WebviewWindow) { a.mainWindow = w }

// startHTTPServer binds to a random loopback port, serves the embedded UI and
// the Wails v3 runtime from http://localhost:PORT/, and returns the address.
// Serving from HTTP instead of the wails:// custom scheme means the embedding
// origin is http://localhost:PORT, which passes Twitch/Kick CSP frame-ancestors checks.
func (a *App) startHTTPServer(embeds fs.FS) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "" // fallback: window will load from wails:// but embeds won't work for streaming
	}
	addr := fmt.Sprintf("localhost:%d", ln.Addr().(*net.TCPAddr).Port)
	handler := a.buildHandler(embeds)
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	return addr
}

// StartURL returns the HTTP URL the webview should load.
func (a *App) StartURL() string {
	if a.httpAddr == "" {
		return "/"
	}
	return "http://" + a.httpAddr + "/"
}

// ServiceStartup satisfies application.ServiceStartup — called by Wails when the app launches.
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.integration = resolveIntegration(runtime.GOOS, currentSession())
	cfg, err := config.Load()
	if err != nil {
		return nil // discovery stays empty; UI will show "not connected" and retry
	}
	startCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if d, err := desktop.Ensure(startCtx, cfg.RuntimeDir, a.launchDaemon); err == nil {
		a.discovery = d
	}
	return nil
}

func (a *App) shutdown() {
	a.daemon.stop()
}

// ---- Bound methods (callable from frontend JS via window.wails.*) ----

// WindowMinimise minimises the main window.
func (a *App) WindowMinimise() { a.mainWindow.Minimise() }

// WindowToggleMaximise toggles the main window between maximised and restored.
func (a *App) WindowToggleMaximise() { a.mainWindow.ToggleMaximise() }

// WindowClose quits the application cleanly.
func (a *App) WindowClose() { a.app.Quit() }

// BrowserOpen opens url in the user's default system browser.
func (a *App) BrowserOpen(url string) {
	_ = a.app.Browser.OpenURL(url)
}

// OpenInspector opens the WebKit developer tools for the main window.
func (a *App) OpenInspector() {
	a.mainWindow.OpenDevTools()
}

// OpenStreamWindow opens url in a second native WebView window sized for video.
func (a *App) OpenStreamWindow(title, url string) {
	w := a.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  title,
		Width:  1280,
		Height: 720,
		URL:    url,
		Linux: application.LinuxWindow{
			WebviewGpuPolicy: application.WebviewGpuPolicyOnDemand,
		},
	})
	w.Show()
}

// ---- HTTP asset handler ----

// wailsRuntimeTag loads the Wails v3 runtime. Must appear before any app JS.
const wailsRuntimeTag = `<script type="module" src="/wails/runtime.js"></script>`

// buildHandler returns the HTTP handler for the embedded UI. Serving over HTTP
// (not the wails:// custom scheme) means location.origin is
// http://localhost:PORT, which Twitch/Kick CSP frame-ancestors accepts.
func (a *App) buildHandler(embeds fs.FS) http.Handler {
	sub, err := fs.Sub(embeds, "assets")
	if err != nil {
		sub = embeds
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/__discovery":
			if a.discovery.Token == "" {
				http.Error(w, "daemon not ready", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(a.discovery)

		case "/__integration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(a.integration)

		case "/wails/runtime.js":
			// Serve the compiled Wails v3 runtime so window.wails is set.
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write(bundledassets.RuntimeJS)

		case "/", "/index.html":
			data, readErr := fs.ReadFile(sub, "index.html")
			if readErr != nil {
				http.Error(w, "index.html not found", http.StatusNotFound)
				return
			}
			html := strings.Replace(string(data), "</head>", wailsRuntimeTag+"</head>", 1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(html))

		default:
			fileServer.ServeHTTP(w, r)
		}
	})
}

// assetHandler returns the same handler — used by Wails' AssetOptions so the
// wails:// scheme still works as a fallback (e.g. for popout panels opened by
// Wails itself). The main window loads from the HTTP server via StartURL().
func (a *App) assetHandler() http.Handler {
	// The HTTP server already has the correct handler; return a no-op that
	// only handles Wails internal paths (/wails/...) so the AssetServer
	// doesn't 404 on them.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/wails/runtime.js" {
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write(bundledassets.RuntimeJS)
			return
		}
		http.NotFound(w, r)
	})
}
