CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    display_name    TEXT NOT NULL DEFAULT '',
    email           TEXT UNIQUE,
    is_admin        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_permissions (
    user_id         BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    can_upload      BOOLEAN NOT NULL DEFAULT FALSE,
    can_download    BOOLEAN NOT NULL DEFAULT TRUE,
    can_edit_metadata BOOLEAN NOT NULL DEFAULT FALSE,
    can_manage_library BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE user_settings (
    user_id         BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    theme           TEXT NOT NULL DEFAULT 'dark',
    settings_json   JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BYTEA NOT NULL,
    expiry TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

CREATE TABLE libraries (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    icon        TEXT NOT NULL DEFAULT 'book',
    watch       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE library_paths (
    id          BIGSERIAL PRIMARY KEY,
    library_id  BIGINT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    UNIQUE (library_id, path)
);

CREATE TABLE books (
    id              BIGSERIAL PRIMARY KEY,
    library_id      BIGINT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    library_path_id BIGINT REFERENCES library_paths(id) ON DELETE SET NULL,
    file_name       TEXT NOT NULL,
    file_sub_path   TEXT NOT NULL DEFAULT '',
    format          TEXT NOT NULL,
    file_size       BIGINT NOT NULL DEFAULT 0,
    file_hash       TEXT,
    deleted         BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_at      TIMESTAMPTZ,
    added_on        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (library_id, file_sub_path, file_name)
);
CREATE INDEX books_library_idx ON books (library_id) WHERE deleted = FALSE;
CREATE INDEX books_format_idx ON books (format);

CREATE TABLE book_metadata (
    book_id         BIGINT PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
    title           TEXT,
    subtitle        TEXT,
    description     TEXT,
    publisher       TEXT,
    published_date  DATE,
    isbn10          TEXT,
    isbn13          TEXT,
    page_count      INT,
    language        TEXT,
    series_name     TEXT,
    series_number   REAL,
    series_total    INT,
    cover_path      TEXT,
    rating          REAL,
    search_vector   tsvector
);
CREATE INDEX book_metadata_search_idx ON book_metadata USING GIN (search_vector);
CREATE INDEX book_metadata_title_trgm ON book_metadata USING GIN (title gin_trgm_ops);

CREATE TABLE authors (
    id   BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE book_authors (
    book_id   BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    author_id BIGINT NOT NULL REFERENCES authors(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, author_id)
);

CREATE TABLE categories (
    id   BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE book_categories (
    book_id     BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    category_id BIGINT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, category_id)
);

CREATE TABLE shelves (
    id       BIGSERIAL PRIMARY KEY,
    user_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name     TEXT NOT NULL,
    icon     TEXT NOT NULL DEFAULT 'bookmark',
    UNIQUE (user_id, name)
);

CREATE TABLE shelf_books (
    shelf_id BIGINT NOT NULL REFERENCES shelves(id) ON DELETE CASCADE,
    book_id  BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (shelf_id, book_id)
);

CREATE TABLE magic_shelves (
    id       BIGSERIAL PRIMARY KEY,
    user_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name     TEXT NOT NULL,
    icon     TEXT NOT NULL DEFAULT 'sparkles',
    rules    JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (user_id, name)
);

CREATE TABLE user_book_progress (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id     BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    percent     REAL NOT NULL DEFAULT 0,
    position    JSONB NOT NULL DEFAULT '{}'::jsonb,
    status      TEXT NOT NULL DEFAULT 'unread',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, book_id)
);

CREATE TABLE bookdrop_files (
    id               BIGSERIAL PRIMARY KEY,
    path             TEXT NOT NULL UNIQUE,
    file_name        TEXT NOT NULL,
    format           TEXT,
    status           TEXT NOT NULL DEFAULT 'pending',
    metadata_json    JSONB NOT NULL DEFAULT '{}'::jsonb,
    cover_path       TEXT,
    error_message    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE opds_users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    user_id       BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE app_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE OR REPLACE FUNCTION book_metadata_search_update() RETURNS trigger AS $$
BEGIN
  NEW.search_vector :=
    setweight(to_tsvector('english', coalesce(NEW.title, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(NEW.subtitle, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(NEW.series_name, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(NEW.description, '')), 'C');
  RETURN NEW;
END
$$ LANGUAGE plpgsql;

CREATE TRIGGER book_metadata_search_trg
  BEFORE INSERT OR UPDATE ON book_metadata
  FOR EACH ROW EXECUTE FUNCTION book_metadata_search_update();
