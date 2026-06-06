package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
)

// Scope is a capability a third-party API token may be granted (ADR-017 / docs-15 §1). The
// frontends' own root token implicitly holds every scope; minted tokens carry an explicit subset.
type Scope string

const (
	ScopeRead     Scope = "read"     // feed stream, history, stats, capabilities, profiles (read)
	ScopeSend     Scope = "send"     // send messages via connected accounts
	ScopeModerate Scope = "moderate" // moderation actions where the account is permitted
	ScopeControl  Scope = "control"  // profile switching, channel join/leave, settings read
	ScopeAdmin    Scope = "admin"    // settings write, token management, storage ops
)

// AllScopes is the full grant (what the frontends' root token holds).
var AllScopes = []Scope{ScopeRead, ScopeSend, ScopeModerate, ScopeControl, ScopeAdmin}

// ValidScope reports whether s names a real scope, so the mint endpoint rejects typos.
func ValidScope(s string) bool {
	for _, v := range AllScopes {
		if Scope(s) == v {
			return true
		}
	}
	return false
}

// TokenInfo is a minted token's metadata — never the secret — for the Integrations list.
type TokenInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Scopes       []string `json:"scopes"`
	CreatedAtMs  int64    `json:"created_at_ms"`
	LastUsedAtMs int64    `json:"last_used_at_ms,omitempty"`
}

// MintedToken is returned exactly once, at creation — the only time the secret is shown.
type MintedToken struct {
	TokenInfo
	Token string `json:"token"`
}

// Tokens is the scoped-token control surface (Integrations settings). The store keeps only a hash
// of each secret; the plaintext exists only in the MintedToken returned at creation.
type Tokens interface {
	List() []TokenInfo
	Mint(name string, scopes []Scope) (MintedToken, error)
	Revoke(id string) error
	// Verify resolves a presented secret to its granted scopes (ok=false if unknown) and records
	// last-used. Called on the hot auth path.
	Verify(token string) ([]Scope, bool)
}

// SetTokens installs the scoped-token controller. Until called, only the root token authenticates.
func (s *Server) SetTokens(t Tokens) { s.tokens = t }

// presentedToken pulls the bearer token from the Authorization header or the `token` query param
// (a browser can't set headers on a WebSocket handshake).
func presentedToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	return r.URL.Query().Get("token")
}

func hasScope(granted []Scope, need Scope) bool {
	for _, g := range granted {
		if g == ScopeAdmin || g == need { // admin is a superuser
			return true
		}
	}
	return false
}

// scoped wraps a handler so it admits the root token (all scopes) or a minted token that holds the
// required scope; an empty scope means "any valid token". Insufficient scope is 403, not 401, so a
// caller can tell "wrong token" from "token lacks this capability".
func (s *Server) scoped(scope Scope, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := presentedToken(r)
		if tok == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) == 1 {
			next.ServeHTTP(w, r) // root token: every scope
			return
		}
		if s.tokens != nil {
			if granted, ok := s.tokens.Verify(tok); ok {
				if scope == "" || hasScope(granted, scope) {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "insufficient scope: requires "+string(scope), http.StatusForbidden)
				return
			}
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// HashToken is the stored form of a token secret (sha256 hex). Exported so the wiring-layer store
// hashes consistently with Verify.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewTokenSecret mints a random token string with a recognizable prefix.
func NewTokenSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "vk_" + hex.EncodeToString(b), nil
}

// ---- handlers ----

func (s *Server) handleListTokens(w http.ResponseWriter, _ *http.Request) {
	if s.tokens == nil {
		http.Error(w, "token management unavailable", http.StatusServiceUnavailable)
		return
	}
	list := s.tokens.List()
	if list == nil {
		list = []TokenInfo{}
	}
	writeJSON(w, map[string]any{"tokens": list})
}

type mintTokenRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

func (s *Server) handleMintToken(w http.ResponseWriter, r *http.Request) {
	if s.tokens == nil {
		http.Error(w, "token management unavailable", http.StatusServiceUnavailable)
		return
	}
	var req mintTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || len(req.Scopes) == 0 {
		http.Error(w, "expected JSON body with name and scopes", http.StatusBadRequest)
		return
	}
	scopes := make([]Scope, 0, len(req.Scopes))
	for _, sc := range req.Scopes {
		if !ValidScope(sc) {
			http.Error(w, fmt.Sprintf("unknown scope %q", sc), http.StatusBadRequest)
			return
		}
		scopes = append(scopes, Scope(sc))
	}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i] < scopes[j] })
	minted, err := s.tokens.Mint(req.Name, scopes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, minted)
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	if s.tokens == nil {
		http.Error(w, "token management unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing token id", http.StatusBadRequest)
		return
	}
	if err := s.tokens.Revoke(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
