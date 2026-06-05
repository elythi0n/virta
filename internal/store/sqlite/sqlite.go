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
	}
	return &DB{Core: sqlcommon.New(sqldb, clk, gen, dia)}, nil
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
