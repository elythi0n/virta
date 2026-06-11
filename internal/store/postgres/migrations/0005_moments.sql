-- Migration 0005: detected hype moments (Postgres variant).
CREATE TABLE IF NOT EXISTS moments (
    id          TEXT             PRIMARY KEY,
    channel_key TEXT             NOT NULL DEFAULT '',
    platform    TEXT             NOT NULL DEFAULT '',
    slug        TEXT             NOT NULL DEFAULT '',
    started_at  BIGINT           NOT NULL DEFAULT 0,
    ended_at    BIGINT           NOT NULL DEFAULT 0,
    peak_rate   DOUBLE PRECISION NOT NULL DEFAULT 0,
    baseline    DOUBLE PRECISION NOT NULL DEFAULT 0,
    excerpt     TEXT             NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_moments_channel ON moments (channel_key, id DESC);
