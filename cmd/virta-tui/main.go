// Command virta-tui is the Virta terminal frontend. It connects to a running virtad
// daemon (started automatically if none is found) and renders the live feed in your terminal.
// Requires a truecolor-capable terminal for the full palette; degrades to 256-color and 16-color.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elythi0n/virta/internal/buildinfo"
	"github.com/elythi0n/virta/internal/config"
	"github.com/elythi0n/virta/internal/tui"
)

func main() {
	var (
		addr     = flag.String("addr", "", "virtad address (host:port); defaults to the daemon discovered from the runtime dir")
		token    = flag.String("token", "", "bearer token; defaults to the one in the discovery file")
		channels = flag.String("channels", "", "comma-separated channel keys, e.g. twitch:forsen,kick:xqc; empty = all")
		theme    = flag.String("theme", "dark", "theme: dark | light")
		profile  = flag.String("profile", "", "activate this profile on connect (by name)")
		version  = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *version {
		fmt.Printf("virta-tui %s\n", buildinfo.String())
		return
	}

	// Discover addr + token from the runtime dir if not provided.
	a, t := *addr, *token
	if a == "" || t == "" {
		cfg, err := config.Load()
		if err == nil {
			if disc, err := discoverDaemon(cfg.RuntimeDir); err == nil {
				if a == "" {
					a = disc.Addr
				}
				if t == "" {
					t = disc.Token
				}
			}
		}
	}
	if a == "" || t == "" {
		fmt.Fprintln(os.Stderr, "virta-tui: could not discover the daemon. Start virtad first, or pass --addr and --token.")
		os.Exit(1)
	}

	var chans []string
	if *channels != "" {
		chans = strings.Split(*channels, ",")
	}
	_ = profile // TODO: activate the named profile via API on connect

	m := tui.New(tui.Config{
		Addr:     a,
		Token:    t,
		Channels: chans,
		Theme:    *theme,
	})
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "virta-tui: %v\n", err)
		os.Exit(1)
	}
}

func discoverDaemon(runtimeDir string) (struct{ Addr, Token string }, error) {
	disc, err := readDiscovery(runtimeDir)
	return disc, err
}
