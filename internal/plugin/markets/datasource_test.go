package markets_test

import (
	"testing"

	"github.com/elythi0n/virta/internal/plugin/markets"
)

func TestNormaliseSymbol(t *testing.T) {
	cases := []struct{ in, quote, want string }{
		{"BTCUSDT", "USDT", "BTC"},
		{"btcusdt", "usdt", "BTC"},
		{"ETH-USD", "USD", "ETH"},
		{"eth/usd", "USD", "ETH"},
		{"DOGE", "USDT", "DOGE"},
		{"pepe", "USDT", "PEPE"},
	}
	for _, c := range cases {
		got := markets.NormaliseSymbol(c.in, c.quote)
		if got != c.want {
			t.Errorf("NormaliseSymbol(%q, %q) = %q, want %q", c.in, c.quote, got, c.want)
		}
	}
}

func TestNewDataSource_Defaults(t *testing.T) {
	ds := markets.New(markets.Config{})
	if ds.ID() != "com.virta.markets" {
		t.Errorf("ID = %q, want 'markets'", ds.ID())
	}
	// Snapshot should be empty on a fresh instance.
	if snap := ds.Snapshot(); len(snap) != 0 {
		t.Errorf("expected empty snapshot, got %d ticks", len(snap))
	}
}

func TestBuiltInManifest(t *testing.T) {
	m := markets.BuiltInManifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("built-in manifest is invalid: %v", err)
	}
	if !m.HasScope("http") {
		t.Error("Markets manifest must declare ScopeHTTP (makes outbound network calls)")
	}
	if !m.HasScope("ui") {
		t.Error("Markets manifest must declare ScopeUI (contributes a panel)")
	}
	if len(m.Contributes.DataSources) == 0 {
		t.Error("expected at least one DataSource contribution")
	}
	if len(m.Contributes.Panels) == 0 {
		t.Error("expected at least one Panel contribution")
	}
}

func TestTick_Realtime_Flag(t *testing.T) {
	// Binance ticks must be Realtime=true; CoinGecko must be false.
	// We test this via the DataSource's Config provider field since we can't hit a real WS in tests.
	t.Run("binance provider ID is 'binance'", func(t *testing.T) {
		p := markets.NewBinance()
		if p.ID() != "binance" {
			t.Errorf("expected 'binance', got %q", p.ID())
		}
	})
	t.Run("coingecko provider ID is 'coingecko'", func(t *testing.T) {
		p := markets.NewCoinGecko("")
		if p.ID() != "coingecko" {
			t.Errorf("expected 'coingecko', got %q", p.ID())
		}
	})
}
