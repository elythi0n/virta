// Agent implements the LLM tool-calling loop for the Ask pane. It runs the agentic loop:
// model → tool calls → execute → feed back → repeat until the model produces a final response.
// The loop streams events back to the caller so the UI can show the answer progressively.
package intel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/llm"
)

// AgentEvent is one streamed item from a Run call.
type AgentEvent struct {
	Kind    AgentEventKind
	Text    string       // AEKText: one text delta
	ToolUse *ToolUse     // AEKToolUse: the tool call about to be executed
	Result  *ToolResult  // AEKToolResult: the tool's response
	Usage   *llm.Usage   // AEKDone: final usage (may be nil if the provider doesn't report it)
	Err     error        // AEKError: terminal error
}

// AgentEventKind classifies an AgentEvent.
type AgentEventKind string

const (
	AEKText       AgentEventKind = "text"
	AEKToolUse    AgentEventKind = "tool_use"
	AEKToolResult AgentEventKind = "tool_result"
	AEKDone       AgentEventKind = "done"
	AEKError      AgentEventKind = "error"
)

// ToolUse is a pending tool call.
type ToolUse struct {
	ID   string
	Name string
	Args string // raw JSON
}

// ToolResult is a tool's response.
type ToolResult struct {
	ToolUseID string
	Name      string
	JSON      string // the serialised result
}

const maxToolRounds = 4 // 4 back-and-forth cycles is enough for any reasonable query

// AskContext carries per-request context injected into the system prompt so the AI knows
// the current state of the daemon and can give actionable guidance when tools fail.
type AskContext struct {
	LoggingEnabled bool
	ChannelCount   int
	MCPRelayURL    string    // public URL for the MCP server (empty = not configured)
	Now            time.Time // current wall-clock time so the AI knows "today"
}

// buildSystemPrompt assembles a rich system prompt from the stable base and the per-request
// context. The context section is never empty — even "logging off, 0 channels" is useful
// because it lets the AI explain what the user needs to do.
func buildSystemPrompt(ac AskContext) string {
	var b strings.Builder
	b.WriteString(`You are an assistant for a live chat aggregation app called Virta.
You have access to tools that query the user's logged chat history (messages, top chatters, stats).
Always answer based on the data your tools return — do not invent numbers or quotes.
For every factual claim, cite the tool call that produced it.
Be concise.

IMPORTANT — tool failure guidance:
When a tool returns {"error": "..."}, read the error message and explain it to the user in plain
language. Always suggest the specific fix (e.g. "enable logging in Settings → Chat").
Never silently ignore a tool error or pretend it succeeded.`)

	// Always inject the current date so the AI can correctly interpret "today", "this week", etc.
	now := ac.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	b.WriteString(fmt.Sprintf("\n\nCurrent date/time: %s (UTC)\n", now.Format("2006-01-02 15:04 MST")))
	b.WriteString("When the user says 'today', use " + now.Format("2006-01-02") + " as the date.\n")
	b.WriteString("When the user says 'this week', use " + now.AddDate(0, 0, -int(now.Weekday())).Format("2006-01-02") + " as the week start.\n")
	b.WriteString("Always call list_channels first to learn the correct platform:slug channel keys before querying messages.\n\n")

	b.WriteString("Current daemon state:\n")

	if ac.LoggingEnabled {
		b.WriteString("- Message logging: ENABLED — chat history is available for queries.\n")
	} else {
		b.WriteString("- Message logging: DISABLED — history tools will return an error.\n")
		b.WriteString("  To fix: go to Settings → Chat, turn on \"Log messages\". History accumulates from that point.\n")
	}

	if ac.ChannelCount == 0 {
		b.WriteString("- Channels: none joined yet — add streams in the Streams panel first.\n")
	} else {
		b.WriteString(fmt.Sprintf("- Channels: %d joined.\n", ac.ChannelCount))
	}

	if ac.MCPRelayURL != "" {
		b.WriteString(fmt.Sprintf(
			"\nExternal AI client integration (MCP):\n"+
				"- MCP server endpoint: %s/mcp\n"+
				"- External AI clients (Claude Desktop, Cursor, etc.) can connect to this URL.\n"+
				"- They need the API bearer token, which the user can find in Settings → Integrations → API tokens.\n",
			ac.MCPRelayURL))
	} else {
		b.WriteString("\nExternal AI client integration (MCP):\n" +
			"- No public relay URL is configured (VIRTA_MCP_RELAY_URL).\n" +
			"- Cloud AI providers cannot reach the local MCP server directly.\n" +
			"- For hosted deployments, the operator sets VIRTA_MCP_RELAY_URL to the public base URL.\n" +
			"- For local use, tools run inside Virta — cloud AI (Grok, Claude) does NOT need to reach your machine.\n")
	}

	return b.String()
}

