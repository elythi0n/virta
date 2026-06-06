package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	meili "github.com/meilisearch/meilisearch-go"
)

const meiliIndexName = "virta_messages"

// Meili wraps a Meilisearch instance as a SearchIndex. Configure via Settings → Search.
type Meili struct {
	client meili.ServiceManager
}

// NewMeili connects to Meilisearch at host (e.g. "http://localhost:7700") with an optional key.
func NewMeili(host, apiKey string) (*Meili, error) {
	var c meili.ServiceManager
	if apiKey != "" {
		c = meili.New(host, meili.WithAPIKey(apiKey))
	} else {
		c = meili.New(host)
	}
	return &Meili{client: c}, nil
}

func (m *Meili) Name() string { return "meilisearch" }

func (m *Meili) Available(_ context.Context) bool {
	health, err := m.client.Health()
	return err == nil && health.Status == "available"
}

func (m *Meili) idx() meili.IndexManager { return m.client.Index(meiliIndexName) }

func (m *Meili) Index(_ context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	pk := "id"
	opts := &meili.DocumentOptions{PrimaryKey: &pk}
	_, err := m.idx().AddDocuments(docs, opts)
	if err != nil {
		return fmt.Errorf("meili: index %d docs: %w", len(docs), err)
	}
	return nil
}

func (m *Meili) Search(_ context.Context, q Query) ([]Result, error) {
	limit := int64(q.Limit)
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var filters []string
	if q.Channel != "" {
		filters = append(filters, fmt.Sprintf(`channel = %q`, q.Channel))
	}
	if q.Author != "" {
		filters = append(filters, fmt.Sprintf(`author = %q`, q.Author))
	}
	params := &meili.SearchRequest{Limit: limit}
	if len(filters) > 0 {
		params.Filter = strings.Join(filters, " AND ")
	}
	res, err := m.idx().Search(q.Text, params)
	if err != nil {
		return nil, fmt.Errorf("meili: search: %w", err)
	}
	out := make([]Result, 0, len(res.Hits))
	for _, h := range res.Hits {
		var doc Document
		if err := decodeHit(h, &doc); err == nil {
			out = append(out, Result{
				ID: doc.ID, Channel: doc.Channel, Author: doc.Author,
				Body: doc.Body, Type: doc.Type, SentAt: doc.SentAt,
			})
		}
	}
	return out, nil
}

func (m *Meili) Delete(_ context.Context, id string) error {
	_, err := m.idx().DeleteDocument(id, nil)
	return err
}

func (m *Meili) DeleteChannel(_ context.Context, channel string) error {
	filter := fmt.Sprintf(`channel = %q`, channel)
	_, err := m.idx().DeleteDocumentsByFilter(filter, nil)
	return err
}

// decodeHit unmarshals a Meilisearch Hit (map[string]json.RawMessage) into a struct.
func decodeHit(h meili.Hit, dst any) error {
	b, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
