// Package postgres implements the store.Store contract on PostgreSQL via the pgx driver's
// database/sql adapter, reusing the shared SQL core in internal/store/sqlcommon. It is the
// opt-in backend for users who want a server-grade database; SQLite remains the default. Both
// run the same conformance suite, so they behave identically.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/store/sqlcommon"
)

// DB is a Postgres-backed store.Store: the shared SQL core plus Postgres' migrations.
type DB struct {
	*sqlcommon.Core
}

// Open connects to the database at dsn (e.g. "postgres://user:pass@host:5432/db?sslmode=…"),
// applies migrations, and returns a ready store.
func Open(dsn string, clk clock.Clock, gen id.Generator) (*DB, error) {
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}
	if err := migrate(context.Background(), sqldb); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	dia := sqlcommon.Dialect{Rebind: rebind, IsUnique: isUnique, Search: searchSQL}
	return &DB{Core: sqlcommon.New(sqldb, clk, gen, dia)}, nil
}

// searchSQL builds the Postgres full-text query: a tsvector match (the generated body_tsv column
// is GIN-indexed) with websearch_to_tsquery so user input is parsed leniently, plus the optional
// channel/author/cursor filters. Placeholders are positional ($N) as Postgres expects.
func searchSQL(q store.SearchQuery, limit int) (string, []any) {
	var sb strings.Builder
	sb.WriteString(`SELECT id, channel_id, platform, type, author_uid, author_name, body, segments, sent_at, received_at, deleted
	                FROM messages WHERE body_tsv @@ websearch_to_tsquery('english', $1)`)
	args := []any{q.Text}
	n := 2
	if q.ChannelID != "" {
		fmt.Fprintf(&sb, " AND channel_id = $%d", n)
		args = append(args, q.ChannelID)
		n++
	}
	if q.Author != "" {
		fmt.Fprintf(&sb, " AND (author_uid = $%d OR lower(author_name) = lower($%d))", n, n+1)
		args = append(args, q.Author, q.Author)
		n += 2
	}
	if q.Before != "" {
		fmt.Fprintf(&sb, " AND id < $%d", n)
		args = append(args, q.Before)
		n++
	}
	fmt.Fprintf(&sb, " ORDER BY id DESC LIMIT $%d", n)
	args = append(args, limit)
	return sb.String(), args
}

// Migrate brings the schema to the current version. Safe to call repeatedly.
func (d *DB) Migrate(ctx context.Context) error { return migrate(ctx, d.Conn()) }

// rebind rewrites `?` placeholders to Postgres' positional `$1, $2, …`. The shared queries
// never contain a literal `?`, so a straight scan is safe.
func rebind(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 8)
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

// isUnique reports whether err is a Postgres unique-violation (SQLSTATE 23505).
func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

var _ store.Store = (*DB)(nil)
