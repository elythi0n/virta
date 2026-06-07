// Package search defines the SearchIndex port: a typed abstraction over the Tier-2 optional search
// backend (Meilisearch). Tier-1 (FTS5/tsvector) is always available via the store and continues to
// power the /v1/search endpoint. A SearchIndex upgrades the search quality with typo tolerance,
// instant results, and (optionally) hybrid keyword+semantic search; it is entirely opt-in.
//
// The architecture matches the other ports in this project: one interface, two implementations
// (noop and meilisearch), both satisfying the same contract so callers never branch on the backend.
package search

import (
	"context"
	"time"
)

// SearchMode controls which matching strategy the backend uses.
type SearchMode string

const (
	ModeKeyword  SearchMode = "keyword"
	ModeSemantic SearchMode = "semantic"
	ModeHybrid   SearchMode = "hybrid"
)

// Query is the unified search request.
type Query struct {
	Text     string
	Channel  string // "platform:slug"; "" = all
	Author   string
	Platform string
	TimeFrom time.Time
	TimeTo   time.Time
	Mode     SearchMode // defaults to keyword
	Limit    int
}

// Result is one search hit.
type Result struct {
	ID      string    `json:"id"`
	Channel string    `json:"channel"`
	Author  string    `json:"author"`
	Body    string    `json:"body"`
	Type    string    `json:"type"`
	SentAt  time.Time `json:"sent_at"`
	Score   float64   `json:"score,omitempty"` // relevance, 0 if unavailable
}

// Document is the indexable form of a message.
type Document struct {
	ID      string    `json:"id"`
	Channel string    `json:"channel"`
	Author  string    `json:"author"`
	Body    string    `json:"body"`
	Type    string    `json:"type"`
	SentAt  time.Time `json:"sent_at"`
}

// Index is the search backend port. Implementations: Noop (zero-config FTS5 fallback) and
// Meilisearch (opt-in, better quality).
type Index interface {
	// Name identifies the implementation for diagnostics.
	Name() string
	// Available reports whether the backend is reachable and ready.
	Available(ctx context.Context) bool
	// Index adds or replaces documents in the index. Idempotent on the doc ID.
	Index(ctx context.Context, docs []Document) error
	// Search returns hits for the query.
	Search(ctx context.Context, q Query) ([]Result, error)
	// Delete removes a document by id.
	Delete(ctx context.Context, id string) error
	// DeleteChannel removes all documents for a channel.
	DeleteChannel(ctx context.Context, channel string) error
}
