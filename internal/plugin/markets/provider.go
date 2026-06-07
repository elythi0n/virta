// Package markets implements the Markets DataSource: a live feed of cryptocurrency price ticks
// from exchange WebSocket streams (Binance by default) with a CoinGecko REST fallback.
// The package is wired into the plugin DataSource seam so the Markets panel receives updates
// over the existing virta WebSocket bus without any direct renderer ↔ exchange communication.
package markets

import (
	"context"
	"strings"
	"time"
)

// Tick is one price update for a single trading pair.
type Tick struct {
	// Symbol is the base asset, upper-case (e.g. "BTC", "DOGE").
	Symbol string `json:"symbol"`
	// Quote is the denominator currency (e.g. "USDT", "USD").
	Quote string `json:"quote"`
	// Price is the current last-trade price in the quote currency.
	Price float64 `json:"price"`
	// Change24h is the 24-hour price change as a percentage (e.g. 3.14 = +3.14%).
	Change24h float64 `json:"change_24h"`
	// Change1h is the 1-hour price change percentage (may be zero for REST-polled sources).
	Change1h float64 `json:"change_1h,omitempty"`
	// Volume24h is the 24-hour trading volume in the quote currency.
	Volume24h float64 `json:"volume_24h,omitempty"`
	// High24h / Low24h are the 24-hour range extremes.
	High24h float64 `json:"high_24h,omitempty"`
	Low24h  float64 `json:"low_24h,omitempty"`
	// Realtime is true when the tick comes from a live WebSocket stream.
	// false means the price is polled/delayed (honest label required in the UI).
	Realtime bool `json:"realtime"`
	// Provider identifies the source (e.g. "binance", "coingecko").
	Provider string `json:"provider"`
	// Timestamp is when this tick was produced.
	Timestamp time.Time `json:"ts"`
}

// Status is published on the "plugin.markets.status" stream whenever
// the provider's connection state changes.
type Status struct {
	State   string `json:"state"` // "connecting" | "connected" | "degraded" | "disconnected"
	Message string `json:"message,omitempty"`
}

// Provider streams price ticks for a watchlist of symbols.
type Provider interface {
	// ID returns a stable lower-case provider identifier ("binance", "coingecko").
	ID() string
	// Name returns the human-readable provider name.
	Name() string
	// Stream publishes Tick values for the requested symbols (upper-case base asset,
	// e.g. "BTC", "DOGE") quoted in quoteCurrency ("USDT", "USD") until ctx is cancelled.
	// Status changes are sent to statusFn; tick data to tickFn.
	Stream(ctx context.Context, symbols []string, quoteCurrency string,
		tickFn func(Tick), statusFn func(Status)) error
}

// NormaliseSymbol cleans user-entered symbols: upper-cases and strips common suffixes
// so that "btcusdt", "BTC-USD", "eth/usdt", and "ETH" all normalise to the base asset.
func NormaliseSymbol(s, quoteCurrency string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	q := strings.ToUpper(quoteCurrency)
	// Try each separator variant, longest match first.
	for _, sep := range []string{"-", "/", ""} {
		if strings.HasSuffix(s, sep+q) {
			s = strings.TrimSuffix(s, sep+q)
			break
		}
	}
	return s
}
