package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAICompatProvider covers any OpenAI-compatible endpoint: OpenAI, xAI (Grok), Ollama,
// LM Studio, vLLM, OpenRouter. Tool calling is translated to the function-calling format.
type OpenAICompatProvider struct {
	id          string
	displayName string
	baseURL     string
	apiKey      string
	httpClient  *http.Client
}

// NewOpenAI returns an OpenAI provider.
func NewOpenAI(apiKey string) *OpenAICompatProvider {
	return newCompat("openai", "OpenAI", "https://api.openai.com/v1", apiKey)
}

// NewXAI returns an xAI (Grok) provider.
func NewXAI(apiKey string) *OpenAICompatProvider {
	return newCompat("xai", "xAI (Grok)", "https://api.x.ai/v1", apiKey)
}

// NewOllama returns an Ollama provider (local, no key needed).
// baseURL should be the root of the Ollama instance; /v1 is appended automatically if absent.
// Examples: http://ollama:11434 (Docker Compose service name), http://localhost:11434 (native).
func NewOllama(baseURL string) *OpenAICompatProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	return newCompat("ollama", "Ollama (local · free)", baseURL, "")
}

// NewCustom returns a provider pointing at any OpenAI-compatible endpoint.
func NewCustom(id, displayName, baseURL, apiKey string) *OpenAICompatProvider {
	return newCompat(id, displayName, baseURL, apiKey)
}

func newCompat(id, name, base, key string) *OpenAICompatProvider {
	return &OpenAICompatProvider{
		id:          id,
		displayName: name,
		baseURL:     strings.TrimRight(base, "/"),
		apiKey:      key,
		httpClient:  &http.Client{},
	}
}

func (p *OpenAICompatProvider) ID() string          { return p.id }
func (p *OpenAICompatProvider) DisplayName() string { return p.displayName }

func (p *OpenAICompatProvider) Verify(ctx context.Context) error {
	_, err := p.listModelsRaw(ctx)
	return err
}

func (p *OpenAICompatProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	raw, err := p.listModelsRaw(ctx)
	if err != nil {
		return nil, err
	}

	// Ollama native /api/tags → {"models":[{"name":"llama3.2:latest",...}]}
	// Parse this shape directly; don't fall through to the OpenAI path which would
	// silently succeed with an empty Data slice (JSON ignores unknown fields).
	if p.id == "ollama" {
		var ollamaR struct {
			Models []struct{ Name string `json:"name"` } `json:"models"`
		}
		if err := json.Unmarshal(raw, &ollamaR); err != nil {
			return nil, fmt.Errorf("ollama: parse models: %w", err)
		}
		out := make([]ModelInfo, 0, len(ollamaR.Models))
		for _, m := range ollamaR.Models {
			out = append(out, ModelInfo{ID: m.Name, DisplayName: ollamaDisplayName(m.Name), SupportsTools: modelSupportsTools(m.Name)})
		}
		return out, nil
	}

	// OpenAI-compat shape → {"data":[{"id":"..."},...]}
	type model struct {
		ID string `json:"id"`
	}
	var r struct {
		Data []model `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("%s: parse models: %w", p.id, err)
	}
	out := make([]ModelInfo, 0, len(r.Data))
	for _, m := range r.Data {
		out = append(out, ModelInfo{
			ID:            m.ID,
			DisplayName:   ollamaDisplayName(m.ID),
			SupportsTools: modelSupportsTools(m.ID),
		})
	}
	return out, nil
}

func (p *OpenAICompatProvider) Complete(ctx context.Context, req CompletionRequest) (Stream, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type fnParam struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	}
	type fn struct {
		Type     string  `json:"type"`
		Function fnParam `json:"function"`
	}
	msgs := make([]msg, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, msg{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, msg{Role: string(m.Role), Content: m.Content})
	}
	body := map[string]any{
		"model":    req.Model,
		"messages": msgs,
		"stream":   true,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		tools := make([]fn, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, fn{Type: "function", Function: fnParam{Name: t.Name, Description: t.Description, Parameters: sanitizeSchema(t.InputSchema)}})
		}
		body["tools"] = tools
	}
	b, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: complete: %w", p.id, err)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s: %d: %s", p.id, resp.StatusCode, string(body))
	}
	return &openaiStream{body: resp.Body}, nil
}

// openaiStream reads OpenAI-style `data: {...}` SSE lines.
type openaiStream struct {
	body io.ReadCloser
	sc   *bufio.Scanner
}

func (s *openaiStream) Close() error { return s.body.Close() }

func (s *openaiStream) Next() (Event, error) {
	if s.sc == nil {
		s.sc = bufio.NewScanner(s.body)
	}
	for s.sc.Scan() {
		line := s.sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := line[6:]
		if payload == "[DONE]" {
			return Event{Kind: EventDone}, io.EOF
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			if chunk.Choices[0].FinishReason == "stop" {
				return Event{Kind: EventDone}, io.EOF
			}
			if t := chunk.Choices[0].Delta.Content; t != "" {
				return Event{Kind: EventText, Text: t}, nil
			}
		}
	}
	return Event{Kind: EventDone}, io.EOF
}

func (p *OpenAICompatProvider) listModelsRaw(ctx context.Context) ([]byte, error) {
	// Ollama: use the native /api/tags endpoint (available in all versions; more reliable
	// than the OpenAI-compat /v1/models which was added later and requires no auth).
	url := p.baseURL + "/models"
	if p.id == "ollama" {
		url = strings.TrimSuffix(p.baseURL, "/v1") + "/api/tags"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: list models: %w", p.id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("%s: %d: %s", p.id, resp.StatusCode, string(body))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// sanitizeSchema ensures "required" in a JSON Schema map is never null — some strict providers
// (xAI, Grok) reject null as an invalid type for a required JSON Schema keyword.
func sanitizeSchema(s map[string]any) map[string]any {
	if s == nil {
		return s
	}
	out := make(map[string]any, len(s))
	for k, v := range s {
		if k == "required" && v == nil {
			out[k] = []string{} // null → empty array
			continue
		}
		if sub, ok := v.(map[string]any); ok {
			out[k] = sanitizeSchema(sub)
			continue
		}
		out[k] = v
	}
	return out
}

// ollamaDisplayName converts a raw model ID into a human-readable display name.
// Strips the `:latest` tag (redundant noise), keeps other tags (e.g. `:7b`, `:q4`), and
// replaces hyphens with spaces so "llama3.2" renders as "llama3.2" not "llama3-2:latest".
func ollamaDisplayName(id string) string {
	// Strip bare ":latest" suffix — it adds no information.
	name := strings.TrimSuffix(id, ":latest")
	// For model IDs like "grok-beta", humanize the separators.
	name = strings.ReplaceAll(name, "-", " ")
	return name
}

// modelSupportsTools is a heuristic: assume tool support unless the model ID indicates otherwise.
// Providers don't reliably expose this in /models, so we use known patterns.
func modelSupportsTools(id string) bool {
	low := strings.ToLower(id)
	// Models known to NOT support tool/function calling.
	for _, nosup := range []string{"instruct", "babbage", "davinci", "text-", "embed", "whisper", "tts", "dall-e", "vision"} {
		if strings.Contains(low, nosup) {
			return false
		}
	}
	return true
}
