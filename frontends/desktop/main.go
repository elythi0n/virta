// Command virta-desktop is the Virta desktop shell: a native window hosting the web UI that
// supervises or attaches to a local virtad daemon.
//
// The UI is served from an embedded HTTP server at http://localhost:PORT/ so that
// location.origin is a real HTTP origin. This allows iframe embeds from Twitch and Kick
// to pass their CSP frame-ancestors checks (which allow localhost but not wails://).
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// assets holds the built web UI (frontends/web/dist), copied in by `make app` before the build.
// The `all:` prefix keeps the embed valid via the committed .gitkeep before the first build.
//
//go:embed all:assets
var assets embed.FS

func main() {
	svc := newApp(assets)

	app := application.New(application.Options{
		Name:        "Virta",
		Description: "Unified live chat for Twitch, Kick, and X",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: svc.assetHandler(assets),
		},
		OnShutdown: svc.shutdown,
	})

	svc.setApp(app)

	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Virta",
		Width:            1280,
		Height:           832,
		MinWidth:         960,
		MinHeight:        600,
		Frameless:        true,
		URL:              svc.StartURL(), // http://localhost:PORT/ — real HTTP origin
		BackgroundColour: application.NewRGBA(14, 15, 18, 255),
		DevToolsEnabled:  devToolsEnabled,
	})

	svc.setMainWindow(mainWindow)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
