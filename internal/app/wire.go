// Package app wires the concrete implementations together. It is the single place allowed
// to import implementation packages (platform adapters, storage backends, secret vaults);
// every other package depends only on the interfaces. Keeping construction here means the
// rest of the codebase never hard-codes a choice of backend.
package app

import (
	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/secrets/filevault"
	"github.com/elythi0n/virta/internal/secrets/keychain"
)

// SelectVault chooses where credentials are stored: the OS credential store when one is
// available (the strong, preferred option), otherwise an encrypted file under fileVaultDir
// (the fallback for systems with no keychain). The chosen backend can be read from the
// returned vault's Backend method, which the UI surfaces so the user knows where their
// secrets live.
func SelectVault(fileVaultDir string) (secrets.Vault, error) {
	if keychain.Available() {
		return keychain.New(), nil
	}
	return filevault.New(fileVaultDir)
}
