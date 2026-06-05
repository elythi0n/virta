// Package filevault stores secrets in an encrypted local file. It is the fallback for
// systems with no OS credential store (e.g. a headless Linux box without a Secret Service).
//
// Secrets are kept in a single AES-256-GCM-encrypted blob, so neither the secret values nor
// even the key names are readable on disk. The master key is a random 32-byte file written
// with owner-only permissions; this is weaker than an OS keychain (anyone who can read the
// key file and the data file can decrypt), which is why it is only used when no keychain is
// available. The vault and key files are created on first use.
package filevault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/elythi0n/virta/internal/secrets"
)

const (
	keyFileName  = "vault.key"
	dataFileName = "vault.enc"
	keySize      = 32 // AES-256
)

// Vault is a secrets.Vault backed by an encrypted file. Safe for concurrent use.
type Vault struct {
	dir      string
	keyPath  string
	dataPath string

	mu   sync.Mutex
	aead cipher.AEAD
}

// New opens (creating if needed) an encrypted vault in dir. It loads or generates the master
// key and prepares the cipher.
func New(dir string) (*Vault, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("filevault: create dir: %w", err)
	}
	v := &Vault{
		dir:      dir,
		keyPath:  filepath.Join(dir, keyFileName),
		dataPath: filepath.Join(dir, dataFileName),
	}
	key, err := v.loadOrCreateKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("filevault: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("filevault: gcm: %w", err)
	}
	v.aead = aead
	return v, nil
}

func (v *Vault) loadOrCreateKey() ([]byte, error) {
	key, err := os.ReadFile(v.keyPath)
	switch {
	case err == nil:
		if len(key) != keySize {
			return nil, fmt.Errorf("filevault: key file has wrong size %d", len(key))
		}
		return key, nil
	case errors.Is(err, os.ErrNotExist):
		key = make([]byte, keySize)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("filevault: generate key: %w", err)
		}
		if err := os.WriteFile(v.keyPath, key, 0o600); err != nil {
			return nil, fmt.Errorf("filevault: write key: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("filevault: read key: %w", err)
	}
}

func (v *Vault) Get(_ context.Context, key string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	m, err := v.load()
	if err != nil {
		return "", err
	}
	val, ok := m[key]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return val, nil
}

func (v *Vault) Set(_ context.Context, key, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	m, err := v.load()
	if err != nil {
		return err
	}
	m[key] = value
	return v.save(m)
}

func (v *Vault) Delete(_ context.Context, key string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	m, err := v.load()
	if err != nil {
		return err
	}
	if _, ok := m[key]; !ok {
		return nil // idempotent
	}
	delete(m, key)
	return v.save(m)
}

func (v *Vault) Backend() secrets.Backend { return secrets.BackendFileVault }

// load reads and decrypts the secret map. A missing file is an empty map.
func (v *Vault) load() (map[string]string, error) {
	raw, err := os.ReadFile(v.dataPath)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("filevault: read data: %w", err)
	}
	ns := v.aead.NonceSize()
	if len(raw) < ns {
		return nil, errors.New("filevault: data file truncated")
	}
	nonce, ciphertext := raw[:ns], raw[ns:]
	plain, err := v.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("filevault: decrypt: %w", err)
	}
	m := map[string]string{}
	if err := json.Unmarshal(plain, &m); err != nil {
		return nil, fmt.Errorf("filevault: parse: %w", err)
	}
	return m, nil
}

// save encrypts and atomically writes the secret map (write to a temp file, then rename).
func (v *Vault) save(m map[string]string) error {
	plain, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("filevault: marshal: %w", err)
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("filevault: nonce: %w", err)
	}
	sealed := v.aead.Seal(nonce, nonce, plain, nil)

	tmp, err := os.CreateTemp(v.dir, dataFileName+".*")
	if err != nil {
		return fmt.Errorf("filevault: temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op if the rename already moved it
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(sealed); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, v.dataPath); err != nil {
		return fmt.Errorf("filevault: rename: %w", err)
	}
	return nil
}

var _ secrets.Vault = (*Vault)(nil)
