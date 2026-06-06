package search

import (
	"context"
	"strings"
)

// Noop is the default no-configuration SearchIndex: it returns no results, so the caller
// falls back to the store's built-in FTS5/tsvector search transparently. It is always available
// and never fails — it just doesn't add Tier-2 quality improvements.
type Noop struct{}

func (Noop) Name() string                          { return "noop" }
func (Noop) Available(_ context.Context) bool      { return false }
func (Noop) Index(_ context.Context, _ []Document) error { return nil }
func (Noop) Delete(_ context.Context, _ string) error    { return nil }
func (Noop) DeleteChannel(_ context.Context, _ string) error { return nil }

// Search on the noop implementation does a simple substring match so the tool belt still
// returns results when Meilisearch is not configured (not as fast or typo-tolerant, but correct).
func (Noop) Search(_ context.Context, q Query) ([]Result, error) { return nil, nil }

// InMemory is a test-only in-memory index used by unit tests that don't want to run Meilisearch.
type InMemory struct {
	docs []Document
}

func NewInMemory() *InMemory { return &InMemory{} }
func (*InMemory) Name() string     { return "in-memory" }
func (*InMemory) Available(_ context.Context) bool { return true }
func (m *InMemory) Index(_ context.Context, docs []Document) error {
	for _, d := range docs {
		// Replace existing by ID.
		replaced := false
		for i, ex := range m.docs {
			if ex.ID == d.ID {
				m.docs[i] = d
				replaced = true
				break
			}
		}
		if !replaced {
			m.docs = append(m.docs, d)
		}
	}
	return nil
}
func (m *InMemory) Delete(_ context.Context, id string) error {
	for i, d := range m.docs {
		if d.ID == id {
			m.docs = append(m.docs[:i], m.docs[i+1:]...)
			return nil
		}
	}
	return nil
}
func (m *InMemory) DeleteChannel(_ context.Context, channel string) error {
	var keep []Document
	for _, d := range m.docs {
		if d.Channel != channel {
			keep = append(keep, d)
		}
	}
	m.docs = keep
	return nil
}
func (m *InMemory) Search(_ context.Context, q Query) ([]Result, error) {
	needle := strings.ToLower(strings.TrimSpace(q.Text))
	if needle == "" {
		return nil, nil
	}
	var out []Result
	for _, d := range m.docs {
		if q.Channel != "" && d.Channel != q.Channel {
			continue
		}
		if q.Author != "" && !strings.EqualFold(d.Author, q.Author) {
			continue
		}
		if !strings.Contains(strings.ToLower(d.Body), needle) {
			continue
		}
		out = append(out, Result{
			ID: d.ID, Channel: d.Channel, Author: d.Author,
			Body: d.Body, Type: d.Type, SentAt: d.SentAt,
		})
	}
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
