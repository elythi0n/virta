// Agent implements the LLM tool-calling loop for the Ask pane. It runs the agentic loop:
// model → tool calls → execute → feed back → repeat until the model produces a final response.
// The loop streams events back to the caller so the UI can show the answer progressively.
package intel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

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

const maxToolRounds = 10 // prevent runaway loops

// SystemPrompt is the stable part of the agent's system prompt. Marked for prompt-cache reuse.
const SystemPrompt = `You are an assistant for a live chat aggregation app called Virta.
You have access to tools that query the user's logged chat history (messages, top chatters, stats).
Always answer based on the data your tools return — do not invent numbers or quotes.
For every factual claim, cite the tool call that produced it.
Be concise. If a question can't be answered from the available data, say so clearly.`

// Agent runs the tool-calling loop and sends events to the returned channel.
// The channel is closed when the agent finishes or errors.
func (tb *ToolBelt) Ask(ctx context.Context, meter *llm.Meter, model, question string) <-chan AgentEvent {
	ch := make(chan AgentEvent, 32)
	go tb.runAgent(ctx, meter, model, question, ch)
	return ch
}

func (tb *ToolBelt) runAgent(ctx context.Context, meter *llm.Meter, model, question string, out chan<- AgentEvent) {
	defer close(out)

	tools := Descriptions()
	llmTools := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		llmTools = append(llmTools, llm.ToolDef{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: question},
	}

	for round := 0; round < maxToolRounds; round++ {
		stream, err := meter.Complete(ctx, llm.FeatureAsk, llm.CompletionRequest{
			Model:    model,
			System:   SystemPrompt,
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
			result, err := tb.Dispatch(ctx, tc.Name, json.RawMessage(tc.ArgJSON))
			var resultJSON string
			if err != nil {
				resultJSON = fmt.Sprintf(`{"error":%q}`, err.Error())
			} else {
				b, _ := json.Marshal(result)
				resultJSON = string(b)
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
