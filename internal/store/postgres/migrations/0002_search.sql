-- Full-text search over logged message bodies. A generated tsvector column tracks the body
-- automatically, and a GIN index keeps MessageRepo.Search fast on large logs. The 'english'
-- configuration must match the one websearch_to_tsquery uses at query time.
ALTER TABLE messages ADD COLUMN body_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('english', body)) STORED;

CREATE INDEX idx_messages_body_tsv ON messages USING GIN (body_tsv);
