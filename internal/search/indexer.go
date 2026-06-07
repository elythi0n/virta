package search

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// Indexer is a pipeline Sink that fans logged messages to the SearchIndex in async batches,
// keeping the hot path latency-free. It honours the same logging-opt-in rule as the logbook:
// only non-ephemeral messages reach the index.
type Indexer struct {
	idx  Index
	buf  chan platform.UnifiedMessage
	log  *slog.Logger
	quit chan struct{}
}

// NewIndexer wraps an Index as a pipeline sink with a buffer of depth.
func NewIndexer(idx Index, log *slog.Logger, depth int) *Indexer {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Indexer{idx: idx, buf: make(chan platform.UnifiedMessage, depth), log: log, quit: make(chan struct{})}
}

// Name identifies the sink for diagnostics.
func (i *Indexer) Name() string { return "search-indexer/" + i.idx.Name() }

// Consume enqueues a message for async indexing; ephemeral messages and non-chat events are skipped.
func (i *Indexer) Consume(_ context.Context, ev platform.Event) error {
	me, ok := ev.(platform.MessageEvent)
	if !ok || me.Message.Ephemeral {
		return nil
	}
	select {
	case i.buf <- me.Message:
	default:
		// Buffer full — drop rather than block. The backfill job can recover gaps.
	}
	return nil
}

// Start drains the buffer in batches. Call once after building the pipeline.
func (i *Indexer) Start() {
	go i.loop()
}

// Close drains remaining messages and shuts down.
func (i *Indexer) Close() error {
	close(i.quit)
	return nil
}

func (i *Indexer) loop() {
	const batchSize = 50
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	var batch []platform.UnifiedMessage
	flush := func() {
		if len(batch) == 0 {
			return
		}
		docs := make([]Document, 0, len(batch))
		for _, m := range batch {
			chKey := string(m.Platform) + ":" + m.Channel.Slug
			body := bodyText(m)
			if body == "" {
				continue
			}
			docs = append(docs, Document{
				ID:      m.ID,
				Channel: chKey,
				Author:  m.Author.DisplayName,
				Body:    body,
				Type:    string(m.Type),
				SentAt:  m.SentAt,
			})
		}
		if len(docs) > 0 {
			if err := i.idx.Index(context.Background(), docs); err != nil {
				i.log.Warn("search indexer: batch failed", "err", err, "n", len(docs))
			}
		}
		batch = batch[:0]
	}
	for {
		select {
		case <-i.quit:
			flush()
			return
		case m := <-i.buf:
			batch = append(batch, m)
			if len(batch) >= batchSize {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}

func bodyText(m platform.UnifiedMessage) string {
	if len(m.Segments) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, seg := range m.Segments {
		sb.WriteString(seg.Text)
	}
	return strings.TrimSpace(sb.String())
}
