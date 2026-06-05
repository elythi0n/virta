-- Initial schema. Timestamps are stored as int64 nanoseconds since the Unix epoch, or 0 to
-- mean "unset"/zero time. JSON documents (settings data, profile doc, channel meta, message
-- segments, account scopes, emote set data) are stored as TEXT.

CREATE TABLE settings (
    scope      TEXT PRIMARY KEY,
    data       TEXT    NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE profiles (
    id         TEXT PRIMARY KEY,
    name       TEXT    NOT NULL UNIQUE,
    doc        TEXT    NOT NULL,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- Tokens are never stored here; secret_ref points at the OS keychain entry.
CREATE TABLE accounts (
    id           TEXT PRIMARY KEY,
    platform     TEXT    NOT NULL,
    platform_uid TEXT    NOT NULL,
    login        TEXT    NOT NULL DEFAULT '',
    display_name TEXT    NOT NULL DEFAULT '',
    secret_ref   TEXT    NOT NULL DEFAULT '',
    scopes       TEXT    NOT NULL DEFAULT '[]',
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    UNIQUE (platform, platform_uid)
);

-- meta holds platform-specific JSON, e.g. the Kick chatroom id (cached once resolved).
CREATE TABLE channels (
    id           TEXT PRIMARY KEY,
    platform     TEXT    NOT NULL,
    platform_id  TEXT    NOT NULL DEFAULT '',
    slug         TEXT    NOT NULL,
    display_name TEXT    NOT NULL DEFAULT '',
    meta         TEXT,
    last_seen_at INTEGER NOT NULL DEFAULT 0,
    UNIQUE (platform, slug)
);

-- Chat log. Empty unless the user turns logging on. id is the engine ULID, so ordering and
-- cursor pagination are pure lexicographic id comparisons.
CREATE TABLE messages (
    id          TEXT PRIMARY KEY,
    channel_id  TEXT    NOT NULL,
    platform    TEXT    NOT NULL,
    type        TEXT    NOT NULL,
    author_uid  TEXT    NOT NULL DEFAULT '',
    author_name TEXT    NOT NULL DEFAULT '',
    body        TEXT    NOT NULL DEFAULT '',
    segments    TEXT    NOT NULL DEFAULT '[]',
    sent_at     INTEGER NOT NULL DEFAULT 0,
    received_at INTEGER NOT NULL DEFAULT 0,
    deleted     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_messages_channel_id ON messages (channel_id, id);

-- Disk-cache index for resolved emote/badge sets and image files.
CREATE TABLE emote_sets (
    key        TEXT PRIMARY KEY,
    data       TEXT    NOT NULL,
    fetched_at INTEGER NOT NULL
);

CREATE TABLE emote_files (
    url_hash   TEXT PRIMARY KEY,
    path       TEXT    NOT NULL,
    bytes      INTEGER NOT NULL,
    fetched_at INTEGER NOT NULL
);
