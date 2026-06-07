package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// IsURLValue reports whether a provider-key value is a URL rather than an API key secret.
// URL values are not masked since they are not sensitive credentials.
func IsURLValue(v string) bool {
	return strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://")
}

// Intel is the intelligence surface: Ask (agent loop), model listing, config, and conversations.
// Injected via SetIntel.
type Intel interface {
	// ListModels returns available models grouped by provider, from the live registry.
	ListModels(ctx context.Context) ([]ModelGroup, error)
	// Ask starts an agent run and streams events. The caller reads from the channel until closed.
	Ask(ctx context.Context, model, question string) (<-chan AskEvent, error)
	// GenerateTitle streams a short title for the given user message using the specified model.
	GenerateTitle(ctx context.Context, model, firstMessage string) (<-chan AskEvent, error)
	// Config returns the current meter/LLM configuration.
	Config() IntelConfig
	// SetConfig replaces the configuration (from the settings UI).
	SetConfig(ctx context.Context, cfg IntelConfig) error
	// ListConversations returns recent conversations newest-first.
	ListConversations(ctx context.Context) ([]ConversationSummary, error)
	// SaveConversation creates or updates a conversation.
	SaveConversation(ctx context.Context, id, title, model string, messages []byte) error
	// DeleteConversation removes a conversation.
	DeleteConversation(ctx context.Context, id string) error
}

// ConversationSummary is the list-view form of a conversation (no messages).
type ConversationSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Model     string `json:"model"`
	UpdatedAt string `json:"updated_at"` // RFC3339
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
	// Mask API key values so secrets never leave the daemon in plaintext.
	// URL-type values (Ollama base URL) are not secrets and are returned as-is.
	for k := range cfg.ProviderKeys {
		v := cfg.ProviderKeys[k]
		if len(v) > 8 && !IsURLValue(v) {
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

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	if s.intel == nil {
		http.Error(w, "intelligence unavailable", http.StatusServiceUnavailable)
		return
	}
	list, err := s.intel.ListConversations(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []ConversationSummary{}
	}
	writeJSON(w, map[string]any{"conversations": list})
}

func (s *Server) handleSaveConversation(w http.ResponseWriter, r *http.Request) {
	if s.intel == nil {
		http.Error(w, "intelligence unavailable", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		ID       string          `json:"id"`
		Title    string          `json:"title"`
		Model    string          `json:"model"`
		Messages json.RawMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		http.Error(w, "expected JSON with id", http.StatusBadRequest)
		return
	}
	if err := s.intel.SaveConversation(r.Context(), req.ID, req.Title, req.Model, req.Messages); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	if s.intel == nil {
		http.Error(w, "intelligence unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if err := s.intel.DeleteConversation(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGenerateTitle streams a short AI-generated title for the given first message.
// Uses the same NDJSON format as /v1/intel/ask so the frontend can stream the title in.
func (s *Server) handleGenerateTitle(w http.ResponseWriter, r *http.Request) {
	if s.intel == nil {
		http.Error(w, "intelligence unavailable", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Message string `json:"message"`
		Model   string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		http.Error(w, "expected JSON with message", http.StatusBadRequest)
		return
	}
	ch, err := s.intel.GenerateTitle(r.Context(), req.Model, req.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	enc := json.NewEncoder(w)
	flush, _ := w.(http.Flusher)
	for ev := range ch {
		_ = enc.Encode(ev)
		if flush != nil {
			flush.Flush()
		}
	}
}
