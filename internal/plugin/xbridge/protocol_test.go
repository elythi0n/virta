package xbridge

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	frames := []Frame{
		{Type: FrameMessage, Payload: MarshalPayload(MessagePayload{BroadcastID: "test", Author: "bob", Text: "hello"})},
		{Type: FrameStatus, Payload: MarshalPayload(StatusPayload{State: StateConnected})},
		{Type: FrameError, Payload: MarshalPayload(ErrorPayload{Code: "selector_missing", Detail: ".chat-row"})},
	}
	for _, f := range frames {
		if err := enc.Write(f); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	for i, want := range frames {
		got, err := dec.Next()
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if got.Type != want.Type {
			t.Errorf("frame %d: type = %q, want %q", i, got.Type, want.Type)
		}
	}
	if _, err := dec.Next(); err != io.EOF {
		t.Error("expected EOF after all frames")
	}
}

func TestMarshalPayload(t *testing.T) {
	p := MessagePayload{BroadcastID: "live123", Author: "alice", Text: "hi!", ContentHash: "abc"}
	raw := MarshalPayload(p)
	if len(raw) == 0 {
		t.Error("MarshalPayload returned empty")
	}
}

func TestXMessageToEvent(t *testing.T) {
	p := MessagePayload{BroadcastID: "live123", Author: "alice", Text: "hi!", ContentHash: "sha1"}
	ev := xMessageToEvent(p)
	me, ok := ev.(interface{ isEvent() bool })
	if ok {
		_ = me
	}
	// Check it's a platform.MessageEvent with the right fields.
	type msgEv interface {
		GetMessage() interface{}
	}
	// Just verify the event is non-nil and carries the content hash as ID.
	if ev == nil {
		t.Fatal("nil event")
	}
	// Use fmt to introspect without importing platform here.
	s := fmt.Sprintf("%+v", ev)
	if !strings.Contains(s, "sha1") || !strings.Contains(s, "alice") {
		t.Errorf("event missing expected fields: %s", s)
	}
}
