// Package store defines the persistence contract: the Store port, its repositories, and
// the domain records they hold. Implementations live in subpackages (store/sqlite,
// store/postgres), import only this package, and are wired in internal/app.
//
// Default storage is SQLite; Postgres is a drop-in alternative behind the same
// contract. The shared conformance suite (store/storetest) runs against every impl AND the
// in-memory fake, so no backend can quietly diverge.
//
// Persistence is opt-in: chat messages are written only when logging is on.
// MessageRepo.Append enforces this — it is the single choke point that refuses to write an
// Ephemeral message — so "logging off ⇒ nothing stored" is a store-level invariant, not a
// convention scattered across callers.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// Sentinel errors. Implementations and the fake return these so callers can branch with
// errors.Is regardless of backend.
var (
	// ErrNotFound is returned when a requested record does not exist.
	ErrNotFound = errors.New("store: not found")
	// ErrConflict is returned on a uniqueness violation (e.g. duplicate profile name).
	ErrConflict = errors.New("store: conflict")
	// ErrEphemeral is returned by MessageRepo.Append for a message marked Ephemeral. It is
	// the single backstop for the "nothing is written unless logging is on" guarantee:
	// callers should not reach Append while logging is off, and if they do, nothing is
	// persisted and this error is returned.
	ErrEphemeral = errors.New("store: refusing to persist an ephemeral message")
)

// Store is the top-level persistence handle. Repositories group related operations; the
// lifecycle methods manage the backend itself.
type Store interface {
	Settings() SettingsRepo
	Profiles() ProfileRepo
	Accounts() AccountRepo
	Channels() ChannelRepo
	Messages() MessageRepo
	Emotes() EmoteRepo

	// Migrate brings the schema to the current version. Safe to call on every startup.
	Migrate(ctx context.Context) error
	// Ping verifies connectivity (used by the settings backend-switch flow).
	Ping(ctx context.Context) error
	// Close releases the backend.
	Close() error
}

// ---- Settings ----

