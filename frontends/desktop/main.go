// Command virta-desktop is the Virta desktop shell: a native window hosting the web UI that
// supervises or attaches to a local virtad daemon.
//
// Wails v3 supports multiple windows, so stream panels open as native windows instead of
// browser popups, and the WebKit inspector is available per-window via DevToolsEnabled.
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
	svc := newApp()

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
		URL:              "/",
		BackgroundColour: application.NewRGBA(14, 15, 18, 255),
		DevToolsEnabled:  devToolsEnabled,
	})

	svc.setMainWindow(mainWindow)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
