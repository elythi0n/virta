-- Full-text search over logged message bodies. An external-content FTS5 table shadows the
-- messages table (indexing only `body`, keyed by the implicit rowid), kept in sync by triggers so
-- search never drifts from the log. MessageRepo.Search MATCHes this table and joins back for the
-- full row.
CREATE VIRTUAL TABLE messages_fts USING fts5(body, content='messages', content_rowid='rowid');

-- Backfill any rows already logged before this migration.
INSERT INTO messages_fts (rowid, body) SELECT rowid, body FROM messages;

CREATE TRIGGER messages_fts_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts (rowid, body) VALUES (new.rowid, new.body);
END;

CREATE TRIGGER messages_fts_ad AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts (messages_fts, rowid, body) VALUES ('delete', old.rowid, old.body);
END;

CREATE TRIGGER messages_fts_au AFTER UPDATE ON messages BEGIN
    INSERT INTO messages_fts (messages_fts, rowid, body) VALUES ('delete', old.rowid, old.body);
    INSERT INTO messages_fts (rowid, body) VALUES (new.rowid, new.body);
END;
