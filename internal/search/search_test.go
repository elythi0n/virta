package search

import (
	"context"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

func TestInMemory_IndexAndSearch(t *testing.T) {
	idx := NewInMemory()
	docs := []Document{
		{ID: "1", Channel: "twitch:forsen", Author: "Alice", Body: "hello world", SentAt: time.Now()},
		{ID: "2", Channel: "twitch:forsen", Author: "Bob", Body: "goodbye world", SentAt: time.Now()},
		{ID: "3", Channel: "kick:xqc", Author: "Alice", Body: "hello there", SentAt: time.Now()},
	}
	if err := idx.Index(context.Background(), docs); err != nil {
		t.Fatalf("Index: %v", err)
	}
	// Search across all channels.
	hits, err := idx.Search(context.Background(), Query{Text: "hello", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits for 'hello', got %d", len(hits))
	}
	// Filter by channel.
	channelHits, _ := idx.Search(context.Background(), Query{Text: "hello", Channel: "twitch:forsen"})
	if len(channelHits) != 1 || channelHits[0].ID != "1" {
		t.Errorf("channel filter hits = %v", channelHits)
	}
	// Filter by author.
	authorHits, _ := idx.Search(context.Background(), Query{Text: "world", Author: "Bob"})
	if len(authorHits) != 1 || authorHits[0].ID != "2" {
		t.Errorf("author filter hits = %v", authorHits)
	}
}

func TestInMemory_Delete(t *testing.T) {
	idx := NewInMemory()
	_ = idx.Index(context.Background(), []Document{{ID: "a", Channel: "twitch:x", Author: "u", Body: "test"}})
	_ = idx.Delete(context.Background(), "a")
	hits, _ := idx.Search(context.Background(), Query{Text: "test"})
	if len(hits) != 0 {
		t.Error("document should be deleted")
	}
}

func TestInMemory_DeleteChannel(t *testing.T) {
	idx := NewInMemory()
	_ = idx.Index(context.Background(), []Document{
		{ID: "1", Channel: "twitch:a", Body: "test"},
		{ID: "2", Channel: "twitch:b", Body: "test"},
	})
	_ = idx.DeleteChannel(context.Background(), "twitch:a")
	hits, _ := idx.Search(context.Background(), Query{Text: "test"})
	if len(hits) != 1 || hits[0].ID != "2" {
		t.Errorf("after DeleteChannel: %v", hits)
	}
}

func TestNoop_AlwaysEmpty(t *testing.T) {
	idx := Noop{}
	if idx.Available(context.Background()) {
		t.Error("Noop should not be available")
	}
	hits, err := idx.Search(context.Background(), Query{Text: "hello"})
	if err != nil || len(hits) != 0 {
		t.Errorf("Noop.Search should return empty: err=%v hits=%v", err, hits)
	}
}

func TestIndexer_IndexesMessages(t *testing.T) {
	idx := NewInMemory()
	indexer := NewIndexer(idx, nil, 100)
	indexer.Start()
	defer indexer.Close()

	_ = indexer.Consume(context.Background(), platform.MessageEvent{Message: platform.UnifiedMessage{
		ID:       "m1",
		Platform: platform.Twitch,
		Channel:  platform.ChannelRef{Slug: "forsen"},
		Type:     platform.TypeChat,
		Author:   platform.Author{DisplayName: "Alice"},
		Segments: []platform.Segment{{Kind: platform.SegText, Text: "hello from Alice"}},
		SentAt:   time.Now(),
		Ephemeral: false,
	}})
	// Give the async loop time to flush.
	time.Sleep(700 * time.Millisecond)
	hits, _ := idx.Search(context.Background(), Query{Text: "alice"})
	if len(hits) == 0 {
		t.Error("expected indexed message to be searchable")
	}
}

func TestIndexer_SkipsEphemeral(t *testing.T) {
	idx := NewInMemory()
	indexer := NewIndexer(idx, nil, 100)
	indexer.Start()
	defer indexer.Close()

	_ = indexer.Consume(context.Background(), platform.MessageEvent{Message: platform.UnifiedMessage{
		ID: "m1", Platform: platform.Twitch, Channel: platform.ChannelRef{Slug: "ch"},
		Type: platform.TypeChat, Author: platform.Author{DisplayName: "u"},
		Segments: []platform.Segment{{Kind: platform.SegText, Text: "ephemeral"}},
		Ephemeral: true,
	}})
	time.Sleep(700 * time.Millisecond)
	hits, _ := idx.Search(context.Background(), Query{Text: "ephemeral"})
	if len(hits) != 0 {
		t.Error("ephemeral message should not be indexed")
	}
}
