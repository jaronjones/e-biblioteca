# e-biblioteca

Self-hosted digital library (Go · templ · HTMX · PostgreSQL · Docker).

## Phase 0 — Scaffold

- Go module and chi-ready server entrypoint
- Env-based config (`HTTP_PORT`, `DATABASE_URL`, paths)
- Postgres migrations on boot
- Docker Compose (app + Postgres 16)
- Theme token CSS + base layout styles
- `/healthz`

Later phases add auth, catalog, metadata, BookDrop, shelves, readers, and OPDS.
