package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/store/sqlite"
	"github.com/elythi0n/virta/internal/store/storetest"
)

func open(t *testing.T) *sqlite.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	clk := clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC))
	db, err := sqlite.Open(path, clk, id.NewFake("rec"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// SQLite must satisfy the same store contract as the in-memory and Postgres backends.
func TestSQLite_Contract(t *testing.T) {
	storetest.RunContract(t, func(t *testing.T) store.Store {
		return open(t)
	})
}

func TestSQLite_MigrateIsIdempotent(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	// Open already migrated; running again must be a no-op and must not error.
	for range 3 {
		if err := db.Migrate(ctx); err != nil {
			t.Fatalf("repeat Migrate: %v", err)
		}
	}
	// Data written before a re-migrate survives it.
	if err := db.Settings().Put(ctx, store.Setting{Scope: "app", Data: []byte(`{"k":1}`)}); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := db.Settings().Get(ctx, "app")
	if err != nil || string(got.Data) != `{"k":1}` {
		t.Fatalf("data lost across re-migrate: %v %s", err, got.Data)
	}
}

func TestSQLite_OpenFailsOnUnwritablePath(t *testing.T) {
	// A path under a nonexistent directory: the file can't be created, so migration's first
	// write fails and Open reports an error rather than returning a half-open store.
	bad := filepath.Join(t.TempDir(), "no-such-dir", "x.db")
	clk := clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC))
	if _, err := sqlite.Open(bad, clk, id.NewFake("rec")); err == nil {
		t.Fatal("Open on an uncreatable path returned nil error")
	}
}

func TestSQLite_MemoryDSN(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC))
	db, err := sqlite.Open(":memory:", clk, id.NewFake("rec"))
	if err != nil {
		t.Fatalf("open memory: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Profiles().Create(context.Background(), "p", nil); err != nil {
		t.Fatalf("create on memory db: %v", err)
	}
}

// Every repo method must return an error (never panic) once the database is closed. This
// also exercises the error-propagation branches across the repositories.
func TestSQLite_OperationsAfterCloseReturnErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closed.db")
	clk := clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC))
	db, err := sqlite.Open(path, clk, id.NewFake("rec"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	checks := []struct {
		name string
		err  error
	}{
		{"settings.Get", func() error { _, e := db.Settings().Get(ctx, "x"); return e }()},
		{"settings.Put", db.Settings().Put(ctx, store.Setting{Scope: "x", Data: []byte("1")})},
		{"settings.All", func() error { _, e := db.Settings().All(ctx); return e }()},
		{"profiles.Create", func() error { _, e := db.Profiles().Create(ctx, "p", nil); return e }()},
		{"profiles.Get", func() error { _, e := db.Profiles().Get(ctx, "id"); return e }()},
		{"profiles.List", func() error { _, e := db.Profiles().List(ctx); return e }()},
		{"profiles.Update", func() error { _, e := db.Profiles().Update(ctx, "id", nil); return e }()},
		{"profiles.Delete", db.Profiles().Delete(ctx, "id")},
		{"profiles.SetDefault", db.Profiles().SetDefault(ctx, "id")},
		{"profiles.Default", func() error { _, e := db.Profiles().Default(ctx); return e }()},
		{"accounts.Upsert", func() error { _, e := db.Accounts().Upsert(ctx, store.Account{PlatformUID: "u"}); return e }()},
		{"accounts.Get", func() error { _, e := db.Accounts().Get(ctx, "id"); return e }()},
		{"accounts.List", func() error { _, e := db.Accounts().List(ctx); return e }()},
		{"accounts.Delete", db.Accounts().Delete(ctx, "id")},
		{"channels.Upsert", func() error { _, e := db.Channels().Upsert(ctx, store.Channel{Slug: "s"}); return e }()},
		{"channels.GetBySlug", func() error { _, e := db.Channels().GetBySlug(ctx, "twitch", "s"); return e }()},
		{"channels.List", func() error { _, e := db.Channels().List(ctx); return e }()},
		{"channels.Delete", db.Channels().Delete(ctx, "id")},
		{"messages.History", func() error { _, e := db.Messages().History(ctx, store.HistoryQuery{ChannelID: "c"}); return e }()},
		{"messages.MarkDeleted", db.Messages().MarkDeleted(ctx, "id")},
		{"messages.Sweep", func() error { _, e := db.Messages().Sweep(ctx, "c", time.Now()); return e }()},
		{"emotes.PutSet", db.Emotes().PutSet(ctx, store.EmoteSet{Key: "k", Data: []byte("[]")})},
		{"emotes.GetSet", func() error { _, e := db.Emotes().GetSet(ctx, "k"); return e }()},
		{"emotes.GetFile", func() error { _, e := db.Emotes().GetFile(ctx, "h"); return e }()},
	}
	for _, c := range checks {
		if c.err == nil {
			t.Errorf("%s on closed db returned nil error, want an error", c.name)
		}
	}
	// Append on a closed db (non-empty, non-ephemeral) must also error, not panic.
	if err := db.Messages().Append(ctx, []platform.UnifiedMessage{{ID: "x", Channel: platform.ChannelRef{ID: "c"}}}); err == nil {
		t.Error("messages.Append on closed db returned nil error")
	}
}

func TestSQLite_PersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	clk := clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC))
	ctx := context.Background()

	db1, err := sqlite.Open(path, clk, id.NewFake("rec"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := db1.Profiles().Create(ctx, "main", []byte(`{"v":1}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = db1.Close()

	// Reopen the same file: the profile is still there.
	db2, err := sqlite.Open(path, clk, id.NewFake("rec"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db2.Close() })
	got, err := db2.Profiles().Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("profile not persisted across reopen: %v", err)
	}
	if got.Name != "main" || string(got.Doc) != `{"v":1}` {
		t.Errorf("reopened profile = %+v", got)
	}
}
