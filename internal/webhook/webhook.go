// Package webhook delivers outbound events to user-configured HTTPS endpoints (docs/15 §2).
// Each endpoint has its own ordered delivery queue with 3 retries, jittered exponential backoff,
// a 10 s timeout, and auto-pause after sustained failures. Delivery carries a SHA-256 HMAC so
// receivers can verify authenticity.
//
// The daemon side is pure event-routing: events flowing through the pipeline reach a Sink, which
// filters them and enqueues deliveries. The endpoint config (URL, events, filter, secret ref)
// lives in the store; secrets live in the OS keychain via the vault.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// maxQueueDepth is how many pending deliveries an endpoint buffers before dropping the oldest.
const maxQueueDepth = 200
const maxRetries = 3
const deliveryTimeout = 10 * time.Second

// initialBackoff is the starting retry interval; exposed as a variable so tests can shorten it.
var initialBackoff = 1 * time.Second

// Endpoint is a single outbound destination.
type Endpoint struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events"` // event type prefixes, e.g. "event.raid" or "message.highlighted"
	Active bool     `json:"active"`
	Paused bool     `json:"paused"`           // auto-paused after sustained failure
	Secret string   `json:"secret,omitempty"` // HMAC key (stored per-endpoint in keychain, not here)
}

// Delivery is one outgoing payload.
type Delivery struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	CreatedAt time.Time       `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

// AttemptRecord is a delivery attempt in the log.
type AttemptRecord struct {
	AtMs       int64  `json:"at_ms"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
	LatencyMs  int64  `json:"latency_ms"`
}

// DeliveryLog holds the last 100 attempts for an endpoint, for the settings UI.
type DeliveryLog struct {
	mu      sync.Mutex
	entries []AttemptRecord
}

func (l *DeliveryLog) Record(r AttemptRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, r)
	if len(l.entries) > 100 {
		l.entries = l.entries[len(l.entries)-100:]
	}
}

func (l *DeliveryLog) Snapshot() []AttemptRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]AttemptRecord, len(l.entries))
	copy(out, l.entries)
	return out
}

// Manager holds all endpoints and their worker goroutines, running for the daemon's lifetime.
type Manager struct {
	mu         sync.RWMutex
	endpoints  map[string]*endpointWorker
	log        *slog.Logger
	httpClient *http.Client
}

type endpointWorker struct {
	ep     Endpoint
	secret string
	queue  chan Delivery
	log    *DeliveryLog
	paused bool
	mu     sync.Mutex
	quit   chan struct{}
	wg     sync.WaitGroup
}

// NewManager builds a Manager with the given http.Client (nil = default with 10s timeout).
func NewManager(log *slog.Logger, hc *http.Client) *Manager {
	if hc == nil {
		hc = &http.Client{
			Timeout: deliveryTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 0 {
					return http.ErrUseLastResponse // no redirects
				}
				return nil
			},
		}
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Manager{endpoints: map[string]*endpointWorker{}, log: log, httpClient: hc}
}

// Register starts a delivery worker for an endpoint. Safe to call multiple times (replaces).
func (m *Manager) Register(ep Endpoint, secret string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.endpoints[ep.ID]; ok {
		close(w.quit)
		w.wg.Wait()
	}
	if !ep.Active {
		return
	}
	w := &endpointWorker{ep: ep, secret: secret, queue: make(chan Delivery, maxQueueDepth), log: &DeliveryLog{}, quit: make(chan struct{})}
	m.endpoints[ep.ID] = w
	w.wg.Add(1)
	go m.runWorker(w)
}

// Deregister stops and removes an endpoint's worker.
func (m *Manager) Deregister(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.endpoints[id]; ok {
		close(w.quit)
		w.wg.Wait()
		delete(m.endpoints, id)
	}
}

// Dispatch routes an event to every matching endpoint's queue.
func (m *Manager) Dispatch(d Delivery) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.endpoints {
		if w.matches(d.Type) {
			select {
			case w.queue <- d:
			default:
				<-w.queue // drop oldest
				w.queue <- d
			}
		}
	}
}

// DeliveryLog returns the last 100 attempt records for an endpoint.
func (m *Manager) DeliveryLog(id string) []AttemptRecord {
	m.mu.RLock()
	w := m.endpoints[id]
	m.mu.RUnlock()
	if w == nil {
		return nil
	}
	return w.log.Snapshot()
}

// IsPaused reports whether an endpoint is auto-paused.
func (m *Manager) IsPaused(id string) bool {
	m.mu.RLock()
	w := m.endpoints[id]
	m.mu.RUnlock()
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.paused
}

// Resume un-pauses a paused endpoint.
func (m *Manager) Resume(id string) {
	m.mu.RLock()
	w := m.endpoints[id]
	m.mu.RUnlock()
	if w == nil {
		return
	}
	w.mu.Lock()
	w.paused = false
	w.mu.Unlock()
}

