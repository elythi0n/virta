package hosted

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/elythi0n/virta/internal/id"
)

// SQLStore is a database/sql-backed implementation of hosted.Store.
// It is intentionally minimal and depends only on database/sql so it works with both
// the sqlite and postgres backends without importing either.
type SQLStore struct {
	db     *sql.DB
	gen    id.Generator
	rebind func(string) string // ? → $N for postgres; identity for sqlite
}

// NewSQLStore wraps an open *sql.DB. rebind is the placeholder adapter from sqlcommon.Dialect.
func NewSQLStore(db *sql.DB, gen id.Generator, rebind func(string) string) *SQLStore {
	return &SQLStore{db: db, gen: gen, rebind: rebind}
}

func (s *SQLStore) CreateUser(ctx context.Context, email, displayName, passwordHash string) (User, error) {
	uid := s.gen.New()
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, s.rebind(
		`INSERT INTO users (id, email, display_name, password_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`),
		uid, email, displayName, passwordHash, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	return User{ID: uid, Email: email, DisplayName: displayName, CreatedAt: time.UnixMilli(now)}, nil
}

func (s *SQLStore) UserByEmail(ctx context.Context, email string) (User, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`SELECT id, email, display_name, created_at FROM users WHERE email = ?`), email)
	return scanUser(row)
}

func (s *SQLStore) UserByID(ctx context.Context, userID string) (User, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`SELECT id, email, display_name, created_at FROM users WHERE id = ?`), userID)
	return scanUser(row)
}

func (s *SQLStore) PasswordHash(ctx context.Context, userID string) (string, error) {
	var h string
	err := s.db.QueryRowContext(ctx, s.rebind(`SELECT password_hash FROM users WHERE id = ?`), userID).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return h, err
}

func (s *SQLStore) CreateSession(ctx context.Context, userID string, expiresAt time.Time) (string, error) {
	plaintext, err := NewSessionSecret()
	if err != nil {
		return "", err
	}
	hash := HashToken(plaintext)
	_, err = s.db.ExecContext(ctx, s.rebind(
		`INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`),
		hash, userID, expiresAt.UnixMilli(), time.Now().UnixMilli())
	if err != nil {
		return "", err
	}
	return plaintext, nil
}

func (s *SQLStore) SessionUser(ctx context.Context, tokenHash string) (User, error) {
	var userID string
	var expiresAt int64
	err := s.db.QueryRowContext(ctx, s.rebind(
		`SELECT user_id, expires_at FROM sessions WHERE token = ?`), tokenHash).
		Scan(&userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, err
	}
	if time.Now().UnixMilli() > expiresAt {
		_ = s.DeleteSession(ctx, tokenHash)
		return User{}, ErrUnauthorized
	}
	return s.UserByID(ctx, userID)
}

func (s *SQLStore) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM sessions WHERE token = ?`), tokenHash)
	return err
}

func (s *SQLStore) SweepSessions(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM sessions WHERE expires_at < ?`), time.Now().UnixMilli())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanUser(row *sql.Row) (User, error) {
	var u User
	var createdAt int64
	if err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	u.CreatedAt = time.UnixMilli(createdAt)
	return u, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return contains(s, "UNIQUE constraint failed") || contains(s, "unique_violation") || contains(s, "duplicate key")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

var _ = fmt.Sprintf // keep import