// Agent runs the tool-calling loop and sends events to the returned channel.
// The channel is closed when the agent finishes or errors.
func (tb *ToolBelt) Ask(ctx context.Context, meter *llm.Meter, model, question string) <-chan AgentEvent {
	return tb.AskWithContext(ctx, meter, model, question, AskContext{})
}

// AskWithContext is Ask with an explicit AskContext that shapes the system prompt.
func (tb *ToolBelt) AskWithContext(ctx context.Context, meter *llm.Meter, model, question string, ac AskContext) <-chan AgentEvent {
	ch := make(chan AgentEvent, 32)
	go tb.runAgent(ctx, meter, model, question, buildSystemPrompt(ac), ch)
	return ch
}

func (tb *ToolBelt) runAgent(ctx context.Context, meter *llm.Meter, model, question, systemPrompt string, out chan<- AgentEvent) {
	defer close(out)

	tools := Descriptions()
	llmTools := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		llmTools = append(llmTools, llm.ToolDef{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: question},
	}

	// toolCache deduplicates identical (name, args) calls within one agent run.
	// If the model calls the same tool with the same arguments twice, return the cached
	// result instead of hitting the DB again (prevents runaway loops).
	type cacheKey struct{ name, args string }
	toolCache := map[cacheKey]string{}

	for round := 0; round < maxToolRounds; round++ {
		stream, err := meter.Complete(ctx, llm.FeatureAsk, llm.CompletionRequest{
			Model:    model,
			System:   systemPrompt,
			Messages: messages,
			Tools:    llmTools,
		})
		if err != nil {
			out <- AgentEvent{Kind: AEKError, Err: err}
			return
		}

		var textBuf string
		var toolCalls []llm.ToolCall
		for {
			ev, err := stream.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				out <- AgentEvent{Kind: AEKError, Err: err}
				_ = stream.Close()
				return
			}
			switch ev.Kind {
			case llm.EventText:
				textBuf += ev.Text
				out <- AgentEvent{Kind: AEKText, Text: ev.Text}
			case llm.EventToolCall:
				if ev.ToolCall != nil {
					toolCalls = append(toolCalls, *ev.ToolCall)
				}
			case llm.EventDone:
				out <- AgentEvent{Kind: AEKDone, Usage: ev.Usage}
			}
		}
		_ = stream.Close()

		if len(toolCalls) == 0 {
			// Model gave a final answer — done.
			return
		}

		// Append the model's message with its tool calls.
		assistantMsg := llm.Message{Role: llm.RoleAssistant}
		if textBuf != "" {
			assistantMsg.Content = textBuf
		}
		assistantMsg.ToolCalls = toolCalls
		messages = append(messages, assistantMsg)

		// Execute each tool call and append the results.
		for _, tc := range toolCalls {
			out <- AgentEvent{Kind: AEKToolUse, ToolUse: &ToolUse{ID: tc.ID, Name: tc.Name, Args: tc.ArgJSON}}
			key := cacheKey{tc.Name, tc.ArgJSON}
			var resultJSON string
			if cached, hit := toolCache[key]; hit {
				// Return cached result — model already has this data.
				resultJSON = fmt.Sprintf(`{"cached":true,"note":"already called with these arguments","result":%s}`, cached)
				out <- AgentEvent{Kind: AEKToolResult, Result: &ToolResult{ToolUseID: tc.ID, Name: tc.Name, JSON: resultJSON}}
				messages = append(messages, llm.Message{Role: llm.RoleTool, Content: resultJSON, ToolCallID: tc.ID})
				continue
			}
			result, err := tb.Dispatch(ctx, tc.Name, json.RawMessage(tc.ArgJSON))
			if err != nil {
				resultJSON = fmt.Sprintf(`{"error":%q}`, err.Error())
			} else {
				b, _ := json.Marshal(result)
				resultJSON = string(b)
				toolCache[key] = resultJSON // cache successful results only
			}
			out <- AgentEvent{Kind: AEKToolResult, Result: &ToolResult{ToolUseID: tc.ID, Name: tc.Name, JSON: resultJSON}}
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    resultJSON,
				ToolCallID: tc.ID,
			})
		}
	}
	out <- AgentEvent{Kind: AEKError, Err: fmt.Errorf("agent: reached max tool rounds (%d)", maxToolRounds)}
}
