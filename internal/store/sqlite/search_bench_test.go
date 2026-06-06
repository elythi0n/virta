package sqlite_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/store/sqlite"
)

// words is a small vocabulary the synthetic message bodies draw from, so search terms match a
// realistic fraction of a large log rather than everything or nothing.
var words = []string{
	"hello", "world", "stream", "clip", "poggers", "kappa", "gg", "lol", "nice", "play",
	"raid", "sub", "thanks", "chat", "vod", "emote", "mod", "ban", "timeout", "hype",
}

// BenchmarkSearch_LargeDB records full-text search latency over a 1M-row log at three
// selectivities, so the exit criterion ("what did X say" < 100 ms) is measured against a realistic
// query rather than a pathological one. Run with:
//
//	go test ./internal/store/sqlite -run=^$ -bench=Search
//
// A distinctive term ("needle", seeded into ~0.05% of rows) is the "what did X say" shape; a
// common word ("poggers", ~5% of rows) is the worst case, slower because FTS must walk every match.
func BenchmarkSearch_LargeDB(b *testing.B) {
	const rows = 1_000_000
	const batch = 5_000

	path := filepath.Join(b.TempDir(), "bench.db")
	clk := clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC))
	db, err := sqlite.Open(path, clk, id.NewFake("rec"))
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	b.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	buf := make([]platform.UnifiedMessage, 0, batch)
	for i := 0; i < rows; i++ {
		body := fmt.Sprintf("%s %s %s %s %s %s",
			words[i%20], words[(i+3)%20], words[(i+7)%20], words[(i+11)%20], words[(i+13)%20], words[(i+17)%20])
		if i%2000 == 0 {
			body += " needle" // a distinctive term in ~0.05% of rows
		}
		buf = append(buf, platform.UnifiedMessage{
			ID:       fmt.Sprintf("msg-%09d", i),
			Channel:  platform.ChannelRef{ID: "ch", Platform: platform.Twitch},
			Platform: platform.Twitch, Type: platform.TypeChat,
			Author:     platform.Author{ID: fmt.Sprintf("u%d", i%20), DisplayName: "user"},
			Segments:   []platform.Segment{{Kind: platform.SegText, Text: body}},
			ReceivedAt: base.Add(time.Duration(i) * time.Millisecond),
			SentAt:     base,
		})
		if len(buf) == batch {
			if err := db.Messages().Append(ctx, buf); err != nil {
				b.Fatalf("append: %v", err)
			}
			buf = buf[:0]
		}
	}

	run := func(b *testing.B, q store.SearchQuery) {
		b.Helper()
		for i := 0; i < b.N; i++ {
			hits, err := db.Messages().Search(ctx, q)
			if err != nil {
				b.Fatalf("search: %v", err)
			}
			if len(hits) == 0 {
				b.Fatal("expected matches")
			}
		}
	}

	b.Run("distinctive_term", func(b *testing.B) { run(b, store.SearchQuery{Text: "needle", Limit: 100}) })
	b.Run("distinctive_by_author", func(b *testing.B) { run(b, store.SearchQuery{Text: "needle", Author: "u0", Limit: 100}) })
	b.Run("common_term_worstcase", func(b *testing.B) { run(b, store.SearchQuery{Text: "poggers", Limit: 100}) })
}
