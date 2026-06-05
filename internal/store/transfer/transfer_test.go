package transfer_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/store/sqlite"
	"github.com/elythi0n/virta/internal/store/transfer"
)

// Copy is backend-agnostic, so this exercises a real SQLite source → in-memory destination —
// the same path a SQLite → Postgres switch takes.
func TestCopy_MovesDurableConfig(t *testing.T) {
	ctx := context.Background()
	clk := clock.NewFake(time.Unix(1000, 0))

	src, err := sqlite.Open(":memory:", clk, id.NewULID(clk))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = src.Close() })

	// Seed the source: settings, an account, a channel with meta, two profiles (2nd default).
	if err := src.Settings().Put(ctx, store.Setting{Scope: "app", Data: []byte(`{"theme":"dark"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Accounts().Upsert(ctx, store.Account{Platform: platform.Twitch, PlatformUID: "42", Login: "streamer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Channels().Upsert(ctx, store.Channel{Platform: platform.Kick, Slug: "xqc", Meta: []byte(`{"chatroom_id":"99"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Profiles().Create(ctx, "main", []byte(`{"version":1}`)); err != nil {
		t.Fatal(err)
	}
	modDuty, err := src.Profiles().Create(ctx, "mod-duty", []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := src.Profiles().SetDefault(ctx, modDuty.ID); err != nil {
		t.Fatal(err)
	}

	dst := store.NewMemory(clock.NewFake(time.Unix(2000, 0)))

	if err := transfer.Copy(ctx, src, dst); err != nil {
		t.Fatalf("Copy: %v", err)
	}

	// Settings.
	if s, err := dst.Settings().Get(ctx, "app"); err != nil || string(s.Data) != `{"theme":"dark"}` {
		t.Errorf("setting not copied: %v %s", err, s.Data)
	}
	// Account (by platform/uid).
	accs, _ := dst.Accounts().List(ctx)
	if len(accs) != 1 || accs[0].Login != "streamer" {
		t.Errorf("accounts = %+v", accs)
	}
	// Channel with meta preserved.
	ch, err := dst.Channels().GetBySlug(ctx, platform.Kick, "xqc")
	if err != nil || string(ch.Meta) != `{"chatroom_id":"99"}` {
		t.Errorf("channel/meta not copied: %v %s", err, ch.Meta)
	}
	// Both profiles, with the default preserved by name (not by reassigned id).
	profs, _ := dst.Profiles().List(ctx)
	if len(profs) != 2 {
		t.Fatalf("profiles = %d, want 2", len(profs))
	}
	def, err := dst.Profiles().Default(ctx)
	if err != nil || def.Name != "mod-duty" {
		t.Errorf("default profile = %q (err %v), want mod-duty", def.Name, err)
	}
}

func TestCopy_PropagatesSourceError(t *testing.T) {
	ctx := context.Background()
	clk := clock.NewFake(time.Unix(1000, 0))
	src, err := sqlite.Open(":memory:", clk, id.NewULID(clk))
	if err != nil {
		t.Fatal(err)
	}
	_ = src.Close() // a closed source: every read fails

	dst := store.NewMemory(clk)
	if err := transfer.Copy(ctx, src, dst); err == nil {
		t.Error("Copy from a closed source returned nil error")
	}
}

func TestCopy_PropagatesDestError(t *testing.T) {
	ctx := context.Background()
	clk := clock.NewFake(time.Unix(1000, 0))
	src := store.NewMemory(clk)
	if err := src.Settings().Put(ctx, store.Setting{Scope: "app", Data: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	dst, err := sqlite.Open(":memory:", clk, id.NewULID(clk))
	if err != nil {
		t.Fatal(err)
	}
	_ = dst.Close() // a closed destination: every write fails
	if err := transfer.Copy(ctx, src, dst); err == nil {
		t.Error("Copy into a closed destination returned nil error")
	}
}

var errBoom = errorString("boom")

type errorString string

func (e errorString) Error() string { return string(e) }

// failing repos embed the interface (so they satisfy it) and override only the write Copy
// calls, so we can fail a specific copy step.
type failAccounts struct{ store.AccountRepo }

func (failAccounts) Upsert(context.Context, store.Account) (store.Account, error) {
	return store.Account{}, errBoom
}

type failChannels struct{ store.ChannelRepo }

func (failChannels) Upsert(context.Context, store.Channel) (store.Channel, error) {
	return store.Channel{}, errBoom
}

type failProfiles struct{ store.ProfileRepo }

func (failProfiles) Create(context.Context, string, json.RawMessage) (store.Profile, error) {
	return store.Profile{}, errBoom
}

// failDst is a store whose chosen repo's write fails; the rest delegate to a real memory store.
type failDst struct {
	*store.Memory
	failOn string
}

func (f failDst) Accounts() store.AccountRepo {
	if f.failOn == "accounts" {
		return failAccounts{}
	}
	return f.Memory.Accounts()
}
func (f failDst) Channels() store.ChannelRepo {
	if f.failOn == "channels" {
		return failChannels{}
	}
	return f.Memory.Channels()
}
func (f failDst) Profiles() store.ProfileRepo {
	if f.failOn == "profiles" {
		return failProfiles{}
	}
	return f.Memory.Profiles()
}

func TestCopy_PropagatesPerStepWriteErrors(t *testing.T) {
	ctx := context.Background()
	clk := clock.NewFake(time.Unix(1000, 0))
	src := store.NewMemory(clk)
	_, _ = src.Accounts().Upsert(ctx, store.Account{Platform: platform.Twitch, PlatformUID: "1"})
	_, _ = src.Channels().Upsert(ctx, store.Channel{Platform: platform.Twitch, Slug: "forsen"})
	_, _ = src.Profiles().Create(ctx, "main", []byte(`{}`))

	for _, step := range []string{"accounts", "channels", "profiles"} {
		dst := failDst{Memory: store.NewMemory(clk), failOn: step}
		if err := transfer.Copy(ctx, src, dst); err == nil {
			t.Errorf("Copy did not surface a write failure at the %s step", step)
		}
	}
}

func TestCopy_EmptySource(t *testing.T) {
	ctx := context.Background()
	src := store.NewMemory(clock.NewFake(time.Unix(0, 0)))
	dst := store.NewMemory(clock.NewFake(time.Unix(0, 0)))
	if err := transfer.Copy(ctx, src, dst); err != nil {
		t.Fatalf("Copy of empty source: %v", err)
	}
	if ps, _ := dst.Profiles().List(ctx); len(ps) != 0 {
		t.Errorf("destination not empty: %+v", ps)
	}
}
