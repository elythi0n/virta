// Package storetest is the reusable conformance suite for store.Store. The in-memory fake
// runs it; the SQLite and Postgres backends run the exact same suite, so every backend —
// and the fake the rest of the codebase tests against — behaves identically.
package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/store"
)

// RunContract exercises every repository and the load-bearing invariants. newStore must
// return a fresh, migrated, empty Store on each call.
func RunContract(t *testing.T, newStore func(t *testing.T) store.Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("Migrate and Ping succeed", func(t *testing.T) {
		s := newStore(t)
		if err := s.Migrate(ctx); err != nil {
			t.Fatalf("Migrate: %v", err)
		}
		if err := s.Ping(ctx); err != nil {
			t.Fatalf("Ping: %v", err)
		}
	})

	t.Run("settings upsert and read back", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.Settings().Get(ctx, "app"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("Get unset = %v, want ErrNotFound", err)
		}
		if err := s.Settings().Put(ctx, store.Setting{Scope: "app", Data: json.RawMessage(`{"x":1}`)}); err != nil {
			t.Fatalf("Put: %v", err)
		}
		got, err := s.Settings().Get(ctx, "app")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if string(got.Data) != `{"x":1}` {
			t.Errorf("Data = %s", got.Data)
		}
		if got.UpdatedAt.IsZero() {
			t.Error("UpdatedAt not stamped")
		}
	})

	t.Run("profiles: create, default, conflict, update, delete", func(t *testing.T) {
		s := newStore(t)
		p1, err := s.Profiles().Create(ctx, "main", json.RawMessage(`{"v":1}`))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if !p1.IsDefault {
			t.Error("first profile should be default")
		}
		if _, err := s.Profiles().Create(ctx, "main", nil); !errors.Is(err, store.ErrConflict) {
			t.Errorf("duplicate name = %v, want ErrConflict", err)
		}
		p2, err := s.Profiles().Create(ctx, "mod", nil)
		if err != nil {
			t.Fatalf("Create p2: %v", err)
		}
		if err := s.Profiles().SetDefault(ctx, p2.ID); err != nil {
			t.Fatalf("SetDefault: %v", err)
		}
		def, err := s.Profiles().Default(ctx)
		if err != nil {
			t.Fatalf("Default: %v", err)
		}
		if def.ID != p2.ID {
			t.Errorf("default = %s, want %s", def.ID, p2.ID)
		}
		// old default cleared
		again, _ := s.Profiles().Get(ctx, p1.ID)
		if again.IsDefault {
			t.Error("old default flag not cleared")
		}
		updated, err := s.Profiles().Update(ctx, p1.ID, json.RawMessage(`{"v":2}`))
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if string(updated.Doc) != `{"v":2}` {
			t.Errorf("Doc = %s", updated.Doc)
		}
		if err := s.Profiles().Delete(ctx, p1.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := s.Profiles().Get(ctx, p1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Get deleted = %v, want ErrNotFound", err)
		}
		// name freed for reuse after delete
		if _, err := s.Profiles().Create(ctx, "main", nil); err != nil {
			t.Errorf("recreate after delete: %v", err)
		}
	})

	t.Run("accounts: upsert is idempotent by (platform, uid)", func(t *testing.T) {
		s := newStore(t)
		a, err := s.Accounts().Upsert(ctx, store.Account{
			Platform: platform.Twitch, PlatformUID: "123", Login: "streamer",
			SecretRef: "platform:twitch:acct", Scopes: []string{"user:read:chat"},
		})
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}
		if a.ID == "" || a.CreatedAt.IsZero() {
			t.Fatal("id/created_at not set")
		}
		a2, err := s.Accounts().Upsert(ctx, store.Account{Platform: platform.Twitch, PlatformUID: "123", Login: "renamed"})
		if err != nil {
			t.Fatalf("re-upsert: %v", err)
		}
		if a2.ID != a.ID {
			t.Errorf("upsert created a new id %s, want reuse of %s", a2.ID, a.ID)
		}
		if list, _ := s.Accounts().ListByPlatform(ctx, platform.Twitch); len(list) != 1 {
			t.Errorf("ListByPlatform len = %d, want 1", len(list))
		}
		if err := s.Accounts().Delete(ctx, a.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})

	t.Run("channels: upsert by (platform, slug), meta preserved", func(t *testing.T) {
		s := newStore(t)
		c, err := s.Channels().Upsert(ctx, store.Channel{
			Platform: platform.Kick, Slug: "xqc", Meta: json.RawMessage(`{"chatroom_id":42}`),
		})
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}
		got, err := s.Channels().GetBySlug(ctx, platform.Kick, "xqc")
		if err != nil {
			t.Fatalf("GetBySlug: %v", err)
		}
		if got.ID != c.ID || string(got.Meta) != `{"chatroom_id":42}` {
			t.Errorf("got %+v", got)
		}
		if _, err := s.Channels().GetBySlug(ctx, platform.Twitch, "xqc"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cross-platform slug = %v, want ErrNotFound", err)
		}
		// Re-upsert the same (platform, slug): updates in place, keeps the id, refreshes fields.
		c2, err := s.Channels().Upsert(ctx, store.Channel{
			Platform: platform.Kick, Slug: "xqc", DisplayName: "xQc", PlatformID: "p123",
			Meta: json.RawMessage(`{"chatroom_id":99}`),
		})
		if err != nil {
			t.Fatalf("re-upsert: %v", err)
		}
		if c2.ID != c.ID {
			t.Errorf("re-upsert changed id: %s != %s", c2.ID, c.ID)
		}
		again, _ := s.Channels().GetBySlug(ctx, platform.Kick, "xqc")
		if again.DisplayName != "xQc" || again.PlatformID != "p123" || string(again.Meta) != `{"chatroom_id":99}` {
			t.Errorf("re-upsert did not refresh fields: %+v", again)
		}
		if list, _ := s.Channels().List(ctx); len(list) != 1 {
			t.Errorf("re-upsert duplicated row: List len = %d, want 1", len(list))
		}
	})

	t.Run("messages: Append of an empty batch is a no-op", func(t *testing.T) {
		s := newStore(t)
		if err := s.Messages().Append(ctx, nil); err != nil {
			t.Errorf("Append(nil) = %v, want nil", err)
		}
		if err := s.Messages().Append(ctx, []platform.UnifiedMessage{}); err != nil {
			t.Errorf("Append(empty) = %v, want nil", err)
		}
	})

	t.Run("messages: append, history ordering + cursor, mark deleted, sweep", func(t *testing.T) {
		s := newStore(t)
		ch := "chan_x"
		base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
		mk := func(id string, at time.Time) platform.UnifiedMessage {
			return platform.UnifiedMessage{
				ID: id, Channel: platform.ChannelRef{ID: ch, Platform: platform.Twitch},
				Platform: platform.Twitch, Type: platform.TypeChat,
				Author:     platform.Author{ID: "u1", DisplayName: "user"},
				Segments:   []platform.Segment{{Kind: platform.SegText, Text: "hi " + id}},
				ReceivedAt: at, SentAt: at,
			}
		}
		msgs := []platform.UnifiedMessage{
			mk("msg-0001", base),
			mk("msg-0002", base.Add(time.Second)),
			mk("msg-0003", base.Add(2*time.Second)),
		}
		if err := s.Messages().Append(ctx, msgs); err != nil {
			t.Fatalf("Append: %v", err)
		}
		// newest-first
		page, err := s.Messages().History(ctx, store.HistoryQuery{ChannelID: ch, Limit: 10})
		if err != nil {
			t.Fatalf("History: %v", err)
		}
		if len(page) != 3 || page[0].ID != "msg-0003" || page[2].ID != "msg-0001" {
			t.Fatalf("History order wrong: %v", ids(page))
		}
		if page[0].Body != "hi msg-0003" {
			t.Errorf("Body = %q", page[0].Body)
		}
		// cursor: before msg-0003 → 0002, 0001
		page2, _ := s.Messages().History(ctx, store.HistoryQuery{ChannelID: ch, Before: "msg-0003", Limit: 10})
		if len(page2) != 2 || page2[0].ID != "msg-0002" {
			t.Errorf("cursor page = %v", ids(page2))
		}
		// mark deleted
		if err := s.Messages().MarkDeleted(ctx, "msg-0002"); err != nil {
			t.Fatalf("MarkDeleted: %v", err)
		}
		page3, _ := s.Messages().History(ctx, store.HistoryQuery{ChannelID: ch, Limit: 10})
		for _, m := range page3 {
			if m.ID == "msg-0002" && !m.Deleted {
				t.Error("msg-0002 not marked deleted")
			}
		}
		// sweep older than base+1.5s removes 0001 and 0002
		removed, err := s.Messages().Sweep(ctx, ch, base.Add(1500*time.Millisecond))
		if err != nil {
			t.Fatalf("Sweep: %v", err)
		}
		if removed != 2 {
			t.Errorf("swept %d, want 2", removed)
		}
		remaining, _ := s.Messages().History(ctx, store.HistoryQuery{ChannelID: ch, Limit: 10})
		if len(remaining) != 1 || remaining[0].ID != "msg-0003" {
			t.Errorf("after sweep: %v", ids(remaining))
		}
	})

	t.Run("messages: full-text search by text, channel, and author", func(t *testing.T) {
		s := newStore(t)
		base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
		mk := func(id, ch, uid, name, body string, at time.Time) platform.UnifiedMessage {
			return platform.UnifiedMessage{
				ID: id, Channel: platform.ChannelRef{ID: ch, Platform: platform.Twitch},
				Platform: platform.Twitch, Type: platform.TypeChat,
				Author:     platform.Author{ID: uid, DisplayName: name},
				Segments:   []platform.Segment{{Kind: platform.SegText, Text: body}},
				ReceivedAt: at, SentAt: at,
			}
		}
		msgs := []platform.UnifiedMessage{
			mk("msg-0001", "ch-a", "u1", "Alice", "hello world", base),
			mk("msg-0002", "ch-a", "u2", "Bob", "goodbye world", base.Add(time.Second)),
			mk("msg-0003", "ch-b", "u1", "Alice", "hello there", base.Add(2*time.Second)),
		}
		if err := s.Messages().Append(ctx, msgs); err != nil {
			t.Fatalf("Append: %v", err)
		}

		// "hello" matches across channels, newest-first.
		hits, err := s.Messages().Search(ctx, store.SearchQuery{Text: "hello", Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if got := ids(hits); len(got) != 2 || got[0] != "msg-0003" || got[1] != "msg-0001" {
			t.Fatalf("search hello = %v, want [msg-0003 msg-0001]", got)
		}
		// Narrowed to one channel.
		inA, _ := s.Messages().Search(ctx, store.SearchQuery{Text: "hello", ChannelID: "ch-a", Limit: 10})
		if got := ids(inA); len(got) != 1 || got[0] != "msg-0001" {
			t.Errorf("search hello in ch-a = %v, want [msg-0001]", got)
		}
		// Narrowed by author display name (case-insensitive).
		byBob, _ := s.Messages().Search(ctx, store.SearchQuery{Text: "world", Author: "bob", Limit: 10})
		if got := ids(byBob); len(got) != 1 || got[0] != "msg-0002" {
			t.Errorf("search world by Bob = %v, want [msg-0002]", got)
		}
		// Cursor pages older matches only.
		before, _ := s.Messages().Search(ctx, store.SearchQuery{Text: "world", Before: "msg-0002", Limit: 10})
		if got := ids(before); len(got) != 1 || got[0] != "msg-0001" {
			t.Errorf("search world before msg-0002 = %v, want [msg-0001]", got)
		}
		// Empty text and no-match return nothing.
		if empty, _ := s.Messages().Search(ctx, store.SearchQuery{Text: "  ", Limit: 10}); len(empty) != 0 {
			t.Errorf("empty-text search = %v, want none", ids(empty))
		}
		if none, _ := s.Messages().Search(ctx, store.SearchQuery{Text: "zzznomatch", Limit: 10}); len(none) != 0 {
			t.Errorf("no-match search = %v, want none", ids(none))
		}
	})

	t.Run("messages: Append refuses ephemeral (logging-off invariant)", func(t *testing.T) {
		s := newStore(t)
		ch := "chan_e"
		batch := []platform.UnifiedMessage{
			{ID: "ok-1", Channel: platform.ChannelRef{ID: ch}, Type: platform.TypeChat},
			{ID: "eph-1", Channel: platform.ChannelRef{ID: ch}, Type: platform.TypeChat, Ephemeral: true},
		}
		err := s.Messages().Append(ctx, batch)
		if !errors.Is(err, store.ErrEphemeral) {
			t.Fatalf("Append with ephemeral = %v, want ErrEphemeral", err)
		}
		// the whole batch must be rejected — nothing written
		page, _ := s.Messages().History(ctx, store.HistoryQuery{ChannelID: ch, Limit: 10})
		if len(page) != 0 {
			t.Fatalf("ephemeral batch partially written: %v", ids(page))
		}
	})

	t.Run("not-found and listing paths across repos", func(t *testing.T) {
		s := newStore(t)
		// profiles: every missing-id path returns ErrNotFound
		if _, err := s.Profiles().Get(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Profiles.Get missing = %v", err)
		}
		if _, err := s.Profiles().GetByName(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Profiles.GetByName missing = %v", err)
		}
		if _, err := s.Profiles().Update(ctx, "nope", nil); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Profiles.Update missing = %v", err)
		}
		if err := s.Profiles().Delete(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Profiles.Delete missing = %v", err)
		}
		if err := s.Profiles().SetDefault(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Profiles.SetDefault missing = %v", err)
		}
		if _, err := s.Profiles().Default(ctx); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Profiles.Default with none = %v", err)
		}
		// accounts
		if _, err := s.Accounts().Get(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Accounts.Get missing = %v", err)
		}
		if err := s.Accounts().Delete(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Accounts.Delete missing = %v", err)
		}
		// channels
		if err := s.Channels().Delete(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Channels.Delete missing = %v", err)
		}

		// listing: create a spread and verify counts + GetByName
		p, _ := s.Profiles().Create(ctx, "p", nil)
		if got, _ := s.Profiles().GetByName(ctx, "p"); got.ID != p.ID {
			t.Errorf("GetByName roundtrip: %s != %s", got.ID, p.ID)
		}
		if list, _ := s.Profiles().List(ctx); len(list) != 1 {
			t.Errorf("Profiles.List = %d, want 1", len(list))
		}
		_, _ = s.Accounts().Upsert(ctx, store.Account{Platform: platform.Kick, PlatformUID: "k1"})
		_, _ = s.Accounts().Upsert(ctx, store.Account{Platform: platform.Twitch, PlatformUID: "t1"})
		if all, _ := s.Accounts().List(ctx); len(all) != 2 {
			t.Errorf("Accounts.List = %d, want 2", len(all))
		}
		if kick, _ := s.Accounts().ListByPlatform(ctx, platform.Kick); len(kick) != 1 {
			t.Errorf("Accounts.ListByPlatform(kick) = %d, want 1", len(kick))
		}
		_, _ = s.Channels().Upsert(ctx, store.Channel{Platform: platform.Twitch, Slug: "a"})
		if list, _ := s.Channels().List(ctx); len(list) != 1 {
			t.Errorf("Channels.List = %d, want 1", len(list))
		}
		_ = s.Settings().Put(ctx, store.Setting{Scope: "app", Data: json.RawMessage(`1`)})
		_ = s.Settings().Put(ctx, store.Setting{Scope: "ui", Data: json.RawMessage(`2`)})
		if all, _ := s.Settings().All(ctx); len(all) != 2 {
			t.Errorf("Settings.All = %d, want 2", len(all))
		}
	})

	t.Run("channels: Delete then GetBySlug is not found", func(t *testing.T) {
		s := newStore(t)
		c, _ := s.Channels().Upsert(ctx, store.Channel{Platform: platform.Kick, Slug: "gone"})
		if err := s.Channels().Delete(ctx, c.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := s.Channels().GetBySlug(ctx, platform.Kick, "gone"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetBySlug after delete = %v, want ErrNotFound", err)
		}
		// re-upsert same slug works (index cleared)
		if _, err := s.Channels().Upsert(ctx, store.Channel{Platform: platform.Kick, Slug: "gone"}); err != nil {
			t.Errorf("re-upsert after delete: %v", err)
		}
	})

	t.Run("moments: add, list with channel filter and cursor, delete", func(t *testing.T) {
		s := newStore(t)
		base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
		mk := func(id string, ch platform.ChannelRef, at time.Time) platform.Moment {
			return platform.Moment{
				ID: id, Channel: ch,
				StartedAt: at, EndedAt: at.Add(8 * time.Second),
				PeakRate: 12.5, Baseline: 1.25,
				Excerpt: []platform.MomentMessage{
					{Author: "alice", Body: "POG " + id, SentAt: at.UnixMilli()},
					{Author: "bob", Body: "LETSGO", SentAt: at.Add(time.Second).UnixMilli()},
				},
			}
		}
		forsen := platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}
		xqc := platform.ChannelRef{Platform: platform.Kick, Slug: "xqc"}
		for _, m := range []platform.Moment{
			mk("mom-0001", forsen, base),
			mk("mom-0002", xqc, base.Add(time.Minute)),
			mk("mom-0003", forsen, base.Add(2*time.Minute)),
		} {
			if err := s.Moments().Add(ctx, m); err != nil {
				t.Fatalf("Add %s: %v", m.ID, err)
			}
		}
		// newest-first across all channels
		all, err := s.Moments().List(ctx, store.MomentQuery{Limit: 10})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(all) != 3 || all[0].ID != "mom-0003" || all[2].ID != "mom-0001" {
			t.Fatalf("List order wrong: %v", momentIDs(all))
		}
		// round-trip of every field
		got := all[2]
		if got.Channel.Platform != platform.Twitch || got.Channel.Slug != "forsen" {
			t.Errorf("Channel = %+v", got.Channel)
		}
		if !got.StartedAt.Equal(base) || !got.EndedAt.Equal(base.Add(8*time.Second)) {
			t.Errorf("times = %v .. %v", got.StartedAt, got.EndedAt)
		}
		if got.PeakRate != 12.5 || got.Baseline != 1.25 {
			t.Errorf("rates = %v / %v", got.PeakRate, got.Baseline)
		}
		if len(got.Excerpt) != 2 || got.Excerpt[0].Author != "alice" || got.Excerpt[0].Body != "POG mom-0001" ||
			got.Excerpt[0].SentAt != base.UnixMilli() {
			t.Errorf("excerpt = %+v", got.Excerpt)
		}
		// narrowed to one channel
		inForsen, _ := s.Moments().List(ctx, store.MomentQuery{Channel: "twitch:forsen", Limit: 10})
		if got := momentIDs(inForsen); len(got) != 2 || got[0] != "mom-0003" || got[1] != "mom-0001" {
			t.Errorf("List twitch:forsen = %v, want [mom-0003 mom-0001]", got)
		}
		// cursor pages older moments only
		before, _ := s.Moments().List(ctx, store.MomentQuery{Before: "mom-0003", Limit: 10})
		if got := momentIDs(before); len(got) != 2 || got[0] != "mom-0002" {
			t.Errorf("List before mom-0003 = %v, want [mom-0002 mom-0001]", got)
		}
		// limit clamps the page
		one, _ := s.Moments().List(ctx, store.MomentQuery{Limit: 1})
		if got := momentIDs(one); len(got) != 1 || got[0] != "mom-0003" {
			t.Errorf("List limit 1 = %v, want [mom-0003]", got)
		}
		// delete removes the row; a missing id is ErrNotFound
		if err := s.Moments().Delete(ctx, "mom-0002"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if err := s.Moments().Delete(ctx, "mom-0002"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Delete missing = %v, want ErrNotFound", err)
		}
		remaining, _ := s.Moments().List(ctx, store.MomentQuery{Limit: 10})
		if len(remaining) != 2 {
			t.Errorf("after delete: %v", momentIDs(remaining))
		}
	})

	t.Run("emote cache: sets and files round-trip", func(t *testing.T) {
		s := newStore(t)
		if err := s.Emotes().PutSet(ctx, store.EmoteSet{Key: "7tv:twitch:1", Data: json.RawMessage(`[]`)}); err != nil {
			t.Fatalf("PutSet: %v", err)
		}
		set, err := s.Emotes().GetSet(ctx, "7tv:twitch:1")
		if err != nil || string(set.Data) != `[]` {
			t.Fatalf("GetSet = %+v, %v", set, err)
		}
		if _, err := s.Emotes().GetSet(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetSet missing = %v, want ErrNotFound", err)
		}
		if err := s.Emotes().PutFile(ctx, store.EmoteFile{URLHash: "abc", Path: "/cache/abc.webp", Bytes: 1234}); err != nil {
			t.Fatalf("PutFile: %v", err)
		}
		f, err := s.Emotes().GetFile(ctx, "abc")
		if err != nil || f.Bytes != 1234 {
			t.Fatalf("GetFile = %+v, %v", f, err)
		}
	})
}

func ids(ms []store.StoredMessage) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func momentIDs(ms []platform.Moment) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}
