// Package plugins holds the daemon-side seams for the plugin system (ADR-035). It defines the
// DataSource contract — a server-side poller or socket that publishes live external data as
// namespaced plugin.<id>.* events — so a contributing panel subscribes to it over the existing WS
// bus the same way the feed subscribes to messages, and no API keys, CORS, or rate limits ever
// reach the renderer. There is no plugin host yet (remote loading is Phase 8); this is the seam our
// own future panels (e.g. Markets) flow through, kept real so the contract is exercised early.
package source

import (
	"context"
	"encoding/json"

	"github.com/elythi0n/virta/internal/platform"
)

// Emitter is the pipeline entry point a DataSource feeds its events into (*pipeline.Runner satisfies it).
type Emitter interface {
	Submit(ev platform.Event)
}

// DataSource is a server-side source of live external data. Run drives until ctx is cancelled,
// calling publish for each update; the host namespaces and forwards those onto the WS bus. ID is a
// stable plugin id (e.g. "markets"); a stream name groups updates within a plugin (e.g. "tick").
type DataSource interface {
	ID() string
	Run(ctx context.Context, publish func(stream string, data any)) error
}

// Run hosts a DataSource: it supplies a publish function that marshals each update and submits it
// as a PluginEvent on the namespaced stream "plugin.<id>.<stream>". A payload that fails to marshal
// is dropped (never poisons the bus). Blocks until the source's Run returns (ctx cancel).
func Run(ctx context.Context, ds DataSource, emit Emitter) error {
	prefix := "plugin." + ds.ID() + "."
	return ds.Run(ctx, func(stream string, data any) {
		b, err := json.Marshal(data)
		if err != nil {
			return
		}
		emit.Submit(platform.PluginEvent{Stream: prefix + stream, Data: b})
	})
}
