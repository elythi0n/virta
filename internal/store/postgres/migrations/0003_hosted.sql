-- Migration 0003: hosted multi-user mode tables (Postgres variant).
CREATE TABLE IF NOT EXISTS users (
    id           TEXT PRIMARY KEY,
    email        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    created_at   BIGINT NOT NULL DEFAULT 0,
    updated_at   BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS sessions (
    token       TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  BIGINT NOT NULL,
    created_at  BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sessions_user    ON sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions (expires_at);

ALTER TABLE profiles   ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE channels   ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE settings   ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE messages   ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE emote_sets ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts   ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_profiles_user ON profiles (user_id);
CREATE INDEX IF NOT EXISTS idx_channels_user ON channels (user_id, platform, slug);
CREATE INDEX IF NOT EXISTS idx_settings_user ON settings (user_id, scope);
CREATE INDEX IF NOT EXISTS idx_messages_user  ON messages (user_id, channel_id, id);
