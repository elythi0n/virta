package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/store/storetest"
)

// dsnEnv names the env var holding a Postgres DSN. When unset, the DB-backed tests skip, so
// `make ci` stays offline; the CI postgres-service job and local dev set it to run for real.
const dsnEnv = "VIRTA_TEST_POSTGRES"

func TestRebind(t *testing.T) {
	cases := map[string]string{
		"SELECT 1":                       "SELECT 1",
		"WHERE a = ? AND b = ?":          "WHERE a = $1 AND b = $2",
		"VALUES (?, ?, ?)":               "VALUES ($1, $2, $3)",
		"ON CONFLICT(x) DO UPDATE":       "ON CONFLICT(x) DO UPDATE",
		"SET a = ? WHERE id = ? LIMIT ?": "SET a = $1 WHERE id = $2 LIMIT $3",
	}
	for in, want := range cases {
		if got := rebind(in); got != want {
			t.Errorf("rebind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsUnique_NonPgError(t *testing.T) {
	if isUnique(nil) || isUnique(context.Canceled) {
		t.Error("isUnique should be false for nil and non-pg errors")
	}
}

// resetSchema drops and recreates the public schema so each contract store starts empty.
func resetSchema(t *testing.T, dsn string) {
	t.Helper()
	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("reset open: %v", err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
}

// TestPostgres_Contract runs the shared store conformance suite against a real Postgres, so
// Postgres is proven to behave identically to SQLite and the in-memory fake.
func TestPostgres_Contract(t *testing.T) {
	dsn := os.Getenv(dsnEnv)
	if dsn == "" {
		t.Skipf("set %s to run the Postgres contract suite", dsnEnv)
	}
	storetest.RunContract(t, func(t *testing.T) store.Store {
		resetSchema(t, dsn)
		db, err := Open(dsn, clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)), id.NewFake("rec"))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	})
}

func TestPostgres_MigrateIdempotent(t *testing.T) {
	dsn := os.Getenv(dsnEnv)
	if dsn == "" {
		t.Skipf("set %s to run the Postgres migrate test", dsnEnv)
	}
	resetSchema(t, dsn)
	db, err := Open(dsn, clock.NewFake(time.Now()), id.NewFake("rec"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for range 3 {
		if err := db.Migrate(context.Background()); err != nil {
			t.Fatalf("repeat Migrate: %v", err)
		}
	}
}
