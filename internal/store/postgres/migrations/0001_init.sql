-- Initial schema (Postgres). Mirrors the SQLite schema so both backends pass the same store
-- contract. Timestamps are int64 nanoseconds since the Unix epoch (0 = unset) → BIGINT, since
-- nanoseconds overflow a 32-bit INTEGER. JSON documents are stored as TEXT (the repo layer
-- treats them as opaque JSON; a JSONB refinement for indexing can come with the search layer).

CREATE TABLE settings (
    scope      TEXT PRIMARY KEY,
    data       TEXT   NOT NULL,
    updated_at BIGINT NOT NULL
);

CREATE TABLE profiles (
    id         TEXT PRIMARY KEY,
    name       TEXT   NOT NULL UNIQUE,
    doc        TEXT   NOT NULL,
    is_default BIGINT NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

-- Tokens are never stored here; secret_ref points at the OS keychain entry.
CREATE TABLE accounts (
    id           TEXT PRIMARY KEY,
    platform     TEXT   NOT NULL,
    platform_uid TEXT   NOT NULL,
    login        TEXT   NOT NULL DEFAULT '',
    display_name TEXT   NOT NULL DEFAULT '',
    secret_ref   TEXT   NOT NULL DEFAULT '',
    scopes       TEXT   NOT NULL DEFAULT '[]',
    created_at   BIGINT NOT NULL,
    updated_at   BIGINT NOT NULL,
    UNIQUE (platform, platform_uid)
);

-- meta holds platform-specific JSON, e.g. the Kick chatroom id (cached once resolved).
CREATE TABLE channels (
    id           TEXT PRIMARY KEY,
    platform     TEXT   NOT NULL,
    platform_id  TEXT   NOT NULL DEFAULT '',
    slug         TEXT   NOT NULL,
    display_name TEXT   NOT NULL DEFAULT '',
    meta         TEXT,
    last_seen_at BIGINT NOT NULL DEFAULT 0,
    UNIQUE (platform, slug)
);

-- Chat log. Empty unless the user turns logging on. id is the engine ULID, so ordering and
-- cursor pagination are pure lexicographic id comparisons.
CREATE TABLE messages (
    id          TEXT PRIMARY KEY,
    channel_id  TEXT   NOT NULL,
    platform    TEXT   NOT NULL,
    type        TEXT   NOT NULL,
    author_uid  TEXT   NOT NULL DEFAULT '',
    author_name TEXT   NOT NULL DEFAULT '',
    body        TEXT   NOT NULL DEFAULT '',
    segments    TEXT   NOT NULL DEFAULT '[]',
    sent_at     BIGINT NOT NULL DEFAULT 0,
    received_at BIGINT NOT NULL DEFAULT 0,
    deleted     BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_messages_channel_id ON messages (channel_id, id);

-- Disk-cache index for resolved emote/badge sets and image files.
CREATE TABLE emote_sets (
    key        TEXT PRIMARY KEY,
    data       TEXT   NOT NULL,
    fetched_at BIGINT NOT NULL
);

CREATE TABLE emote_files (
    url_hash   TEXT PRIMARY KEY,
    path       TEXT   NOT NULL,
    bytes      BIGINT NOT NULL,
    fetched_at BIGINT NOT NULL
);
