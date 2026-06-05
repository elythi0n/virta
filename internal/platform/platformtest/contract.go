// Package platformtest provides a reusable conformance suite for platform.Adapter
// implementations. The fake runs it here; each real adapter (twitch/kick/x) runs the same
// suite, so an implementation can never quietly diverge from the contract (ADR-024).
//
// It asserts only the mode-independent invariants that hold without network access, so it
// is safe in `make ci`. Behavior that needs a live endpoint is covered by build-tagged
// live tests (docs/live-debt.md).
package platformtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// RunAdapterContract verifies the universal Adapter invariants. newReadOnly must return a
// fresh adapter in a read-only state (Capabilities().Send == false) — e.g. anonymous mode —
// so Send/Moderate are expected to be unsupported.
func RunAdapterContract(t *testing.T, newReadOnly func(t *testing.T) platform.Adapter) {
	t.Helper()

	t.Run("read-only adapter rejects Send and Moderate", func(t *testing.T) {
		a := newReadOnly(t)
		t.Cleanup(func() { _ = a.Close() })
		if a.Capabilities().Send {
			t.Fatal("newReadOnly returned an adapter with Send capability; contract expects read-only")
		}
		ch := platform.ChannelRef{Platform: a.Platform(), Slug: "somechannel"}
		if err := a.Send(context.Background(), ch, "hi", platform.SendOpts{}); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("Send on read-only adapter = %v, want ErrUnsupported", err)
		}
		if err := a.Moderate(context.Background(), platform.ModAction{Type: platform.ModBan, Channel: ch}); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("Moderate on read-only adapter = %v, want ErrUnsupported", err)
		}
	})

	t.Run("Events closes after Close", func(t *testing.T) {
		a := newReadOnly(t)
		evs := a.Events()
		if err := a.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		select {
		case _, open := <-evs:
			if open {
				// Drain any buffered events, then expect close.
				for range evs {
				}
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Events() not closed within 2s of Close()")
		}
	})

	t.Run("Close is idempotent", func(t *testing.T) {
		a := newReadOnly(t)
		if err := a.Close(); err != nil {
			t.Fatalf("first Close: %v", err)
		}
		if err := a.Close(); err != nil {
			t.Errorf("second Close: %v, want nil", err)
		}
	})

	t.Run("Platform is reported", func(t *testing.T) {
		a := newReadOnly(t)
		t.Cleanup(func() { _ = a.Close() })
		if a.Platform() == "" {
			t.Error("Platform() returned empty")
		}
	})
}
