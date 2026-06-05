package keychain_test

import (
	"testing"

	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/secrets/keychain"
	"github.com/elythi0n/virta/internal/secrets/secretstest"
)

// When an OS credential store is present (developer machines, packaged installs), the
// keychain vault must satisfy the same contract as every other backend. Headless CI has no
// credential store, so the suite skips there; that path is verified manually per OS instead.
func TestKeychain_Contract(t *testing.T) {
	if !keychain.Available() {
		t.Skip("no OS credential store available (headless environment)")
	}
	secretstest.RunContract(t, func(t *testing.T) secrets.Vault {
		return keychain.New()
	})
}

func TestKeychain_Backend(t *testing.T) {
	if keychain.New().Backend() != secrets.BackendKeychain {
		t.Error("unexpected backend")
	}
}
