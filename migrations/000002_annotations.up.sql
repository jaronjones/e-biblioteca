CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS annotations (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id      BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('highlight', 'note', 'bookmark')),
    color        TEXT,
    quote        TEXT NOT NULL DEFAULT '',
    note         TEXT NOT NULL DEFAULT '',
    tags         TEXT[] NOT NULL DEFAULT '{}',
    anchor       JSONB NOT NULL,
    sort_key     TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS annotations_user_book_idx ON annotations (user_id, book_id);
CREATE INDEX IF NOT EXISTS annotations_user_created_idx ON annotations (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS annotations_tags_idx ON annotations USING GIN (tags);
CREATE INDEX IF NOT EXISTS annotations_quote_trgm ON annotations USING GIN (quote gin_trgm_ops);
CREATE INDEX IF NOT EXISTS annotations_note_trgm ON annotations USING GIN (note gin_trgm_ops);
