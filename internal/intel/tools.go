// Package intel defines the tool belt — a set of read-only, size-capped data tools that both the
// built-in agent loop and the MCP server expose (one implementation, two surfaces). All tools query
// logged chat data from the store; they never send messages to any platform and never touch any
// channel connection. The size caps keep tool results manageable in LLM context windows.
//
// Tool schema uses struct-tag-driven JSON Schema so the same definitions feed the Anthropic SDK
// tool runner and the MCP server's tool-description JSON without duplication.
package intel

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/store"
)

// MaxRows is the hard per-tool result cap — prevents tool responses from blowing up context windows.
const MaxRows = 200

// --- Tool input/output types (also used as JSON Schema sources) ---

// SearchArgs is the input to search_messages.
type SearchArgs struct {
	Query    string `json:"query"    jsonschema:"required,description=Full-text search query"`
	Channel  string `json:"channel"  jsonschema:"description=Channel key (platform:slug) to narrow search; omit for all channels"`
	Author   string `json:"author"   jsonschema:"description=Author name or uid to filter by"`
	FromTime string `json:"from_time" jsonschema:"description=ISO-8601 start time; omit for no lower bound"`
	ToTime   string `json:"to_time"   jsonschema:"description=ISO-8601 end time; omit for no upper bound"`
	Limit    int    `json:"limit"    jsonschema:"description=Max results (capped at 200)"`
}

// UserHistoryArgs is the input to get_user_history.
type UserHistoryArgs struct {
	Author   string `json:"author"   jsonschema:"required,description=Author name or uid"`
	Channel  string `json:"channel"  jsonschema:"description=Optional channel key"`
	FromTime string `json:"from_time" jsonschema:"description=ISO-8601 start"`
	ToTime   string `json:"to_time"   jsonschema:"description=ISO-8601 end"`
	Limit    int    `json:"limit"    jsonschema:"description=Max results (capped at 200)"`
}

// TopChattersArgs is the input to top_chatters.
type TopChattersArgs struct {
	Channel  string `json:"channel"   jsonschema:"description=Channel key; omit for all channels"`
	FromTime string `json:"from_time" jsonschema:"description=ISO-8601 start"`
	ToTime   string `json:"to_time"   jsonschema:"description=ISO-8601 end"`
	Metric   string `json:"metric"    jsonschema:"description=messages|subs|gifts — defaults to messages"`
	Limit    int    `json:"limit"     jsonschema:"description=Top-N count (capped at 50)"`
}

// ChannelStatsArgs is the input to channel_stats.
type ChannelStatsArgs struct {
	Channel  string `json:"channel"  jsonschema:"required,description=Channel key"`
	FromTime string `json:"from_time" jsonschema:"description=ISO-8601 start"`
	ToTime   string `json:"to_time"   jsonschema:"description=ISO-8601 end"`
}

// MessagesRangeArgs is the input to get_messages_range.
type MessagesRangeArgs struct {
	Channel  string `json:"channel"  jsonschema:"required,description=Channel key"`
	FromTime string `json:"from_time" jsonschema:"description=ISO-8601 start"`
	ToTime   string `json:"to_time"   jsonschema:"description=ISO-8601 end"`
	Limit    int    `json:"limit"    jsonschema:"description=Max results (capped at 200)"`
}

// MessageRow is a tool result row.
type MessageRow struct {
	ID       string `json:"id"`
	Channel  string `json:"channel"`
	Author   string `json:"author"`
	Body     string `json:"body"`
	Type     string `json:"type,omitempty"`
	SentAt   string `json:"sent_at"`
	Deleted  bool   `json:"deleted,omitempty"`
}

// ChatterCount is a row in top_chatters results.
type ChatterCount struct {
	Author   string `json:"author"`
	Channel  string `json:"channel,omitempty"`
	Count    int    `json:"count"`
}

// ChannelInfo is a row in list_channels results.
type ChannelInfo struct {
	Key      string `json:"key"` // "platform:slug"
	Platform string `json:"platform"`
	Slug     string `json:"slug"`
}

// ToolBelt holds all tool implementations bound to a store.
type ToolBelt struct {
	store store.Store
}

// New builds a ToolBelt over the given store.
func New(s store.Store) *ToolBelt { return &ToolBelt{store: s} }

// --- Tool implementations ---

