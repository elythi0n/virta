package secrets

import (
	"context"
	"sync"
)

// Memory is an in-memory Vault for tests. It is a real, behaving Vault (it runs the same
// conformance suite as the keychain and age-vault backends), just not persistent.
type Memory struct {
	mu sync.Mutex
	kv map[string]string
}

// NewMemory creates an empty in-memory vault.
func NewMemory() *Memory { return &Memory{kv: map[string]string{}} }

func (v *Memory) Get(_ context.Context, key string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	val, ok := v.kv[key]
	if !ok {
		return "", ErrNotFound
	}
	return val, nil
}

func (v *Memory) Set(_ context.Context, key, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.kv[key] = value
	return nil
}

func (v *Memory) Delete(_ context.Context, key string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.kv, key)
	return nil
}

func (v *Memory) Backend() Backend { return BackendMemory }

var _ Vault = (*Memory)(nil)
