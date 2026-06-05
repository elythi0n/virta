package pipeline_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/pipeline"
	"github.com/elythi0n/virta/internal/platform"
)

func newRunner(t *testing.T, stages []pipeline.Stage, sinkBuf int, sinks ...pipeline.Sink) *pipeline.Runner {
	t.Helper()
	r := pipeline.NewRunner(pipeline.Options{
		Clock:      clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)),
		Stages:     stages,
		Sinks:      sinks,
		SinkBuffer: sinkBuf,
	})
	r.Start()
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// waitFor polls cond until true or the deadline, failing the test otherwise.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func msg(id string) platform.MessageEvent {
	return platform.MessageEvent{Message: platform.UnifiedMessage{
		ID: id, Type: platform.TypeChat,
		Segments: []platform.Segment{{Kind: platform.SegText, Text: id}},
	}}
}

func TestRunner_FanInOrderingAndReceivedAtStamp(t *testing.T) {
	sink := pipeline.NewRecordingSink("sink")
	r := newRunner(t, nil, 0, sink)

	// Fan in from a fake adapter's event channel.
	adapter := platform.NewFakeAdapter(platform.Twitch, platform.Capabilities{ReadAnonymous: true})
	t.Cleanup(func() { _ = adapter.Close() })
	r.Attach(adapter.Events())

	for _, id := range []string{"a", "b", "c"} {
		adapter.EmitMessage(platform.UnifiedMessage{ID: id, Type: platform.TypeChat})
	}

	waitFor(t, "3 messages consumed", func() bool { return len(sink.Messages()) == 3 })
	got := sink.Messages()
	if got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" {
		t.Fatalf("order = %v, want a,b,c", []string{got[0].ID, got[1].ID, got[2].ID})
	}
	for _, m := range got {
		if m.ReceivedAt.IsZero() {
			t.Errorf("message %s missing ReceivedAt stamp", m.ID)
		}
	}
}

func TestRunner_StagesAnnotateInOrderBeforeSink(t *testing.T) {
	sink := pipeline.NewRecordingSink("sink")
	r := newRunner(t, []pipeline.Stage{pipeline.NewTagStage("x"), pipeline.NewTagStage("y")}, 0, sink)

	r.Submit(msg("m"))
	waitFor(t, "message consumed", func() bool { return len(sink.Messages()) == 1 })

	if got := sink.Messages()[0].PlainText(); got != "m x y" {
		t.Errorf("annotated = %q, want %q", got, "m x y")
	}
}

func TestRunner_SlowSinkDoesNotStallOthers(t *testing.T) {
	fast := pipeline.NewRecordingSink("fast")
	slow := pipeline.NewBlockingSink("slow")
	r := newRunner(t, nil, 100, fast, slow)

	for _, id := range []string{"1", "2", "3"} {
		r.Submit(msg(id))
	}
	// The fast sink gets everything even though slow is blocked in its first Consume.
	waitFor(t, "fast sink drained while slow blocked", func() bool { return len(fast.Messages()) == 3 })
	<-slow.Entered() // slow really is blocked
	if len(slow.Messages()) != 0 {
		t.Errorf("slow consumed %d while blocked, want 0", len(slow.Messages()))
	}
	slow.Release()
	waitFor(t, "slow sink drains after release", func() bool { return len(slow.Messages()) == 3 })
}

func TestRunner_DropOldestAndCounts(t *testing.T) {
	slow := pipeline.NewBlockingSink("slow")
	r := newRunner(t, nil, 2, slow) // tiny per-sink buffer

	r.Submit(msg("m1"))
	<-slow.Entered() // worker pulled m1 and is now blocked; its buffer is empty

	for _, id := range []string{"m2", "m3", "m4", "m5"} {
		r.Submit(msg(id))
	}
	// All 5 dispatched ⇒ all pushes to the (cap-2) buffer done; oldest two (m2,m3) dropped.
	waitFor(t, "5 dispatched", func() bool { return r.Stats().Dispatched == 5 })
	waitFor(t, "2 drops counted", func() bool { return r.Stats().SinkDrops["slow"] == 2 })

	slow.Release()
	// Consumed: m1 (in-flight) + the newest survivors m4, m5.
	waitFor(t, "3 consumed after release", func() bool { return len(slow.Messages()) == 3 })
	got := slow.Messages()
	ids := []string{got[0].ID, got[1].ID, got[2].ID}
	if ids[0] != "m1" || ids[1] != "m4" || ids[2] != "m5" {
		t.Errorf("consumed = %v, want [m1 m4 m5] (drop-oldest kept newest)", ids)
	}
}