// Close shuts down all workers.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.endpoints {
		close(w.quit)
	}
	for _, w := range m.endpoints {
		w.wg.Wait()
	}
	m.endpoints = map[string]*endpointWorker{}
}

func (m *Manager) runWorker(w *endpointWorker) {
	defer w.wg.Done()
	consecFail := 0
	for {
		select {
		case <-w.quit:
			return
		case d := <-w.queue:
			w.mu.Lock()
			paused := w.paused
			w.mu.Unlock()
			if paused {
				continue
			}
			ok := m.deliver(w, d)
			if ok {
				consecFail = 0
			} else {
				consecFail++
				if consecFail >= 5 {
					w.mu.Lock()
					w.paused = true
					w.mu.Unlock()
					m.log.Warn("webhook auto-paused after sustained failure", "endpoint", w.ep.ID, "url", w.ep.URL)
				}
			}
		}
	}
}

func (m *Manager) deliver(w *endpointWorker, d Delivery) bool {
	body, err := json.Marshal(d)
	if err != nil {
		return false
	}
	backoff := initialBackoff
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(backoff / 4)))
			time.Sleep(backoff + jitter)
			backoff *= 2
		}
		start := time.Now()
		code, err := m.post(w.ep.URL, w.secret, d.ID, body)
		rec := AttemptRecord{AtMs: start.UnixMilli(), LatencyMs: time.Since(start).Milliseconds()}
		if err != nil {
			rec.Error = err.Error()
			w.log.Record(rec)
			m.log.Debug("webhook delivery failed", "attempt", attempt+1, "err", err)
			continue
		}
		rec.StatusCode = code
		w.log.Record(rec)
		if code >= 200 && code < 300 {
			return true
		}
		m.log.Debug("webhook delivery non-2xx", "attempt", attempt+1, "status", code)
	}
	return false
}

func (m *Manager) post(url, secret, deliveryID string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	now := fmt.Sprintf("%d", time.Now().Unix())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Virta-Delivery-Id", deliveryID)
	req.Header.Set("X-Virta-Timestamp", now)
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(now))
		mac.Write(body)
		req.Header.Set("X-Virta-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}

func (w *endpointWorker) matches(eventType string) bool {
	for _, e := range w.ep.Events {
		if e == eventType || hasPrefix(eventType, e) {
			return true
		}
	}
	return false
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// EventFromPipeline converts a pipeline event to a webhook delivery, returning ok=false for
// event types that don't have a registered webhook mapping (health state, plugin events, etc.).
func EventFromPipeline(ev platform.Event, id string) (Delivery, bool) {
	evType := pipelineEventType(ev)
	if evType == "" {
		return Delivery{}, false
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return Delivery{}, false
	}
	return Delivery{ID: id, Type: evType, CreatedAt: time.Now(), Data: data}, true
}

func pipelineEventType(ev platform.Event) string {
	switch e := ev.(type) {
	case platform.MessageEvent:
		m := e.Message
		if m.Type == platform.TypeChat {
			if m.Annotations != nil && m.Annotations.Highlight != "" {
				return "message.highlighted"
			}
			return "message.chat"
		}
		switch m.Type {
		case platform.TypeSub:
			return "event.subscription"
		case platform.TypeResub:
			return "event.subscription"
		case platform.TypeGiftSub:
			return "event.subscription"
		case platform.TypeRaid:
			return "event.raid"
		case platform.TypeFollow:
			return "event.follow"
		case platform.TypeAnnouncement:
			return "event.announcement"
		}
		return "event.other"
	case platform.HealthEvent:
		if e.Status.State == platform.HealthDegraded {
			return "adapter.degraded"
		}
		return ""
	case platform.ProfileChangedEvent:
		return "profile.changed"
	case platform.ChannelClearEvent:
		return ""
	default:
		return ""
	}
}

// Sink is a pipeline.Sink that fans events to the webhook manager.
type Sink struct {
	mgr *Manager
	gen func() string // delivery ID generator
}

// NewSink wraps a Manager as a pipeline sink.
func NewSink(mgr *Manager, gen func() string) *Sink {
	return &Sink{mgr: mgr, gen: gen}
}

func (s *Sink) Name() string { return "webhooks" }

func (s *Sink) Consume(_ context.Context, ev platform.Event) error {
	d, ok := EventFromPipeline(ev, s.gen())
	if ok {
		s.mgr.Dispatch(d)
	}
	return nil
}

func (s *Sink) Close() error { return nil }

// EventCatalog returns the sorted list of supported event type names for the config UI.
func EventCatalog() []string {
	types := []string{
		"message.chat", "message.highlighted",
		"event.subscription", "event.raid", "event.follow", "event.announcement", "event.other",
		"adapter.degraded", "profile.changed",
	}
	sort.Strings(types)
	return types
}
