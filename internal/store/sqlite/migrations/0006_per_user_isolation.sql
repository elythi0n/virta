-- Migration 0006: complete the multi-tenancy story for settings + moments.
-- Single-user installs keep working because user_id stays the empty string ('') for every row
-- the daemon writes outside hosted mode — the new queries that filter by uid(ctx) bind '' in
-- that mode and match the existing rows transparently.

-- ── settings: re-primary-key by (user_id, scope) ──────────────────────────────────────────────
-- The original PK was just `scope`, which prevented two hosted users from owning the same
-- scope (e.g. each user's own webhooks.foo). SQLite can't rewrite a PRIMARY KEY in place;
-- standard recipe is rename → recreate → re-insert → drop.
ALTER TABLE settings RENAME TO settings_old;

CREATE TABLE settings (
    user_id    TEXT    NOT NULL DEFAULT '',
    scope      TEXT    NOT NULL,
    data       TEXT    NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, scope)
);

-- Preserve every existing row verbatim. user_id was added in 0003 (default '') so this is a
-- direct copy; the empty user_id continues to be the single-user namespace.
INSERT INTO settings (user_id, scope, data, updated_at)
    SELECT user_id, scope, data, updated_at FROM settings_old;

DROP TABLE settings_old;

-- The original idx_settings_user (from 0003) was dropped with the table; recreate it.
CREATE INDEX IF NOT EXISTS idx_settings_user ON settings (user_id, scope);

-- ── moments: gain user_id ─────────────────────────────────────────────────────────────────────
-- Moments captured before this migration belong to the implicit single-user namespace ('').
ALTER TABLE moments ADD COLUMN user_id TEXT NOT NULL DEFAULT '';

-- Replace the existing single-index (channel_key, id) with one that leads on user_id, so the
-- common query "list this user's moments for this channel" stays cheap. SQLite keeps the old
-- index too; we add the new one alongside it.
CREATE INDEX IF NOT EXISTS idx_moments_user ON moments (user_id, channel_key, id DESC);
