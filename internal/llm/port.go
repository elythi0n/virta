// Package llm defines the Provider port for LLM integration. Every LLM feature
// (the Ask pane agent loop, translation batches) goes through this interface, which means:
// 1. Adding a new provider is one new implementation file.
// 2. The usage meter can wrap any provider without changing callers.
// 3. All providers can be toggled off uniformly (the privacy contract).
//
// BYOK (bring your own key): users paste API keys into Settings; keys go into the OS keychain
// (never the DB). The Verify() call on entry catches bad keys early.
package llm

import (
	"context"
	"errors"
	"io"
)

// Provider is one LLM backend. All methods must be safe for concurrent use.
type Provider interface {
	// ID returns a stable machine identifier (anthropic | openai | xai | ollama | custom).
	ID() string
	// DisplayName is shown in the settings UI.
	DisplayName() string
	// Verify makes a cheap authenticated request to confirm the key is valid. Called on key paste.
	Verify(ctx context.Context) error
	// ListModels returns the provider's available models (results cached by the caller for 24 h).
	ListModels(ctx context.Context) ([]ModelInfo, error)
	// Complete runs the completion request and returns a streaming response.
	Complete(ctx context.Context, req CompletionRequest) (Stream, error)
}

// ModelInfo describes one model from a provider.
type ModelInfo struct {
	ID            string
	DisplayName   string
	Family        string // drives UI grouping, e.g. "Opus", "GPT-4"
	ContextWindow int
	SupportsTools bool     // models without tool use are shown disabled in the model picker
	Pricing       *Pricing // nil when the provider doesn't expose it
	Deprecated    bool     // shown but sorted last, marked in the UI
}

// Pricing is per-million-token pricing (input and output).
type Pricing struct {
	InputPerMTok  float64 // USD
	OutputPerMTok float64 // USD
}

// Role is a conversation turn role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one turn in the conversation.
type Message struct {
	Role       Role
	Content    string
	ToolCallID string     // set when Role=tool, the id of the tool call being answered
	ToolCalls  []ToolCall // set when Role=assistant + a tool-use response
}

// ToolCall is one tool invocation from the model.
type ToolCall struct {
	ID      string
	Name    string
	ArgJSON string // raw JSON arguments
}

// ToolDef defines a callable tool for the model.
type ToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any // JSON Schema
}

// CompletionRequest is a single completion turn.
type CompletionRequest struct {
	Model       string
	Messages    []Message
	Tools       []ToolDef
	MaxTokens   int    // 0 = provider default
	System      string // system prompt (optional; may be in Messages[0] instead)
	Temperature float64
}

// Stream is the async response from a Complete call. Callers read events until io.EOF.
type Stream interface {
	// Next reads the next event. Returns io.EOF at end of stream.
	Next() (Event, error)
	// Close releases stream resources.
	io.Closer
}

// EventKind classifies a stream event.
type EventKind string

const (
	EventText     EventKind = "text"      // a text delta
	EventToolCall EventKind = "tool_call" // a tool call (complete, not streamed)
	EventDone     EventKind = "done"      // stream finished; Usage is set
)

// Event is one item from a Stream.
type Event struct {
	Kind     EventKind
	Text     string    // EventText: the delta
	ToolCall *ToolCall // EventToolCall: the complete call
	Usage    *Usage    // EventDone: token counts
}

// Usage is the token usage for a completed request.
type Usage struct {
	InputTokens  int
	OutputTokens int
	CacheHits    int // Anthropic prompt-cache: tokens served from cache
}

// ErrProviderUnavailable is returned when no provider is configured or the key is invalid.
var ErrProviderUnavailable = errors.New("llm: no provider configured")
