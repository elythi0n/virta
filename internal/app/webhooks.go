package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/elythi0n/virta/internal/api"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/webhook"
)

// webhookControl adapts the webhook.Manager to the API's Webhooks interface. Endpoint configs are
// persisted in the settings repo (scope "webhooks.<id>") so they survive restarts. Secrets are
// stored inline for now (with the config); a future iteration moves them to the keychain.
type webhookControl struct {
	mu      sync.Mutex
	mgr     *webhook.Manager
	store   store.SettingsRepo
	configs map[string]webhook.Endpoint
}

func newWebhookControl(mgr *webhook.Manager, settings store.SettingsRepo) api.Webhooks {
	c := &webhookControl{mgr: mgr, store: settings, configs: map[string]webhook.Endpoint{}}
	// Reload persisted endpoints.
	if all, err := settings.All(context.Background()); err == nil {
		for _, s := range all {
			var ep webhook.Endpoint
			if len(s.Data) == 0 || string(s.Data) == "null" {
				continue
			}
			if err := json.Unmarshal(s.Data, &ep); err == nil {
				c.configs[ep.ID] = ep
				c.mgr.Register(ep, ep.Secret)
			}
		}
	}
	return c
}

func (c *webhookControl) List() []api.WebhookEndpointInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	list := make([]api.WebhookEndpointInfo, 0, len(c.configs))
	for _, ep := range c.configs {
		events := ep.Events
		if events == nil {
			events = []string{}
		}
		list = append(list, api.WebhookEndpointInfo{
			ID: ep.ID, Name: ep.Name, URL: ep.URL,
			Events: events, Active: ep.Active,
			Paused: c.mgr.IsPaused(ep.ID),
		})
	}
	return list
}

func (c *webhookControl) Create(_ context.Context, name, url string, events []string, secret string) (api.WebhookEndpointInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, _ := api.NewTokenSecret()
	id := s[:12]
	ep := webhook.Endpoint{ID: id, Name: name, URL: url, Events: events, Active: true, Secret: secret}
	c.configs[id] = ep
	c.mgr.Register(ep, secret)
	c.persist(ep)
	return api.WebhookEndpointInfo{ID: id, Name: name, URL: url, Events: events, Active: true}, nil
}

func (c *webhookControl) Update(_ context.Context, id, name, url string, events []string, active bool) (api.WebhookEndpointInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ep, ok := c.configs[id]
	if !ok {
		return api.WebhookEndpointInfo{}, fmt.Errorf("webhook %q not found", id)
	}
	ep.Name, ep.URL, ep.Events, ep.Active = name, url, events, active
	c.configs[id] = ep
	c.mgr.Register(ep, ep.Secret)
	c.persist(ep)
	return api.WebhookEndpointInfo{ID: id, Name: name, URL: url, Events: events, Active: active}, nil
}

func (c *webhookControl) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.configs[id]; !ok {
		return fmt.Errorf("webhook %q not found", id)
	}
	delete(c.configs, id)
	c.mgr.Deregister(id)
	_ = c.store.Put(context.Background(), store.Setting{Scope: "webhooks." + id, Data: []byte("null")})
	return nil
}

func (c *webhookControl) Log(id string) []api.WebhookAttempt {
	recs := c.mgr.DeliveryLog(id)
	out := make([]api.WebhookAttempt, len(recs))
	for i, r := range recs {
		out[i] = api.WebhookAttempt{AtMs: r.AtMs, StatusCode: r.StatusCode, Error: r.Error, LatencyMs: r.LatencyMs}
	}
	return out
}

func (c *webhookControl) Resume(id string) error {
	c.mu.Lock()
	_, ok := c.configs[id]
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("webhook %q not found", id)
	}
	c.mgr.Resume(id)
	return nil
}

func (c *webhookControl) EventCatalog() []string { return webhook.EventCatalog() }

func (c *webhookControl) persist(ep webhook.Endpoint) {
	data, _ := json.Marshal(ep)
	_ = c.store.Put(context.Background(), store.Setting{Scope: "webhooks." + ep.ID, Data: data})
}
