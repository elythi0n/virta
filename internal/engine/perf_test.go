package engine

import (
	"context"
	"encoding/json"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/pipeline"
	"github.com/elythi0n/virta/internal/platform"
)

// measuringSink mimics the real WebSocket hub's per-event cost (JSON-encode every message)
// and records end-to-end latency, so the benchmark and soak exercise the full ingest path:
// adapter → engine (ULID mint) → pipeline (stamp + fan-out) → sink encode.
type measuringSink struct {
	mu        sync.Mutex
	count     int
	latencies []time.Duration
}

func (s *measuringSink) Name() string { return "measure" }

func (s *measuringSink) Consume(_ context.Context, ev platform.Event) error {
	me, ok := ev.(platform.MessageEvent)
	if !ok {
		return nil
	}
	if _, err := json.Marshal(me.Message); err != nil {
		return err
	}
	s.mu.Lock()
	s.count++
	// The emit timestamp travels in the body so we can measure latency without a spare field.
	if len(me.Message.Segments) > 0 {
		if ns, err := strconv.ParseInt(me.Message.Segments[0].Text, 10, 64); err == nil {
			s.latencies = append(s.latencies, time.Since(time.Unix(0, ns)))
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *measuringSink) Close() error { return nil }

func (s *measuringSink) got() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

func (s *measuringSink) p95() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.latencies) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), s.latencies...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[(len(sorted)*95)/100]
}

func stampedMessage() platform.MessageEvent {
	return platform.MessageEvent{Message: platform.UnifiedMessage{
		Platform:          platform.Twitch,
		Channel:           platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"},
		PlatformMessageID: "p",
		Author:            platform.Author{Login: "user", DisplayName: "User"},
		Segments:          []platform.Segment{{Kind: platform.SegText, Text: strconv.FormatInt(time.Now().UnixNano(), 10)}},
	}}
}

// BenchmarkFullPath measures throughput of the whole ingest path (ULID mint → stamp →
// fan-out → JSON encode). Closing the runner inside the timed region drains every buffered
// event, so the measurement includes processing, not just enqueueing — and it terminates
// cleanly despite the pipeline's drop-oldest backpressure.
func BenchmarkFullPath(b *testing.B) {
	sink := &measuringSink{}
	runner := pipeline.NewRunner(pipeline.Options{Clock: clock.System{}, Sinks: []pipeline.Sink{sink}})
	runner.Start()
	eng := New(runner, id.NewULID(clock.System{}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.ingest(stampedMessage())
	}
	_ = runner.Close() // drains the ingest queue and sink buffers before the timer stops
	b.StopTimer()
}

// TestFullPath_SustainedRateNoDrops drives the full path at 200 msg/s and asserts the feed
// keeps up with zero drops, recording p95 latency for the baseline.
func TestFullPath_SustainedRateNoDrops(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test skipped in -short")
	}
	const rate = 200
	const seconds = 2
	const want = rate * seconds

	sink := &measuringSink{}
	runner := pipeline.NewRunner(pipeline.Options{Clock: clock.System{}, Sinks: []pipeline.Sink{sink}})
	runner.Start()
	t.Cleanup(func() { _ = runner.Close() })

	eng := New(runner, id.NewULID(clock.System{}))
	tw := platform.NewFakeAdapter(platform.Twitch, platform.Capabilities{ReadAnonymous: true})
	eng.Register(tw)
	t.Cleanup(func() { _ = eng.Close() })

	tick := time.NewTicker(time.Second / rate)
	defer tick.Stop()
	for i := 0; i < want; i++ {
		<-tick.C
		tw.EmitMessage(stampedMessage().Message)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && sink.got() < want {
		time.Sleep(5 * time.Millisecond)
	}
	if got := sink.got(); got != want {
		t.Errorf("delivered %d/%d messages at %d msg/s", got, want, rate)
	}
	if drops := runner.Stats().SinkDrops["measure"]; drops != 0 {
		t.Errorf("sink dropped %d events at %d msg/s — feed cannot keep up", drops, rate)
	}

	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	t.Logf("baseline: %d msg/s sustained, p95 ingest→encode latency = %v, drops = 0, heap = %d KiB, goroutines = %d",
		rate, sink.p95(), ms.HeapAlloc/1024, runtime.NumGoroutine())
}
