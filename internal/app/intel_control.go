package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/api"
	"github.com/elythi0n/virta/internal/intel"
	"github.com/elythi0n/virta/internal/llm"
	"github.com/elythi0n/virta/internal/store"
)

// intelControl wires the intelligence layer to the API's Intel surface. It owns:
//   - The LLM registry (connected providers and their model caches).
//   - The usage meter (kill switch, per-feature toggles, budget limits).
//   - The tool belt (already built at daemon startup, passed in).
//   - Config persistence (settings repo, scope "intel.config").
//
// Ask streaming, model listing, and config CRUD all go through this single struct.
type intelControl struct {
	tb            *intel.ToolBelt
	registry      *llm.Registry
	meter         *llm.Meter
	settings      store.SettingsRepo
	conversations store.ConversationRepo
	channels      store.ChannelRepo // for context: how many channels are joined
	loggingActive func() bool       // injected from the logbook sink
	mcpRelayURL   string            // public relay URL for external AI clients
	mu            sync.RWMutex
	cfg           api.IntelConfig
}

// newIntelControl constructs the intelligence controller and reloads any previously-persisted
// configuration (so provider keys and model selection survive daemon restarts).
func newIntelControl(tb *intel.ToolBelt, settings store.SettingsRepo, conversations store.ConversationRepo, channels store.ChannelRepo, loggingActive func() bool, mcpRelayURL string) *intelControl {
	reg := llm.NewRegistry()
	cfg := api.IntelConfig{
		Enabled:        false,
		FeatureEnabled: map[string]bool{},
	}
	// Load persisted config.
	if raw, err := settings.Get(context.Background(), "intel.config"); err == nil && len(raw.Data) > 0 {
		_ = json.Unmarshal(raw.Data, &cfg)
	}
	meter := llm.NewMeter(reg, apiCfgToMeterCfg(cfg))
	c := &intelControl{tb: tb, registry: reg, meter: meter, settings: settings, conversations: conversations, channels: channels, loggingActive: loggingActive, mcpRelayURL: mcpRelayURL, cfg: cfg}
	// Share logging state with the tool belt so it can return actionable errors when logging is off.
	tb.SetLogging(loggingActive)
	// Re-register any previously-saved providers.
	c.applyProviders(cfg)
	return c
}

// ListModels returns all provider models, refreshing caches where stale.
func (c *intelControl) ListModels(ctx context.Context) ([]api.ModelGroup, error) {
	groups, err := c.registry.AllModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.ModelGroup, 0, len(groups))
	for _, g := range groups {
		items := make([]api.ModelItem, 0, len(g.Models))
		for _, m := range g.Models {
			item := api.ModelItem{
				ID:            m.ID,
				DisplayName:   m.DisplayName,
				Family:        m.Family,
				ContextWindow: m.ContextWindow,
				SupportsTools: m.SupportsTools,
				Deprecated:    m.Deprecated,
			}
			if m.Pricing != nil {
				item.PriceIn = m.Pricing.InputPerMTok
				item.PriceOut = m.Pricing.OutputPerMTok
			}
			items = append(items, item)
		}
		out = append(out, api.ModelGroup{
			ProviderID:  g.ProviderID,
			DisplayName: g.DisplayName,
			Models:      items,
		})
	}
	return out, nil
}

// Ask runs the agent loop and returns a channel of streamed events.
func (c *intelControl) Ask(ctx context.Context, model, question string) (<-chan api.AskEvent, error) {
	if model == "" {
		model = c.registry.SelectedModel()
	}
	// Build per-request context so the AI knows the current daemon state.
	ac := intel.AskContext{
		LoggingEnabled: c.loggingActive != nil && c.loggingActive(),
		MCPRelayURL:    c.mcpRelayURL,
	}
	if c.channels != nil {
		if list, err := c.channels.List(ctx); err == nil {
			ac.ChannelCount = len(list)
		}
	}
	agentCh := c.tb.AskWithContext(ctx, c.meter, model, question, ac)
	out := make(chan api.AskEvent, 32)
	go func() {
		defer close(out)
		for ev := range agentCh {
			apiEv := api.AskEvent{Kind: string(ev.Kind)}
			switch ev.Kind {
			case intel.AEKText:
				apiEv.Text = ev.Text
			case intel.AEKToolUse:
				if ev.ToolUse != nil {
					apiEv.ToolName = ev.ToolUse.Name
					apiEv.ToolArgs = ev.ToolUse.Args
				}
			case intel.AEKToolResult:
				if ev.Result != nil {
					apiEv.ToolName = ev.Result.Name
					apiEv.ToolResult = ev.Result.JSON
				}
			case intel.AEKDone:
				if ev.Usage != nil {
					apiEv.InputTokens = ev.Usage.InputTokens
					apiEv.OutputTokens = ev.Usage.OutputTokens
				}
			case intel.AEKError:
				if ev.Err != nil {
					apiEv.ErrorMsg = ev.Err.Error()
				}
			}
			out <- apiEv
		}
	}()
	return out, nil
}

// Config returns a copy with API key values masked.
// URL-type values (e.g. the Ollama base URL) are returned as-is — they are not secrets.
func (c *intelControl) Config() api.IntelConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	copy := c.cfg
	masked := map[string]string{}
	for k, v := range copy.ProviderKeys {
		if api.IsURLValue(v) {
			masked[k] = v // URLs are not credentials — return plaintext
		} else if len(v) > 8 {
			masked[k] = v[:8] + "••••"
		} else {
			masked[k] = "••••"
		}
	}
	copy.ProviderKeys = masked
	return copy
}

