// Package pipeline defines the single spine every message flows through: adapters produce
// platform.Events, an ordered set of pure Stages annotate each message, and concurrent
// Sinks consume the result behind per-sink buffers. Keeping one spine means new behavior
// is a new Stage or Sink, never a change threaded through the whole engine.
//
// Only MessageEvents run through Stages; every event (messages, deletions, clears, health)
// fans out to Sinks. The Runner (runner.go) implements the Pipeline interface defined here.
package pipeline

import (
	"context"

	"github.com/elythi0n/virta/internal/platform"
)

// Stage annotates a message in place. Stages are PURE: no I/O, no blocking, no mutation of
// shared state; lookup data (emote sets, word lists) is read from a snapshot refreshed
// outside the hot path. Stages run ordered and synchronously, microseconds each.
//
// A returned error means the message is dropped to diagnostics — it never panics the
// pipeline and never blocks the feed (the runner recovers stage panics).
type Stage interface {
	// Name identifies the stage in diagnostics (e.g. "filter", "annotate", "velocity").
	Name() string
	// Annotate enriches msg in place. Must not retain msg beyond the call.
	Annotate(ctx context.Context, msg *platform.UnifiedMessage) error
}

// Sink consumes events after all stages have run. Sinks run concurrently, each behind its
// own ring buffer managed by the runner — so a slow sink (e.g. webhook delivery) can never
// stall the feed. Sinks see the full event stream; each applies what's relevant
// (frontends honor velocity annotations, the logger ignores them).
type Sink interface {
	// Name identifies the sink in diagnostics (e.g. "wsclients", "logger", "webhooks").
	Name() string
	// Consume handles one event. Returning an error is for diagnostics only; the runner
	// keeps going. Consume must not retain the event's message beyond the call.
	Consume(ctx context.Context, ev platform.Event) error
	// Close releases the sink. Called once when the pipeline shuts down.
	Close() error
}

// Pipeline ingests events from any producer (adapters or the engine), runs stages on
// messages, and fans out to sinks. The Runner in this package implements it.
type Pipeline interface {
	// Submit enqueues an event. Non-blocking from the producer's perspective: the runner
	// owns buffering and drop-oldest backpressure per sink.
	Submit(ev platform.Event)
	// Close stops the pipeline and closes every sink. Idempotent.
	Close() error
}
