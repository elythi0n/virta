// Package sqlcommon implements the store.Store repositories once, over database/sql, so every
// SQL backend (SQLite, Postgres, …) shares identical query logic and behavior. Backends differ
// only in a small Dialect (placeholder style, unique-violation detection) and their own
// migrations; they wrap a Core and add Migrate. The shared store contract suite then verifies
// all backends behave the same.
package sqlcommon

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/store"
)

// Dialect captures the per-backend SQL differences.
type Dialect struct {
	// Rebind adapts a query written with `?` placeholders to the backend's style (identity for
	// SQLite, `?`→`$N` for Postgres).
	Rebind func(string) string
	// IsUnique reports whether err is a unique-constraint violation.
	IsUnique func(error) bool
}

// Core is a database/sql-backed implementation of the store repositories.
type Core struct {
	db  *sql.DB
	clk clock.Clock
	gen id.Generator
	dia Dialect
}

// New wraps an open *sql.DB (already migrated) with the given clock, id generator, and dialect.
func New(db *sql.DB, clk clock.Clock, gen id.Generator, dia Dialect) *Core {
	return &Core{db: db, clk: clk, gen: gen, dia: dia}
}

// Conn exposes the underlying handle so a backend can run its own migrations.
func (c *Core) Conn() *sql.DB { return c.db }

func (c *Core) Ping(ctx context.Context) error { return c.db.PingContext(ctx) }
func (c *Core) Close() error                   { return c.db.Close() }

func (c *Core) Settings() store.SettingsRepo { return settingsRepo{c} }
func (c *Core) Profiles() store.ProfileRepo  { return profileRepo{c} }
func (c *Core) Accounts() store.AccountRepo  { return accountRepo{c} }
func (c *Core) Channels() store.ChannelRepo  { return channelRepo{c} }
func (c *Core) Messages() store.MessageRepo  { return messageRepo{c} }
func (c *Core) Emotes() store.EmoteRepo      { return emoteRepo{c} }

// rebinding query helpers — every query flows through Rebind so placeholders are adapted once.
func (c *Core) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return c.db.ExecContext(ctx, c.dia.Rebind(q), args...)
}
func (c *Core) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return c.db.QueryRowContext(ctx, c.dia.Rebind(q), args...)
}
func (c *Core) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return c.db.QueryContext(ctx, c.dia.Rebind(q), args...)
}

// ---- shared helpers ----

