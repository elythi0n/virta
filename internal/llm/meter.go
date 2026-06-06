package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Feature identifies which product feature made an LLM call — used for per-feature usage tracking
// and the per-feature toggles in the Usage panel.
type Feature string

const (
	FeatureAsk         Feature = "ask"
	FeatureTranslation Feature = "translation"
	FeatureEmbedding   Feature = "embedding"
)

// BudgetPeriod is daily or monthly.
type BudgetPeriod string

const (
	PeriodDaily   BudgetPeriod = "daily"
	PeriodMonthly BudgetPeriod = "monthly"
)

// BudgetLimit is one spending limit.
type BudgetLimit struct {
	Period BudgetPeriod
	USD    float64 // 0 = no limit
}

// UsageRecord is one LLM call's recorded cost, stored in the in-memory aggregate.
type UsageRecord struct {
	At           time.Time
	Provider     string
	Model        string
	Feature      Feature
	InputTokens  int
	OutputTokens int
	CacheHits    int
	CostUSD      float64
}

// MeterConfig holds limits and feature toggles. Persisted to the settings repo.
type MeterConfig struct {
	Enabled         bool                       // master kill switch — if false, all LLM calls return ErrLLMDisabled
	FeatureEnabled  map[Feature]bool            // per-feature toggles
	Limits          []BudgetLimit
	PricingOverride map[string]Pricing          // user-entered prices for providers that don't expose them
}

// ErrLLMDisabled is returned when the master kill switch is off.
var ErrLLMDisabled = errors.New("llm: all AI features are disabled")

// ErrBudgetExceeded is returned when a spending limit is about to be crossed.
type ErrBudgetExceeded struct {
	Period  BudgetPeriod
	LimitUSD float64
	SpentUSD float64
}

func (e *ErrBudgetExceeded) Error() string {
	return fmt.Sprintf("llm: %s budget of $%.2f would be exceeded (spent $%.2f)", e.Period, e.LimitUSD, e.SpentUSD)
}

// Meter wraps a Registry and tracks every call's token usage. It is the mandatory wrapper —
// nothing in the codebase calls a provider directly; everything goes through the Meter.
// This structural guarantee means "all toggles off → zero calls" can be proven by test.
type Meter struct {
	registry *Registry
	mu       sync.Mutex
	records  []UsageRecord
	cfg      MeterConfig
}

// NewMeter wraps the registry. The caller must supply a non-nil registry.
func NewMeter(reg *Registry, cfg MeterConfig) *Meter {
	if cfg.FeatureEnabled == nil {
		cfg.FeatureEnabled = map[Feature]bool{}
	}
	return &Meter{registry: reg, cfg: cfg}
}

// SetConfig atomically replaces the configuration (for live settings changes).
func (m *Meter) SetConfig(cfg MeterConfig) {
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
}

// Config returns a copy of the current configuration.
func (m *Meter) Config() MeterConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// Complete wraps Registry.Complete with meter checks and usage recording.
func (m *Meter) Complete(ctx context.Context, feature Feature, req CompletionRequest) (Stream, error) {
	m.mu.Lock()
	cfg := m.cfg
	m.mu.Unlock()

	if !cfg.Enabled {
		return nil, ErrLLMDisabled
	}
	if on, set := cfg.FeatureEnabled[feature]; set && !on {
		return nil, fmt.Errorf("llm: feature %q is disabled", feature)
	}
	if err := m.checkBudgets(cfg); err != nil {
		return nil, err
	}
	stream, err := m.registry.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	return &meteredStream{inner: stream, meter: m, feature: feature, model: req.Model, provider: m.providerID(req.Model)}, nil
}

// UsageSince returns all usage records from the given time forward.
func (m *Meter) UsageSince(since time.Time) []UsageRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []UsageRecord
	for _, r := range m.records {
		if !r.At.Before(since) {
			out = append(out, r)
		}
	}
	return out
}

// TodayUSD returns the total cost today across all features and providers.
func (m *Meter) TodayUSD() float64 {
	midnight := midnightToday()
	var sum float64
	for _, r := range m.UsageSince(midnight) {
		sum += r.CostUSD
	}
	return sum
}

// ThisMonthUSD returns the total cost this calendar month.
func (m *Meter) ThisMonthUSD() float64 {
	first := firstOfMonth()
	var sum float64
	for _, r := range m.UsageSince(first) {
		sum += r.CostUSD
	}
	return sum
}

// record appends a usage record thread-safely.
func (m *Meter) record(r UsageRecord) {
	m.mu.Lock()
	m.records = append(m.records, r)
	// Prune records older than 90 days.
	cutoff := time.Now().AddDate(0, -3, 0)
	i := 0
	for i < len(m.records) && m.records[i].At.Before(cutoff) {
		i++
	}
	m.records = m.records[i:]
	m.mu.Unlock()
}

func (m *Meter) checkBudgets(cfg MeterConfig) error {
	for _, limit := range cfg.Limits {
		if limit.USD <= 0 {
			continue
		}
		var spent float64
		switch limit.Period {
		case PeriodDaily:
			spent = m.TodayUSD()
		case PeriodMonthly:
			spent = m.ThisMonthUSD()
		}
		if spent >= limit.USD {
			return &ErrBudgetExceeded{Period: limit.Period, LimitUSD: limit.USD, SpentUSD: spent}
		}
	}
	return nil
}

func (m *Meter) providerID(modelID string) string {
	m.registry.mu.RLock()
	defer m.registry.mu.RUnlock()
	for _, c := range m.registry.cache {
		for _, model := range c.models {
			if model.ID == modelID {
				// Find which provider this model belongs to.
				for id, p := range m.registry.providers {
					for _, pm := range m.registry.cache[p.ID()].models {
						if pm.ID == modelID {
							return id
						}
					}
				}
			}
		}
	}
	return "unknown"
}

func (m *Meter) pricing(providerID, modelID string) Pricing {
	m.mu.Lock()
	cfg := m.cfg
	m.mu.Unlock()
	if p, ok := cfg.PricingOverride[modelID]; ok {
		return p
	}
	m.registry.mu.RLock()
	defer m.registry.mu.RUnlock()
	if c, ok := m.registry.cache[providerID]; ok {
		for _, model := range c.models {
			if model.ID == modelID && model.Pricing != nil {
				return *model.Pricing
			}
		}
	}
	return Pricing{}
}

// meteredStream wraps a Stream and records usage on EventDone.
type meteredStream struct {
	inner    Stream
	meter    *Meter
	feature  Feature
	model    string
	provider string
}

func (s *meteredStream) Next() (Event, error) {
	ev, err := s.inner.Next()
	if ev.Kind == EventDone && ev.Usage != nil {
		pr := s.meter.pricing(s.provider, s.model)
		cost := float64(ev.Usage.InputTokens)/1e6*pr.InputPerMTok +
			float64(ev.Usage.OutputTokens)/1e6*pr.OutputPerMTok
		s.meter.record(UsageRecord{
			At:           time.Now(),
			Provider:     s.provider,
			Model:        s.model,
			Feature:      s.feature,
			InputTokens:  ev.Usage.InputTokens,
			OutputTokens: ev.Usage.OutputTokens,
			CacheHits:    ev.Usage.CacheHits,
			CostUSD:      cost,
		})
	}
	return ev, err
}

func (s *meteredStream) Close() error { return s.inner.Close() }

func midnightToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

func firstOfMonth() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
}
