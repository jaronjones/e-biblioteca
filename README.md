# e-biblioteca

A self-hosted digital library for ebooks, comics, and audiobooks.

**Stack:** Go · templ · HTMX · vanilla JavaScript · PostgreSQL · Docker

## Features

- Multi-user accounts with permissions (admin, upload, download, metadata, library)
- Libraries with filesystem scan under a books root
- BookDrop watched folder with review-before-import
- Metadata extraction (EPUB/CBZ) plus Open Library and Google Books lookup
- Cover grid, full-text search, shelves, and rule-based magic shelves
- In-browser readers: EPUB, PDF, CBZ, and HTML5 audio
- Per-user reading progress
- OPDS 1.2 catalog for compatible clients
- User-selectable themes (DaisyUI-compatible token packs via `data-theme`)

### Formats

| Category | Formats |
| --- | --- |
| eBooks | EPUB |
| Documents | PDF |
| Comics | CBZ |
| Audiobooks | M4B, M4A, MP3, OPUS |

## Quick start (Docker)

```bash
cp .env.example .env
# edit POSTGRES_PASSWORD
docker compose up -d --build
```

Open `http://localhost:8080` (or the host port from `HTTP_PORT` in `.env`).

1. Complete first-run setup (admin account)
2. Create a library (path relative to `/books`, e.g. `fiction`)
3. Put files in `./books/fiction` and click **Scan**, or use **Upload** / BookDrop

Volumes:

- `./books` → library files
- `./bookdrop` → inbound review queue
- `./data` → covers and app data

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `HTTP_PORT` | `8080` | Host port published by Compose |
| `DATABASE_URL` | (compose) | Postgres connection string |
| `DATA_DIR` | `/data` | Covers and app files |
| `BOOKS_DIR` | `/books` | Library root |
| `BOOKDROP_DIR` | `/bookdrop` | Drop folder |
| `SECURE_COOKIES` | `false` | Set `true` behind HTTPS |

Sessions are stored in Postgres (cookie holds only the session ID).

## Local development

Requirements: Go 1.22+, PostgreSQL 16, [templ](https://templ.guide).

```bash
# start postgres (example)
docker compose up -d db

export DATABASE_URL=postgres://ebiblioteca:ebiblioteca@localhost:5432/ebiblioteca?sslmode=disable
export DATA_DIR=./data BOOKS_DIR=./books BOOKDROP_DIR=./bookdrop HTTP_PORT=8080

make run
```

## OPDS

Create an OPDS user under **Settings**. Point a client at:

```
http://localhost:8080/opds
```

Use the OPDS username/password (HTTP basic auth).

## Themes

Themes live in `web/static/css/themes.css` as CSS custom property packs. UI components in `base.css` use only semantic tokens (`--color-base-100`, `--color-primary`, etc.) so themes swap via `data-theme` without rewriting markup.

## License

MIT
