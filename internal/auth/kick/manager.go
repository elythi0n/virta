package kick

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/store"
)

const (
	refreshSkew = 60 * time.Second
	authTimeout = 5 * time.Minute // abandon a pending authorization after this long
)

// State is the status of an authorization session.
type State string

const (
	StatePending    State = "pending"
	StateAuthorized State = "authorized"
	StateError      State = "error"
	StateExpired    State = "expired"
)

// Session is the user-facing status of an authorization.
type Session struct {
	ID           string `json:"id"`
	AuthorizeURL string `json:"authorize_url"`
	State        State  `json:"state"`
	Login        string `json:"login,omitempty"`
	Error        string `json:"error,omitempty"`
}

// pending is the manager's internal state for an in-flight authorization.
type pending struct {
	session     Session
	verifier    string
	oauthState  string
	redirectURI string
	srv         *http.Server
	ln          net.Listener
}

// Manager runs Kick authorization-code/PKCE sessions over a loopback redirect and owns Kick
// token storage (vault + account row), mirroring the Twitch auth manager.
type Manager struct {
	client   *Client
	vault    secrets.Vault
	accounts store.AccountRepo
	gen      id.Generator
	clk      clock.Clock

	mu       sync.Mutex
	sessions map[string]*pending

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager builds a Kick token manager.
func NewManager(client *Client, vault secrets.Vault, accounts store.AccountRepo, gen id.Generator, clk clock.Clock) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		client: client, vault: vault, accounts: accounts, gen: gen, clk: clk,
		sessions: map[string]*pending{}, ctx: ctx, cancel: cancel,
	}
}

// StartAuth begins an authorization: it generates PKCE material, opens a loopback redirect
// server, and returns the URL the user should open. The redirect is handled in the background.
func (m *Manager) StartAuth(ctx context.Context) (Session, error) {
	verifier, err := id.RandomToken(48)
	if err != nil {
		return Session{}, err
	}
	stateVal, err := id.RandomToken(24)
	if err != nil {
		return Session{}, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Session{}, fmt.Errorf("kick: open loopback: %w", err)
	}
	redirectURI := "http://" + ln.Addr().String() + "/callback"
	authURL := m.client.AuthorizeURL(redirectURI, DefaultScopes, pkceChallenge(verifier), stateVal)

	p := &pending{
		session:     Session{ID: m.gen.New(), AuthorizeURL: authURL, State: StatePending},
		verifier:    verifier,
		oauthState:  stateVal,
		redirectURI: redirectURI,
		ln:          ln,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", m.callback(p))
	p.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	m.mu.Lock()
	m.sessions[p.session.ID] = p
	m.mu.Unlock()

	m.wg.Add(1)
	go func() { defer m.wg.Done(); _ = p.srv.Serve(ln) }()

	// Abandon the session (and free the port) if it isn't completed in time, or on shutdown.
	m.wg.Add(1)
	go m.expire(p)

	return p.session, nil
}

// callback handles the browser redirect: verify state, exchange the code, persist, then shut
// the loopback server down.
func (m *Manager) callback(p *pending) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Shut the loopback server in a goroutine: calling Shutdown synchronously from inside a
		// handler deadlocks (it waits for this very request to finish).
		defer func() { go m.shutdown(p) }()
		q := r.URL.Query()
		if q.Get("state") != p.oauthState {
			m.setState(p.session.ID, StateError, "", "state mismatch")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			m.setState(p.session.ID, StateError, "", e)
			_, _ = fmt.Fprint(w, "Authorization was declined. You can close this window.")
			return
		}
		tok, err := m.client.Exchange(r.Context(), q.Get("code"), p.verifier, p.redirectURI)
		if err != nil {
			m.setState(p.session.ID, StateError, "", err.Error())
			http.Error(w, "token exchange failed", http.StatusBadGateway)
			return
		}
		acc, err := m.persist(r.Context(), tok)
		if err != nil {
			m.setState(p.session.ID, StateError, "", err.Error())
			http.Error(w, "could not save account", http.StatusInternalServerError)
			return
		}
		m.setState(p.session.ID, StateAuthorized, acc.Login, "")
		_, _ = fmt.Fprint(w, "Authorized! You can close this window and return to Virta.")
	}
}

// expire shuts the session down after authTimeout (or on manager close), marking it expired if
// it never completed.
func (m *Manager) expire(p *pending) {
	defer m.wg.Done()
	t := time.NewTimer(authTimeout)
	defer t.Stop()
	select {
	case <-m.ctx.Done():
	case <-t.C:
		if s, ok := m.Status(p.session.ID); ok && s.State == StatePending {
			m.setState(p.session.ID, StateExpired, "", "")
		}
	}
	m.shutdown(p)
}

// shutdown stops a session's loopback server (idempotent).
func (m *Manager) shutdown(p *pending) {
	_ = p.srv.Shutdown(context.Background())
}

// Status returns a snapshot of a session.
func (m *Manager) Status(id string) (Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.sessions[id]
	if !ok {
		return Session{}, false
	}
	return p.session, true
}

func (m *Manager) setState(id string, st State, login, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.sessions[id]; ok {
		p.session.State, p.session.Login, p.session.Error = st, login, errMsg
	}
}

func (m *Manager) persist(ctx context.Context, tok Token) (store.Account, error) {
	ident, err := m.client.Identity(ctx, tok.Access)
	if err != nil {
		return store.Account{}, fmt.Errorf("kick: identity: %w", err)
	}
	ref := SecretRef(ident.UserID)
	if err := m.writeToken(ctx, ref, tok); err != nil {
		return store.Account{}, err
	}
	return m.accounts.Upsert(ctx, store.Account{
		Platform:    platform.Kick,
		PlatformUID: ident.UserID,
		Login:       ident.Login,
		DisplayName: ident.Login,
		SecretRef:   ref,
		Scopes:      tok.Scopes,
	})
}

// AccessToken returns a valid access token for ref, refreshing and rotating when near expiry.
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
		return "", fmt.Errorf("kick: refresh: %w", err)
	}
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
		return Token{}, fmt.Errorf("kick: decode stored token: %w", err)
	}
	return tok, nil
}

// Close shuts down all pending authorization servers.
func (m *Manager) Close() error {
	m.cancel()
	m.wg.Wait()
	return nil
}

// SecretRef is the vault key for a Kick account's tokens.
func SecretRef(userID string) string { return "platform:kick:" + userID }
