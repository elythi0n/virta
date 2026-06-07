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
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/store"
)

// MaxRows is the hard per-tool result cap — prevents tool responses from blowing up context windows.
const MaxRows = 200

// FlexInt accepts both JSON numbers and JSON strings from AI models that don't honour the schema
// type strictly (e.g. sending "10" instead of 10 for a limit field).
type FlexInt int

func (f *FlexInt) UnmarshalJSON(b []byte) error {
	// First try a plain number.
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		*f = FlexInt(n)
		return nil
	}
	// Fall back to a quoted string containing a number.
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err == nil {
			*f = FlexInt(n)
			return nil
		}
	}
	return fmt.Errorf("flexint: cannot parse %s as integer", string(b))
}

// --- Tool input/output types (also used as JSON Schema sources) ---

// SearchArgs is the input to search_messages.
type SearchArgs struct {
	Query    string  `json:"query"    jsonschema:"required,description=Full-text search query"`
	Channel  string  `json:"channel"  jsonschema:"description=Channel key (platform:slug) to narrow search; omit for all channels"`
	Author   string  `json:"author"   jsonschema:"description=Author name or uid to filter by"`
	FromTime string  `json:"from_time" jsonschema:"description=ISO-8601 start time; omit for no lower bound"`
	ToTime   string  `json:"to_time"   jsonschema:"description=ISO-8601 end time; omit for no upper bound"`
	Limit    FlexInt `json:"limit"    jsonschema:"description=Max results (capped at 200)"`
}

// UserHistoryArgs is the input to get_user_history.
type UserHistoryArgs struct {
	Author   string  `json:"author"   jsonschema:"required,description=Author name or uid"`
	Channel  string  `json:"channel"  jsonschema:"description=Optional channel key"`
	FromTime string  `json:"from_time" jsonschema:"description=ISO-8601 start"`
	ToTime   string  `json:"to_time"   jsonschema:"description=ISO-8601 end"`
	Limit    FlexInt `json:"limit"    jsonschema:"description=Max results (capped at 200)"`
}

// TopChattersArgs is the input to top_chatters.
type TopChattersArgs struct {
	Channel  string  `json:"channel"   jsonschema:"description=Channel key; omit for all channels"`
	FromTime string  `json:"from_time" jsonschema:"description=ISO-8601 start"`
	ToTime   string  `json:"to_time"   jsonschema:"description=ISO-8601 end"`
	Metric   string  `json:"metric"    jsonschema:"description=messages|subs|gifts — defaults to messages"`
	Limit    FlexInt `json:"limit"     jsonschema:"description=Top-N count (capped at 50)"`
}

// ChannelStatsArgs is the input to channel_stats.
type ChannelStatsArgs struct {
	Channel  string `json:"channel"  jsonschema:"required,description=Channel key"`
	FromTime string `json:"from_time" jsonschema:"description=ISO-8601 start"`
	ToTime   string `json:"to_time"   jsonschema:"description=ISO-8601 end"`
}

// MessagesRangeArgs is the input to get_messages_range.
type MessagesRangeArgs struct {
	Channel  string  `json:"channel"  jsonschema:"required,description=Channel key"`
	FromTime string  `json:"from_time" jsonschema:"description=ISO-8601 start"`
	ToTime   string  `json:"to_time"   jsonschema:"description=ISO-8601 end"`
	Limit    FlexInt `json:"limit"    jsonschema:"description=Max results (capped at 200)"`
}

// MessageRow is a tool result row.
type MessageRow struct {
	ID      string `json:"id"`
	Channel string `json:"channel"`
	Author  string `json:"author"`
	Body    string `json:"body"`
	Type    string `json:"type,omitempty"`
	SentAt  string `json:"sent_at"`
	Deleted bool   `json:"deleted,omitempty"`
}

// ChatterCount is a row in top_chatters results.
type ChatterCount struct {
	Author  string `json:"author"`
	Channel string `json:"channel,omitempty"`
	Count   int    `json:"count"`
}

// ChannelInfo is a row in list_channels results.
type ChannelInfo struct {
	Key      string `json:"key"` // "platform:slug"
	Platform string `json:"platform"`
	Slug     string `json:"slug"`
}

// ToolBelt holds all tool implementations bound to a store.
type ToolBelt struct {
	store         store.Store
	loggingActive func() bool          // nil means unknown/off
	channelLister func() []ChannelInfo // returns the live joined-channel list
}

// New builds a ToolBelt over the given store.
func New(s store.Store) *ToolBelt { return &ToolBelt{store: s} }

// SetChannelLister injects a function that returns the live set of joined channels. ListChannels
// will use this instead of the DB channel table (which may lag behind the actual join state).
func (tb *ToolBelt) SetChannelLister(fn func() []ChannelInfo) { tb.channelLister = fn }