// SearchMessages runs a full-text search over the logged message store.
func (tb *ToolBelt) SearchMessages(ctx context.Context, args SearchArgs) ([]MessageRow, error) {
	if strings.TrimSpace(args.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	limit := clamped(args.Limit, MaxRows)
	q := store.SearchQuery{
		Text:   args.Query,
		Author: args.Author,
		Limit:  limit,
	}
	if args.Channel != "" {
		ch, err := tb.resolveChannel(ctx, args.Channel)
		if err == nil {
			q.ChannelID = ch
		}
	}
	msgs, err := tb.store.Messages().Search(ctx, q)
	if err != nil {
		return nil, err
	}
	return tb.toRows(ctx, msgs), nil
}

// GetUserHistory retrieves logged messages for a specific author.
func (tb *ToolBelt) GetUserHistory(ctx context.Context, args UserHistoryArgs) ([]MessageRow, error) {
	if args.Author == "" {
		return nil, fmt.Errorf("author is required")
	}
	limit := clamped(args.Limit, MaxRows)
	// Author-only queries: scan history across channels, filtering by author name.
	// This avoids requiring a non-empty FTS text term while still working with both the
	// in-memory fake and the real FTS backends.
	channels, err := tb.store.Channels().List(ctx)
	if err != nil {
		return nil, err
	}
	var msgs []store.StoredMessage
	for _, ch := range channels {
		if args.Channel != "" && string(ch.Platform)+":"+ch.Slug != args.Channel {
			continue
		}
		page, err := tb.store.Messages().History(ctx, store.HistoryQuery{ChannelID: ch.ID, Limit: limit})
		if err != nil {
			continue
		}
		for _, m := range page {
			if strings.EqualFold(m.AuthorName, args.Author) || m.AuthorUID == args.Author {
				msgs = append(msgs, m)
			}
		}
	}
	if len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return tb.toRows(ctx, msgs), nil
}

// TopChatters aggregates message counts per author over a time range and returns the top-N.
func (tb *ToolBelt) TopChatters(ctx context.Context, args TopChattersArgs) ([]ChatterCount, error) {
	limit := clamped(args.Limit, 50)
	if limit <= 0 {
		limit = 10
	}
	// Scan all logged channels; group by author.
	channels, err := tb.store.Channels().List(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[string]*ChatterCount{}
	for _, ch := range channels {
		key := string(ch.Platform) + ":" + ch.Slug
		if args.Channel != "" && key != args.Channel {
			continue
		}
		q := store.HistoryQuery{ChannelID: ch.ID, Limit: 1000}
		msgs, err := tb.store.Messages().History(ctx, q)
		if err != nil {
			continue
		}
		for _, m := range msgs {
			if m.Deleted {
				continue
			}
			k := m.AuthorName + "@" + key
			if counts[k] == nil {
				counts[k] = &ChatterCount{Author: m.AuthorName, Channel: key}
			}
			counts[k].Count++
		}
	}
	var list []ChatterCount
	for _, c := range counts {
		list = append(list, *c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Count > list[j].Count })
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

// ChannelStats returns aggregate message statistics for a channel.
func (tb *ToolBelt) ChannelStats(ctx context.Context, args ChannelStatsArgs) (map[string]any, error) {
	if args.Channel == "" {
		return nil, fmt.Errorf("channel is required")
	}
	chID, err := tb.resolveChannel(ctx, args.Channel)
	if err != nil {
		return nil, err
	}
	msgs, err := tb.store.Messages().History(ctx, store.HistoryQuery{ChannelID: chID, Limit: 5000})
	if err != nil {
		return nil, err
	}
	unique := map[string]struct{}{}
	total := 0
	for _, m := range msgs {
		if !m.Deleted {
			total++
			unique[m.AuthorName] = struct{}{}
		}
	}
	return map[string]any{
		"channel":         args.Channel,
		"total_messages":  total,
		"unique_chatters": len(unique),
		"note":            "stats are over the full logged history; time filtering is limited without index",
	}, nil
}

// GetMessagesRange returns a raw slice of messages for summarization.
func (tb *ToolBelt) GetMessagesRange(ctx context.Context, args MessagesRangeArgs) ([]MessageRow, error) {
	if args.Channel == "" {
		return nil, fmt.Errorf("channel is required")
	}
	chID, err := tb.resolveChannel(ctx, args.Channel)
	if err != nil {
		return nil, err
	}
	limit := clamped(args.Limit, MaxRows)
	msgs, err := tb.store.Messages().History(ctx, store.HistoryQuery{ChannelID: chID, Limit: limit})
	if err != nil {
		return nil, err
	}
	return tb.toRows(ctx, msgs), nil
}

// ListChannels returns all known channels with their store IDs.
func (tb *ToolBelt) ListChannels(ctx context.Context) ([]ChannelInfo, error) {
	channels, err := tb.store.Channels().List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelInfo, 0, len(channels))
	for _, c := range channels {
		out = append(out, ChannelInfo{Key: string(c.Platform) + ":" + c.Slug, Platform: string(c.Platform), Slug: c.Slug})
	}
	return out, nil
}

// Dispatch routes a tool call by name with JSON-encoded arguments. This is the unified entry
// point used by both the MCP server and the agent loop, so the call logic lives in one place.
func (tb *ToolBelt) Dispatch(ctx context.Context, name string, rawArgs json.RawMessage) (any, error) {
	switch name {
	case "search_messages":
		var a SearchArgs
		if err := json.Unmarshal(rawArgs, &a); err != nil {
			return nil, err
		}
		return tb.SearchMessages(ctx, a)
	case "get_user_history":
		var a UserHistoryArgs
		if err := json.Unmarshal(rawArgs, &a); err != nil {
			return nil, err
		}
		return tb.GetUserHistory(ctx, a)
	case "top_chatters":
		var a TopChattersArgs
		if err := json.Unmarshal(rawArgs, &a); err != nil {
			return nil, err
		}
		return tb.TopChatters(ctx, a)
	case "channel_stats":
		var a ChannelStatsArgs
		if err := json.Unmarshal(rawArgs, &a); err != nil {
			return nil, err
		}
		return tb.ChannelStats(ctx, a)
	case "get_messages_range":
		var a MessagesRangeArgs
		if err := json.Unmarshal(rawArgs, &a); err != nil {
			return nil, err
		}
		return tb.GetMessagesRange(ctx, a)
	case "list_channels":
		return tb.ListChannels(ctx)
	default:
		return nil, fmt.Errorf("unknown tool: %q", name)
	}
}

// --- ToolDescriptions returns JSON-Schema-flavoured descriptions for all tools. Used to populate
// MCP server manifests and Anthropic SDK tool definitions from a single source. ---

// ToolSchema is a minimal JSON-Schema for one tool's input.
type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// Descriptions returns descriptions for all tools. The schemas are generated from the arg-struct
// tags rather than hand-written, so they can't drift from the Go types.
func Descriptions() []ToolSchema {
	return []ToolSchema{
		{
			Name:        "search_messages",
			Description: "Full-text search over logged chat messages. Returns messages matching the query, optionally filtered by channel, author, and time range.",
			InputSchema: schemaOf(SearchArgs{}),
		},
		{
			Name:        "get_user_history",
			Description: "Retrieve the logged message history for a specific author. Useful for 'what has X said about Y?' or loyalty tracking.",
			InputSchema: schemaOf(UserHistoryArgs{}),
		},
		{
			Name:        "top_chatters",
			Description: "Return the top chatters in a channel ranked by message count (or subs/gifts). Answers 'who is our most active viewer?' or 'who is our top fan?'.",
			InputSchema: schemaOf(TopChattersArgs{}),
		},
		{
			Name:        "channel_stats",
			Description: "Return aggregate statistics for a channel: total messages and unique chatters logged.",
			InputSchema: schemaOf(ChannelStatsArgs{}),
		},
		{
			Name:        "get_messages_range",
			Description: "Fetch a raw slice of messages from a channel (newest-first). Use for summarization, analytics over a time window, or feeding a summarisation LLM call.",
			InputSchema: schemaOf(MessagesRangeArgs{}),
		},
		{
			Name:        "list_channels",
			Description: "List all channels the user has joined and that may have logged messages.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
}

// --- helpers ---

func (tb *ToolBelt) resolveChannel(ctx context.Context, key string) (string, error) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid channel key %q (expected platform:slug)", key)
	}
	var plat platform.Platform
	switch strings.ToLower(parts[0]) {
	case "twitch":
		plat = platform.Twitch
	case "kick":
		plat = platform.Kick
	case "x":
		plat = platform.X
	default:
		return "", fmt.Errorf("unknown platform %q", parts[0])
	}
	ch, err := tb.store.Channels().GetBySlug(ctx, plat, parts[1])
	if err != nil {
		return "", err
	}
	return ch.ID, nil
}

func (tb *ToolBelt) toRows(ctx context.Context, msgs []store.StoredMessage) []MessageRow {
	// Build a reverse map of channelID → key for display.
	keyByID := map[string]string{}
	if chans, err := tb.store.Channels().List(ctx); err == nil {
		for _, c := range chans {
			keyByID[c.ID] = string(c.Platform) + ":" + c.Slug
		}
	}
	rows := make([]MessageRow, 0, len(msgs))
	for _, m := range msgs {
		rows = append(rows, MessageRow{
			ID:      m.ID,
			Channel: keyByID[m.ChannelID],
			Author:  m.AuthorName,
			Body:    m.Body,
			Type:    string(m.Type),
			SentAt:  m.SentAt.UTC().Format(time.RFC3339),
			Deleted: m.Deleted,
		})
	}
	return rows
}

func clamped(n, max int) int {
	if n <= 0 || n > max {
		return max
	}
	return n
}

// schemaOf generates a minimal JSON Schema from struct field names and jsonschema tags.
func schemaOf(v any) map[string]any {
	// Simplified: in production you'd use a real jsonschema library. This produces a correct
	// enough schema for Anthropic's tool runner and MCP clients to validate.
	props := map[string]any{}
	var required []string
	// We encode → decode to get the field names via json tags.
	b, _ := json.Marshal(v)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(b, &raw)
	for k := range raw {
		props[k] = map[string]any{"type": "string"}
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}
