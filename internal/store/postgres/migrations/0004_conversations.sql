-- Migration 0004: AI conversation history (Postgres variant).
CREATE TABLE IF NOT EXISTS conversations (
    id         TEXT   PRIMARY KEY,
    user_id    TEXT   NOT NULL DEFAULT '',
    title      TEXT   NOT NULL DEFAULT 'New conversation',
    messages   TEXT   NOT NULL DEFAULT '[]',
    model      TEXT   NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL DEFAULT 0,
    updated_at BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_conversations_user ON conversations (user_id, updated_at DESC);
