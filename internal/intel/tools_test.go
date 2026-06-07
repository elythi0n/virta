package intel

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/store"
)

func makeStore(t *testing.T) store.Store {
	t.Helper()
	s := store.NewMemory(clock.NewFake(time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)))
	ctx := context.Background()
	// Register a channel.
	ch, _ := s.Channels().Upsert(ctx, store.Channel{
		Platform: platform.Twitch, Slug: "forsen",
	})
	chID := ch.ID
	// Log some messages.
	mk := func(id, slug, uid, name, body string) platform.UnifiedMessage {
		return platform.UnifiedMessage{
			ID:         id,
			Channel:    platform.ChannelRef{ID: chID, Platform: platform.Twitch, Slug: slug},
			Platform:   platform.Twitch,
			Type:       platform.TypeChat,
			Author:     platform.Author{ID: uid, DisplayName: name},
			Segments:   []platform.Segment{{Kind: platform.SegText, Text: body}},
			SentAt:     time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC),
			ReceivedAt: time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC),
			Ephemeral:  false, // logging on — must be non-ephemeral to persist
		}
	}
	msgs := []platform.UnifiedMessage{
		mk("m1", "forsen", "u1", "Alice", "hello world"),
		mk("m2", "forsen", "u2", "Bob", "greetings"),
		mk("m3", "forsen", "u1", "Alice", "hello again"),
	}
	_ = s.Messages().Append(ctx, msgs)
	return s
}

func TestToolBelt_SearchMessages(t *testing.T) {
	tb := New(makeStore(t))
	tb.SetLogging(func() bool { return true })
	rows, err := tb.SearchMessages(context.Background(), SearchArgs{Query: "hello", Limit: 10})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 matches for 'hello', got %d", len(rows))
	}
}

func TestToolBelt_GetUserHistory(t *testing.T) {
	tb := New(makeStore(t))
	tb.SetLogging(func() bool { return true })
	rows, err := tb.GetUserHistory(context.Background(), UserHistoryArgs{Author: "Alice", Limit: 10})
	if err != nil {
		t.Fatalf("GetUserHistory: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 messages for Alice, got %d", len(rows))
	}
	for _, r := range rows {
		if r.Author != "Alice" {
			t.Errorf("expected Alice, got %q", r.Author)
		}
	}
}

func TestToolBelt_TopChatters(t *testing.T) {
	tb := New(makeStore(t))
	tb.SetLogging(func() bool { return true })
	top, err := tb.TopChatters(context.Background(), TopChattersArgs{Limit: 5})
	if err != nil {
		t.Fatalf("TopChatters: %v", err)
	}
	if len(top) == 0 {
		t.Fatal("expected at least one chatter")
	}
	// Alice has 2 messages, Bob has 1 — Alice should be first.
	if top[0].Author != "Alice" {
		t.Errorf("top chatter = %q, want Alice", top[0].Author)
	}
	if top[0].Count != 2 {
		t.Errorf("top count = %d, want 2", top[0].Count)
	}
}

func TestToolBelt_ListChannels(t *testing.T) {
	tb := New(makeStore(t))
	tb.SetLogging(func() bool { return true })
	channels, err := tb.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 1 || channels[0].Key != "twitch:forsen" {
		t.Errorf("channels = %+v", channels)
	}
}

func TestToolBelt_Dispatch_SearchMessages(t *testing.T) {
	tb := New(makeStore(t))
	tb.SetLogging(func() bool { return true }) // enable so history tools work in test
	args, _ := json.Marshal(SearchArgs{Query: "hello", Limit: 10})
	result, err := tb.Dispatch(context.Background(), "search_messages", args)
	if err != nil {
		t.Fatalf("Dispatch search_messages: %v", err)
	}
	rows, ok := result.([]MessageRow)
	if !ok || len(rows) == 0 {
		t.Errorf("expected non-empty rows, got %v", result)
	}
}

func TestToolBelt_Dispatch_UnknownTool(t *testing.T) {
	tb := New(makeStore(t))
	tb.SetLogging(func() bool { return true })
	_, err := tb.Dispatch(context.Background(), "nonexistent_tool", []byte(`{}`))
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestDescriptions(t *testing.T) {
	descs := Descriptions()
	if len(descs) < 6 {
		t.Fatalf("expected at least 6 tool descriptions, got %d", len(descs))
	}
	for _, d := range descs {
		if d.Name == "" || d.Description == "" {
			t.Errorf("tool missing name or description: %+v", d)
		}
	}
}