func tsStore(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func tsLoad(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func encodeStrings(ss []string) string {
	if ss == nil {
		ss = []string{}
	}
	b, _ := json.Marshal(ss)
	return string(b)
}

func decodeStrings(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// ---- settings ----

type settingsRepo struct{ c *Core }

func (r settingsRepo) Get(ctx context.Context, scope string) (store.Setting, error) {
	var s store.Setting
	var data string
	var updated int64
	err := r.c.queryRow(ctx, `SELECT scope, data, updated_at FROM settings WHERE scope = ?`, scope).
		Scan(&s.Scope, &data, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Setting{}, store.ErrNotFound
	}
	if err != nil {
		return store.Setting{}, err
	}
	s.Data = json.RawMessage(data)
	s.UpdatedAt = tsLoad(updated)
	return s, nil
}

func (r settingsRepo) Put(ctx context.Context, s store.Setting) error {
	_, err := r.c.exec(ctx,
		`INSERT INTO settings (scope, data, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(scope) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		s.Scope, string(s.Data), tsStore(r.c.clk.Now()))
	return err
}

func (r settingsRepo) All(ctx context.Context) ([]store.Setting, error) {
	rows, err := r.c.query(ctx, `SELECT scope, data, updated_at FROM settings ORDER BY scope`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []store.Setting
	for rows.Next() {
		var s store.Setting
		var data string
		var updated int64
		if err := rows.Scan(&s.Scope, &data, &updated); err != nil {
			return nil, err
		}
		s.Data = json.RawMessage(data)
		s.UpdatedAt = tsLoad(updated)
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---- profiles ----

type profileRepo struct{ c *Core }

func (r profileRepo) Create(ctx context.Context, name string, doc json.RawMessage) (store.Profile, error) {
	now := r.c.clk.Now()
	p := store.Profile{ID: r.c.gen.New(), Name: name, Doc: doc, CreatedAt: now, UpdatedAt: now}
	if p.Doc == nil {
		p.Doc = json.RawMessage("null")
	}
	var count int
	if err := r.c.queryRow(ctx, `SELECT COUNT(*) FROM profiles`).Scan(&count); err != nil {
		return store.Profile{}, err
	}
	p.IsDefault = count == 0

	_, err := r.c.exec(ctx,
		`INSERT INTO profiles (id, name, doc, is_default, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, string(p.Doc), boolToInt(p.IsDefault), tsStore(now), tsStore(now))
	if r.c.dia.IsUnique(err) {
		return store.Profile{}, store.ErrConflict
	}
	if err != nil {
		return store.Profile{}, err
	}
	return p, nil
}

func (r profileRepo) scan(row interface{ Scan(...any) error }) (store.Profile, error) {
	var p store.Profile
	var doc string
	var isDefault int
	var created, updated int64
	if err := row.Scan(&p.ID, &p.Name, &doc, &isDefault, &created, &updated); err != nil {
		return store.Profile{}, err
	}
	p.Doc = json.RawMessage(doc)
	p.IsDefault = isDefault != 0
	p.CreatedAt = tsLoad(created)
	p.UpdatedAt = tsLoad(updated)
	return p, nil
}

const profileCols = `id, name, doc, is_default, created_at, updated_at`

func (r profileRepo) Get(ctx context.Context, id string) (store.Profile, error) {
	p, err := r.scan(r.c.queryRow(ctx, `SELECT `+profileCols+` FROM profiles WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return store.Profile{}, store.ErrNotFound
	}
	return p, err
}

func (r profileRepo) GetByName(ctx context.Context, name string) (store.Profile, error) {
	p, err := r.scan(r.c.queryRow(ctx, `SELECT `+profileCols+` FROM profiles WHERE name = ?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return store.Profile{}, store.ErrNotFound
	}
	return p, err
}

func (r profileRepo) List(ctx context.Context) ([]store.Profile, error) {
	rows, err := r.c.query(ctx, `SELECT `+profileCols+` FROM profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []store.Profile
	for rows.Next() {
		p, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r profileRepo) Update(ctx context.Context, id string, doc json.RawMessage) (store.Profile, error) {
	if doc == nil {
		doc = json.RawMessage("null")
	}
	res, err := r.c.exec(ctx, `UPDATE profiles SET doc = ?, updated_at = ? WHERE id = ?`,
		string(doc), tsStore(r.c.clk.Now()), id)
	if err != nil {
		return store.Profile{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.Profile{}, store.ErrNotFound
	}
	return r.Get(ctx, id)
}

func (r profileRepo) Delete(ctx context.Context, id string) error {
	res, err := r.c.exec(ctx, `DELETE FROM profiles WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r profileRepo) SetDefault(ctx context.Context, id string) error {
	tx, err := r.c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRowContext(ctx, r.c.dia.Rebind(`SELECT COUNT(*) FROM profiles WHERE id = ?`), id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return store.ErrNotFound
	}
	// CASE (not `is_default = (id = ?)`) so the result is an integer in both dialects —
	// Postgres won't assign a boolean to an integer column.
	if _, err := tx.ExecContext(ctx, r.c.dia.Rebind(`UPDATE profiles SET is_default = CASE WHEN id = ? THEN 1 ELSE 0 END`), id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r profileRepo) Default(ctx context.Context) (store.Profile, error) {
	p, err := r.scan(r.c.queryRow(ctx, `SELECT `+profileCols+` FROM profiles WHERE is_default = 1 LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return store.Profile{}, store.ErrNotFound
	}
	return p, err
}

// ---- accounts ----

type accountRepo struct{ c *Core }

const accountCols = `id, platform, platform_uid, login, display_name, secret_ref, scopes, created_at, updated_at`

func (r accountRepo) scan(row interface{ Scan(...any) error }) (store.Account, error) {
	var a store.Account
	var plat, scopes string
	var created, updated int64
	if err := row.Scan(&a.ID, &plat, &a.PlatformUID, &a.Login, &a.DisplayName, &a.SecretRef, &scopes, &created, &updated); err != nil {
		return store.Account{}, err
	}
	a.Platform = platform.Platform(plat)
	a.Scopes = decodeStrings(scopes)
	a.CreatedAt = tsLoad(created)
	a.UpdatedAt = tsLoad(updated)
	return a, nil
}

func (r accountRepo) Upsert(ctx context.Context, a store.Account) (store.Account, error) {
	now := r.c.clk.Now()
	var existingID string
	var createdNanos int64
	err := r.c.queryRow(ctx,
		`SELECT id, created_at FROM accounts WHERE platform = ? AND platform_uid = ?`,
		string(a.Platform), a.PlatformUID).Scan(&existingID, &createdNanos)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		a.ID = r.c.gen.New()
		a.CreatedAt = now
		a.UpdatedAt = now
		_, err := r.c.exec(ctx,
			`INSERT INTO accounts (`+accountCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, string(a.Platform), a.PlatformUID, a.Login, a.DisplayName, a.SecretRef,
			encodeStrings(a.Scopes), tsStore(now), tsStore(now))
		if err != nil {
			return store.Account{}, err
		}
		return a, nil
	case err != nil:
		return store.Account{}, err
	default:
		a.ID = existingID
		a.CreatedAt = tsLoad(createdNanos)
		a.UpdatedAt = now
		_, err := r.c.exec(ctx,
			`UPDATE accounts SET login = ?, display_name = ?, secret_ref = ?, scopes = ?, updated_at = ? WHERE id = ?`,
			a.Login, a.DisplayName, a.SecretRef, encodeStrings(a.Scopes), tsStore(now), a.ID)
		if err != nil {
			return store.Account{}, err
		}
		return a, nil
	}
}

func (r accountRepo) Get(ctx context.Context, id string) (store.Account, error) {
	a, err := r.scan(r.c.queryRow(ctx, `SELECT `+accountCols+` FROM accounts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return store.Account{}, store.ErrNotFound
	}
	return a, err
}

func (r accountRepo) List(ctx context.Context) ([]store.Account, error) {
	return r.queryAll(ctx, `SELECT `+accountCols+` FROM accounts ORDER BY id`)
}

func (r accountRepo) ListByPlatform(ctx context.Context, p platform.Platform) ([]store.Account, error) {
	return r.queryAll(ctx, `SELECT `+accountCols+` FROM accounts WHERE platform = ? ORDER BY id`, string(p))
}

func (r accountRepo) queryAll(ctx context.Context, q string, args ...any) ([]store.Account, error) {
	rows, err := r.c.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []store.Account
	for rows.Next() {
		a, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r accountRepo) Delete(ctx context.Context, id string) error {
	res, err := r.c.exec(ctx, `DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ---- channels ----

type channelRepo struct{ c *Core }

const channelCols = `id, platform, platform_id, slug, display_name, meta, last_seen_at`

func (r channelRepo) scan(row interface{ Scan(...any) error }) (store.Channel, error) {
	var c store.Channel
	var plat string
	var meta sql.NullString
	var lastSeen int64
	if err := row.Scan(&c.ID, &plat, &c.PlatformID, &c.Slug, &c.DisplayName, &meta, &lastSeen); err != nil {
		return store.Channel{}, err
	}
	c.Platform = platform.Platform(plat)
	if meta.Valid {
		c.Meta = json.RawMessage(meta.String)
	}
	c.LastSeenAt = tsLoad(lastSeen)
	return c, nil
}

func (r channelRepo) Upsert(ctx context.Context, c store.Channel) (store.Channel, error) {
	var existingID string
	err := r.c.queryRow(ctx, `SELECT id FROM channels WHERE platform = ? AND slug = ?`,
		string(c.Platform), c.Slug).Scan(&existingID)
	var metaArg any
	if c.Meta != nil {
		metaArg = string(c.Meta)
	}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		c.ID = r.c.gen.New()
		_, err := r.c.exec(ctx,
			`INSERT INTO channels (`+channelCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			c.ID, string(c.Platform), c.PlatformID, c.Slug, c.DisplayName, metaArg, tsStore(c.LastSeenAt))
		if err != nil {
			return store.Channel{}, err
		}
		return c, nil
	case err != nil:
		return store.Channel{}, err
	default:
		c.ID = existingID
		_, err := r.c.exec(ctx,
			`UPDATE channels SET platform_id = ?, display_name = ?, meta = ?, last_seen_at = ? WHERE id = ?`,
			c.PlatformID, c.DisplayName, metaArg, tsStore(c.LastSeenAt), c.ID)
		if err != nil {
			return store.Channel{}, err
		}
		return c, nil
	}
}

func (r channelRepo) GetBySlug(ctx context.Context, p platform.Platform, slug string) (store.Channel, error) {
	c, err := r.scan(r.c.queryRow(ctx, `SELECT `+channelCols+` FROM channels WHERE platform = ? AND slug = ?`, string(p), slug))
	if errors.Is(err, sql.ErrNoRows) {
		return store.Channel{}, store.ErrNotFound
	}
	return c, err
}

func (r channelRepo) List(ctx context.Context) ([]store.Channel, error) {
	rows, err := r.c.query(ctx, `SELECT `+channelCols+` FROM channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []store.Channel
	for rows.Next() {
		c, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r channelRepo) Delete(ctx context.Context, id string) error {
	res, err := r.c.exec(ctx, `DELETE FROM channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ---- messages ----

type messageRepo struct{ c *Core }

func (r messageRepo) Append(ctx context.Context, msgs []platform.UnifiedMessage) error {
	// Choke point: refuse the whole batch if any message is ephemeral, so logging-off code
	// can never accidentally persist chat.
	for i := range msgs {
		if msgs[i].Ephemeral {
			return store.ErrEphemeral
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	tx, err := r.c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, r.c.dia.Rebind(
		`INSERT INTO messages (id, channel_id, platform, type, author_uid, author_name, body, segments, sent_at, received_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`))
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for i := range msgs {
		m := &msgs[i]
		segs, err := json.Marshal(m.Segments)
		if err != nil {
			return fmt.Errorf("sqlcommon: marshal segments: %w", err)
		}
		if _, err := stmt.ExecContext(ctx, m.ID, m.Channel.ID, string(m.Platform), string(m.Type),
			m.Author.ID, m.Author.DisplayName, m.PlainText(), string(segs),
			tsStore(m.SentAt), tsStore(m.ReceivedAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r messageRepo) History(ctx context.Context, q store.HistoryQuery) ([]store.StoredMessage, error) {
	limit := q.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query := `SELECT id, channel_id, platform, type, author_uid, author_name, body, segments, sent_at, received_at, deleted
	          FROM messages WHERE channel_id = ?`
	args := []any{q.ChannelID}
	if q.Before != "" {
		query += ` AND id < ?`
		args = append(args, q.Before)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.c.query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []store.StoredMessage
	for rows.Next() {
		var m store.StoredMessage
		var plat, typ, segs string
		var sent, received int64
		var deleted int
		if err := rows.Scan(&m.ID, &m.ChannelID, &plat, &typ, &m.AuthorUID, &m.AuthorName,
			&m.Body, &segs, &sent, &received, &deleted); err != nil {
			return nil, err
		}
		m.Platform = platform.Platform(plat)
		m.Type = platform.MessageType(typ)
		m.Segments = json.RawMessage(segs)
		m.SentAt = tsLoad(sent)
		m.ReceivedAt = tsLoad(received)
		m.Deleted = deleted != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r messageRepo) MarkDeleted(ctx context.Context, id string) error {
	res, err := r.c.exec(ctx, `UPDATE messages SET deleted = 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r messageRepo) Sweep(ctx context.Context, channelID string, olderThan time.Time) (int, error) {
	res, err := r.c.exec(ctx, `DELETE FROM messages WHERE channel_id = ? AND received_at < ?`,
		channelID, tsStore(olderThan))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ---- emotes ----

type emoteRepo struct{ c *Core }

func (r emoteRepo) PutSet(ctx context.Context, s store.EmoteSet) error {
	_, err := r.c.exec(ctx,
		`INSERT INTO emote_sets (key, data, fetched_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET data = excluded.data, fetched_at = excluded.fetched_at`,
		s.Key, string(s.Data), tsStore(s.FetchedAt))
	return err
}

func (r emoteRepo) GetSet(ctx context.Context, key string) (store.EmoteSet, error) {
	var s store.EmoteSet
	var data string
	var fetched int64
	err := r.c.queryRow(ctx, `SELECT key, data, fetched_at FROM emote_sets WHERE key = ?`, key).
		Scan(&s.Key, &data, &fetched)
	if errors.Is(err, sql.ErrNoRows) {
		return store.EmoteSet{}, store.ErrNotFound
	}
	if err != nil {
		return store.EmoteSet{}, err
	}
	s.Data = json.RawMessage(data)
	s.FetchedAt = tsLoad(fetched)
	return s, nil
}

func (r emoteRepo) PutFile(ctx context.Context, f store.EmoteFile) error {
	_, err := r.c.exec(ctx,
		`INSERT INTO emote_files (url_hash, path, bytes, fetched_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(url_hash) DO UPDATE SET path = excluded.path, bytes = excluded.bytes, fetched_at = excluded.fetched_at`,
		f.URLHash, f.Path, f.Bytes, tsStore(f.FetchedAt))
	return err
}

func (r emoteRepo) GetFile(ctx context.Context, urlHash string) (store.EmoteFile, error) {
	var f store.EmoteFile
	var fetched int64
	err := r.c.queryRow(ctx, `SELECT url_hash, path, bytes, fetched_at FROM emote_files WHERE url_hash = ?`, urlHash).
		Scan(&f.URLHash, &f.Path, &f.Bytes, &fetched)
	if errors.Is(err, sql.ErrNoRows) {
		return store.EmoteFile{}, store.ErrNotFound
	}
	if err != nil {
		return store.EmoteFile{}, err
	}
	f.FetchedAt = tsLoad(fetched)
	return f, nil
}
