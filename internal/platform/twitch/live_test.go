//go:build live

// Live smoke test against real Twitch IRC. Excluded from the normal suite; run on demand:
//
//	go test -tags live -run TestLive ./internal/platform/twitch/...
//
// It connects anonymously to a busy channel and waits for a real chat message, proving the
// WebSocket transport, handshake, and parsing work end to end against the actual service.
package twitch

import (
	"context"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

func TestLive_AnonymousReadsRealChat(t *testing.T) {
	a := New(Options{})
	t.Cleanup(func() { _ = a.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.Join(ctx, platform.ChannelRef{Slug: "xqc"}, platform.ModeAnonymous); err != nil {
		t.Fatalf("join: %v", err)
	}

	for {
		select {
		case ev := <-a.Events():
			if me, ok := ev.(platform.MessageEvent); ok {
				t.Logf("received: %s: %s", me.Message.Author.DisplayName, me.Message.PlainText())
				return
			}
		case <-ctx.Done():
			t.Fatal("no chat message received within timeout (is the channel live and chatty?)")
		}
	}
}
