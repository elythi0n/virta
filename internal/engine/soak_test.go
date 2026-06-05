//go:build soak

// Soak harness — excluded from `make ci`, run on demand via `make soak` (optionally
// SOAK_SECONDS / SOAK_RATE). It drives the full ingest path at a sustained rate for a long
// duration and watches for memory growth, goroutine leaks, and drops — the offline
// counterpart to the live 30-minute run on two busy channels (tracked in docs/live-debt.md).
package engine

import (
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/pipeline"
	"github.com/elythi0n/virta/internal/platform"
)

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func heapKiB() uint64 {
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc / 1024
}

func TestSoak_FullPath(t *testing.T) {
	seconds := envInt("SOAK_SECONDS", 60)
	rate := envInt("SOAK_RATE", 200)

	sink := &measuringSink{}
	runner := pipeline.NewRunner(pipeline.Options{Clock: clock.System{}, Sinks: []pipeline.Sink{sink}})
	runner.Start()
	t.Cleanup(func() { _ = runner.Close() })

	eng := New(runner, id.NewULID(clock.System{}))
	tw := platform.NewFakeAdapter(platform.Twitch, platform.Capabilities{ReadAnonymous: true})
	eng.Register(tw)
	t.Cleanup(func() { _ = eng.Close() })

	startHeap := heapKiB()
	startGoroutines := runtime.NumGoroutine()
	var peakHeap uint64

	tick := time.NewTicker(time.Second / time.Duration(rate))
	defer tick.Stop()
	report := time.NewTicker(10 * time.Second)
	defer report.Stop()
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	sent := 0

	for time.Now().Before(deadline) {
		select {
		case <-tick.C:
			tw.EmitMessage(stampedMessage().Message)
			sent++
		case <-report.C:
			if h := heapKiB(); h > peakHeap {
				peakHeap = h
			}
			t.Logf("t+%ds: sent=%d delivered=%d heap=%d KiB goroutines=%d drops=%d",
				int(time.Until(deadline).Seconds()), sent, sink.got(), heapKiB(),
				runtime.NumGoroutine(), runner.Stats().SinkDrops["measure"])
		}
	}

	endHeap := heapKiB()
	endGoroutines := runtime.NumGoroutine()
	drops := runner.Stats().SinkDrops["measure"]
	t.Logf("SOAK DONE: %ds @ %d msg/s — sent=%d delivered=%d drops=%d p95=%v",
		seconds, rate, sent, sink.got(), drops, sink.p95())
	t.Logf("memory: heap %d→%d KiB (peak %d), goroutines %d→%d",
		startHeap, endHeap, peakHeap, startGoroutines, endGoroutines)

	if drops != 0 {
		t.Errorf("dropped %d events over the soak at %d msg/s", drops, rate)
	}
	// Goroutines must not grow without bound (allow small scheduler slack).
	if endGoroutines > startGoroutines+5 {
		t.Errorf("goroutine leak: %d → %d", startGoroutines, endGoroutines)
	}
}
