-- Migration 0005: detected hype moments (chat-activity spikes bookmarked by the moments detector).
CREATE TABLE IF NOT EXISTS moments (
    id          TEXT    PRIMARY KEY,
    channel_key TEXT    NOT NULL DEFAULT '',
    platform    TEXT    NOT NULL DEFAULT '',
    slug        TEXT    NOT NULL DEFAULT '',
    started_at  INTEGER NOT NULL DEFAULT 0,
    ended_at    INTEGER NOT NULL DEFAULT 0,
    peak_rate   REAL    NOT NULL DEFAULT 0,
    baseline    REAL    NOT NULL DEFAULT 0,
    excerpt     TEXT    NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_moments_channel ON moments (channel_key, id DESC);
