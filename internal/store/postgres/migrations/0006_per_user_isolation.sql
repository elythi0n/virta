-- Migration 0006: complete the multi-tenancy story for settings + moments (Postgres variant).
-- See the sqlite sibling for the rationale; Postgres lets us swap a primary key in place via
-- ALTER TABLE, so the recipe is much shorter than SQLite's rename-and-recreate dance.

-- ── settings: re-primary-key by (user_id, scope) ──────────────────────────────────────────────
ALTER TABLE settings DROP CONSTRAINT IF EXISTS settings_pkey;
ALTER TABLE settings ADD PRIMARY KEY (user_id, scope);

-- ── moments: gain user_id ─────────────────────────────────────────────────────────────────────
ALTER TABLE moments ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_moments_user ON moments (user_id, channel_key, id DESC);
