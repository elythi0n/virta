package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// MemTokens is an in-memory scoped token store. It hashes each secret on Mint and never stores the
// plaintext, so a process dump or log capture can't expose active tokens. Metadata (name, scopes,
// last-used) persists across the struct lifetime; tokens survive until Revoke is called.
//
// For production persistence, a caller can wrap this with serialisation to the SettingsRepo.
// The wiring layer constructs this directly for simplicity: token metadata is small (each row is a
// handful of bytes) and the set is tiny in practice.
type MemTokens struct {
	mu   sync.Mutex
	rows []*tokenRow
}

type tokenRow struct {
	info       TokenInfo
	hashSecret string // sha256(plaintext)
}

// NewMemTokens builds an empty in-memory store.
func NewMemTokens() *MemTokens { return &MemTokens{} }

// LoadFromJSON restores a previously serialised token list (no secrets — only metadata + hash). A
// token whose hash is lost can't be used again; the user must revoke and re-mint.
func (m *MemTokens) LoadFromJSON(data []byte) error {
	var rows []tokenRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	m.mu.Lock()
	for i := range rows {
		m.rows = append(m.rows, &rows[i])
	}
	m.mu.Unlock()
	return nil
}

// DumpJSON serialises all token metadata + hashes (no plaintext secrets) for persistence.
func (m *MemTokens) DumpJSON() ([]byte, error) {
	m.mu.Lock()
	rows := make([]tokenRow, len(m.rows))
	for i, r := range m.rows {
		rows[i] = *r
	}
	m.mu.Unlock()
	return json.Marshal(rows)
}

func (m *MemTokens) List() []TokenInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]TokenInfo, len(m.rows))
	for i, r := range m.rows {
		out[i] = r.info
	}
	return out
}

func (m *MemTokens) Mint(name string, scopes []Scope) (MintedToken, error) {
	secret, err := NewTokenSecret()
	if err != nil {
		return MintedToken{}, err
	}
	strs := make([]string, len(scopes))
	for i, s := range scopes {
		strs[i] = string(s)
	}
	info := TokenInfo{
		ID:          newID(),
		Name:        name,
		Scopes:      strs,
		CreatedAtMs: time.Now().UnixMilli(),
	}
	row := &tokenRow{info: info, hashSecret: HashToken(secret)}
	m.mu.Lock()
	m.rows = append(m.rows, row)
	m.mu.Unlock()
	return MintedToken{TokenInfo: info, Token: secret}, nil
}

func (m *MemTokens) Revoke(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, r := range m.rows {
		if r.info.ID == id {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			return nil
		}
	}
	return errors.New("token not found")
}

func (m *MemTokens) Verify(token string) ([]Scope, bool) {
	hash := HashToken(token)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if subtle.ConstantTimeCompare([]byte(hash), []byte(r.hashSecret)) == 1 {
			r.info.LastUsedAtMs = time.Now().UnixMilli()
			scopes := make([]Scope, len(r.info.Scopes))
			for i, s := range r.info.Scopes {
				scopes[i] = Scope(s)
			}
			return scopes, true
		}
	}
	return nil, false
}

// newID generates a short hex token id.
func newID() string {
	s, _ := NewTokenSecret()
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
