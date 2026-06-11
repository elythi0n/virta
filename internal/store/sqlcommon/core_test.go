package sqlcommon_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/store/sqlcommon"
	"github.com/elythi0n/virta/internal/store/storetest"
)

// schema is the table set the repos operate on (kept in step with the backends' 0001 migration;
// it's a test fixture so the shared core can run the full store contract directly, against an
// in-memory SQLite, with no backend dependency).
const schema = `
CREATE TABLE settings (scope TEXT PRIMARY KEY, data TEXT NOT NULL, updated_at INTEGER NOT NULL, user_id TEXT NOT NULL DEFAULT '');
CREATE TABLE profiles (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, doc TEXT NOT NULL, is_default INTEGER NOT NULL DEFAULT 0, user_id TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
CREATE TABLE accounts (id TEXT PRIMARY KEY, platform TEXT NOT NULL, platform_uid TEXT NOT NULL, login TEXT NOT NULL DEFAULT '', display_name TEXT NOT NULL DEFAULT '', secret_ref TEXT NOT NULL DEFAULT '', scopes TEXT NOT NULL DEFAULT '[]', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, user_id TEXT NOT NULL DEFAULT '', UNIQUE(platform, platform_uid, user_id));
CREATE TABLE channels (id TEXT PRIMARY KEY, platform TEXT NOT NULL, platform_id TEXT NOT NULL DEFAULT '', slug TEXT NOT NULL, display_name TEXT NOT NULL DEFAULT '', meta TEXT, last_seen_at INTEGER NOT NULL DEFAULT 0, user_id TEXT NOT NULL DEFAULT '', UNIQUE(platform, slug));
CREATE TABLE messages (id TEXT PRIMARY KEY, channel_id TEXT NOT NULL, platform TEXT NOT NULL, type TEXT NOT NULL, author_uid TEXT NOT NULL DEFAULT '', author_name TEXT NOT NULL DEFAULT '', body TEXT NOT NULL DEFAULT '', segments TEXT NOT NULL DEFAULT '[]', sent_at INTEGER NOT NULL DEFAULT 0, received_at INTEGER NOT NULL DEFAULT 0, deleted INTEGER NOT NULL DEFAULT 0);
CREATE TABLE emote_sets (key TEXT PRIMARY KEY, data TEXT NOT NULL, fetched_at INTEGER NOT NULL);
CREATE TABLE moments (id TEXT PRIMARY KEY, channel_key TEXT NOT NULL DEFAULT '', platform TEXT NOT NULL DEFAULT '', slug TEXT NOT NULL DEFAULT '', started_at INTEGER NOT NULL DEFAULT 0, ended_at INTEGER NOT NULL DEFAULT 0, peak_rate REAL NOT NULL DEFAULT 0, baseline REAL NOT NULL DEFAULT 0, excerpt TEXT NOT NULL DEFAULT '[]');
CREATE TABLE emote_files (url_hash TEXT PRIMARY KEY, path TEXT NOT NULL, bytes INTEGER NOT NULL, fetched_at INTEGER NOT NULL);
CREATE VIRTUAL TABLE messages_fts USING fts5(body, content='messages', content_rowid='rowid');
CREATE TRIGGER messages_fts_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts (rowid, body) VALUES (new.rowid, new.body);
END;
CREATE TRIGGER messages_fts_ad AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts (messages_fts, rowid, body) VALUES ('delete', old.rowid, old.body);
END;
CREATE TRIGGER messages_fts_au AFTER UPDATE ON messages BEGIN
    INSERT INTO messages_fts (messages_fts, rowid, body) VALUES ('delete', old.rowid, old.body);
    INSERT INTO messages_fts (rowid, body) VALUES (new.rowid, new.body);
END;
`

// ftsSearch mirrors the SQLite backend's full-text builder so the shared core runs the same search
// contract here (the real builder lives in the sqlite package, which can't be imported from this
// shared package).
func ftsSearch(q store.SearchQuery, limit int) (string, []any) {
	sql := `SELECT m.id, m.channel_id, m.platform, m.type, m.author_uid, m.author_name, m.body, m.segments, m.sent_at, m.received_at, m.deleted
	        FROM messages_fts f JOIN messages m ON m.rowid = f.rowid WHERE messages_fts MATCH ?`
	args := []any{`"` + strings.ToLower(strings.TrimSpace(q.Text)) + `"*`}
	if q.ChannelID != "" {
		sql += " AND m.channel_id = ?"
		args = append(args, q.ChannelID)
	}
	if q.Author != "" {
		sql += " AND (m.author_uid = ? OR m.author_name = ? COLLATE NOCASE)"
		args = append(args, q.Author, q.Author)
	}
	if q.Before != "" {
		sql += " AND m.id < ?"
		args = append(args, q.Before)
	}
	sql += " ORDER BY m.rowid DESC LIMIT ?"
	return sql, append(args, limit)
}

func newCore(t *testing.T) store.Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1) // one shared in-memory connection
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	dia := sqlcommon.Dialect{
		Rebind:   func(q string) string { return q },
		IsUnique: func(e error) bool { return e != nil && strings.Contains(e.Error(), "UNIQUE constraint failed") },
		Search:   ftsSearch,
	}
	return wrap{sqlcommon.New(db, clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)), id.NewFake("rec"), dia)}
}

// wrap adds the no-op Migrate the store.Store interface needs (the schema is applied directly
// in newCore for the test).
type wrap struct{ *sqlcommon.Core }

func (wrap) Migrate(context.Context) error { return nil }

// TestCore_Contract runs the full store conformance suite against the shared core, so its
// repository logic is verified independently of any one backend.
func TestCore_Contract(t *testing.T) {
	storetest.RunContract(t, newCore)
}
