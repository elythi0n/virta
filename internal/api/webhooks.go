package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// validateWebhookURL rejects non-HTTPS/HTTP schemes and resolves the hostname to block
// loopback, private, link-local, unspecified, and multicast addresses (SSRF guard).
func validateWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "http") {
		return fmt.Errorf("URL scheme must be http or https")
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("URL host %q is not allowed", host)
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("cannot resolve host %q: %w", host, err)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("URL host %q resolves to a private or reserved address; not allowed", host)
		}
	}
	return nil
}

// WebhookEndpointInfo is the API view of a webhook endpoint (no secret).
type WebhookEndpointInfo struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Active bool     `json:"active"`
	Paused bool     `json:"paused"`
}

// WebhookAttempt is one delivery attempt in the log.
type WebhookAttempt struct {
	AtMs       int64  `json:"at_ms"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
	LatencyMs  int64  `json:"latency_ms"`
}

// Webhooks is the webhook management surface.
type Webhooks interface {
	List() []WebhookEndpointInfo
	Create(ctx context.Context, name, url string, events []string, secret string) (WebhookEndpointInfo, error)
	Update(ctx context.Context, id string, name, url string, events []string, active bool) (WebhookEndpointInfo, error)
	Delete(id string) error
	Log(id string) []WebhookAttempt
	Resume(id string) error
	EventCatalog() []string
}

// SetWebhooks installs the webhook controller.
func (s *Server) SetWebhooks(w Webhooks) { s.webhooks = w }

func (s *Server) handleListWebhooks(w http.ResponseWriter, _ *http.Request) {
	if s.webhooks == nil {
		http.Error(w, "webhooks unavailable", http.StatusServiceUnavailable)
		return
	}
	list := s.webhooks.List()
	if list == nil {
		list = []WebhookEndpointInfo{}
	}
	writeJSON(w, map[string]any{"endpoints": list, "event_catalog": s.webhooks.EventCatalog()})
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		http.Error(w, "webhooks unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req struct {
		Name   string   `json:"name"`
		URL    string   `json:"url"`
		Events []string `json:"events"`
		Secret string   `json:"secret,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.URL == "" {
		http.Error(w, "expected JSON body with name, url, and events", http.StatusBadRequest)
		return
	}
	if err := validateWebhookURL(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Auto-generate a secret if the caller did not provide one. The plaintext is returned once
	// in the response so the caller can record it; only the hash is stored thereafter.
	generatedSecret := ""
	if req.Secret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			http.Error(w, "failed to generate secret", http.StatusInternalServerError)
			return
		}
		req.Secret = hex.EncodeToString(b)
		generatedSecret = req.Secret
	}
	info, err := s.webhooks.Create(r.Context(), req.Name, req.URL, req.Events, req.Secret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := map[string]any{"endpoint": info}
	if generatedSecret != "" {
		resp["secret"] = generatedSecret
	}
	writeJSON(w, resp)
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		http.Error(w, "webhooks unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if err := s.webhooks.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWebhookLog(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		http.Error(w, "webhooks unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	log := s.webhooks.Log(id)
	if log == nil {
		log = []WebhookAttempt{}
	}
	writeJSON(w, map[string]any{"attempts": log})
}

func (s *Server) handleResumeWebhook(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		http.Error(w, "webhooks unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.webhooks.Resume(r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
