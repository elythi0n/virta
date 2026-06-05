package sqlcommon

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// Migrate applies every migration in fsys (files "migrations/NNNN_desc.sql") whose version
// exceeds the recorded schema version, each in its own transaction, in ascending order. It is
// idempotent. rebind adapts the version-insert placeholder to the backend (identity for
// SQLite, ?→$1 for Postgres). Shared by every SQL backend so the runner is tested once.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS, rebind func(string) string) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version BIGINT NOT NULL)`); err != nil {
		return fmt.Errorf("migrate: create schema_version: %w", err)
	}

	var current int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("migrate: read schema_version: %w", err)
	}

	migrations, err := loadMigrations(fsys)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("migrate: begin %d: %w", m.version, err)
		}
		if _, err := tx.ExecContext(ctx, m.body); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate: apply %d: %w", m.version, err)
		}
		if _, err := tx.ExecContext(ctx, rebind(`INSERT INTO schema_version (version) VALUES (?)`), m.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate: record %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrate: commit %d: %w", m.version, err)
		}
	}
	return nil
}

type migration struct {
	version int
	body    string
}

func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, fmt.Errorf("migrate: read migrations dir: %w", err)
	}
	var out []migration
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		us := strings.IndexByte(name, '_')
		if us < 0 {
			return nil, fmt.Errorf("migrate: malformed migration name %q (want NNNN_desc.sql)", name)
		}
		version, err := strconv.Atoi(name[:us])
		if err != nil {
			return nil, fmt.Errorf("migrate: migration %q has non-numeric version: %w", name, err)
		}
		body, err := fs.ReadFile(fsys, "migrations/"+name)
		if err != nil {
			return nil, fmt.Errorf("migrate: read migration %q: %w", name, err)
		}
		out = append(out, migration{version: version, body: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}
