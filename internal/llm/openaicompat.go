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
// baseURL should be the root of the Ollama instance (e.g. http://localhost:11434 or
// http://host.docker.internal:11434); /v1 is appended automatically if not already present.
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
	type model struct {
		ID string `json:"id"`
	}
	type resp struct {
		Data []model `json:"data"`
	}
	raw, err := p.listModelsRaw(ctx)
	if err != nil {
		return nil, err
	}
	var r resp
	if err := json.Unmarshal(raw, &r); err != nil {
		// Ollama returns a different shape: {"models":[{"name":"..."}]}
		var ollamaR struct {
			Models []struct{ Name string `json:"name"` } `json:"models"`
		}
		if err2 := json.Unmarshal(raw, &ollamaR); err2 == nil {
			out := make([]ModelInfo, 0, len(ollamaR.Models))
			for _, m := range ollamaR.Models {
				out = append(out, ModelInfo{ID: m.Name, DisplayName: m.Name, SupportsTools: modelSupportsTools(m.Name)})
			}
			return out, nil
		}
		return nil, fmt.Errorf("%s: parse models: %w", p.id, err)
	}
	out := make([]ModelInfo, 0, len(r.Data))
	for _, m := range r.Data {
		out = append(out, ModelInfo{
			ID:            m.ID,
			DisplayName:   m.ID,
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
			tools = append(tools, fn{Type: "function", Function: fnParam{Name: t.Name, Description: t.Description, Parameters: t.InputSchema}})
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
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

// modelSupportsTools is a heuristic based on model ID since OpenAI/xAI don't expose this in /models.
func modelSupportsTools(id string) bool {
	low := strings.ToLower(id)
	// Known no-tool models.
	for _, nosup := range []string{"instruct", "babbage", "davinci", "text-"} {
		if strings.Contains(low, nosup) {
			return false
		}
	}
	return true
}
