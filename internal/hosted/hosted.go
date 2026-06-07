// Package hosted implements the multi-tenant user account system for the hosted deployment.
// It is a self-contained layer on top of the existing single-user daemon: in local mode
// (the default), none of this code runs and there are no users — the daemon is just
// auth'd by the static bearer token. In hosted mode (VIRTA_HOSTED=1), users register and
// log in via email+password, sessions are JWT-like short tokens stored in the DB, and every
// store operation is scoped by the resolved user_id from the request.
//
// Security choices:
//   - Passwords hashed with bcrypt (cost 12) — standard, widely audited, easy to tune.
//   - Sessions are random 32-byte tokens stored as sha256(token) in the DB (same pattern as
//     API tokens); the plaintext is sent to the client in an HttpOnly + Secure cookie AND
//     also returned as a JSON field for programmatic clients.
//   - Sessions expire after SessionTTL (default 30 days); a background sweeper deletes stale rows.
//   - Per-user rate limiting on login to prevent brute-force (5 attempts / 10 min per IP).
package hosted

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/elythi0n/virta/internal/id"
)

const (
	bcryptCost = 12
	SessionTTL = 30 * 24 * time.Hour
	cookieName = "virta_session"
)

// ErrNotFound is returned when a user or session is not found.
var ErrNotFound = errors.New("hosted: not found")

// dummyHash is a pre-computed bcrypt hash used to consume time when a login email is not
// found, so the response time is indistinguishable from a real credential mismatch.
var dummyHash = []byte("$2a$12$dummy.hash.to.prevent.timing.attacks.injectedXXXXXXXXXXX")

// ErrUnauthorized is returned when credentials are invalid.
var ErrUnauthorized = errors.New("hosted: invalid credentials")

// ErrEmailTaken is returned when a registration email is already in use.
var ErrEmailTaken = errors.New("hosted: email already registered")

// User is a registered account.
type User struct {
	ID          string
	Email       string
	DisplayName string
	CreatedAt   time.Time
}

// Store is the persistence port for the hosted user/session layer. Implemented by the SQL
// backends (a thin layer over the existing sqlcommon.Core).
type Store interface {
	// CreateUser inserts a new user, returning ErrEmailTaken on duplicate email.
	CreateUser(ctx context.Context, email, displayName, passwordHash string) (User, error)
	// UserByEmail retrieves a user by email.
	UserByEmail(ctx context.Context, email string) (User, error)
	// UserByID retrieves a user by id.
	UserByID(ctx context.Context, id string) (User, error)
	// PasswordHash returns the stored bcrypt hash for a user id.
	PasswordHash(ctx context.Context, userID string) (string, error)
	// CreateSession stores a new session token (hash only), returning the plaintext.
	CreateSession(ctx context.Context, userID string, expiresAt time.Time) (plaintext string, err error)
	// SessionUser resolves a session token to a User (and renews it if it expires soon).
	SessionUser(ctx context.Context, tokenHash string) (User, error)
	// DeleteSession removes a specific session.
	DeleteSession(ctx context.Context, tokenHash string) error
	// SweepSessions deletes expired sessions.
	SweepSessions(ctx context.Context) (int, error)
}

// Manager handles registration, login, logout, and session resolution.
type Manager struct {
	store Store
	gen   id.Generator
	// in-memory login rate limiter: IP → [attempt_count, window_start].
	mu      sync.Mutex
	limiter map[string][2]int64 // ip → {count, window_unix_sec}
}

// NewManager creates a hosted manager over the given store.
func NewManager(s Store, gen id.Generator) *Manager {
	return &Manager{store: s, gen: gen, limiter: map[string][2]int64{}}
}

// Register creates a new user account.
func (m *Manager) Register(ctx context.Context, email, displayName, password string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return User{}, fmt.Errorf("hosted: invalid email")
	}
	if len(password) < 8 {
		return User{}, fmt.Errorf("hosted: password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return User{}, err
	}
	return m.store.CreateUser(ctx, email, displayName, string(hash))
}

// Login verifies credentials, creates a session, and returns the session token.
func (m *Manager) Login(ctx context.Context, ip, email, password string) (User, string, error) {
	if !m.rateOK(ip) {
		return User{}, "", fmt.Errorf("hosted: too many login attempts — try again in 10 minutes")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	user, err := m.store.UserByEmail(ctx, email)
	if err != nil {
		// Run a dummy bcrypt compare so the response time matches a real credential check,
		// preventing email enumeration via timing.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		m.recordAttempt(ip)
		return User{}, "", ErrUnauthorized
	}
	hash, err := m.store.PasswordHash(ctx, user.ID)
	if err != nil {
		return User{}, "", ErrUnauthorized
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		m.recordAttempt(ip)
		return User{}, "", ErrUnauthorized
	}
	token, err := m.store.CreateSession(ctx, user.ID, time.Now().Add(SessionTTL))
	if err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

// Resolve validates a session token from a request (cookie or bearer) and returns the user.
func (m *Manager) Resolve(ctx context.Context, r *http.Request) (User, error) {
	tok := extractToken(r)
	if tok == "" {
		return User{}, ErrUnauthorized
	}
	hash := hashToken(tok)
	return m.store.SessionUser(ctx, hash)
}

// Logout invalidates a specific session.
func (m *Manager) Logout(ctx context.Context, r *http.Request) error {
	tok := extractToken(r)
	if tok == "" {
		return nil
	}
	return m.store.DeleteSession(ctx, hashToken(tok))
}

// SetSessionCookie writes the session token as an HttpOnly Secure cookie.
func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	c := &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, c)
}

// ClearSessionCookie deletes the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1})
}

func extractToken(r *http.Request) string {
	// Cookie takes priority (browser sessions).
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		return c.Value
	}
	// Bearer header for API clients.
	if h := r.Header.Get("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	return r.URL.Query().Get("session")
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

// newSessionSecret generates a 32-byte random token with the "vs_" prefix.
func newSessionSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "vs_" + hex.EncodeToString(b), nil
}

// VerifyToken performs constant-time comparison of two session token hashes.
func VerifyToken(presented, stored string) bool {
	return subtle.ConstantTimeCompare([]byte(presented), []byte(stored)) == 1
}

// ---- Rate limiter ----

func (m *Manager) rateOK(ip string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().Unix()
	window := int64(600) // 10 minutes
	entry := m.limiter[ip]
	if now-entry[1] > window {
		return true // window elapsed — fresh
	}
	return entry[0] < 5
}

func (m *Manager) recordAttempt(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().Unix()
	window := int64(600)
	entry := m.limiter[ip]
	if now-entry[1] > window {
		m.limiter[ip] = [2]int64{1, now}
		return
	}
	m.limiter[ip] = [2]int64{entry[0] + 1, entry[1]}
}

// Expose for wiring.
var HashToken = hashToken
var NewSessionSecret = newSessionSecret
