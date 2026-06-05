package sqlcommon_test

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"

	"github.com/elythi0n/virta/internal/store/sqlcommon"
)

func identity(q string) string { return q }

func memDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMigrate_AppliesAndIsIdempotent(t *testing.T) {
	db := memDB(t)
	fsys := fstest.MapFS{
		"migrations/0001_init.sql": {Data: []byte(`CREATE TABLE t (x INTEGER);`)},
		"migrations/0002_more.sql": {Data: []byte(`CREATE TABLE u (y INTEGER);`)},
		"migrations/readme.txt":    {Data: []byte("ignored")}, // non-.sql is skipped
	}
	ctx := context.Background()
	if err := sqlcommon.Migrate(ctx, db, fsys, identity); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// Re-running is a no-op.
	if err := sqlcommon.Migrate(ctx, db, fsys, identity); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO u (y) VALUES (1)`); err != nil {
		t.Errorf("migration 0002 not applied: %v", err)
	}
}

func TestMigrate_MalformedName(t *testing.T) {
	fsys := fstest.MapFS{"migrations/nounderscore.sql": {Data: []byte(`SELECT 1;`)}}
	if err := sqlcommon.Migrate(context.Background(), memDB(t), fsys, identity); err == nil {
		t.Error("malformed migration name did not error")
	}
}

func TestMigrate_NonNumericVersion(t *testing.T) {
	fsys := fstest.MapFS{"migrations/xx_init.sql": {Data: []byte(`SELECT 1;`)}}
	if err := sqlcommon.Migrate(context.Background(), memDB(t), fsys, identity); err == nil {
		t.Error("non-numeric version did not error")
	}
}

func TestMigrate_BadSQL(t *testing.T) {
	fsys := fstest.MapFS{"migrations/0001_init.sql": {Data: []byte(`THIS IS NOT SQL;`)}}
	if err := sqlcommon.Migrate(context.Background(), memDB(t), fsys, identity); err == nil {
		t.Error("invalid SQL in a migration did not error")
	}
}

func TestMigrate_ClosedDB(t *testing.T) {
	db := memDB(t)
	_ = db.Close()
	fsys := fstest.MapFS{"migrations/0001_init.sql": {Data: []byte(`CREATE TABLE t (x INTEGER);`)}}
	if err := sqlcommon.Migrate(context.Background(), db, fsys, identity); err == nil {
		t.Error("migrate on a closed db did not error")
	}
}

func TestMigrate_MissingDir(t *testing.T) {
	if err := sqlcommon.Migrate(context.Background(), memDB(t), fstest.MapFS{}, identity); err == nil {
		t.Error("missing migrations dir did not error")
	}
}
