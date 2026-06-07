package anthropic

import (
	"context"
	"fmt"
	"io"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/elythi0n/virta/internal/llm"
)

// anthropicPricing is the shipped per-model pricing table (USD per million tokens).
var anthropicPricing = map[string]llm.Pricing{
	"claude-opus-4-8":           {InputPerMTok: 5.00, OutputPerMTok: 25.00},
	"claude-sonnet-4-6":         {InputPerMTok: 3.00, OutputPerMTok: 15.00},
	"claude-haiku-4-5-20251001": {InputPerMTok: 1.00, OutputPerMTok: 5.00},
}

// AnthropicProvider wraps the official Anthropic Go SDK.
type AnthropicProvider struct {
	client anthropicsdk.Client
}

// NewAnthropic creates an Anthropic provider with the given API key.
func NewAnthropic(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{client: anthropicsdk.NewClient(option.WithAPIKey(apiKey))}
}

func (p *AnthropicProvider) ID() string          { return "anthropic" }
func (p *AnthropicProvider) DisplayName() string { return "Anthropic" }

func (p *AnthropicProvider) Verify(ctx context.Context) error {
	_, err := p.client.Models.List(ctx, anthropicsdk.ModelListParams{})
	if err != nil {
		return fmt.Errorf("anthropic: verify: %w", err)
	}
	return nil
}

func (p *AnthropicProvider) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	resp, err := p.client.Models.List(ctx, anthropicsdk.ModelListParams{})
	if err != nil {
		return nil, fmt.Errorf("anthropic: list models: %w", err)
	}
	out := make([]llm.ModelInfo, 0, len(resp.Data))
	for _, m := range resp.Data {
		id := string(m.ID)
		info := llm.ModelInfo{
			ID:            id,
			DisplayName:   string(m.DisplayName),
			SupportsTools: true,
			Family:        anthropicFamily(id),
		}
		if pr, ok := anthropicPricing[id]; ok {
			info.Pricing = &pr
		}
		out = append(out, info)
	}
	return out, nil
}

func (p *AnthropicProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Stream, error) {
	maxTok := int64(req.MaxTokens)
	if maxTok <= 0 {
		maxTok = 8096
	}
	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(req.Model),
		MaxTokens: maxTok,
	}
	if req.System != "" {
		params.System = []anthropicsdk.TextBlockParam{{Text: req.System}}
	}
	for _, m := range req.Messages {
		text := anthropicsdk.NewTextBlock(m.Content)
		switch m.Role {
		case llm.RoleUser:
			params.Messages = append(params.Messages, anthropicsdk.NewUserMessage(text))
		case llm.RoleAssistant:
			params.Messages = append(params.Messages, anthropicsdk.NewAssistantMessage(text))
		}
	}
	for _, t := range req.Tools {
		schemaParam := anthropicsdk.ToolInputSchemaParam{}
		if props, ok := t.InputSchema["properties"]; ok {
			schemaParam.Properties = props
		}
		params.Tools = append(params.Tools, anthropicsdk.ToolUnionParam{
			OfTool: &anthropicsdk.ToolParam{
				Name:        t.Name,
				Description: anthropicsdk.String(t.Description),
				InputSchema: schemaParam,
			},
		})
	}
	stream := p.client.Messages.NewStreaming(ctx, params)
	return newAnthropicStream(stream), nil
}

// anthropicStream wraps ssestream.Stream[MessageStreamEventUnion] in our Stream interface.
type anthropicStream struct {
	s    *ssestream.Stream[anthropicsdk.MessageStreamEventUnion]
	done bool
}

func newAnthropicStream(s *ssestream.Stream[anthropicsdk.MessageStreamEventUnion]) *anthropicStream {
	return &anthropicStream{s: s}
}

func (a *anthropicStream) Next() (llm.Event, error) {
	if a.done {
		return llm.Event{}, io.EOF
	}
	for a.s.Next() {
		ev := a.s.Current()
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				return llm.Event{Kind: llm.EventText, Text: ev.Delta.Text}, nil
			}
		case "message_stop":
			a.done = true
			return llm.Event{Kind: llm.EventDone}, io.EOF
		}
	}
	if err := a.s.Err(); err != nil {
		return llm.Event{}, err
	}
	a.done = true
	return llm.Event{Kind: llm.EventDone}, io.EOF
}

func (a *anthropicStream) Close() error { return a.s.Close() }

func anthropicFamily(id string) string {
	low := strings.ToLower(id)
	switch {
	case strings.Contains(low, "opus"):
		return "Opus"
	case strings.Contains(low, "sonnet"):
		return "Sonnet"
	case strings.Contains(low, "haiku"):
		return "Haiku"
	default:
		return "Claude"
	}
}
