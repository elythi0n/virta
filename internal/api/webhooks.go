package api

import (
	"context"
	"encoding/json"
	"net/http"
)

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
	info, err := s.webhooks.Create(r.Context(), req.Name, req.URL, req.Events, req.Secret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, info)
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
