// Package keychain stores secrets in the operating system's native credential store —
// macOS Keychain, Windows Credential Manager, or the Linux Secret Service (GNOME Keyring /
// KWallet over D-Bus). This is the preferred place for credentials: the OS guards them and
// they never sit in a file we manage.
//
// On systems without a credential store (a headless Linux box with no Secret Service), the
// operations fail; callers detect that with Available and fall back to the file vault.
package keychain

import (
	"context"
	"errors"

	"github.com/zalando/go-keyring"

	"github.com/elythi0n/virta/internal/secrets"
)

// Vault is a secrets.Vault backed by the OS credential store. All entries are stored under
// a single service name, keyed by the secret key.
type Vault struct {
	service string
}

// New returns a keychain vault using the standard service name.
func New() *Vault { return &Vault{service: secrets.ServiceName} }

func (v *Vault) Get(_ context.Context, key string) (string, error) {
	val, err := keyring.Get(v.service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", secrets.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

func (v *Vault) Set(_ context.Context, key, value string) error {
	return keyring.Set(v.service, key, value)
}

func (v *Vault) Delete(_ context.Context, key string) error {
	err := keyring.Delete(v.service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil // deleting a missing key is a no-op
	}
	return err
}

func (v *Vault) Backend() secrets.Backend { return secrets.BackendKeychain }

// Available reports whether a usable OS credential store is present, by attempting a real
// round trip on a throwaway key. It returns false on any error (e.g. no Secret Service on a
// headless Linux machine), which tells callers to fall back to the file vault.
func Available() bool {
	const probe = "virta-availability-probe"
	if err := keyring.Set(secrets.ServiceName, probe, "1"); err != nil {
		return false
	}
	_, getErr := keyring.Get(secrets.ServiceName, probe)
	_ = keyring.Delete(secrets.ServiceName, probe)
	return getErr == nil
}

var _ secrets.Vault = (*Vault)(nil)