// SetConfig applies new configuration and persists it.
func (c *intelControl) SetConfig(ctx context.Context, cfg api.IntelConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Merge provider keys: masked values in the incoming payload mean "keep existing".
	// "•" is U+2022 (3 UTF-8 bytes), so the mask suffix "••••" is 12 bytes — byte-slice
	// indexing with [len-4:] would extract 4 bytes, never matching 12-byte "••••".
	// strings.HasSuffix compares correctly regardless of multi-byte encoding.
	for k, v := range cfg.ProviderKeys {
		if v == "" || strings.HasSuffix(v, "••••") {
			// Keep existing key.
			if old, ok := c.cfg.ProviderKeys[k]; ok {
				cfg.ProviderKeys[k] = old
			} else {
				delete(cfg.ProviderKeys, k)
			}
		}
	}
	c.cfg = cfg
	c.applyProviders(cfg)
	c.registry.SetModel(cfg.SelectedModel)
	c.meter.SetConfig(apiCfgToMeterCfg(cfg))
	// Persist (with real keys, never the masked form).
	b, err := json.Marshal(cfg)
	if err == nil {
		_ = c.settings.Put(ctx, store.Setting{Scope: "intel.config", Data: b})
	}
	return nil
}

// applyProviders registers/deregisters LLM providers based on the stored config keys.
func (c *intelControl) applyProviders(cfg api.IntelConfig) {
	if k := cfg.ProviderKeys["anthropic"]; k != "" {
		c.registry.Register(llm.NewAnthropic(k))
	} else {
		c.registry.Deregister("anthropic")
	}
	if k := cfg.ProviderKeys["openai"]; k != "" {
		c.registry.Register(llm.NewOpenAI(k))
	} else {
		c.registry.Deregister("openai")
	}
	if k := cfg.ProviderKeys["xai"]; k != "" {
		c.registry.Register(llm.NewXAI(k))
	} else {
		c.registry.Deregister("xai")
	}
	// Ollama is always available if the user has it running (no key needed).
	if base := cfg.ProviderKeys["ollama"]; base != "" {
		c.registry.Register(llm.NewOllama(base))
	} else {
		// Register with the default localhost URL so it auto-discovers when running.
		c.registry.Register(llm.NewOllama(""))
	}
}

// apiCfgToMeterCfg converts the API config struct to the meter's config type.
func apiCfgToMeterCfg(cfg api.IntelConfig) llm.MeterConfig {
	feats := map[llm.Feature]bool{}
	for k, v := range cfg.FeatureEnabled {
		feats[llm.Feature(k)] = v
	}
	mc := llm.MeterConfig{Enabled: cfg.Enabled, FeatureEnabled: feats}
	if cfg.DailyLimitUSD > 0 {
		mc.Limits = append(mc.Limits, llm.BudgetLimit{Period: llm.PeriodDaily, USD: cfg.DailyLimitUSD})
	}
	if cfg.MonthlyLimitUSD > 0 {
		mc.Limits = append(mc.Limits, llm.BudgetLimit{Period: llm.PeriodMonthly, USD: cfg.MonthlyLimitUSD})
	}
	return mc
}

// GenerateTitle streams a short AI-generated title for the first user message of a conversation.
// It uses a simple completion (no tools) with a title-focused system prompt.
func (c *intelControl) GenerateTitle(ctx context.Context, model, firstMessage string) (<-chan api.AskEvent, error) {
	c.mu.RLock()
	cfg := c.cfg
	c.mu.RUnlock()
	if !cfg.Enabled {
		return nil, fmt.Errorf("AI features are disabled")
	}
	if model == "" {
		model = c.registry.SelectedModel()
	}

	ch := make(chan api.AskEvent, 32)
	go func() {
		defer close(ch)
		req := llm.CompletionRequest{
			Model:     model,
			MaxTokens: 20,
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "You are a conversation title generator. Given a user's first message, write a short title of 3-6 words. Output ONLY the title — no punctuation at the end, no quotes, no explanation."},
				{Role: llm.RoleUser, Content: firstMessage},
			},
		}
		stream, err := c.registry.Complete(ctx, req)
		if err != nil {
			ch <- api.AskEvent{Kind: "error", ErrorMsg: err.Error()}
			return
		}
		defer stream.Close()
		for {
			ev, err := stream.Next()
			if err != nil {
				ch <- api.AskEvent{Kind: "done"}
				return
			}
			switch ev.Kind {
			case llm.EventText:
				ch <- api.AskEvent{Kind: "text", Text: ev.Text}
			case llm.EventDone:
				ch <- api.AskEvent{Kind: "done"}
				return
			}
		}
	}()
	return ch, nil
}

func (c *intelControl) ListConversations(ctx context.Context) ([]api.ConversationSummary, error) {
	if c.conversations == nil {
		return []api.ConversationSummary{}, nil
	}
	list, err := c.conversations.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.ConversationSummary, 0, len(list))
	for _, conv := range list {
		out = append(out, api.ConversationSummary{
			ID:        conv.ID,
			Title:     conv.Title,
			Model:     conv.Model,
			UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func (c *intelControl) SaveConversation(ctx context.Context, id, title, model string, messages []byte) error {
	if c.conversations == nil {
		return nil
	}
	if title == "" {
		title = "New conversation"
	}
	msgs := json.RawMessage(messages)
	if len(msgs) == 0 {
		msgs = json.RawMessage("[]")
	}
	// Upsert: try update first, create on not-found.
	err := c.conversations.Update(ctx, id, title, model, msgs)
	if err != nil && err.Error() == "store: not found" {
		_, err = c.conversations.Create(ctx, id, title, model, msgs)
	}
	return err
}

func (c *intelControl) DeleteConversation(ctx context.Context, id string) error {
	if c.conversations == nil {
		return nil
	}
	return c.conversations.Delete(ctx, id)
}
