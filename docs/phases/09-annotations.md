# Phase 9 — Annotations, highlights & bookmarks

## Scope

Private, multi-format annotations that match or exceed browser-based peers
(Calibre viewer / Kavita / BookLore):

- EPUB CFI highlights + notes + location pins
- PDF page bookmarks + text highlights (pdf.js upgrade)
- CBZ page pins/notes
- Audiobook timestamp pins
- In-reader panel, deep links, book badge
- Global `/annotations` browser with search/filters
- Markdown export (per-book + bulk concatenated, Obsidian-ready)

**PRD:** `tasks/prd-annotations.md` (locked via Lavish review)

## Non-goals (this phase)

Sharing, e-reader sync, token API, write-back into files, Calibre/KOReader import,
native mobile apps (PWA later).

## Primary packages

- `internal/db/migrations/000002_annotations.*`
- `internal/models` — `Annotation`, limits, colors
- `internal/store/annotations.go` (+ unit tests for validation)
- `internal/http/handlers/annotations.go` — CRUD API, page, export
- `web/static/js/readers/annotations.js` — shared panel/API
- `web/static/js/readers/{epub,pdf,cbz,audio}.js`
- `components/pages.templ` — reader chrome, `AnnotationsPage`, book badge
- `web/static/css/base.css` — annotation UI

## Routes

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/annotations` | Global browser |
| GET | `/annotations/export` | Bulk Markdown |
| GET | `/books/{id}/annotations/export` | Per-book Markdown |
| GET | `/api/books/{id}/annotations` | List for book |
| POST | `/api/books/{id}/annotations` | Create |
| PATCH | `/api/annotations/{id}` | Update note/color/tags |
| DELETE | `/api/annotations/{id}` | Delete |

Deep link: `/read/{id}?annotation={annotationId}`

## Decisions (locked)

- Soft cap 5,000 annotations / user / book
- Colors: yellow, green, blue, pink, purple
- Notes: plain text only
- Bulk export: single concatenated `.md`
- EPUB `sort_key`: percent from locations at save time
- Audio labels: `h:mm:ss` (+ optional note)
- PDF: pdf.js in this phase

## Verify

```bash
make generate
go test ./internal/store/
docker compose up -d --build   # applies migration 000002
# Login → open EPUB → select text → highlight → open panel → export MD
# /annotations search and filter
```
