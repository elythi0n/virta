package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Embedder generates vector embeddings for text. Used for semantic search (Tier 3).
// Implementations: OllamaEmbedder (local, free), VoyageEmbedder (hosted).
type Embedder interface {
	ID() string
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// OllamaEmbedder calls the Ollama local embeddings API (POST /api/embed).
// Recommended models: nomic-embed-text (768-dim), bge-m3 (1024-dim).
type OllamaEmbedder struct {
	baseURL string
	model   string
	dims    int
}

// NewOllamaEmbedder creates an embedder using Ollama. 0 dims = auto (768 for nomic-embed-text).
func NewOllamaEmbedder(baseURL, model string, dims int) *OllamaEmbedder {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	if dims == 0 {
		dims = 768
	}
	return &OllamaEmbedder{baseURL: baseURL, model: model, dims: dims}
}

func (o *OllamaEmbedder) ID() string      { return "ollama" }
func (o *OllamaEmbedder) Dimensions() int { return o.dims }

func (o *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": o.model, "input": texts})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ollama embed: parse: %w", err)
	}
	return out.Embeddings, nil
}

// VoyageEmbedder calls the Voyage AI embedding API. Recommended for production quality with Anthropic.
type VoyageEmbedder struct {
	apiKey string
	model  string
	dims   int
}

// NewVoyageEmbedder creates a Voyage AI embedder (model: voyage-3, voyage-3-lite recommended).
func NewVoyageEmbedder(apiKey, model string) *VoyageEmbedder {
	if model == "" {
		model = "voyage-3"
	}
	return &VoyageEmbedder{apiKey: apiKey, model: model, dims: 1024}
}

func (v *VoyageEmbedder) ID() string      { return "voyage" }
func (v *VoyageEmbedder) Dimensions() int { return v.dims }

func (v *VoyageEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": v.model, "input": texts})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.voyageai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+v.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage embed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("voyage embed: parse: %w", err)
	}
	result := make([][]float32, len(out.Data))
	for i, d := range out.Data {
		result[i] = d.Embedding
	}
	return result, nil
}
