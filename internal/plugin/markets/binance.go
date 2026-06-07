package markets

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
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

	// Build the combined stream URL.
	quote := strings.ToLower(quoteCurrency)
	streams := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		streams = append(streams, strings.ToLower(NormaliseSymbol(sym, quoteCurrency))+quote+"@miniTicker")
	}
	rawURL := "wss://stream.binance.com:9443/stream?streams=" + strings.Join(streams, "/")

	const maxRetries = 5
	backoff := 3 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			statusFn(Status{State: "degraded", Message: fmt.Sprintf("reconnecting (attempt %d/%d)", attempt+1, maxRetries)})
			select {
			case <-ctx.Done():
				statusFn(Status{State: "disconnected"})
				return ctx.Err()
			case <-time.After(backoff):
				backoff = minDur(backoff*2, 30*time.Second)
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
	}
	statusFn(Status{State: "disconnected", Message: "max retries reached"})
	return fmt.Errorf("binance: max retries (%d) exhausted", maxRetries)
}

func (b *BinanceProvider) streamOnce(ctx context.Context, rawURL, quoteCurrency string,
	tickFn func(Tick), statusFn func(Status)) error {

	// Force HTTP/1.1: WebSocket cannot run over HTTP/2.
	// coder/websocket may advertise h2 in ALPN on some builds; this prevents that.
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(dialCtx, rawURL, &websocket.DialOptions{ //nolint:bodyclose
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				// Disable HTTP/2: WebSocket upgrade only works over HTTP/1.1.
				// Some coder/websocket builds advertise h2 in TLS ALPN; this prevents that.
				TLSClientConfig:   &tls.Config{NextProtos: []string{"http/1.1"}},
				ForceAttemptHTTP2: false,
				TLSNextProto:      make(map[string]func(string, *tls.Conn) http.RoundTripper),
			},
		},
		HTTPHeader: http.Header{
			"User-Agent": []string{"Mozilla/5.0 (compatible; VirtaBot/1.0)"},
		},
	})
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.CloseNow() }()
	// Lift the read limit — Binance combined streams can have large payloads.
	conn.SetReadLimit(256 * 1024)
	statusFn(Status{State: "connected"})

	type miniTicker struct {
		EventTime   int64  `json:"E"`
		Symbol      string `json:"s"`
		ClosePrice  string `json:"c"`
		OpenPrice   string `json:"o"`
		HighPrice   string `json:"h"`
		LowPrice    string `json:"l"`
		QuoteVolume string `json:"q"`
	}
	type envelope struct {
		Stream string          `json:"stream"`
		Data   json.RawMessage `json:"data"`
	}

	q := strings.ToUpper(quoteCurrency)

	for {
		// Use raw Read to avoid wsjson's framing assumptions.
		_, raw, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}
		var mt miniTicker
		if err := json.Unmarshal(env.Data, &mt); err != nil {
			continue
		}

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

func minDur(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
