package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/store"
)

// DefaultScopes are the chat scopes the app requests: read chat, send chat, read the user's
// emotes.
var DefaultScopes = []string{"user:read:chat", "user:write:chat", "user:read:emotes"}

// refreshSkew refreshes a token this long before it actually expires, so a send never races
// expiry.
const refreshSkew = 60 * time.Second

// State is the status of a device-authorization session.
type State string

const (
	StatePending    State = "pending"
	StateAuthorized State = "authorized"
	StateDenied     State = "denied"
	StateExpired    State = "expired"
	StateError      State = "error"
)

// Session is a device-flow session's user-facing status.
type Session struct {
	ID              string `json:"id"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	State           State  `json:"state"`
	Login           string `json:"login,omitempty"`
	Error           string `json:"error,omitempty"`
}

// Manager runs device-authorization sessions to completion and owns Twitch token storage:
// tokens live in the vault keyed by secret_ref, with the account row in the store. It refreshes
// tokens on demand with single-use rotation.
type Manager struct {
	client   *Client
	vault    secrets.Vault
	accounts store.AccountRepo
	gen      id.Generator
	clk      clock.Clock
	unit     time.Duration // poll-interval unit (seconds in prod; tests shrink it)

	mu       sync.Mutex
	sessions map[string]*Session

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager builds a token manager.
func NewManager(client *Client, vault secrets.Vault, accounts store.AccountRepo, gen id.Generator, clk clock.Clock) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		client:   client,
		vault:    vault,
		accounts: accounts,
		gen:      gen,
		clk:      clk,
		unit:     time.Second,
		sessions: map[string]*Session{},
		ctx:      ctx,
		cancel:   cancel,
	}
}

// StartDevice begins a device-authorization session and polls it to completion in the
// background. It returns the session (with the code to display) immediately.
func (m *Manager) StartDevice(ctx context.Context) (Session, error) {
	da, err := m.client.StartDevice(ctx, DefaultScopes)
	if err != nil {
		return Session{}, err
	}
	s := &Session{
		ID:              m.gen.New(),
		UserCode:        da.UserCode,
		VerificationURI: da.VerificationURI,
		ExpiresIn:       da.ExpiresIn,
		Interval:        da.Interval,
		State:           StatePending,
	}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()

	m.wg.Add(1)
	go m.poll(s.ID, da)
	return *s, nil
}

// Status returns a snapshot of a session, or ok=false if unknown.
func (m *Manager) Status(id string) (Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return Session{}, false
	}
	return *s, true
}

func (m *Manager) setState(id string, state State, login, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.State, s.Login, s.Error = state, login, errMsg
	}
}

// poll drives one device session: poll at the interval (backing off on slow_down) until the
// token arrives, the code expires, the user declines, or the adapter shuts down.
func (m *Manager) poll(sessionID string, da DeviceAuth) {
	defer m.wg.Done()
	interval := time.Duration(da.Interval) * m.unit
	deadline := m.clk.Now().Add(time.Duration(da.ExpiresIn) * time.Second)

	for {
		t := time.NewTimer(interval)
		select {
		case <-m.ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		if m.clk.Now().After(deadline) {
			m.setState(sessionID, StateExpired, "", "")
			return
		}
		tok, err := m.client.PollToken(m.ctx, da.DeviceCode)
		switch {
		case err == nil:
			acc, perr := m.persist(m.ctx, tok)
			if perr != nil {
				m.setState(sessionID, StateError, "", perr.Error())
				return
			}
			m.setState(sessionID, StateAuthorized, acc.Login, "")
			return
		case isPending(err):
			continue
		case isSlowDown(err):
			interval += m.unit
		case isExpired(err):
			m.setState(sessionID, StateExpired, "", "")
			return
		case isDenied(err):
			m.setState(sessionID, StateDenied, "", "")
			return
		default:
			m.setState(sessionID, StateError, "", err.Error())
			return
		}
	}
}

// persist validates the token, stores it in the vault, and upserts the account row.
func (m *Manager) persist(ctx context.Context, tok Token) (store.Account, error) {
	ident, err := m.client.Validate(ctx, tok.Access)
	if err != nil {
		return store.Account{}, fmt.Errorf("twitch: validate new token: %w", err)
	}
	ref := SecretRef(ident.UserID)
	if err := m.writeToken(ctx, ref, tok); err != nil {
		return store.Account{}, err
	}
	acc := store.Account{
		Platform:    platform.Twitch,
		PlatformUID: ident.UserID,
		Login:       ident.Login,
		DisplayName: ident.Login,
		SecretRef:   ref,
		Scopes:      tok.Scopes,
	}
	return m.accounts.Upsert(ctx, acc)
}

// AccessToken returns a valid access token for the account at ref, refreshing (and rotating the
// stored refresh token) when it's at or near expiry.
func (m *Manager) AccessToken(ctx context.Context, ref string) (string, error) {
	tok, err := m.readToken(ctx, ref)
	if err != nil {
		return "", err
	}
	if m.clk.Now().Before(tok.ExpiresAt.Add(-refreshSkew)) {
		return tok.Access, nil
	}
	fresh, err := m.client.Refresh(ctx, tok.Refresh)
	if err != nil {
		return "", fmt.Errorf("twitch: refresh: %w", err)
	}
	// Atomic rotation: persist the new (single-use) token set before relying on it, so a crash
	// can't strand us with a refresh token Twitch has already invalidated.
	if err := m.writeToken(ctx, ref, fresh); err != nil {
		return "", err
	}
	return fresh.Access, nil
}

func (m *Manager) writeToken(ctx context.Context, ref string, tok Token) error {
	b, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	return m.vault.Set(ctx, ref, string(b))
}

func (m *Manager) readToken(ctx context.Context, ref string) (Token, error) {
	raw, err := m.vault.Get(ctx, ref)
	if err != nil {
		return Token{}, err
	}
	var tok Token
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		return Token{}, fmt.Errorf("twitch: decode stored token: %w", err)
	}
	return tok, nil
}

// Close stops in-flight device polls.
func (m *Manager) Close() error {
	m.cancel()
	m.wg.Wait()
	return nil
}

// SecretRef is the vault key for a Twitch account's tokens.
func SecretRef(userID string) string { return "platform:twitch:" + userID }

func isPending(err error) bool  { return err == ErrAuthorizationPending }
func isSlowDown(err error) bool { return err == ErrSlowDown }
func isExpired(err error) bool  { return err == ErrExpired }
func isDenied(err error) bool   { return err == ErrAccessDenied }
