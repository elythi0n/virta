package app

import (
	"context"
	"encoding/json"
	"sync"

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
	tb       *intel.ToolBelt
	registry *llm.Registry
	meter    *llm.Meter
	settings store.SettingsRepo
	mu       sync.RWMutex
	cfg      api.IntelConfig
}

// newIntelControl constructs the intelligence controller and reloads any previously-persisted
// configuration (so provider keys and model selection survive daemon restarts).
func newIntelControl(tb *intel.ToolBelt, settings store.SettingsRepo) *intelControl {
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
	c := &intelControl{tb: tb, registry: reg, meter: meter, settings: settings, cfg: cfg}
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
	agentCh := c.tb.Ask(ctx, c.meter, model, question)
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
	for k, v := range cfg.ProviderKeys {
		if v == "" || len(v) > 4 && v[len(v)-4:] == "••••" {
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
