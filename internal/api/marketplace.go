package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const (
	marketplaceURL     = "https://virta.lol/api/marketplace.json"
	marketplaceTTL     = 24 * time.Hour
	marketplaceTimeout = 10 * time.Second
)

// MarketplacePlugin is one entry in the plugin marketplace registry.
type MarketplacePlugin struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Publisher   string   `json:"publisher"`
	Icon        string   `json:"icon,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	InstallURL  string   `json:"install_url"`
	Verified    bool     `json:"verified"`
}

type marketplaceCache struct {
	mu        sync.Mutex
	plugins   []MarketplacePlugin
	fetchedAt time.Time
}

func (c *marketplaceCache) fetch() []MarketplacePlugin {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.fetchedAt) < marketplaceTTL && c.plugins != nil {
		return c.plugins
	}

	ctx, cancel := context.WithTimeout(context.Background(), marketplaceTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, marketplaceURL, nil)
	if err != nil {
		return c.plugins
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return c.plugins
	}
	defer func() { _ = resp.Body.Close() }()

	var payload struct {
		Plugins []MarketplacePlugin `json:"plugins"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return c.plugins
	}

	c.plugins = payload.Plugins
	c.fetchedAt = time.Now()
	return c.plugins
}

func (s *Server) handleListMarketplace(w http.ResponseWriter, _ *http.Request) {
	plugins := s.marketplace.fetch()
	if plugins == nil {
		plugins = []MarketplacePlugin{}
	}
	writeJSON(w, map[string]any{"plugins": plugins})
}
