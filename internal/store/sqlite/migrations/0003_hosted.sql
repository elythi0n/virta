-- Migration 0003: hosted multi-user mode tables.
-- These tables are created in every deployment but are only *used* when VIRTA_HOSTED=1;
-- in local single-user mode the users table stays empty and all store operations use
-- the empty-string user_id (""), which is the implicit single-user namespace.

-- users: one row per registered account.
CREATE TABLE IF NOT EXISTS users (
    id           TEXT PRIMARY KEY,       -- ULID
    email        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,          -- argon2id hash
    created_at   INTEGER NOT NULL DEFAULT 0,
    updated_at   INTEGER NOT NULL DEFAULT 0
);

-- sessions: bearer tokens issued at login. Expired rows are pruned by the sweeper.
CREATE TABLE IF NOT EXISTS sessions (
    token       TEXT PRIMARY KEY,         -- sha256 of the presented secret
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  INTEGER NOT NULL,
    created_at  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions (expires_at);

-- Add user_id column to profiles so each user has their own workspace set.
-- NULL user_id = the local single-user installation's profiles.
ALTER TABLE profiles    ADD COLUMN user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE channels    ADD COLUMN user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE settings    ADD COLUMN user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE messages    ADD COLUMN user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE emote_sets  ADD COLUMN user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts    ADD COLUMN user_id TEXT NOT NULL DEFAULT '';

-- Re-index the tables that need per-user scoping.
CREATE INDEX IF NOT EXISTS idx_profiles_user ON profiles (user_id);
CREATE INDEX IF NOT EXISTS idx_channels_user ON channels (user_id, platform, slug);
CREATE INDEX IF NOT EXISTS idx_settings_user ON settings (user_id, scope);
CREATE INDEX IF NOT EXISTS idx_messages_user  ON messages (user_id, channel_id, id);
