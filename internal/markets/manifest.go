package markets

import (
	"encoding/json"

	"github.com/elythi0n/virta/internal/pluginhost"
)

// BuiltInManifest returns the canonical Manifest for the built-in Markets plugin.
func BuiltInManifest() *pluginhost.Manifest {
	configSchema, _ := json.Marshal(configSchemaJSON)
	return &pluginhost.Manifest{
		ID:          "com.virta.markets",
		Name:        "Markets",
		Version:     "1.0.0",
		Publisher:   "Virta",
		Description: "Real-time crypto ticker and price board via free exchange WebSockets.",
		Tags:        []string{"data", "ticker", "crypto"},
		Scopes: []pluginhost.Scope{
			pluginhost.ScopeUI,
			pluginhost.ScopeHTTP, // outbound WS+REST to Binance/CoinGecko
		},
		Contributes: pluginhost.Contributes{
			Panels: []pluginhost.PanelContrib{
				{Kind: "markets", Title: "Markets", Icon: "stats"},
			},
			Commands: []pluginhost.CommandContrib{
				{Name: "markets", Title: "Markets — look up a symbol", Scope: "both"},
			},
			DataSources: []pluginhost.DataSourceContrib{
				{ID: "tick"},
				{ID: "status"},
			},
		},
		Config:  configSchema,
		BuiltIn: true,
	}
}

// configSchemaJSON is the JSON Schema for the Markets plugin configuration, used to
// drive the auto-generated settings form in the Plugins panel.
var configSchemaJSON = map[string]any{
	"$schema": "https://json-schema.org/draft/2020-12/schema",
	"type":    "object",
	"properties": map[string]any{
		"watchlist": map[string]any{
			"type":        "array",
			"title":       "Watchlist",
			"description": "Symbols to track (e.g. BTC, ETH, DOGE).",
			"items":       map[string]any{"type": "string"},
			"default":     []string{"BTC", "ETH", "DOGE", "PEPE", "SOL"},
		},
		"quote_currency": map[string]any{
			"type":        "string",
			"title":       "Quote currency",
			"description": "Denominator for prices.",
			"enum":        []string{"USDT", "USD", "EUR", "BTC"},
			"default":     "USDT",
		},
		"provider": map[string]any{
			"type":        "string",
			"title":       "Data provider",
			"description": "Primary price source. Binance = real-time WS; CoinGecko = REST (delayed).",
			"enum":        []string{"binance", "coingecko"},
			"default":     "coingecko",
		},
		"display_mode": map[string]any{
			"type":        "string",
			"title":       "Display mode",
			"description": "How to show prices in the panel.",
			"enum":        []string{"board", "ticker", "compact"},
			"default":     "board",
		},
		"refresh_seconds": map[string]any{
			"type":    "integer",
			"title":   "Refresh interval (seconds)",
			"default": 30,
			"minimum": 10,
			"maximum": 300,
		},
		"coingecko_api_key": map[string]any{
			"type":        "string",
			"title":       "CoinGecko API key (optional)",
			"description": "Leave empty for the free tier (rate-limited). Get a key at coingecko.com.",
			"default":     "",
		},
	},
	"required": []string{"watchlist", "quote_currency", "provider", "display_mode"},
}