// panicStage panics when it sees a message whose ID equals trigger.
type panicStage struct{ trigger string }

func (panicStage) Name() string { return "panic" }
func (s panicStage) Annotate(_ context.Context, m *platform.UnifiedMessage) error {
	if m.ID == s.trigger {
		panic("stage exploded on " + m.ID)
	}
	return nil
}

func TestRunner_StagePanicIsIsolated(t *testing.T) {
	sink := pipeline.NewRecordingSink("sink")
	r := newRunner(t, []pipeline.Stage{panicStage{trigger: "boom"}}, 0, sink)

	r.Submit(msg("ok1"))
	r.Submit(msg("boom")) // panics inside the stage
	r.Submit(msg("ok2"))

	// The runner survives: the two good messages reach the sink, the bad one is dropped.
	waitFor(t, "2 good messages consumed", func() bool { return len(sink.Messages()) == 2 })
	if r.Stats().StagePanics != 1 {
		t.Errorf("StagePanics = %d, want 1", r.Stats().StagePanics)
	}
	for _, m := range sink.Messages() {
		if m.ID == "boom" {
			t.Error("panicking message reached the sink")
		}
	}
}

// errStage returns an error for a triggering message, which drops it.
type errStage struct{ trigger string }

func (errStage) Name() string { return "err" }
func (s errStage) Annotate(_ context.Context, m *platform.UnifiedMessage) error {
	if m.ID == s.trigger {
		return errors.New("rejected")
	}
	return nil
}

func TestRunner_StageErrorDropsMessage(t *testing.T) {
	sink := pipeline.NewRecordingSink("sink")
	r := newRunner(t, []pipeline.Stage{errStage{trigger: "bad"}}, 0, sink)

	r.Submit(msg("good"))
	r.Submit(msg("bad"))
	waitFor(t, "good consumed", func() bool { return len(sink.Messages()) == 1 })
	if sink.Messages()[0].ID != "good" {
		t.Errorf("consumed %q, want good", sink.Messages()[0].ID)
	}
	if r.Stats().StageErrors != 1 {
		t.Errorf("StageErrors = %d, want 1", r.Stats().StageErrors)
	}
}

// alwaysPanic panics on every message it sees.
type alwaysPanic struct{}

func (alwaysPanic) Name() string                                             { return "always-panic" }
func (alwaysPanic) Annotate(context.Context, *platform.UnifiedMessage) error { panic("should not run") }

func TestRunner_NonMessageEventsBypassStagesAndReachSinks(t *testing.T) {
	sink := pipeline.NewRecordingSink("sink")
	// If stages ran on non-message events this stage would panic — so a clean run proves
	// deletions/health events skip the stage pipeline and go straight to sinks.
	r := newRunner(t, []pipeline.Stage{alwaysPanic{}}, 0, sink)
	r.Submit(platform.MessageDeletedEvent{PlatformMessageID: "p1"})
	r.Submit(platform.HealthEvent{Status: platform.HealthStatus{State: platform.HealthDegraded}})

	waitFor(t, "2 events consumed", func() bool { return len(sink.Events()) == 2 })
	if r.Stats().StagePanics != 0 {
		t.Errorf("StagePanics = %d, want 0 (stages must not run on non-message events)", r.Stats().StagePanics)
	}
}

func TestRunner_CloseIsIdempotentAndClosesSinks(t *testing.T) {
	sink := pipeline.NewRecordingSink("sink")
	r := pipeline.NewRunner(pipeline.Options{Clock: clock.NewFake(time.Now()), Sinks: []pipeline.Sink{sink}})
	r.Start()
	r.Submit(msg("x"))
	waitFor(t, "consumed", func() bool { return len(sink.Messages()) == 1 })

	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Errorf("second Close: %v, want nil", err)
	}
	if !sink.Closed() {
		t.Error("sink not closed by runner Close")
	}
}
