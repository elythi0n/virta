package hosted

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// stubStore is an in-memory implementation of Store for tests.
type stubStore struct {
	users    map[string]*User     // email → user
	hashes   map[string]string    // userID → hash
	sessions map[string]*session  // tokenHash → session
}

type session struct {
	userID    string
	expiresAt time.Time
}

func newStub() *stubStore {
	return &stubStore{
		users:    map[string]*User{},
		hashes:   map[string]string{},
		sessions: map[string]*session{},
	}
}

func (s *stubStore) Conn() *sql.DB   { return nil }
func (s *stubStore) Rebind(q string) string { return q }

func (s *stubStore) CreateUser(ctx context.Context, email, displayName, passwordHash string) (User, error) {
	if _, ok := s.users[email]; ok {
		return User{}, ErrEmailTaken
	}
	u := User{ID: "u-" + email, Email: email, DisplayName: displayName, CreatedAt: time.Now()}
	s.users[email] = &u
	s.hashes[u.ID] = passwordHash
	return u, nil
}
func (s *stubStore) UserByEmail(_ context.Context, email string) (User, error) {
	u, ok := s.users[email]
	if !ok {
		return User{}, ErrNotFound
	}
	return *u, nil
}
func (s *stubStore) UserByID(_ context.Context, id string) (User, error) {
	for _, u := range s.users {
		if u.ID == id {
			return *u, nil
		}
	}
	return User{}, ErrNotFound
}
func (s *stubStore) PasswordHash(_ context.Context, userID string) (string, error) {
	h, ok := s.hashes[userID]
	if !ok {
		return "", ErrNotFound
	}
	return h, nil
}
func (s *stubStore) CreateSession(_ context.Context, userID string, expiresAt time.Time) (string, error) {
	tok, err := NewSessionSecret()
	if err != nil {
		return "", err
	}
	s.sessions[HashToken(tok)] = &session{userID: userID, expiresAt: expiresAt}
	return tok, nil
}
func (s *stubStore) SessionUser(_ context.Context, tokenHash string) (User, error) {
	sess, ok := s.sessions[tokenHash]
	if !ok || time.Now().After(sess.expiresAt) {
		return User{}, ErrUnauthorized
	}
	for _, u := range s.users {
		if u.ID == sess.userID {
			return *u, nil
		}
	}
	return User{}, ErrNotFound
}
func (s *stubStore) DeleteSession(_ context.Context, tokenHash string) error {
	delete(s.sessions, tokenHash)
	return nil
}
func (s *stubStore) SweepSessions(_ context.Context) (int, error) {
	n := 0
	for k, sess := range s.sessions {
		if time.Now().After(sess.expiresAt) {
			delete(s.sessions, k)
			n++
		}
	}
	return n, nil
}

// idGen is a simple counter-based id generator for tests.
type idGen struct{ n int }
func (g *idGen) New() string { g.n++; return "id" + string(rune('0'+g.n)) }

func TestManager_RegisterAndLogin(t *testing.T) {
	m := NewManager(newStub(), &idGen{})
	ctx := context.Background()
	user, err := m.Register(ctx, "alice@example.com", "Alice", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("email = %q", user.Email)
	}
	// Duplicate registration should fail.
	if _, err := m.Register(ctx, "alice@example.com", "Alice", "other"); err == nil {
		t.Error("expected error for duplicate email")
	}
	// Login with correct password.
	_, tok, err := m.Login(ctx, "127.0.0.1", "alice@example.com", "password123")
	if err != nil || tok == "" {
		t.Fatalf("Login: %v, tok=%q", err, tok)
	}
	// Login with wrong password.
	if _, _, err := m.Login(ctx, "127.0.0.1", "alice@example.com", "wrongpass"); err == nil {
		t.Error("expected error for wrong password")
	}
}

func TestManager_ShortPassword(t *testing.T) {
	m := NewManager(newStub(), &idGen{})
	if _, err := m.Register(context.Background(), "bob@example.com", "Bob", "short"); err == nil {
		t.Error("expected error for short password")
	}
}

func TestManager_RateLimit(t *testing.T) {
	m := NewManager(newStub(), &idGen{})
	// Register a user.
	_ , _ = m.Register(context.Background(), "c@x.com", "C", "correctpass")
	// Flood login attempts.
	for i := 0; i < 5; i++ {
		_, _, _ = m.Login(context.Background(), "10.0.0.1", "c@x.com", "wrong")
	}
	// 6th attempt should be rate-limited regardless of password correctness.
	_, _, err := m.Login(context.Background(), "10.0.0.1", "c@x.com", "correctpass")
	if err == nil {
		t.Error("expected rate-limit error after 5 failed attempts")
	}
}

func TestHashToken_Stable(t *testing.T) {
	h1 := HashToken("vs_test123")
	h2 := HashToken("vs_test123")
	if h1 != h2 {
		t.Error("HashToken not stable")
	}
}
