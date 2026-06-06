package plugins

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

type captureEmitter struct {
	mu     sync.Mutex
	events []platform.PluginEvent
}

func (c *captureEmitter) Submit(ev platform.Event) {
	if pe, ok := ev.(platform.PluginEvent); ok {
		c.mu.Lock()
		c.events = append(c.events, pe)
		c.mu.Unlock()
	}
}

// fakeSource publishes two updates on two streams, then returns.
type fakeSource struct{ id string }

func (f fakeSource) ID() string { return f.id }
func (f fakeSource) Run(_ context.Context, publish func(stream string, data any)) error {
	publish("tick", map[string]any{"sym": "BTC", "px": 42000})
	publish("status", "ok")
	return nil
}

func TestRun_NamespacesAndForwards(t *testing.T) {
	emit := &captureEmitter{}
	if err := Run(context.Background(), fakeSource{id: "markets"}, emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(emit.events) != 2 {
		t.Fatalf("emitted %d events, want 2", len(emit.events))
	}
	if emit.events[0].Stream != "plugin.markets.tick" {
		t.Errorf("stream[0] = %q, want plugin.markets.tick", emit.events[0].Stream)
	}
	if emit.events[1].Stream != "plugin.markets.status" {
		t.Errorf("stream[1] = %q, want plugin.markets.status", emit.events[1].Stream)
	}
	var tick map[string]any
	if err := json.Unmarshal(emit.events[0].Data, &tick); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if tick["sym"] != "BTC" {
		t.Errorf("payload = %v, want sym BTC", tick)
	}
}

// unmarshalable payloads are dropped rather than poisoning the bus.
type badSource struct{}

func (badSource) ID() string { return "bad" }
func (badSource) Run(_ context.Context, publish func(stream string, data any)) error {
	publish("x", make(chan int)) // channels don't marshal
	publish("y", 1)
	return nil
}

func TestRun_DropsUnmarshalablePayload(t *testing.T) {
	emit := &captureEmitter{}
	_ = Run(context.Background(), badSource{}, emit)
	if len(emit.events) != 1 || emit.events[0].Stream != "plugin.bad.y" {
		t.Fatalf("events = %+v, want only plugin.bad.y", emit.events)
	}
}
