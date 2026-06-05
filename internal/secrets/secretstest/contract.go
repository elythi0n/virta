// Package secretstest is the reusable conformance suite for secrets.Vault. The in-memory
// fake runs it; the keychain and age-vault backends run the exact same suite so the
// fallback rung behaves identically to the native one.
package secretstest

import (
	"context"
	"errors"
	"testing"

	"github.com/elythi0n/virta/internal/secrets"
)

// RunContract verifies the Vault contract. newVault must return a fresh, empty vault.
func RunContract(t *testing.T, newVault func(t *testing.T) secrets.Vault) {
	t.Helper()
	ctx := context.Background()

	t.Run("Get missing returns ErrNotFound", func(t *testing.T) {
		v := newVault(t)
		if _, err := v.Get(ctx, secrets.LLMKey("anthropic")); !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("Get missing = %v, want ErrNotFound", err)
		}
	})

	t.Run("Set then Get round-trips", func(t *testing.T) {
		v := newVault(t)
		key := secrets.PlatformKey("twitch", "acct01")
		if err := v.Set(ctx, key, "sk-secret-token"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, err := v.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got != "sk-secret-token" {
			t.Errorf("Get = %q, want %q", got, "sk-secret-token")
		}
	})

	t.Run("Set overwrites", func(t *testing.T) {
		v := newVault(t)
		key := secrets.WebhookKey("wh1")
		_ = v.Set(ctx, key, "first")
		if err := v.Set(ctx, key, "second"); err != nil {
			t.Fatalf("Set overwrite: %v", err)
		}
		got, _ := v.Get(ctx, key)
		if got != "second" {
			t.Errorf("after overwrite = %q, want %q", got, "second")
		}
	})

	t.Run("Delete removes; Delete missing is idempotent", func(t *testing.T) {
		v := newVault(t)
		key := secrets.APITokenKey("tok1")
		_ = v.Set(ctx, key, "x")
		if err := v.Delete(ctx, key); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := v.Get(ctx, key); !errors.Is(err, secrets.ErrNotFound) {
			t.Errorf("Get after Delete = %v, want ErrNotFound", err)
		}
		if err := v.Delete(ctx, key); err != nil {
			t.Errorf("Delete missing = %v, want nil (idempotent)", err)
		}
	})

	t.Run("Backend is reported", func(t *testing.T) {
		v := newVault(t)
		if v.Backend() == "" {
			t.Error("Backend() returned empty")
		}
	})
}
