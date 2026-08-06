# Roadmap

Feature development tracker, seeded from the August 2026 comparison against
Calibre, Kavita, Ubooquity, pyShelf, and BookLore (see
`.lavish/feature-gap-analysis.html` for the full matrix). Items are ordered by
impact for a self-hosted library. Update the **Status** column as work starts
and lands; carve items into `phase/N-*` branches following the existing
convention in `docs/phases/`.

**Status values:** `planned` · `next` · `in progress` · `shipped` · `dropped`

## Overview

| # | Feature | Effort | Status | Phase |
| --- | --- | --- | --- | --- |
| 1 | E-reader sync (Kobo + KOReader) | L | planned | — |
| 2 | Send-to-Kindle / email delivery | S | planned | — |
| 3 | CBR/CB7 + MOBI/FB2 format support | M | planned | — |
| 4 | Format conversion | L (M via sidecar) | planned | — |
| 5 | Annotations, highlights & bookmarks | M–L | in progress | phase/9-annotations |
| 6 | Full-text search inside books | M | planned | — |
| 7 | Reading statistics | S–M | planned | — |
| 8 | Bulk metadata edit + dedupe | M | planned | — |
| 9 | Field-level match review + more providers | M | planned | — |
| 10 | REST API + webhooks | M | planned | — |

## 1. E-reader sync (Kobo + KOReader) — `L`

Reading happens on e-readers; our per-user progress is trapped in the browser.
BookLore's headline feature; Kavita gates cross-device sync behind Kavita+.

- [ ] KOReader sync server endpoints (small REST spec: document hash → position)
- [ ] Map KOReader document hashes to `books.file_hash`
- [ ] Kobo store API compatibility layer (the big lift — separate slice)
- [ ] Surface synced progress in the web UI alongside browser progress

## 2. Send-to-Kindle / email delivery — `S`

Smallest high-impact item. Kindle accepts EPUB by email now, so the main use
case needs no conversion. Calibre, BookLore, and Kavita all have it.

- [ ] SMTP config (env vars + settings UI)
- [ ] Per-user device email address field
- [ ] "Send to device" action on the book page (permission-gated)
- [ ] Attachment size guard + send log

## 3. CBR/CB7 + MOBI/FB2 format support — `M`

Most real-world comic collections are CBR; every other comic-capable app in
the comparison reads it. Even pyShelf indexes MOBI.

- [ ] CBR via a Go unrar/unarr library behind the existing CBZ reader path
- [ ] CB7 (7z) same seam
- [ ] MOBI/FB2 as metadata + download only at first (no reader)
- [ ] Extend `SupportedExt`, scanner, and OPDS mime types

## 4. Format conversion — `L` (M via sidecar)

Calibre's signature capability. Enables Kindle-friendly downloads straight
from OPDS and the book page.

- [ ] Sidecar container shelling out to Calibre's `ebook-convert`
- [ ] On-demand conversion on download with cached results under `data/`
- [ ] Target-format preference per user

## 5. Annotations, highlights & bookmarks — `M–L`

Kavita lets readers highlight, note, share, and export to Obsidian. We have
nothing between "reading" and "finished".

PRD: `tasks/prd-annotations.md` (locked). Phase notes: `docs/phases/09-annotations.md`.

- [x] `annotations` table (kind, color, quote, note, tags, anchor JSONB, sort_key)
- [x] Session JSON API (list/create/patch/delete) + private isolation
- [x] epub.js CFI-anchored highlights + panel
- [x] PDF via pdf.js (page bookmarks + text highlights)
- [x] CBZ page bookmarks; audio timestamp bookmarks
- [x] Library-wide `/annotations` browser + search
- [x] Markdown / Obsidian export (per-book + bulk concatenated)

## 6. Full-text search inside books — `M`

Only Calibre has this. We're unusually well-placed: Postgres FTS is already
wired for metadata search.

- [ ] Extract EPUB XHTML text at scan time (strip tags, chapter granularity)
- [ ] Content tsvector table + GIN index (separate from metadata `search_vector`)
- [ ] PDF text via pdfcpu content streams (best-effort)
- [ ] Search UI: results grouped book → chapter with snippets

## 7. Reading statistics — `S–M`

BookLore and Kavita both ship this; loved by users and cheap for us since
progress events are already recorded per user.

- [ ] Stats page: books finished/year, pages or percent per day, streaks
- [ ] Per-library and per-format breakdowns
- [ ] Optional yearly reading goal

## 8. Bulk metadata edit + dedupe — `M`

Fixing a series name across 30 books is 30 form submissions today. Calibre
sets the bar with bulk edit and duplicate detection.

- [ ] Duplicates report from existing `books.file_hash` (nearly free)
- [ ] Multi-select in the book grid + bulk edit form (series, categories, language)
- [ ] Merge/ignore actions for duplicate pairs

## 9. Field-level match review + more providers — `M`

We auto-apply the first lookup result; BookLore shows a per-field diff picker
across Amazon, Goodreads, Hardcover, and Google. `metadata.Enrich` refactor
(Aug 2026) is the seam to build on.

- [ ] Hardcover provider (GraphQL API)
- [ ] Match UI: candidates side-by-side, pick per field instead of `results[0]`
- [ ] Provenance: record which provider each field came from
- [ ] Lookup cache so re-matching never re-fetches

## 10. REST API + webhooks — `M`

Kavita's API enables an ecosystem (dashboards, scripts, scrobbling). We expose
nothing beyond OPDS.

- [ ] Token-auth JSON API on the existing chi router (`/api/v1`)
- [ ] Cover books, shelves, progress, search
- [ ] Webhooks on import/finish → Discord/ntfy/generic POST

## Honorable mentions

Not ranked, revisit when the top 10 shrinks: want-to-read lists · reading
recommendations · age ratings / parental controls · OPDS-PSE page streaming ·
ebook editing · news-to-ebook recipes · audiobook tag parsing (our own
`m4b/m4a/mp3/opus` extraction stub is still empty — see `Extract` in
`internal/service/metadata/extract.go`).

## Strengths to defend

Capabilities where we already lead the field — don't regress them while
building the above:

- **Audiobook playback** — none of the five compared apps handle audio at all
- **BookDrop review queue** — only BookLore has an equivalent intake flow
- **Magic shelves** — on par with BookLore's magic shelves / Kavita's smart filters
- **Single Go binary + Postgres** — lighter than the Java/.NET alternatives