// Setting is a single-row JSON document per scope ("app", "appearance", "storage", …).
type Setting struct {
	Scope     string          `json:"scope"`
	Data      json.RawMessage `json:"data"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// SettingsRepo stores per-scope settings documents.
type SettingsRepo interface {
	Get(ctx context.Context, scope string) (Setting, error) // ErrNotFound if unset
	Put(ctx context.Context, s Setting) error               // upsert
	All(ctx context.Context) ([]Setting, error)
}

// ---- Profiles ----

// Profile is a saved workspace. The full ProfileDoc (channels/filters/layouts/…) lives in
// Doc as JSON, versioned and interpreted by the engine — the store treats it as
// opaque except for Name/IsDefault which it indexes.
type Profile struct {
	ID        string          `json:"id"` // ULID
	Name      string          `json:"name"`
	Doc       json.RawMessage `json:"doc"`
	IsDefault bool            `json:"is_default"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ProfileRepo manages saved workspaces.
type ProfileRepo interface {
	Create(ctx context.Context, name string, doc json.RawMessage) (Profile, error) // ErrConflict on duplicate name
	Get(ctx context.Context, id string) (Profile, error)
	GetByName(ctx context.Context, name string) (Profile, error)
	List(ctx context.Context) ([]Profile, error)
	Update(ctx context.Context, id string, doc json.RawMessage) (Profile, error)
	Delete(ctx context.Context, id string) error
	// SetDefault marks id as the default and clears the flag on all others (atomic).
	SetDefault(ctx context.Context, id string) error
	// Default returns the default profile, or ErrNotFound if none is set.
	Default(ctx context.Context) (Profile, error)
}

// ---- Accounts ----

// Account is a connected platform identity. Tokens never live here — SecretRef points at
// the OS keychain entry; the DB stores only the reference.
type Account struct {
	ID          string            `json:"id"` // ULID
	Platform    platform.Platform `json:"platform"`
	PlatformUID string            `json:"platform_uid"`
	Login       string            `json:"login"`
	DisplayName string            `json:"display_name"`
	SecretRef   string            `json:"secret_ref"` // keychain key; "" for sessionless (e.g. X)
	Scopes      []string          `json:"scopes"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// AccountRepo manages connected accounts. Multiple accounts per platform are allowed
// (keyed by (platform, platform_uid)); "send as" identity selection builds on this.
type AccountRepo interface {
	Upsert(ctx context.Context, a Account) (Account, error) // by (platform, platform_uid)
	Get(ctx context.Context, id string) (Account, error)
	List(ctx context.Context) ([]Account, error)
	ListByPlatform(ctx context.Context, p platform.Platform) ([]Account, error)
	Delete(ctx context.Context, id string) error // caller also deletes the keychain entry
}

// ---- Channels ----

// Channel is known-channel metadata. Meta holds platform-specific JSON — crucially the Kick
// chatroom id, cached forever once resolved.
type Channel struct {
	ID          string            `json:"id"` // ULID
	Platform    platform.Platform `json:"platform"`
	PlatformID  string            `json:"platform_id"`
	Slug        string            `json:"slug"`
	DisplayName string            `json:"display_name"`
	Meta        json.RawMessage   `json:"meta"`
	LastSeenAt  time.Time         `json:"last_seen_at"`
}

// ChannelRepo manages known-channel metadata.
type ChannelRepo interface {
	Upsert(ctx context.Context, c Channel) (Channel, error) // by (platform, slug)
	GetBySlug(ctx context.Context, p platform.Platform, slug string) (Channel, error)
	List(ctx context.Context) ([]Channel, error)
	Delete(ctx context.Context, id string) error
}

// ---- Messages (opt-in logging) ----

// StoredMessage is the persisted form of a message — the read model for history/search.
type StoredMessage struct {
	ID         string               `json:"id"` // engine ULID
	ChannelID  string               `json:"channel_id"`
	Platform   platform.Platform    `json:"platform"`
	Type       platform.MessageType `json:"type"`
	AuthorUID  string               `json:"author_uid"`
	AuthorName string               `json:"author_name"`
	Body       string               `json:"body"`     // rendered plain text
	Segments   json.RawMessage      `json:"segments"` // []platform.Segment
	SentAt     time.Time            `json:"sent_at"`
	ReceivedAt time.Time            `json:"received_at"`
	Deleted    bool                 `json:"deleted"`
}

// HistoryQuery selects a page of logged messages, newest-first before a cursor.
type HistoryQuery struct {
	ChannelID string // required
	Before    string // ULID cursor; "" = most recent
	Limit     int    // page size (implementation clamps to a sane max)
}

// SearchQuery selects logged messages whose body matches Text, newest-first before a cursor,
// optionally narrowed to one channel and/or author. Backends use a full-text index (SQLite FTS5,
// Postgres tsvector), so matching is by token/prefix rather than raw substring.
type SearchQuery struct {
	Text      string // full-text terms (required; empty returns no rows)
	ChannelID string // "" = across every logged channel
	Author    string // "" = any; matches the author's uid or display name
	Before    string // ULID cursor; "" = most recent match
	Limit     int    // page size (implementation clamps to a sane max)
}

// MessageRepo persists and queries logged messages. Active only when logging is enabled;
// see the package doc for the Ephemeral choke point.
type MessageRepo interface {
	// Append persists messages. It MUST return ErrEphemeral (and write nothing from the
	// batch) if any message is marked Ephemeral — this invariant: logging off means nothing is written.
	Append(ctx context.Context, msgs []platform.UnifiedMessage) error
	History(ctx context.Context, q HistoryQuery) ([]StoredMessage, error)
	// Search returns logged messages whose body matches q.Text, newest-first, optionally narrowed
	// to a channel and/or author. Backed by a full-text index so it stays fast on large logs.
	Search(ctx context.Context, q SearchQuery) ([]StoredMessage, error)
	// MarkDeleted flags a logged message as deleted by its engine ULID. The engine
	// resolves platform deletion ids to ULIDs via its per-channel map before calling this.
	MarkDeleted(ctx context.Context, id string) error
	// Sweep deletes messages in a channel older than the cutoff; returns the count removed.
	Sweep(ctx context.Context, channelID string, olderThan time.Time) (int, error)
}

// ---- Emote cache ----

// EmoteSet is a cached, resolved emote set keyed "provider:scope" (e.g. "7tv:twitch:12345").
type EmoteSet struct {
	Key       string          `json:"key"`
	Data      json.RawMessage `json:"data"` // []platform.EmoteRef
	FetchedAt time.Time       `json:"fetched_at"`
}

// EmoteFile indexes a cached emote/badge image on disk (the bytes live in the cache dir).
type EmoteFile struct {
	URLHash   string    `json:"url_hash"`
	Path      string    `json:"path"`
	Bytes     int64     `json:"bytes"`
	FetchedAt time.Time `json:"fetched_at"`
}

// EmoteRepo caches resolved emote sets and an index of on-disk image files.
type EmoteRepo interface {
	PutSet(ctx context.Context, s EmoteSet) error
	GetSet(ctx context.Context, key string) (EmoteSet, error)
	PutFile(ctx context.Context, f EmoteFile) error
	GetFile(ctx context.Context, urlHash string) (EmoteFile, error)
}
