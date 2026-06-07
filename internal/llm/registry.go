package llm

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

const modelCacheTTL = 24 * time.Hour

// Registry manages a set of connected providers, their model caches, and the per-profile
// selected model. It is the single point callers use — they never call providers directly.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	cache     map[string]cachedModels
	selected  string // model ID selected by the user (persisted per-profile)
}

type cachedModels struct {
	models    []ModelInfo
	fetchedAt time.Time
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: map[string]Provider{},
		cache:     map[string]cachedModels{},
	}
}

// Register adds or replaces a provider. Call with the provider the user just configured.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
	delete(r.cache, p.ID()) // invalidate cache on re-register
}

// Deregister removes a provider (user disconnected it).
func (r *Registry) Deregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, id)
	delete(r.cache, id)
}

// SetModel stores the user's model selection.
func (r *Registry) SetModel(modelID string) {
	r.mu.Lock()
	r.selected = modelID
	r.mu.Unlock()
}

// SelectedModel returns the user's selection or a sensible default.
func (r *Registry) SelectedModel() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.selected != "" {
		return r.selected
	}
	// Default: prefer Anthropic's most capable model.
	if p, ok := r.providers["anthropic"]; ok {
		_ = p
		return "claude-opus-4-8"
	}
	return ""
}

// Complete routes a completion request to the provider that owns the requested model.
// If the request has no model, it uses the selected model.
func (r *Registry) Complete(ctx context.Context, req CompletionRequest) (Stream, error) {
	if req.Model == "" {
		req.Model = r.SelectedModel()
	}
	p, err := r.providerForModel(req.Model)
	if err != nil {
		return nil, err
	}
	return p.Complete(ctx, req)
}

// AllModels returns every model from every registered provider, refreshing caches where stale.
// Results are grouped by provider (default first), suitable for the model picker UI.
func (r *Registry) AllModels(ctx context.Context) ([]GroupedModels, error) {
	r.mu.RLock()
	pids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		pids = append(pids, id)
	}
	r.mu.RUnlock()

	sort.Strings(pids)
	// Prefer Anthropic first.
	sort.SliceStable(pids, func(i, j int) bool {
		if pids[i] == "anthropic" {
			return true
		}
		if pids[j] == "anthropic" {
			return false
		}
		return pids[i] < pids[j]
	})

	var groups []GroupedModels
	for _, id := range pids {
		r.mu.RLock()
		p := r.providers[id]
		c := r.cache[id]
		r.mu.RUnlock()

		if time.Since(c.fetchedAt) < modelCacheTTL {
			groups = append(groups, GroupedModels{ProviderID: id, DisplayName: p.DisplayName(), Models: c.models})
			continue
		}
		models, err := p.ListModels(ctx)
		if err != nil {
			// Do not update fetchedAt on failure — the next call retries immediately.
			// Serve the stale cached list (empty on first attempt) so the UI is not blank.
			groups = append(groups, GroupedModels{ProviderID: id, DisplayName: p.DisplayName(), Models: c.models})
			continue
		}
		r.mu.Lock()
		r.cache[id] = cachedModels{models: models, fetchedAt: time.Now()}
		r.mu.Unlock()
		groups = append(groups, GroupedModels{ProviderID: id, DisplayName: p.DisplayName(), Models: models})
	}
	return groups, nil
}

// GroupedModels is one provider's models for the UI picker.
type GroupedModels struct {
	ProviderID  string
	DisplayName string
	Models      []ModelInfo
}

func (r *Registry) providerForModel(modelID string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// Scan all providers' cached models for ownership; fall back to best-effort.
	for _, c := range r.cache {
		for _, m := range c.models {
			if m.ID == modelID {
				// Find the provider that owns this model.
				for _, p := range r.providers {
					for _, pm := range r.cache[p.ID()].models {
						if pm.ID == modelID {
							return p, nil
						}
					}
				}
			}
		}
	}
	// If still not found, try to use Anthropic for Anthropic-looking IDs, etc.
	if p, ok := r.providers["anthropic"]; ok {
		return p, nil
	}
	for _, p := range r.providers {
		return p, nil // return the first provider as a fallback
	}
	return nil, fmt.Errorf("%w", ErrProviderUnavailable)
}
