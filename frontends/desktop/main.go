// Command virta-desktop is the Virta desktop shell: a native window hosting the web UI.
// Wails v3 is used for multi-window support (stream player windows) and proper DevTools.
package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:assets
var assets embed.FS

func main() {
	svc := newApp()
	svc.pendingDeepLink = parseCLIDeepLink(os.Args)

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
