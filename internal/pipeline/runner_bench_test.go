package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/pipeline"
	"github.com/elythi0n/virta/internal/platform"
)

// countingSink consumes without recording, so the benchmark measures the runner, not the
// growth of a slice.
type countingSink struct {
	name string
	n    int
}

func (c *countingSink) Name() string                                  { return c.name }
func (c *countingSink) Consume(context.Context, platform.Event) error { c.n++; return nil }
func (c *countingSink) Close() error                                  { return nil }

// BenchmarkRunner_Throughput pushes messages through a realistic shape — a couple of
// annotation stages and two sinks — to track per-message overhead. The feed must sustain
// well above real chat rates (hundreds of messages/second aggregate); this guards against
// regressions in the hot path.
func BenchmarkRunner_Throughput(b *testing.B) {
	r := pipeline.NewRunner(pipeline.Options{
		Clock:  clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)),
		Stages: []pipeline.Stage{pipeline.NewTagStage("a"), pipeline.NewTagStage("b")},
		Sinks:  []pipeline.Sink{&countingSink{name: "s1"}, &countingSink{name: "s2"}},
	})
	r.Start()
	defer func() { _ = r.Close() }()

	m := platform.MessageEvent{Message: platform.UnifiedMessage{
		ID: "x", Type: platform.TypeChat,
		Segments: []platform.Segment{{Kind: platform.SegText, Text: "hello world"}},
	}}

	b.ResetTimer()
	for range b.N {
		r.Submit(m)
	}
	// Drain so the work is actually accounted to this run.
	for r.Stats().Dispatched < int64(b.N) {
		time.Sleep(time.Microsecond)
	}
}
