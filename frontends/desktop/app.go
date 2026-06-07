package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/api"
	"github.com/elythi0n/virta/internal/config"
	"github.com/elythi0n/virta/internal/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const startupTimeout = 15 * time.Second

// App is the shell's lifecycle owner and the Wails service (bound methods callable from JS).
type App struct {
	app        *application.App
	mainWindow *application.WebviewWindow
	discovery  api.Discovery
	integration IntegrationReport
	daemon     *daemonProcess
}

func newApp() *App { return &App{} }

func (a *App) setApp(app *application.App)               { a.app = app }
func (a *App) setMainWindow(w *application.WebviewWindow) { a.mainWindow = w }

// ServiceStartup is called by Wails when the app launches.
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.integration = resolveIntegration(runtime.GOOS, currentSession())
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	startCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if d, err := desktop.Ensure(startCtx, cfg.RuntimeDir, a.launchDaemon); err == nil {
		a.discovery = d
	}
	return nil
}

func (a *App) shutdown() { a.daemon.stop() }

// ---- Window controls (called from frontend via window.wails.Window / Application) ----
func (a *App) WindowMinimise()       { a.mainWindow.Minimise() }
func (a *App) WindowToggleMaximise() { a.mainWindow.ToggleMaximise() }
func (a *App) WindowClose()          { a.app.Quit() }

// BrowserOpen opens url in the system browser. In the Wails webview
// window.wails.Browser.OpenURL is available, but this bound method is kept
// as a fallback callable from service code.
func (a *App) BrowserOpen(url string) { _ = a.app.Browser.OpenURL(url) }

// OpenInspector opens the WebKit developer tools for the main window.
func (a *App) OpenInspector() { a.mainWindow.OpenDevTools() }

// OpenStreamWindow opens a native stream player window for the given channel.
// The window loads the actual Twitch/Kick channel page (not the embed iframe)
// so video decoding happens via Chromium's media stack if available, or the
// user can interact with it in a native Wails window.
func (a *App) OpenStreamWindow(platform, slug string) {
	var url string
	switch platform {
	case "twitch":
		url = "https://www.twitch.tv/" + slug
	case "kick":
		url = "https://kick.com/" + slug
	default:
		return
	}
	title := slug + " — " + platform
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

// wailsRuntimeTag must appear in every HTML page so window.wails is available.
const wailsRuntimeTag = `<script type="module" src="/wails/runtime.js"></script>`

// assetHandler is used by Wails' AssetOptions. The wails:// custom scheme
// routes all requests through this handler AND through the HTTPTransport
// middleware (which serves POST /wails/runtime for IPC). Loading from wails://
// keeps IPC, drag, and window controls working.
func (a *App) assetHandler(embeds fs.FS) http.Handler {
	sub, err := fs.Sub(embeds, "assets")
	if err != nil {
		sub = embeds
	}
	bundled := application.BundledAssetFileServer(sub)

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
			bundled.ServeHTTP(w, r)
		}
	})
}