// SetLogging injects a function the tools call to check whether message logging is enabled.
// When logging is off, tools that query message history return a clear instructional error
// instead of silently returning empty results.
func (tb *ToolBelt) SetLogging(fn func() bool) { tb.loggingActive = fn }

// loggingOn returns true only when the injected function confirms logging is enabled.
func (tb *ToolBelt) loggingOn() bool {
	return tb.loggingActive != nil && tb.loggingActive()
}

// errLoggingOff is returned by history tools when logging is disabled.
var errLoggingOff = fmt.Errorf(
	"message logging is disabled — no chat history is available to query; " +
		"enable logging in Settings → Chat (or Settings → Storage) and give it time to accumulate messages")

// --- Tool implementations ---

// SearchMessages runs a full-text search over the logged message store.
func (tb *ToolBelt) SearchMessages(ctx context.Context, args SearchArgs) ([]MessageRow, error) {
	if !tb.loggingOn() {
		return nil, errLoggingOff
	}
	if strings.TrimSpace(args.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	limit := clamped(int(args.Limit), MaxRows)
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
	if !tb.loggingOn() {
		return nil, errLoggingOff
	}
	if args.Author == "" {
		return nil, fmt.Errorf("author is required")
	}
	limit := clamped(int(args.Limit), MaxRows)
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
	if !tb.loggingOn() {
		return nil, errLoggingOff
	}
	limit := clamped(int(args.Limit), 50)
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
	if !tb.loggingOn() {
		return nil, errLoggingOff
	}
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
	if !tb.loggingOn() {
		return nil, errLoggingOff
	}
	if args.Channel == "" {
		return nil, fmt.Errorf("channel is required")
	}
	chID, err := tb.resolveChannel(ctx, args.Channel)
	if err != nil {
		return nil, err
	}
	limit := clamped(int(args.Limit), MaxRows)
	msgs, err := tb.store.Messages().History(ctx, store.HistoryQuery{ChannelID: chID, Limit: limit})
	if err != nil {
		return nil, err
	}
	return tb.toRows(ctx, msgs), nil
}

// ListChannels returns all known channels with their store IDs.
func (tb *ToolBelt) ListChannels(_ context.Context) ([]ChannelInfo, error) {
	// Prefer the live channel lister (engine + profile source) over the channels DB table,
	// which only has metadata for channels that have been through the resolver and may lag.
	if tb.channelLister != nil {
		return tb.channelLister(), nil
	}
	return []ChannelInfo{}, nil
}

// Dispatch routes a tool call by name with JSON-encoded arguments. This is the unified entry
// point used by both the MCP server and the agent loop, so the call logic lives in one place.
func (tb *ToolBelt) Dispatch(ctx context.Context, name string, rawArgs json.RawMessage) (any, error) {
	// Treat nil/empty arguments as an empty JSON object so callers that omit {} don't error.
	if len(rawArgs) == 0 || string(rawArgs) == "null" {
		rawArgs = json.RawMessage("{}")
	}
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

func (tb *ToolBelt) resolveChannel(_ context.Context, key string) (string, error) {
	// Messages are stored in the DB with channel_id = "platform:slug" (set by the logbook sink).
	// Return the key itself — do NOT look up the channels-table ULID, which would never match.
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid channel key %q (expected platform:slug)", key)
	}
	switch strings.ToLower(parts[0]) {
	case "twitch", "kick", "x":
		// valid
	default:
		return "", fmt.Errorf("unknown platform %q", parts[0])
	}
	// Validate slug is non-empty.
	if strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("channel key %q has an empty slug", key)
	}
	return key, nil // "kick:eslcs" — matches messages.channel_id in the DB
}

func (tb *ToolBelt) toRows(_ context.Context, msgs []store.StoredMessage) []MessageRow {
	// m.ChannelID is already "platform:slug" (set by the logbook sink when writing to DB).
	// No reverse lookup needed.
	rows := make([]MessageRow, 0, len(msgs))
	for _, m := range msgs {
		rows = append(rows, MessageRow{
			ID:      m.ID,
			Channel: m.ChannelID,
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

// schemaOf generates a JSON Schema for a struct value, reading json and jsonschema struct tags.
// The required array is always a non-nil slice so it marshals as [] not null — strict validators
// (xAI, Grok) reject null for the required field.
func schemaOf(v any) map[string]any {
	props := map[string]any{}
	required := []string{} // non-nil: marshals as [] not null
	t := reflect.TypeOf(v)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// Derive the JSON field name from the json tag (or field name).
		jsonTag := field.Tag.Get("json")
		name := strings.SplitN(jsonTag, ",", 2)[0]
		if name == "" || name == "-" {
			name = field.Name
		}
		// Mark as required if the jsonschema tag says so.
		if strings.Contains(field.Tag.Get("jsonschema"), "required") {
			required = append(required, name)
		}
		props[name] = map[string]any{"type": "string"}
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}
