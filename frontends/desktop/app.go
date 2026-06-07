package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"runtime"
	"time"

	"github.com/elythi0n/virta/internal/api"
	"github.com/elythi0n/virta/internal/config"
	"github.com/elythi0n/virta/internal/desktop"
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
}

func newApp() *App { return &App{} }

func (a *App) setApp(app *application.App)               { a.app = app }
func (a *App) setMainWindow(w *application.WebviewWindow) { a.mainWindow = w }

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

// ---- Bound methods (callable from frontend JS via window.go.main.App.*) ----

// WindowMinimise minimises the main window.
func (a *App) WindowMinimise() { a.mainWindow.Minimise() }

// WindowToggleMaximise toggles the main window between maximised and restored.
func (a *App) WindowToggleMaximise() { a.mainWindow.ToggleMaximise() }

// WindowClose quits the application cleanly.
func (a *App) WindowClose() { a.app.Quit() }

// BrowserOpen opens url in the user's default system browser (xdg-open on Linux).
func (a *App) BrowserOpen(url string) {
	_ = a.app.Browser.OpenURL(url)
}

// OpenInspector opens the WebKit developer tools for the main window.
// DevToolsEnabled must be true in the window options (set when the debug tag is active).
func (a *App) OpenInspector() {
	a.mainWindow.OpenDevTools()
}

// OpenStreamWindow opens url in a new native WebView window sized for video.
// This works in Wails v3 which supports multiple windows. The window has
// GPU policy OnDemand so Twitch's IVS player can access hardware video decoding.
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

// ---- Asset handler ----

// assetHandler serves requests from the embedded UI:
//   - /__discovery: returns daemon address + token; 503 until ready.
//   - /__integration: host OS/session capabilities.
//   - All other paths: served from the embedded assets FS.
//
// The embed.FS root contains an "assets/" sub-directory (matching the //go:embed
// directive). fs.Sub strips that prefix so "/" maps to assets/index.html, not to a
// directory listing of the "assets/" folder.
func (a *App) assetHandler(embeds fs.FS) http.Handler {
	sub, err := fs.Sub(embeds, "assets")
	if err != nil {
		sub = embeds // should never happen; fallback to raw FS
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
		default:
			fileServer.ServeHTTP(w, r)
		}
	})
}
