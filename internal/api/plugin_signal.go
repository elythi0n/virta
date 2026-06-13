package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// pluginSignalRelay is an in-memory relay for plugin test signals.
// The panel POSTs a signal; the overlay GETs signals newer than its last-seen timestamp.
// Signals are ephemeral: they live only for the daemon's lifetime and only the latest per
// plugin is kept. No persistence is needed — this is purely for the Test button UX.
type pluginSignalRelay struct {
	mu      sync.Mutex
	signals map[string]*signalEntry // keyed by plugin ID
}

type signalEntry struct {
	payload []byte    // raw JSON
	ts      time.Time // when it was stored (UnixMilli)
}

func newPluginSignalRelay() *pluginSignalRelay {
	return &pluginSignalRelay{signals: make(map[string]*signalEntry)}
}

func (r *pluginSignalRelay) store(pluginID string, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signals[pluginID] = &signalEntry{payload: payload, ts: time.Now()}
}

// since returns the stored payload and true when the entry is newer than after.
func (r *pluginSignalRelay) since(pluginID string, after time.Time) ([]byte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.signals[pluginID]
	if !ok || !e.ts.After(after) {
		return nil, false
	}
	// Wrap with the server timestamp so the overlay can advance its cursor.
	wrapped, err := json.Marshal(map[string]any{
		"ts":      e.ts.UnixMilli(),
		"payload": json.RawMessage(e.payload),
	})
	if err != nil {
		return e.payload, true
	}
	return wrapped, true
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

// handlePluginSignalPost stores a test signal for a plugin (called by the panel bridge).
// POST /v1/plugins/{id}/signal
func (s *Server) handlePluginSignalPost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing plugin id", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil || len(body) == 0 {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if !json.Valid(body) {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	s.signalRelay.store(id, body)
	w.WriteHeader(http.StatusNoContent)
}

// handlePluginSignalGet returns the latest signal for a plugin if it is newer than ?since=<ms>.
// GET /v1/plugins/{id}/signal?since=<unixMillis>
// Returns 204 No Content if nothing new; 200 with JSON if new signal available.
func (s *Server) handlePluginSignalGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing plugin id", http.StatusBadRequest)
		return
	}
	var after time.Time
	if ms, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64); err == nil {
		after = time.UnixMilli(ms)
	}
	payload, ok := s.signalRelay.since(id, after)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(payload)
}
