// Command virtad is the Virta engine daemon: it owns all platform connections, message
// normalization, storage, and the local API that every frontend (desktop, terminal, web,
// overlay) connects to as a client.
//
// It binds a loopback HTTP/WebSocket listener and writes a discovery file announcing its
// address and auth token, so a frontend on the same machine can find and authenticate to it
// without any configuration. Run it and leave it running; frontends attach and detach freely.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elythi0n/virta/internal/app"
	"github.com/elythi0n/virta/internal/buildinfo"
	"github.com/elythi0n/virta/internal/config"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("virtad %s\n", buildinfo.String())
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fatal("load config", err)
	}

	d, err := app.NewDaemon(cfg)
	if err != nil {
		fatal("start daemon", err)
	}
	if err := d.Start(); err != nil {
		fatal("listen", err)
	}

	// Run until interrupted, then shut down gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	stop() // restore default signal handling so a second Ctrl-C force-quits

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Close(shutCtx); err != nil {
		fatal("shutdown", err)
	}
}

func fatal(what string, err error) {
	fmt.Fprintf(os.Stderr, "virtad: %s: %v\n", what, err)
	os.Exit(1)
}
