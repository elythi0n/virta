package markets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// BinanceProvider streams real-time price ticks from the Binance public WebSocket API.
// No API key is required for the miniTicker streams used here.
// Docs: https://developers.binance.com/docs/binance-spot-api-docs/web-socket-streams
type BinanceProvider struct{}

func NewBinance() *BinanceProvider { return &BinanceProvider{} }

func (b *BinanceProvider) ID() string   { return "binance" }
func (b *BinanceProvider) Name() string { return "Binance" }

func (b *BinanceProvider) Stream(ctx context.Context, symbols []string, quoteCurrency string,
	tickFn func(Tick), statusFn func(Status)) error {

	if len(symbols) == 0 {
		return nil
	}

	// Build the combined stream URL: each symbol gets its own miniTicker stream.
	// e.g. wss://stream.binance.com:9443/stream?streams=btcusdt@miniTicker/dogeusdts@miniTicker
	quote := strings.ToLower(quoteCurrency)
	streams := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		streams = append(streams, strings.ToLower(NormaliseSymbol(sym, quoteCurrency))+quote+"@miniTicker")
	}
	rawURL := "wss://stream.binance.com:9443/stream?streams=" + strings.Join(streams, "/")

	const maxRetries = 5
	backoff := 2 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			statusFn(Status{State: "degraded", Message: fmt.Sprintf("reconnecting (attempt %d/%d)", attempt+1, maxRetries)})
			select {
			case <-ctx.Done():
				statusFn(Status{State: "disconnected"})
				return ctx.Err()
			case <-time.After(backoff):
				backoff = min(backoff*2, 30*time.Second)
			}
		}

		statusFn(Status{State: "connecting"})
		err := b.streamOnce(ctx, rawURL, quoteCurrency, tickFn, statusFn)
		if ctx.Err() != nil {
			statusFn(Status{State: "disconnected"})
			return ctx.Err()
		}
		if err != nil {
			statusFn(Status{State: "degraded", Message: err.Error()})
			continue
		}
		// Clean disconnect (server closed): retry.
	}
	statusFn(Status{State: "disconnected", Message: "max retries reached"})
	return fmt.Errorf("binance: max retries (%d) exhausted", maxRetries)
}

// streamOnce runs one WS session until the connection closes or ctx is done.
func (b *BinanceProvider) streamOnce(ctx context.Context, rawURL, quoteCurrency string,
	tickFn func(Tick), statusFn func(Status)) error {

	conn, _, err := websocket.Dial(ctx, rawURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.CloseNow()
	statusFn(Status{State: "connected"})

	// Combined stream envelope: {"stream":"btcusdt@miniTicker","data":{...}}
	type miniTicker struct {
		EventTime    int64  `json:"E"`
		Symbol       string `json:"s"` // e.g. "BTCUSDT"
		ClosePrice   string `json:"c"` // last price
		OpenPrice    string `json:"o"` // 24h open
		HighPrice    string `json:"h"` // 24h high
		LowPrice     string `json:"l"` // 24h low
		BaseVolume   string `json:"v"` // 24h base volume
		QuoteVolume  string `json:"q"` // 24h quote volume
	}
	type envelope struct {
		Stream string          `json:"stream"`
		Data   json.RawMessage `json:"data"`
	}

	q := strings.ToUpper(quoteCurrency)

	for {
		var env envelope
		if err := wsjson.Read(ctx, conn, &env); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}

		var mt miniTicker
		if err := json.Unmarshal(env.Data, &mt); err != nil {
			continue // skip malformed frames
		}

		// Strip the quote suffix to get the base symbol.
		baseSym := strings.TrimSuffix(strings.ToUpper(mt.Symbol), q)

		price := parseFloat(mt.ClosePrice)
		open := parseFloat(mt.OpenPrice)
		change24h := 0.0
		if open != 0 {
			change24h = (price - open) / open * 100
		}

		tickFn(Tick{
			Symbol:    baseSym,
			Quote:     q,
			Price:     price,
			Change24h: change24h,
			High24h:   parseFloat(mt.HighPrice),
			Low24h:    parseFloat(mt.LowPrice),
			Volume24h: parseFloat(mt.QuoteVolume),
			Realtime:  true,
			Provider:  "binance",
			Timestamp: time.UnixMilli(mt.EventTime).UTC(),
		})
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
