package markets

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Config holds the persisted Markets plugin configuration.
type Config struct {
	// Watchlist is the list of symbols to track (upper-case base asset, e.g. ["BTC","DOGE","PEPE"]).
	Watchlist []string `json:"watchlist"`
	// QuoteCurrency is the denominator (default "USDT").
	QuoteCurrency string `json:"quote_currency"`
	// Provider selects the primary data source ("binance", "coingecko"). Default "binance".
	Provider string `json:"provider"`
	// RefreshSeconds is the polling interval for REST providers (default 30). Ignored for WS.
	RefreshSeconds int `json:"refresh_seconds,omitempty"`
	// CoinGeckoAPIKey is optional; empty = free tier.
	CoinGeckoAPIKey string `json:"coingecko_api_key,omitempty"`
}

func (c *Config) withDefaults() Config {
	out := *c
	if len(out.Watchlist) == 0 {
		out.Watchlist = []string{"BTC", "ETH", "DOGE", "PEPE", "SOL"}
	}
	if out.QuoteCurrency == "" {
		out.QuoteCurrency = "USDT"
	}
	if out.Provider == "" {
		out.Provider = "binance"
	}
	if out.RefreshSeconds <= 0 {
		out.RefreshSeconds = 30
	}
	return out
}

// DataSource is the Markets DataSource implementing the plugins.DataSource contract.
// It selects the primary provider from Config, with CoinGecko as the automatic fallback.
type DataSource struct {
	cfg      Config
	mu       sync.RWMutex
	latest   map[string]Tick // symbol → last tick (for status/reconnect replay)
}

// New creates a DataSource with the given configuration.
func New(cfg Config) *DataSource {
	return &DataSource{
		cfg:    cfg.withDefaults(),
		latest: map[string]Tick{},
	}
}

// UpdateConfig hot-swaps the configuration; the next Run call or reconnect picks it up.
func (d *DataSource) UpdateConfig(cfg Config) {
	d.mu.Lock()
	d.cfg = cfg.withDefaults()
	d.mu.Unlock()
}

func (d *DataSource) ID() string { return "com.virta.markets" }

// Run implements plugins.DataSource. It streams tick events on "tick" and status updates
// on "status". It tries the configured primary provider first, then falls back to CoinGecko.
func (d *DataSource) Run(ctx context.Context, publish func(stream string, data any)) error {
	d.mu.RLock()
	cfg := d.cfg
	d.mu.RUnlock()

	tickFn := func(t Tick) {
		d.mu.Lock()
		d.latest[t.Symbol] = t
		d.mu.Unlock()
		publish("tick", t)
	}
	statusFn := func(s Status) {
		publish("status", s)
	}

	primary := d.selectProvider(cfg)
	err := primary.Stream(ctx, cfg.Watchlist, cfg.QuoteCurrency, tickFn, statusFn)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Primary failed — fall back to CoinGecko if primary wasn't already CoinGecko.
	if primary.ID() != "coingecko" {
		statusFn(Status{State: "degraded", Message: fmt.Sprintf("%s failed — falling back to CoinGecko (delayed data)", primary.Name())})
		fallback := NewCoinGecko(cfg.CoinGeckoAPIKey)
		if cfg.RefreshSeconds > 0 {
			fallback.interval = time.Duration(cfg.RefreshSeconds) * time.Second
		}
		return fallback.Stream(ctx, cfg.Watchlist, cfg.QuoteCurrency, tickFn, statusFn)
	}
	return err
}

func (d *DataSource) selectProvider(cfg Config) Provider {
	switch cfg.Provider {
	case "coingecko":
		p := NewCoinGecko(cfg.CoinGeckoAPIKey)
		if cfg.RefreshSeconds > 0 {
			p.interval = time.Duration(cfg.RefreshSeconds) * time.Second
		}
		return p
	default:
		return NewBinance()
	}
}

// Snapshot returns the latest known tick for each symbol (used for panel mount hydration).
func (d *DataSource) Snapshot() []Tick {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Tick, 0, len(d.latest))
	for _, t := range d.latest {
		out = append(out, t)
	}
	return out
}

// parseFloat is a best-effort string → float64 parser used across the package.
func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
