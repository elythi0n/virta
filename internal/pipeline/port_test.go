package pipeline_test

import (
	"context"
	"testing"

	"github.com/elythi0n/virta/internal/pipeline"
	"github.com/elythi0n/virta/internal/platform"
)

func TestRecordingSink_CapturesEventsAndMessages(t *testing.T) {
	s := pipeline.NewRecordingSink("test")
	ctx := context.Background()

	msg := platform.MessageEvent{Message: platform.UnifiedMessage{ID: "m1", Type: platform.TypeChat}}
	del := platform.MessageDeletedEvent{PlatformMessageID: "p9"}
	if err := s.Consume(ctx, msg); err != nil {
		t.Fatal(err)
	}
	if err := s.Consume(ctx, del); err != nil {
		t.Fatal(err)
	}

	if got := s.Events(); len(got) != 2 {
		t.Fatalf("Events len = %d, want 2", len(got))
	}
	msgs := s.Messages()
	if len(msgs) != 1 || msgs[0].ID != "m1" {
		t.Fatalf("Messages = %v, want one with id m1", msgs)
	}
	if s.Closed() {
		t.Error("Closed() true before Close()")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if !s.Closed() {
		t.Error("Closed() false after Close()")
	}
}

func TestTagStage_AnnotatesInPlace(t *testing.T) {
	msg := platform.UnifiedMessage{Segments: []platform.Segment{{Kind: platform.SegText, Text: "hello"}}}
	st := pipeline.NewTagStage("[tagged]")
	if st.Name() != "tag:[tagged]" {
		t.Errorf("Name = %q", st.Name())
	}
	if err := st.Annotate(context.Background(), &msg); err != nil {
		t.Fatal(err)
	}
	if got := msg.PlainText(); got != "hello [tagged]" {
		t.Errorf("after stage PlainText = %q, want %q", got, "hello [tagged]")
	}
}

// Ordered stages compose — the property the runner relies on (step 0.3).
func TestStages_ComposeInOrder(t *testing.T) {
	msg := platform.UnifiedMessage{Segments: []platform.Segment{{Kind: platform.SegText, Text: "a"}}}
	stages := []pipeline.Stage{pipeline.NewTagStage("b"), pipeline.NewTagStage("c")}
	for _, st := range stages {
		if err := st.Annotate(context.Background(), &msg); err != nil {
			t.Fatal(err)
		}
	}
	if got := msg.PlainText(); got != "a b c" {
		t.Errorf("composed = %q, want %q", got, "a b c")
	}
}

func TestBlockingSink_BlocksUntilReleased(t *testing.T) {
	s := pipeline.NewBlockingSink("slow")
	done := make(chan error, 1)
	go func() { done <- s.Consume(context.Background(), platform.MessageEvent{}) }()

	select {
	case <-done:
		t.Fatal("Consume returned before Release")
	default:
	}
	s.Release()
	if err := <-done; err != nil {
		t.Fatalf("Consume after Release: %v", err)
	}
	if len(s.Events()) != 1 {
		t.Errorf("expected 1 event recorded after release")
	}
}
