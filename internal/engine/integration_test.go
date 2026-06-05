package engine

import (
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/pipeline"
	"github.com/elythi0n/virta/internal/platform"
)

// TestEngine_MergesTwoChannelsOrderedByReceivedAt is the 1.4 exit criterion: messages from
// two adapters flow engine → pipeline → sink, each gets a ULID, and they arrive ordered by
// the ReceivedAt stamp the pipeline assigns (the feed's merge key).
func TestEngine_MergesTwoChannelsOrderedByReceivedAt(t *testing.T) {
	sink := pipeline.NewRecordingSink("test")
	runner := pipeline.NewRunner(pipeline.Options{Clock: clock.System{}, Sinks: []pipeline.Sink{sink}})
	runner.Start()
	t.Cleanup(func() { _ = runner.Close() })

	eng := New(runner, id.NewULID(clock.System{}))
	tw := platform.NewFakeAdapter(platform.Twitch, platform.Capabilities{ReadAnonymous: true})
	kick := platform.NewFakeAdapter(platform.Kick, platform.Capabilities{ReadAnonymous: true})
	eng.Register(tw)
	eng.Register(kick)
	t.Cleanup(func() { _ = eng.Close() })

	// Interleave messages from both platforms.
	msg := func(p platform.Platform, slug, body string) platform.UnifiedMessage {
		return platform.UnifiedMessage{
			Platform: p,
			Channel:  platform.ChannelRef{Platform: p, Slug: slug},
			Segments: []platform.Segment{{Kind: platform.SegText, Text: body}},
		}
	}
	tw.EmitMessage(msg(platform.Twitch, "forsen", "t1"))
	kick.EmitMessage(msg(platform.Kick, "xqc", "k1"))
	tw.EmitMessage(msg(platform.Twitch, "forsen", "t2"))
	kick.EmitMessage(msg(platform.Kick, "xqc", "k2"))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(sink.Messages()) < 4 {
		time.Sleep(time.Millisecond)
	}
	got := sink.Messages()
	if len(got) != 4 {
		t.Fatalf("sink received %d messages, want 4", len(got))
	}

	platforms := map[platform.Platform]int{}
	var last time.Time
	for i, m := range got {
		if m.ID == "" {
			t.Errorf("message %d has no ULID", i)
		}
		if m.ReceivedAt.IsZero() {
			t.Errorf("message %d has no ReceivedAt stamp", i)
		}
		if m.ReceivedAt.Before(last) {
			t.Errorf("message %d ReceivedAt %v is before previous %v — not feed-ordered", i, m.ReceivedAt, last)
		}
		last = m.ReceivedAt
		platforms[m.Platform]++
	}
	if platforms[platform.Twitch] != 2 || platforms[platform.Kick] != 2 {
		t.Errorf("merged platforms = %v, want 2 twitch + 2 kick", platforms)
	}
}
