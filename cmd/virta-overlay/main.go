// Command virta-overlay opens a native transparent always-on-top click-through window
// that loads an overlay URL. It is a standalone binary separate from virtad.
//
// The window title is set to "virta-overlay:<panel>" so OBS can find and capture it by name
// using the Window Capture source.
//
// Usage:
//
//	virta-overlay --panel chat --token <tok>
//	virta-overlay --url http://localhost:8344/overlay?panel=chat&token=<tok>&transparent=1
package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
)

func main() {
	var (
		rawURL = flag.String("url", "", "full overlay URL (overrides --panel/--port/--token)")
		panel  = flag.String("panel", "", "panel id used to build the overlay URL")
		port   = flag.Int("port", 8344, "virtad HTTP port")
		token  = flag.String("token", "", "auth token passed to the overlay URL")
		width  = flag.Int("width", 400, "window width in pixels")
		height = flag.Int("height", 600, "window height in pixels")
		x      = flag.Int("x", 0, "window X position")
		y      = flag.Int("y", 0, "window Y position")
		title  = flag.String("title", "", "window title (default: virta-overlay:<panel>)")
	)
	flag.Parse()

	target := *rawURL
	if target == "" {
		if *panel == "" {
			fmt.Fprintln(os.Stderr, "virta-overlay: --panel or --url is required")
			os.Exit(1)
		}
		q := url.Values{}
		q.Set("panel", *panel)
		if *token != "" {
			q.Set("token", *token)
		}
		q.Set("transparent", "1")
		target = fmt.Sprintf("http://localhost:%d/overlay?%s", *port, q.Encode())
	}

	wTitle := *title
	if wTitle == "" {
		p := *panel
		if p == "" {
			// Extract panel from URL query if it was provided directly.
			if u, err := url.Parse(target); err == nil {
				p = u.Query().Get("panel")
			}
		}
		if p != "" {
			wTitle = "virta-overlay:" + p
		} else {
			wTitle = "virta-overlay"
		}
	}

	if err := runOverlay(target, wTitle, *x, *y, *width, *height); err != nil {
		fmt.Fprintf(os.Stderr, "virta-overlay: %v\n", err)
		os.Exit(1)
	}
}
