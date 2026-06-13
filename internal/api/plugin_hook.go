package api

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// pluginHookStore is an in-memory ring buffer of custom alert events fired by external tools
// (donation platforms, bots, other plugins) via POST /v1/plugins/{id}/hook/{name}.
// Only the latest 100 events per plugin are kept — this is a live-overlay UX aid, not a log.
type pluginHookStore struct {
	mu     sync.Mutex
	events map[string][]hookEvent // keyed by plugin ID
}

type hookEvent struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
	Ts      int64           `json:"ts"` // UnixMilli
}

const hookRingCap = 100

// validHookName allows lowercase-alpha, digits, and hyphens — safe as a URL path segment.
var validHookName = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{0,63}$`)

func newPluginHookStore() *pluginHookStore {
	return &pluginHookStore{events: make(map[string][]hookEvent)}
}

func (s *pluginHookStore) push(pluginID, name string, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ring := s.events[pluginID]
	ring = append(ring, hookEvent{
		Name:    name,
		Payload: json.RawMessage(payload),
		Ts:      time.Now().UnixMilli(),
	})
	if len(ring) > hookRingCap {
		ring = ring[len(ring)-hookRingCap:]
	}
	s.events[pluginID] = ring
}

// since returns all events newer than afterMs (exclusive).
func (s *pluginHookStore) since(pluginID string, afterMs int64) []hookEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	ring := s.events[pluginID]
	var out []hookEvent
	for _, e := range ring {
		if e.Ts > afterMs {
			out = append(out, e)
		}
	}
	return out
}

// ── HTTP handlers ──────────────────────────────────────────────────────────────────────────

// handlePluginHookPost fires a custom named alert event for a plugin.
// POST /v1/plugins/{id}/hook/{name}?secret=<webhookSecret>
//
// This endpoint does NOT use the standard bearer-token auth — it authenticates via a
// ?secret= query param checked against the webhookSecret stored in the plugin's config.
// That secret lives in the plugin config object so it travels with export/import and can
// be regenerated from the panel UI.  Callers: Streamlabs, Ko-fi, custom bots, other plugins.
func (s *Server) handlePluginHookPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	rawName := r.PathValue("name")
	name := strings.ToLower(rawName)
	if !validHookName.MatchString(name) {
		http.Error(w, "invalid event name: use lowercase letters, digits, hyphens (max 64 chars)", http.StatusBadRequest)
		return
	}

	secret := r.URL.Query().Get("secret")
	if secret == "" {
		http.Error(w, "missing ?secret", http.StatusUnauthorized)
		return
	}

	// Resolve and validate the webhook secret against what's in the plugin config.
	pc, ok := s.plugins.(PluginConfigurer)
	if s.plugins == nil || !ok {
		http.Error(w, "plugin system unavailable", http.StatusServiceUnavailable)
		return
	}
	cfgRaw, err := pc.GetConfig(id)
	if err != nil || len(cfgRaw) == 0 {
		http.Error(w, "unknown plugin", http.StatusNotFound)
		return
	}
	var cfgMap map[string]any
	if err := json.Unmarshal(cfgRaw, &cfgMap); err != nil {
		http.Error(w, "malformed plugin config", http.StatusInternalServerError)
		return
	}
	stored, _ := cfgMap["webhookSecret"].(string)
	if stored == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(stored)) != 1 {
		http.Error(w, "invalid secret", http.StatusUnauthorized)
		return
	}

	// Accept the payload.
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if len(body) == 0 {
		body = []byte("{}")
	}
	if !json.Valid(body) {
		http.Error(w, "body must be a JSON object", http.StatusBadRequest)
		return
	}

	s.hookStore.push(id, name, body)
	w.WriteHeader(http.StatusNoContent)
}

// handlePluginHookEvents returns pending custom events for a plugin since a given timestamp.
// GET /v1/plugins/{id}/hook/events?since=<unixMillis>
// Returns 204 when nothing new; 200 with { events: [...] } otherwise.
// Uses standard ScopeRead auth (the overlay's read token covers this).
func (s *Server) handlePluginHookEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var afterMs int64
	if ms, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64); err == nil {
		afterMs = ms
	}
	events := s.hookStore.since(id, afterMs)
	if len(events) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	type resp struct {
		Events []hookEvent `json:"events"`
	}
	_ = json.NewEncoder(w).Encode(resp{Events: events})
}
