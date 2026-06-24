package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/elythi0n/virta/internal/api"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/userctx"
	"github.com/elythi0n/virta/internal/webhook"
)

// webhookControl adapts the webhook.Manager to the API's Webhooks interface. Endpoint configs
// are persisted in the settings repo (scope "webhooks.<id>"), so they survive restarts. Secrets
// are stored inline for now (with the config); a future iteration moves them to the keychain.
//
// Hosted mode: configs and the manager's registrations are keyed by the *namespaced* id
// "<user_id>|<id>" so two users can have webhooks with the same short id without colliding.
// Single-user mode: user_id is "" so the namespaced id is "|<id>" — still a single namespace,
// behavior identical to before.
type webhookControl struct {
	mu      sync.Mutex
	mgr     *webhook.Manager
	store   store.SettingsRepo
	configs map[string]webhook.Endpoint // namespaced id ("user|id") → endpoint
}

// nsKey composes the per-user namespace + the short id so two users can each own "foo".
func nsKey(userID, id string) string { return userID + "|" + id }

// splitNs returns (userID, id) from a namespaced key. Anything written before this change is
// unprefixed and treated as the single-user namespace (userID="").
func splitNs(k string) (string, string) {
	if i := strings.Index(k, "|"); i >= 0 {
		return k[:i], k[i+1:]
	}
	return "", k
}

func newWebhookControl(mgr *webhook.Manager, settings store.SettingsRepo) api.Webhooks {
	c := &webhookControl{mgr: mgr, store: settings, configs: map[string]webhook.Endpoint{}}
	// Reload persisted endpoints across every user. EachUser yields '' for the single-user
	// namespace, which carries every endpoint a pre-hosted install ever wrote.
	_ = settings.EachUser(context.Background(), func(userID string) error {
		all, err := settings.AllForUser(context.Background(), userID)
		if err != nil {
			return nil // best-effort: a per-user load failure shouldn't tank startup
		}
		for _, s := range all {
			if !strings.HasPrefix(s.Scope, "webhooks.") {
				continue
			}
			if len(s.Data) == 0 || string(s.Data) == "null" {
				continue
			}
			var ep webhook.Endpoint
			if err := json.Unmarshal(s.Data, &ep); err != nil {
				continue
			}
			key := nsKey(userID, ep.ID)
			c.configs[key] = ep
			c.mgr.Register(key, ep, ep.Secret)
		}
		return nil
	})
	return c
}

func (c *webhookControl) List(ctx context.Context) []api.WebhookEndpointInfo {
	user := userctx.FromContext(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	list := make([]api.WebhookEndpointInfo, 0)
	prefix := user + "|"
	for k, ep := range c.configs {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		events := ep.Events
		if events == nil {
			events = []string{}
		}
		list = append(list, api.WebhookEndpointInfo{
			ID: ep.ID, Name: ep.Name, URL: ep.URL,
			Events: events, Active: ep.Active,
			Paused: c.mgr.IsPaused(k),
		})
	}
	return list
}

func (c *webhookControl) Create(ctx context.Context, name, url string, events []string, secret string) (api.WebhookEndpointInfo, error) {
	user := userctx.FromContext(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	s, _ := api.NewTokenSecret()
	id := s[:12]
	ep := webhook.Endpoint{ID: id, Name: name, URL: url, Events: events, Active: true, Secret: secret}
	key := nsKey(user, id)
	c.configs[key] = ep
	c.mgr.Register(key, ep, secret)
	c.persist(ctx, ep)
	return api.WebhookEndpointInfo{ID: id, Name: name, URL: url, Events: events, Active: true}, nil
}

func (c *webhookControl) Update(ctx context.Context, id, name, url string, events []string, active bool) (api.WebhookEndpointInfo, error) {
	user := userctx.FromContext(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	key := nsKey(user, id)
	ep, ok := c.configs[key]
	if !ok {
		return api.WebhookEndpointInfo{}, fmt.Errorf("webhook %q not found", id)
	}
	ep.Name, ep.URL, ep.Events, ep.Active = name, url, events, active
	c.configs[key] = ep
	c.mgr.Register(key, ep, ep.Secret)
	c.persist(ctx, ep)
	return api.WebhookEndpointInfo{ID: id, Name: name, URL: url, Events: events, Active: active}, nil
}

func (c *webhookControl) Delete(ctx context.Context, id string) error {
	user := userctx.FromContext(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	key := nsKey(user, id)
	if _, ok := c.configs[key]; !ok {
		return fmt.Errorf("webhook %q not found", id)
	}
	delete(c.configs, key)
	c.mgr.Deregister(key)
	_ = c.store.Put(ctx, store.Setting{Scope: "webhooks." + id, Data: []byte("null")})
	return nil
}

func (c *webhookControl) Log(ctx context.Context, id string) []api.WebhookAttempt {
	user := userctx.FromContext(ctx)
	c.mu.Lock()
	key := nsKey(user, id)
	_, ok := c.configs[key]
	c.mu.Unlock()
	if !ok {
		return nil
	}
	recs := c.mgr.DeliveryLog(key)
	out := make([]api.WebhookAttempt, len(recs))
	for i, r := range recs {
		out[i] = api.WebhookAttempt{AtMs: r.AtMs, StatusCode: r.StatusCode, Error: r.Error, LatencyMs: r.LatencyMs}
	}
	return out
}

func (c *webhookControl) Resume(ctx context.Context, id string) error {
	user := userctx.FromContext(ctx)
	c.mu.Lock()
	key := nsKey(user, id)
	_, ok := c.configs[key]
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("webhook %q not found", id)
	}
	c.mgr.Resume(key)
	return nil
}

func (c *webhookControl) EventCatalog() []string { return webhook.EventCatalog() }

func (c *webhookControl) persist(ctx context.Context, ep webhook.Endpoint) {
	data, _ := json.Marshal(ep)
	_ = c.store.Put(ctx, store.Setting{Scope: "webhooks." + ep.ID, Data: data})
}
