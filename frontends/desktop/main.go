// Command virta-desktop is the Virta desktop shell: a native window hosting the web UI that
// supervises or attaches to a local virtad daemon. It is a separate Go module so its WebKit/CGO
// dependency (via Wails) stays out of the core module's build.
//
// Wails v2 is single-window, so the dock's pop-out-to-window degrades here to in-app floating
// groups (true native multi-window waits on Wails v3). See docs/08 and ADR-032.
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// assets holds the built web UI (frontends/web/dist), copied in by `make app` before the build.
// The `all:` prefix keeps the embed valid via the committed .gitkeep before the first build.
//
//go:embed all:assets
var assets embed.FS

func main() {
	app := newApp()
	err := wails.Run(&options.App{
		Title:     "Virta",
		Width:     1280,
		Height:    832,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app.assetHandler(),
		},
		BackgroundColour: &options.RGBA{R: 14, G: 15, B: 18, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
	})
	if err != nil {
		panic(err)
	}
}
