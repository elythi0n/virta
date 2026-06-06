package api

import (
	"context"
	"encoding/json"
	"net/http"
)

// Intel is the intelligence surface: Ask (agent loop), model listing, and LLM config.
// Injected via SetIntel.
type Intel interface {
	// ListModels returns available models grouped by provider, from the live registry.
	ListModels(ctx context.Context) ([]ModelGroup, error)
	// Ask starts an agent run and streams events. The caller reads from the channel until closed.
	Ask(ctx context.Context, model, question string) (<-chan AskEvent, error)
	// Config returns the current meter/LLM configuration.
	Config() IntelConfig
	// SetConfig replaces the configuration (from the settings UI).
	SetConfig(ctx context.Context, cfg IntelConfig) error
}

// ModelGroup is one provider's models for the settings/picker UI.
type ModelGroup struct {
	ProviderID  string      `json:"provider_id"`
	DisplayName string      `json:"display_name"`
	Models      []ModelItem `json:"models"`
}

// ModelItem is one model in the picker.
type ModelItem struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name"`
	Family        string   `json:"family"`
	ContextWindow int      `json:"context_window"`
	SupportsTools bool     `json:"supports_tools"`
	Deprecated    bool     `json:"deprecated,omitempty"`
	PriceIn       float64  `json:"price_in_per_mtok,omitempty"`
	PriceOut      float64  `json:"price_out_per_mtok,omitempty"`
}

// AskEvent is one item streamed from an Ask run.
type AskEvent struct {
	Kind        string `json:"kind"` // text|tool_use|tool_result|done|error
	Text        string `json:"text,omitempty"`
	ToolName    string `json:"tool_name,omitempty"`
	ToolArgs    string `json:"tool_args,omitempty"`
	ToolResult  string `json:"tool_result,omitempty"`
	ErrorMsg    string `json:"error,omitempty"`
	InputTokens int    `json:"input_tokens,omitempty"`
	OutputTokens int   `json:"output_tokens,omitempty"`
}

// IntelConfig is the persisted LLM configuration.
type IntelConfig struct {
	Enabled        bool              `json:"enabled"`
	SelectedModel  string            `json:"selected_model"`
	ProviderKeys   map[string]string `json:"provider_keys,omitempty"` // provider id → key (masked in GET)
	FeatureEnabled map[string]bool   `json:"feature_enabled"`
	DailyLimitUSD  float64           `json:"daily_limit_usd,omitempty"`
	MonthlyLimitUSD float64          `json:"monthly_limit_usd,omitempty"`
}

// SetIntel installs the intelligence controller.
func (s *Server) SetIntel(i Intel) { s.intel = i }

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	if s.intel == nil {
		http.Error(w, "intelligence unavailable", http.StatusServiceUnavailable)
		return
	}
	groups, err := s.intel.ListModels(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if groups == nil {
		groups = []ModelGroup{}
	}
	writeJSON(w, map[string]any{"groups": groups})
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if s.intel == nil {
		http.Error(w, "intelligence unavailable", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Question string `json:"question"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Question == "" {
		http.Error(w, "expected JSON body with question", http.StatusBadRequest)
		return
	}
	ch, err := s.intel.Ask(r.Context(), req.Model, req.Question)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Stream as NDJSON (newline-delimited JSON) so the frontend can consume events progressively.
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	enc := json.NewEncoder(w)
	flusher, hasFlusher := w.(http.Flusher)
	for ev := range ch {
		if err := enc.Encode(ev); err != nil {
			return
		}
		if hasFlusher {
			flusher.Flush()
		}
	}
}

func (s *Server) handleGetIntelConfig(w http.ResponseWriter, _ *http.Request) {
	if s.intel == nil {
		http.Error(w, "intelligence unavailable", http.StatusServiceUnavailable)
		return
	}
	cfg := s.intel.Config()
	// Mask provider keys.
	for k := range cfg.ProviderKeys {
		v := cfg.ProviderKeys[k]
		if len(v) > 8 {
			cfg.ProviderKeys[k] = v[:8] + "••••"
		}
	}
	writeJSON(w, cfg)
}

func (s *Server) handleSetIntelConfig(w http.ResponseWriter, r *http.Request) {
	if s.intel == nil {
		http.Error(w, "intelligence unavailable", http.StatusServiceUnavailable)
		return
	}
	var cfg IntelConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := s.intel.SetConfig(r.Context(), cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
