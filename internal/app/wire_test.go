package app_test

import (
	"context"
	"testing"

	"github.com/elythi0n/virta/internal/app"
	"github.com/elythi0n/virta/internal/secrets"
)

// SelectVault must always return a working vault. On headless CI that's the file vault; on a
// machine with a credential store it's the keychain. Either way it round-trips.
func TestSelectVault_ReturnsWorkingVault(t *testing.T) {
	v, err := app.SelectVault(t.TempDir())
	if err != nil {
		t.Fatalf("SelectVault: %v", err)
	}
	switch v.Backend() {
	case secrets.BackendKeychain, secrets.BackendFileVault:
		// expected
	default:
		t.Fatalf("unexpected backend %q", v.Backend())
	}

	ctx := context.Background()
	key := secrets.APITokenKey("wire-test")
	t.Cleanup(func() { _ = v.Delete(ctx, key) })
	if err := v.Set(ctx, key, "secret"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := v.Get(ctx, key)
	if err != nil || got != "secret" {
		t.Fatalf("Get = %q, %v; want secret", got, err)
	}
}
