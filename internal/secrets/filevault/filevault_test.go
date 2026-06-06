package filevault_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/secrets/filevault"
	"github.com/elythi0n/virta/internal/secrets/secretstest"
)

func newVault(t *testing.T) *filevault.Vault {
	t.Helper()
	v, err := filevault.New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return v
}

// The file vault must satisfy the same contract as the keychain and in-memory vaults.
func TestFileVault_Contract(t *testing.T) {
	secretstest.RunContract(t, func(t *testing.T) secrets.Vault {
		return newVault(t)
	})
}

func TestFileVault_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	v1, err := filevault.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := v1.Set(ctx, secrets.LLMKey("anthropic"), "sk-xyz"); err != nil {
		t.Fatal(err)
	}

	// A fresh vault over the same directory reads the same master key and data.
	v2, err := filevault.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := v2.Get(ctx, secrets.LLMKey("anthropic"))
	if err != nil || got != "sk-xyz" {
		t.Fatalf("after reopen Get = %q, %v; want sk-xyz", got, err)
	}
}

func TestFileVault_KeyFileIsOwnerOnly(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("POSIX permission check")
	}
	dir := t.TempDir()
	if _, err := filevault.New(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "vault.key"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("key file perm = %o, want 600", perm)
	}
}

// TestFileVault_GroupReadableKeyRejected: the master key is the vault's whole protection, so a
// key file readable by group or others (as a copy through a permission-dropping filesystem
// produces) must be refused loudly rather than trusted silently.
func TestFileVault_GroupReadableKeyRejected(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("POSIX permission check")
	}
	dir := t.TempDir()
	if _, err := filevault.New(dir); err != nil { // creates the key 0600
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "vault.key"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := filevault.New(dir); err == nil {
		t.Fatal("New accepted a group/world-readable key file, want refusal")
	}
}

func TestFileVault_TamperedDataFails(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	v, err := filevault.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	// Corrupt the encrypted file: GCM authentication must reject it on the next read.
	data := filepath.Join(dir, "vault.enc")
	b, _ := os.ReadFile(data)
	b[len(b)-1] ^= 0xff
	if err := os.WriteFile(data, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get(ctx, "k"); err == nil {
		t.Fatal("Get on tampered data returned nil error, want a decryption failure")
	}
}

func TestFileVault_WrongSizeKeyFileRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vault.key"), []byte("too-short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := filevault.New(dir); err == nil {
		t.Fatal("New with a wrong-size key file returned nil error")
	}
}

func TestFileVault_TruncatedDataFileRejected(t *testing.T) {
	dir := t.TempDir()
	v, err := filevault.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Shorter than a GCM nonce ⇒ unreadable.
	if err := os.WriteFile(filepath.Join(dir, "vault.enc"), []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get(context.Background(), "k"); err == nil {
		t.Fatal("Get on a truncated data file returned nil error")
	}
}

func TestFileVault_NewFailsWhenDirUncreatable(t *testing.T) {
	// Put a regular file where the vault wants a directory: MkdirAll must fail.
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := filevault.New(filepath.Join(blocker, "vault")); err == nil {
		t.Fatal("New under a non-directory path returned nil error")
	}
}

func TestFileVault_MutationsFailOnUnreadableData(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	v, err := filevault.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	// Corrupt the data so load() fails; Set and Delete both load first and must surface it.
	b, _ := os.ReadFile(filepath.Join(dir, "vault.enc"))
	b[len(b)-1] ^= 0xff
	_ = os.WriteFile(filepath.Join(dir, "vault.enc"), b, 0o600)
	if err := v.Set(ctx, "k2", "v2"); err == nil {
		t.Error("Set over corrupt data returned nil error")
	}
	if err := v.Delete(ctx, "k"); err == nil {
		t.Error("Delete over corrupt data returned nil error")
	}
}

func TestFileVault_SaveFailsOnReadonlyDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	dir := t.TempDir()
	ctx := context.Background()
	v, err := filevault.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Make the directory unwritable so the atomic write (temp file create) fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // restore so TempDir cleanup can remove it
	if err := v.Set(ctx, "k", "v"); err == nil {
		t.Error("Set into a read-only directory returned nil error")
	}
}

func TestFileVault_SaveFailsWhenDataPathIsADirectory(t *testing.T) {
	dir := t.TempDir()
	// Occupy the data path with a directory so the final atomic rename can't replace it.
	if err := os.Mkdir(filepath.Join(dir, "vault.enc"), 0o700); err != nil {
		t.Fatal(err)
	}
	v, err := filevault.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set(context.Background(), "k", "v"); err == nil {
		t.Error("Set returned nil error when the data path is a directory")
	}
}

func TestFileVault_WrongKeyFails(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	v, err := filevault.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	// Replace the master key: existing data can no longer be decrypted.
	if err := os.WriteFile(filepath.Join(dir, "vault.key"), make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	v2, err := filevault.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v2.Get(ctx, "k"); err == nil {
		t.Fatal("Get with wrong key returned nil error")
	}
}
