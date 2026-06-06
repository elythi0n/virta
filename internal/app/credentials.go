package app

import (
	"context"
	"sync/atomic"

	"github.com/elythi0n/virta/internal/secrets"
)

// Vault keys for the user-configurable OAuth app credentials.
const (
	credTwitchID   = "oauth.twitch.client_id"
	credKickID     = "oauth.kick.client_id"
	credKickSecret = "oauth.kick.client_secret"
)

// credentials holds the per-platform OAuth app credentials the auth clients read on each call.
// Values live in the secret vault (keychain when available) so they're set once via the UI and
// persist; the in-memory atomics keep reads cheap and lock-free on the auth paths.
type credentials struct {
	vault      secrets.Vault
	twitchID   atomic.Pointer[string]
	kickID     atomic.Pointer[string]
	kickSecret atomic.Pointer[string]
}

func newCredentials(vault secrets.Vault) *credentials { return &credentials{vault: vault} }

func loadStr(p *atomic.Pointer[string]) string {
	if v := p.Load(); v != nil {
		return *v
	}
	return ""
}

func ptr(s string) *string { return &s }

// TwitchID / KickID / KickSecret are the provider closures passed to the auth clients.
func (c *credentials) TwitchID() string   { return loadStr(&c.twitchID) }
func (c *credentials) KickID() string     { return loadStr(&c.kickID) }
func (c *credentials) KickSecret() string { return loadStr(&c.kickSecret) }

// seed populates the in-memory values from the vault, falling back to the given env defaults when
// the vault has none — so VIRTA_*_CLIENT_ID still works for dev/deploy and the UI can override it.
func (c *credentials) seed(ctx context.Context, envTwitchID, envKickID, envKickSecret string) {
	c.twitchID.Store(ptr(c.orVault(ctx, credTwitchID, envTwitchID)))
	c.kickID.Store(ptr(c.orVault(ctx, credKickID, envKickID)))
	c.kickSecret.Store(ptr(c.orVault(ctx, credKickSecret, envKickSecret)))
}

func (c *credentials) orVault(ctx context.Context, key, fallback string) string {
	if got, err := c.vault.Get(ctx, key); err == nil && got != "" {
		return got
	}
	return fallback
}

// SetTwitch persists and applies the Twitch client id.
func (c *credentials) SetTwitch(ctx context.Context, id string) error {
	c.twitchID.Store(ptr(id))
	return c.vault.Set(ctx, credTwitchID, id)
}

// SetKick persists and applies the Kick client id and optional secret.
func (c *credentials) SetKick(ctx context.Context, id, secret string) error {
	c.kickID.Store(ptr(id))
	c.kickSecret.Store(ptr(secret))
	if err := c.vault.Set(ctx, credKickID, id); err != nil {
		return err
	}
	return c.vault.Set(ctx, credKickSecret, secret)
}
