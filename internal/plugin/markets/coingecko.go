package markets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CoinGeckoProvider polls the CoinGecko free REST API for price data.
// It is the fallback when the exchange WS is unavailable or when long-tail tokens
// (PEPE, SHIB, etc.) are requested that may not be on Binance.
// Rate limit on the free tier: ~30 calls/min. We poll on a configurable interval
// defaulting to 30 s, well within the limit for up to 250 symbols per call.
type CoinGeckoProvider struct {
	apiKey   string // optional — empty = free tier
	client   *http.Client
	interval time.Duration
}

func NewCoinGecko(apiKey string) *CoinGeckoProvider {
	return &CoinGeckoProvider{
		apiKey:   apiKey,
		client:   &http.Client{Timeout: 15 * time.Second},
		interval: 30 * time.Second,
	}
}

func (g *CoinGeckoProvider) ID() string   { return "coingecko" }
func (g *CoinGeckoProvider) Name() string { return "CoinGecko" }

func (g *CoinGeckoProvider) Stream(ctx context.Context, symbols []string, quoteCurrency string,
	tickFn func(Tick), statusFn func(Status)) error {

	if len(symbols) == 0 {
		return nil
	}

	// Map common ticker symbols to CoinGecko coin IDs.
	ids := make([]string, 0, len(symbols))
	idBySymbol := map[string]string{}
	for _, sym := range symbols {
		base := NormaliseSymbol(sym, quoteCurrency)
		id := symbolToGeckoID(base)
		ids = append(ids, id)
		idBySymbol[id] = base
	}

	statusFn(Status{State: "connecting", Message: "CoinGecko (delayed)"})
	// Initial fetch immediately.
	g.fetchAndPublish(ctx, ids, idBySymbol, strings.ToLower(quoteCurrency), tickFn, statusFn)

	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			statusFn(Status{State: "disconnected"})
			return ctx.Err()
		case <-ticker.C:
			g.fetchAndPublish(ctx, ids, idBySymbol, strings.ToLower(quoteCurrency), tickFn, statusFn)
		}
	}
}

func (g *CoinGeckoProvider) fetchAndPublish(ctx context.Context, ids []string,
	idBySymbol map[string]string, vs string, tickFn func(Tick), statusFn func(Status)) {

	url := fmt.Sprintf(
		"https://api.coingecko.com/api/v3/coins/markets?vs_currency=%s&ids=%s&per_page=250&price_change_percentage=1h,24h",
		vs, strings.Join(ids, ","),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/json")
	if g.apiKey != "" {
		req.Header.Set("x-cg-demo-api-key", g.apiKey)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		statusFn(Status{State: "degraded", Message: err.Error()})
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 429 {
		statusFn(Status{State: "degraded", Message: "CoinGecko rate limit — retrying later"})
		return
	}
	if resp.StatusCode != 200 {
		statusFn(Status{State: "degraded", Message: fmt.Sprintf("CoinGecko %d", resp.StatusCode)})
		return
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	var coins []struct {
		ID                                string  `json:"id"`
		Symbol                            string  `json:"symbol"`
		CurrentPrice                      float64 `json:"current_price"`
		PriceChangePercentage24h          float64 `json:"price_change_percentage_24h"`
		PriceChangePercentage1hInCurrency float64 `json:"price_change_percentage_1h_in_currency"`
		High24h                           float64 `json:"high_24h"`
		Low24h                            float64 `json:"low_24h"`
		TotalVolume                       float64 `json:"total_volume"`
	}
	if err := json.Unmarshal(body, &coins); err != nil {
		statusFn(Status{State: "degraded", Message: "CoinGecko parse error"})
		return
	}

	statusFn(Status{State: "connected", Message: "CoinGecko (delayed)"})
	now := time.Now().UTC()
	q := strings.ToUpper(vs)
	for _, c := range coins {
		sym := strings.ToUpper(c.Symbol)
		if override, ok := idBySymbol[c.ID]; ok {
			sym = override
		}
		tickFn(Tick{
			Symbol:    sym,
			Quote:     q,
			Price:     c.CurrentPrice,
			Change24h: c.PriceChangePercentage24h,
			Change1h:  c.PriceChangePercentage1hInCurrency,
			High24h:   c.High24h,
			Low24h:    c.Low24h,
			Volume24h: c.TotalVolume,
			Realtime:  false,
			Provider:  "coingecko",
			Timestamp: now,
		})
	}
}

// symbolToGeckoID maps common ticker symbols to CoinGecko coin IDs.
// Unknown symbols are passed through lower-cased (often correct for smaller tokens).
func symbolToGeckoID(sym string) string {
	m := map[string]string{
		"BTC":   "bitcoin",
		"ETH":   "ethereum",
		"BNB":   "binancecoin",
		"SOL":   "solana",
		"XRP":   "ripple",
		"ADA":   "cardano",
		"DOGE":  "dogecoin",
		"SHIB":  "shiba-inu",
		"PEPE":  "pepe",
		"AVAX":  "avalanche-2",
		"DOT":   "polkadot",
		"LINK":  "chainlink",
		"MATIC": "matic-network",
		"UNI":   "uniswap",
		"LTC":   "litecoin",
		"BCH":   "bitcoin-cash",
		"ATOM":  "cosmos",
		"XLM":   "stellar",
		"NEAR":  "near",
		"FTM":   "fantom",
		"ALGO":  "algorand",
		"VET":   "vechain",
		"ICP":   "internet-computer",
		"FIL":   "filecoin",
		"TRX":   "tron",
		"ETC":   "ethereum-classic",
	}
	if id, ok := m[strings.ToUpper(sym)]; ok {
		return id
	}
	return strings.ToLower(sym)
}
