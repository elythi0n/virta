// Package sqlite implements the store.Store contract on SQLite via a pure-Go driver (no cgo),
// so the binary cross-compiles freely. It is the default backend: a single local database
// file, created and migrated on open. The repository logic is shared with every other SQL
// backend in internal/store/sqlcommon; this package only supplies the driver, DSN, dialect, and
// migrations, so all backends run the same conformance suite and behave identically.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/store/sqlcommon"
)

// DB is a SQLite-backed store.Store: the shared SQL core plus SQLite's migrations.
type DB struct {
	*sqlcommon.Core
}

// Open opens (creating if needed) the database at path, applies migrations, and returns a
// ready store. clk stamps record timestamps and gen mints record ids. The path ":memory:"
// is supported for tests.
func Open(path string, clk clock.Clock, gen id.Generator) (*DB, error) {
	sqldb, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	// A single connection serializes all access. For a local single-process app this sidesteps
	// SQLite's writer-locking entirely at a negligible cost, and keeps WAL semantics simple.
	sqldb.SetMaxOpenConns(1)
	if err := migrate(context.Background(), sqldb); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	dia := sqlcommon.Dialect{
		Rebind:   func(q string) string { return q }, // SQLite uses ? placeholders as written
		IsUnique: isUnique,
		Search:   searchSQL,
	}
	return &DB{Core: sqlcommon.New(sqldb, clk, gen, dia)}, nil
}

// searchSQL builds the SQLite full-text query: an FTS5 MATCH over the messages_fts shadow table,
// joined back to messages for the full row and the non-text filters (channel, author, cursor).
func searchSQL(q store.SearchQuery, limit int) (string, []any) {
	var sb strings.Builder
	sb.WriteString(`SELECT m.id, m.channel_id, m.platform, m.type, m.author_uid, m.author_name, m.body, m.segments, m.sent_at, m.received_at, m.deleted
	                FROM messages_fts f JOIN messages m ON m.rowid = f.rowid
	                WHERE messages_fts MATCH ?`)
	args := []any{ftsQuery(q.Text)}
	if q.ChannelID != "" {
		sb.WriteString(" AND m.channel_id = ?")
		args = append(args, q.ChannelID)
	}
	if q.Author != "" {
		sb.WriteString(" AND (m.author_uid = ? OR m.author_name = ? COLLATE NOCASE)")
		args = append(args, q.Author, q.Author)
	}
	if q.Before != "" {
		sb.WriteString(" AND m.id < ?")
		args = append(args, q.Before)
	}
	// Order by rowid, not the ULID id: rowid is monotonic with insertion (and so with the id) but
	// is an integer the bounded LIMIT heap can rank without joining every match to fetch its id,
	// which keeps a broad-term search fast on a large log.
	sb.WriteString(" ORDER BY m.rowid DESC LIMIT ?")
	args = append(args, limit)
	return sb.String(), args
}

// ftsQuery turns free user text into a safe FTS5 MATCH expression: each whitespace-separated word
// is reduced to letters/digits and quoted as a term (implicit AND between them), and the final
// term is prefix-matched so partial words match as the user types. Returns a query that matches
// nothing when the input has no usable characters, rather than an FTS syntax error.
func ftsQuery(s string) string {
	var terms []string
	for _, f := range strings.Fields(s) {
		clean := strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return ' '
		}, f)
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}
		terms = append(terms, `"`+clean+`"`)
	}
	if len(terms) == 0 {
		return `""`
	}
	terms[len(terms)-1] += "*" // prefix-match the last term
	return strings.Join(terms, " ")
}

// buildDSN turns a file path into a modernc DSN with the pragmas we want: WAL journaling,
// NORMAL synchronous (durable enough with WAL, much faster), a busy timeout, and foreign keys.
func buildDSN(path string) string {
	if path == ":memory:" {
		return "file::memory:?_pragma=busy_timeout(5000)"
	}
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(ON)")
	return "file:" + path + "?" + q.Encode()
}

// Migrate brings the schema to the current version. Safe to call repeatedly.
func (d *DB) Migrate(ctx context.Context) error { return migrate(ctx, d.Conn()) }

// isUnique reports whether err is a SQLite UNIQUE-constraint violation.
func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

var _ store.Store = (*DB)(nil)
