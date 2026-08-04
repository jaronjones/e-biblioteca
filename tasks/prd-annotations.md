# PRD: Annotations, Highlights & Bookmarks

## Introduction / Overview

e-biblioteca already has multi-format in-browser readers (EPUB, PDF, CBZ, audio) and per-user reading progress, but nothing between "reading" and "finished." Popular self-hosted peers either ship mature annotation systems (Calibre desktop viewer, BookLore built-in reader, Kavita highlights + notes + Obsidian export) or leave a painful gap (Calibre-Web browser reader, many comic servers).

This feature adds **first-class, private, multi-format annotations** so readers can highlight text, attach notes, place bookmarks, review them across the library, jump back into context, and export to Markdown/Obsidian-ready files. The product bar is **comparable to or better than** Calibre viewer, Kavita, and BookLore for browser-based annotation - not a thin MVP.

**Scope stance (from product decisions):**

| Decision | Choice |
| --- | --- |
| Ambition | Exceed competitor parity |
| Formats (v1) | EPUB full text annotations; PDF page + text where feasible; CBZ page bookmarks/notes; audiobook timestamp bookmarks |
| Visibility | Private per user only |
| Export | Markdown / Obsidian (per-book and bulk) |
| Device sync | Out of scope for this PRD (browser readers only) |
| Mobile apps | Out of scope for v1; explicitly reserved as a later phase (see Future) |

---

## Goals

- Let every authenticated reader create, edit, delete, and jump to personal highlights, notes, and bookmarks while reading.
- Support **EPUB** with CFI-anchored text highlights and notes (parity with epub.js / Calibre / Kavita).
- Support **PDF** with page bookmarks and text highlights when the PDF text layer is available; always support page-level notes.
- Support **CBZ** with page bookmarks and optional page notes (comics rarely have selectable text).
- Support **audiobooks** with timestamp bookmarks and optional note text.
- Provide an in-reader annotation panel (list, filter, jump) and a **library-wide annotations browser** (Calibre "browse annotations" class feature).
- Make annotations **searchable** (quote text, note body, tags) across the user's library.
- Export per-book and bulk annotations as high-quality **Markdown with YAML frontmatter** suitable for Obsidian (and plain MD note tools).
- Persist everything server-side, private to the creating user, with stable anchors that survive reader reloads and progress updates.
- Stay consistent with the stack: Go handlers, Postgres, templ/HTMX shell, vanilla JS readers, DaisyUI-compatible tokens.

---

## Competitive bar (what "exceed" means)

| Capability | Calibre viewer | Kavita | BookLore | e-biblioteca (this PRD) |
| --- | --- | --- | --- | --- |
| EPUB text highlight + note | Yes | Yes | Yes | Yes |
| PDF page / text annotate | Yes (desktop) | Limited / improving | Yes | Yes (page always; text when layer exists) |
| Comic page bookmark / note | N/A | Limited | Partial | Yes (page + note) |
| Audiobook timestamp marks | No | No | No | **Yes (differentiator)** |
| Multi-color highlights | Yes | Yes | Yes | Yes (preset palette) |
| Tags on annotations | Weak | Partial | Partial | **Yes** |
| In-reader list + jump | Yes | Yes | Yes | Yes |
| Library-wide annotation browser | Yes | Weak | Weak | **Yes** |
| Full-text search over quotes/notes | Weak | Partial | Partial | **Yes (FTS/trigram)** |
| Markdown / Obsidian export | Yes (via tools) | Yes (+ community plugins) | Partial | **Yes (first-class, bulk)** |
| Share / social | No | Yes (some) | Some | No (explicit non-goal) |
| Device / KOReader / Kobo annotation sync | Partial (content server) | Partial | Strong | **No (this PRD)** |

---

## User Stories

### US-001: Persist annotations in Postgres
**Description:** As a developer, I need a durable multi-format annotation model so every reader can store private highlights, notes, and bookmarks.

**Acceptance Criteria:**
- [ ] New migration creates `annotations` table (see Technical Considerations for columns)
- [ ] Foreign keys: `user_id` → `users`, `book_id` → `books` with `ON DELETE CASCADE`
- [ ] Indexes: `(user_id, book_id)`, `(user_id, created_at DESC)`, GIN/trigram or tsvector for quote+note search
- [ ] Soft constraints: `kind` enum-like check (`highlight`, `note`, `bookmark`); `color` limited to allowed palette values or null for bookmarks
- [ ] Migration up/down runs cleanly on empty and existing DBs
- [ ] Unit/store tests cover create, update, delete, list-by-book, list-global

### US-002: Annotation REST/JSON API for readers
**Description:** As the reader UI, I need authenticated CRUD endpoints so annotations can be saved without full page reloads.

**Acceptance Criteria:**
- [ ] `GET /api/books/{id}/annotations` returns current user's annotations for that book, ordered by document position then `created_at`
- [ ] `POST /api/books/{id}/annotations` creates; validates kind, color, anchor payload for book format
- [ ] `PATCH /api/annotations/{id}` updates note text, color, tags, quote (if still valid); cannot change book or owner
- [ ] `DELETE /api/annotations/{id}` removes permanently (hard delete is fine for private data)
- [ ] All endpoints require session auth; another user always gets 404 (no existence leak)
- [ ] Invalid anchor shape for format returns 400 with clear error message
- [ ] Typecheck/lint passes

### US-003: EPUB text highlight and note (CFI)
**Description:** As a reader, I want to select text in an EPUB and highlight it (optionally with a note) so I can capture passages while reading.

**Acceptance Criteria:**
- [ ] Selecting text in the EPUB reader shows a floating toolbar: highlight colors + "Add note" + "Bookmark here"
- [ ] Highlight stores EPUB CFI range (`anchor.cfi` start/end or range CFI), selected quote text, color, optional note
- [ ] Highlights re-render after load and on chapter navigation via epub.js annotations API (or equivalent)
- [ ] Clicking an existing highlight opens edit popover (change color, edit note, delete)
- [ ] Creating a highlight does not break progress save (`position.cfi` still works)
- [ ] Works in paginated flow used by current `web/static/js/readers/epub.js`
- [ ] Verify in browser using dev-browser skill

### US-004: EPUB bookmark without selection
**Description:** As a reader, I want to bookmark the current location in an EPUB without selecting text so I can return to a place that is not a quote.

**Acceptance Criteria:**
- [ ] Toolbar or keyboard action creates a bookmark at current CFI
- [ ] Bookmark appears in the in-reader list and jumps to that CFI on click
- [ ] Optional short note on bookmark
- [ ] Verify in browser using dev-browser skill

### US-005: PDF page bookmarks and text highlights
**Description:** As a reader, I want page bookmarks and, when text is selectable, highlights in PDFs so technical manuals and papers are usable.

**Acceptance Criteria:**
- [ ] Bookmark current page action stores `anchor.page` (1-based) and optional note
- [ ] When PDF text layer is available, selection creates highlight with page + text offsets or geometry sufficient to re-draw
- [ ] When text layer is missing/scanned, user can still place page note/bookmark (no failed highlight attempt without feedback)
- [ ] Jump from list navigates to correct page (and scrolls to highlight when possible)
- [ ] Coexists with existing PDF iframe/reader progress percent
- [ ] Verify in browser using dev-browser skill

### US-006: CBZ page bookmarks and page notes
**Description:** As a comic reader, I want to bookmark a page and attach a short note so I can mark panels, arcs, or art I care about.

**Acceptance Criteria:**
- [ ] Bookmark / note on current CBZ page stores `anchor.page` matching stream page numbering
- [ ] No requirement for text selection (comics)
- [ ] Optional color marker on page bookmark for visual scanning in the list
- [ ] Jump opens the correct page in `cbz.js`
- [ ] Verify in browser using dev-browser skill

### US-007: Audiobook timestamp bookmarks
**Description:** As a listener, I want timestamp bookmarks with optional notes so I can mark spoken passages (a gap in peer apps).

**Acceptance Criteria:**
- [ ] "Bookmark here" in audio player stores `anchor.seconds` (float) and optional note
- [ ] List shows human time (`h:mm:ss`) and jumps by seeking audio element
- [ ] Survives pause/resume and page reload
- [ ] Verify in browser using dev-browser skill

### US-008: In-reader annotations panel
**Description:** As a reader, I want a side panel listing annotations for the open book so I can filter and jump without leaving the reader.

**Acceptance Criteria:**
- [ ] Panel toggle in reader chrome (EPUB/PDF/CBZ/audio)
- [ ] Groups or sorts by document order; shows color chip, kind icon, quote snippet or note, relative time
- [ ] Filter chips: All | Highlights | Notes | Bookmarks; optional filter by color
- [ ] Click item jumps to anchor; delete available with confirm for non-empty notes
- [ ] Empty state copy when book has no annotations
- [ ] Panel uses existing theme tokens (`--color-*`); readable in all shipped themes
- [ ] Verify in browser using dev-browser skill

### US-009: Library-wide annotations browser
**Description:** As a user, I want one page that lists my annotations across the whole library so I can review reading notes like Calibre's "Browse annotations."

**Acceptance Criteria:**
- [ ] Route e.g. `/annotations` in main nav for logged-in users
- [ ] Each row: cover thumb, book title, kind, color, quote/note excerpt, tags, created date
- [ ] Filters: library, format, kind, color, tag, date range; free-text search
- [ ] Pagination or infinite-friendly limit (default 50)
- [ ] Click navigates to reader at annotation (deep link with annotation id or anchor query)
- [ ] Empty state explains how to create first highlight
- [ ] Verify in browser using dev-browser skill

### US-010: Search annotations (quote + note + tags)
**Description:** As a user, I want to search my annotation text so I can find a remembered phrase or note without opening each book.

**Acceptance Criteria:**
- [ ] Search box on `/annotations` queries quote, note body, and tags for the current user only
- [ ] Ranking prefers quote matches over note matches when both exist
- [ ] Search works with partial words (trigram or equivalent), not only exact phrase
- [ ] No other user's annotations ever appear
- [ ] Typecheck/lint passes
- [ ] Verify in browser using dev-browser skill

### US-011: Colors and tags
**Description:** As a user, I want a small color palette and freeform tags so I can encode meaning (e.g. yellow = idea, red = disagree).

**Acceptance Criteria:**
- [ ] Preset colors (minimum 5, e.g. yellow, green, blue, pink, purple) consistent across readers
- [ ] Color stored on highlight/note; optional on bookmark
- [ ] Tags: zero or more short strings; UI chip input; normalized (trim, casefold for uniqueness per annotation)
- [ ] Tags filterable on book panel and global browser
- [ ] Verify in browser using dev-browser skill

### US-012: Edit and delete
**Description:** As a user, I want to fix typos in notes and remove bad highlights so the set stays trustworthy.

**Acceptance Criteria:**
- [ ] Edit note text and tags from popover and from global list
- [ ] Change color without recreating annotation
- [ ] Delete from popover, panel, and global list with confirm when note non-empty
- [ ] Edited `updated_at` reflected in UI
- [ ] Verify in browser using dev-browser skill

### US-013: Deep link into reader at annotation
**Description:** As a user, I want links that open the correct book and location so export and global list stay useful.

**Acceptance Criteria:**
- [ ] URL form documented, e.g. `/read/{bookId}?annotation={id}`
- [ ] On load, reader fetches annotation, navigates to anchor, briefly pulses highlight
- [ ] Invalid/foreign annotation id fails soft (open book at last progress, toast error)
- [ ] Verify in browser using dev-browser skill

### US-014: Book detail annotation count
**Description:** As a user, I want to see that a book has my notes before opening it so annotated books are discoverable.

**Acceptance Criteria:**
- [ ] Book detail page shows annotation count for current user (e.g. "12 highlights")
- [ ] Link goes to book-filtered `/annotations?book={id}` or opens reader panel
- [ ] Count is zero-hidden or shows "No annotations yet"
- [ ] Verify in browser using dev-browser skill

### US-015: Markdown / Obsidian export (per book)
**Description:** As a user, I want to export one book's annotations to Markdown with YAML frontmatter so I can paste into Obsidian or other PKM tools.

**Acceptance Criteria:**
- [ ] Action on book detail and in reader panel: "Export annotations"
- [ ] Download `.md` file named from book title (sanitized)
- [ ] YAML frontmatter includes: title, authors, format, book_id, exported_at, annotation_count
- [ ] Body groups by kind or document order; each item includes color, tags, quote blockquote, note, deep link path
- [ ] Empty book returns 404 or friendly empty file policy (prefer 400/empty state in UI before download)
- [ ] Verify in browser using dev-browser skill

### US-016: Bulk Markdown export
**Description:** As a user, I want to export all (or filtered) annotations so I can archive my library notes in one shot.

**Acceptance Criteria:**
- [ ] Export control on `/annotations` respects active filters (kind, tag, library, search)
- [ ] Produces single Markdown file with `## Book Title` sections, or ZIP of per-book MD if > N books (choose one; document it)
- [ ] Same frontmatter quality as per-book export at top level summary
- [ ] Large libraries do not OOM (stream or chunk query)
- [ ] Verify in browser using dev-browser skill

### US-017: Keyboard and touch ergonomics
**Description:** As a power reader, I want fast shortcuts so annotation does not break reading flow.

**Acceptance Criteria:**
- [ ] EPUB/PDF: shortcut to bookmark current location (document key, avoid clashing with existing arrow nav)
- [ ] `Esc` closes toolbar/popover/panel
- [ ] Touch: long-press or selection handles work on mobile viewport widths
- [ ] Shortcuts listed in a small help popover in the reader
- [ ] Verify in browser using dev-browser skill

### US-018: Permission and multi-user isolation
**Description:** As an admin of a household instance, I need annotations to stay private so users never see each other's notes.

**Acceptance Criteria:**
- [ ] No API or HTML path returns another user's annotations
- [ ] Deleting a user cascades their annotations
- [ ] Deleting a book cascades its annotations
- [ ] OPDS clients are unaffected (annotations not exposed via OPDS in v1)
- [ ] Automated tests for isolation

### US-019: Resilience when anchors break
**Description:** As a user, I want graceful behavior if a file is replaced or a CFI/page no longer resolves so I do not lose the note text.

**Acceptance Criteria:**
- [ ] Annotation rows remain even if jump fails
- [ ] Jump failure shows toast: "Location no longer found; note kept"
- [ ] Quote text remains visible in lists/export regardless of jump success
- [ ] Replacing book file does not auto-delete annotations
- [ ] Verify in browser using dev-browser skill

### US-020: Phase docs and roadmap status
**Description:** As a maintainer, I want this work tracked like prior phases so implementation can follow `docs/phases/` convention.

**Acceptance Criteria:**
- [ ] Add `docs/phases/0N-annotations.md` (phase number chosen when branching) summarizing scope and file touch list
- [ ] Update `ROADMAP.md` item 5 status when work starts/ships
- [ ] README features list mentions annotations after ship

---

## Functional Requirements

### Data & ownership
- **FR-1:** The system must store annotations private to `(user_id, book_id)` with no share/public flag in v1.
- **FR-2:** Each annotation must have a `kind` of `highlight`, `note`, or `bookmark`.
- **FR-3:** Each annotation must store an `anchor` JSON document whose shape depends on book format (see below).
- **FR-4:** Highlights must store `quote` text when created from a selection (may be empty for pure bookmarks).
- **FR-5:** Optional `note` (Markdown-ish plain text; no server-side HTML rendering required in v1 - escape on display).
- **FR-6:** Optional `color` from a fixed palette; required for `highlight`, optional otherwise.
- **FR-7:** Optional `tags` as a text array (max length per tag and max tags documented; e.g. 32 tags × 40 chars).
- **FR-8:** Timestamps: `created_at`, `updated_at`.

### Anchor shapes (v1)
- **FR-9:** EPUB: `{ "scheme": "epubcfi", "cfi": "...", "cfi_end": "..."? }`
- **FR-10:** PDF: `{ "scheme": "pdf", "page": N, "rects"?: [...], "text_start"?: ..., "text_end"?: ... }`
- **FR-11:** CBZ: `{ "scheme": "cbz", "page": N }`
- **FR-12:** Audio: `{ "scheme": "audio", "seconds": number }`
- **FR-13:** Unknown schemes rejected at write time; readers ignore unknown schemes gracefully at read time.

### In-reader behavior
- **FR-14:** EPUB selection toolbar must support highlight colors, add note, bookmark location.
- **FR-15:** Existing annotations must rehydrate when a chapter/page/time position loads.
- **FR-16:** In-reader panel must list, filter, jump, edit note/color/tags, and delete.
- **FR-17:** Creating/updating annotations must not interrupt page-turn or audio playback beyond a brief UI flash.

### Global UX
- **FR-18:** `/annotations` library-wide browser with filters and search.
- **FR-19:** Deep links open reader at annotation location.
- **FR-20:** Book detail shows personal annotation count with navigation into the list.

### Export
- **FR-21:** Per-book Markdown export with YAML frontmatter and blockquoted quotes.
- **FR-22:** Bulk export from `/annotations` honoring active filters.
- **FR-23:** Export content must include enough metadata to re-find the passage (title, authors, kind, color, tags, page/CFI/time, in-app path).
- **FR-24:** Export is UTF-8 Markdown; no proprietary binary format in v1.

### Security & multi-user
- **FR-25:** Session-authenticated only; no OPDS annotation endpoints in v1.
- **FR-26:** Authorization is owner-only; admins do **not** read others' annotations by default.
- **FR-27:** Input limits: note max length (e.g. 10_000 chars), quote max length (e.g. 8_000 chars), tags constraints.
- **FR-28:** All user HTML in notes/quotes escaped in templ/JS.

### Performance
- **FR-29:** List annotations for a book under 100ms p95 for ≤ 2_000 annotations/user/book on modest hardware.
- **FR-30:** Global list queries must be indexed; no full table scan of all users.
- **FR-31:** Export for ≤ 10_000 annotations must complete without request timeout under default reverse-proxy limits (or stream download).

---

## Non-Goals (Out of Scope)

- Sharing annotations with other users, public links, or "annotation social" features
- KOReader, Kobo, Kindle, or any e-reader annotation sync (browser-only for this PRD)
- Writing highlights back into the EPUB/PDF file bytes
- Sidecar files on disk next to books (DB is source of truth)
- REST token API / webhooks for third-party tools (roadmap item 10; not required here - export file is enough)
- Import from Calibre annotations DB, KOReader `.sdr`, Readwise, or Goodreads
- Real-time collaborative cursors
- AI summary of highlights
- Full rich-text (images, embeds) inside notes
- Annotation support in OPDS clients
- Drawing freehand ink on comic/PDF pages
- Version history / undo stack beyond single delete
- Cross-book bidirectional links / Zotero-style research graph
- Native or hybrid mobile apps (iOS/Android) - see **Future: mobile clients** (planned later, not v1)

---

## Future: mobile clients

Mobile annotation is **intentionally deferred**, not ruled out. v1 should not invent a second data model for phones; it should leave a clean server contract that a later client can consume.

### Why v1 already enables this
- Annotations are **server-persisted** and private per user, not browser-local only.
- **Format-portable anchors** (`epubcfi`, PDF page/geometry, CBZ page, audio seconds) are not tied to a specific DOM.
- Reader CRUD is **JSON API-shaped**, so a non-browser client can reuse the same resources once auth exists.

### Likely later path (not committed scope)
1. **PWA / responsive polish** - same web readers on phone; lowest cost, reuses this feature as-is.
2. **Token-auth REST API** - personal access tokens (aligns with roadmap item 10); required for true native clients.
3. **Optional native shell (WebView)** - app chrome around existing readers.
4. **Optional native readers** - Swift/Kotlin implementing the same anchor schemes and annotation CRUD (highest cost).

### Design rules to preserve in v1 (so mobile stays cheap later)
- Do not overload `user_book_progress.position` with highlight arrays; keep annotations in their own table.
- Keep anchor `scheme` values stable and documented.
- Prefer owner-scoped REST paths (`/api/books/{id}/annotations`, `/api/annotations/{id}`) over HTML-only form posts for create/update/delete.
- Deep links (`/read/{bookId}?annotation={id}`) should remain valid entry points for app handoff.
- Offline queue, conflict resolution, and push notifications are **mobile-phase** concerns - not required in v1.

### Explicitly separate from e-reader sync
Phone/PWA/native apps are distinct from KOReader/Kobo/Kindle annotation sync. Either can ship without the other. This PRD still excludes device e-reader sync.

---

## Design Considerations

### UI patterns
- Reuse reader chrome patterns from phase 6 (`components` reader page, status bar, theme tokens in `base.css` / `themes.css`).
- Floating selection toolbar: compact, high contrast, keyboard accessible, does not obscure selection.
- Color chips: filled circles with `aria-label`; active state ring using `--color-primary`.
- Annotation panel: slide-over or collapsible column; mobile = bottom sheet.
- Global `/annotations` page: same list density language as books grid/list (cards or table - prefer scannable list rows).

### Visual language
- Highlight underlays should use palette colors at ~35-45% opacity so text remains readable in light and dark themes.
- Bookmark icons distinct from shelf "bookmark" icon usage in shelves UI (consider `flag` / `pin` / `ribbon` metaphor in reader chrome to avoid confusion with shelves).

### Content examples

**Markdown export fragment:**

```markdown
---
title: "Efficient Linux at the Command Line"
authors: ["Daniel J. Barrett"]
format: epub
book_id: 42
exported_at: 2026-08-03T12:00:00Z
annotation_count: 2
---

# Efficient Linux at the Command Line

## Highlights

> Use `find` with `-exec` carefully.

- Color: yellow
- Tags: shell, tips
- Note: revisit for chapter 4 exercises
- Location: epubcfi(/6/8!/4/2)
- Open: /read/42?annotation=1001
```

---

## Technical Considerations

### Suggested schema

```sql
CREATE TABLE annotations (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id      BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('highlight', 'note', 'bookmark')),
    color        TEXT,
    quote        TEXT NOT NULL DEFAULT '',
    note         TEXT NOT NULL DEFAULT '',
    tags         TEXT[] NOT NULL DEFAULT '{}',
    anchor       JSONB NOT NULL,
    -- optional denormalized sort key for document order within a book
    sort_key     TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX annotations_user_book_idx ON annotations (user_id, book_id);
CREATE INDEX annotations_user_created_idx ON annotations (user_id, created_at DESC);
CREATE INDEX annotations_tags_idx ON annotations USING GIN (tags);
-- search: either generated tsvector column or trigram on quote/note
```

### Integration points
- **Readers:** extend `web/static/js/readers/epub.js`, `pdf.js`, `cbz.js`, `audio.js`; shared module e.g. `web/static/js/readers/annotations.js` for toolbar/panel/API client.
- **Progress:** keep `user_book_progress.position` separate from annotations; do not overload progress JSON with highlight arrays.
- **Store layer:** new methods on `internal/store` following existing patterns (`SaveProgress`, etc.).
- **Handlers:** mount under authenticated routes in `internal/http/handlers/app.go`.
- **UI shell:** templ components for `/annotations` and book detail badge; HTMX optional for list filters, JSON for in-reader CRUD.
- **epub.js:** use CFI ranges and rendition annotations; regenerate locations still required for percent, independent of highlights.

### Format feasibility notes
- **EPUB:** highest fidelity; implement first if stories are sequenced.
- **PDF:** depends on current PDF reader implementation (iframe vs pdf.js). If still a plain iframe, promote to pdf.js (or equivalent) as part of US-005 - call that out in the phase doc.
- **CBZ:** page numbers already exist via `/read/{id}/pages/{n}` stream.
- **Audio:** HTML5 `currentTime` is sufficient.

### Sequencing suggestion (implementation order)
1. Schema + API + isolation tests (US-001, US-002, US-018)
2. EPUB highlight/bookmark + panel (US-003, US-004, US-008)
3. Deep link + book badge (US-013, US-014)
4. PDF + CBZ + audio anchors (US-005, US-006, US-007)
5. Global browser + search + tags/colors polish (US-009, US-010, US-011, US-012)
6. Export (US-015, US-016)
7. Ergonomics + resilience + docs (US-017, US-019, US-020)

---

## Success Metrics

- A user can create an EPUB highlight with note in ≤ 3 interactions (select → color → done).
- Jump from global list to correct EPUB location works ≥ 95% for unmodified files.
- Library-wide search returns relevant personal notes in under 1 second for 10k annotations.
- Markdown export opens cleanly in Obsidian without manual cleanup for typical books.
- Zero cross-user annotation leakage in automated tests.
- Feature matches or exceeds the competitive table for browser-only, private annotation workflows - especially **audiobook timestamps**, **library-wide browser**, and **first-class MD export**.

---

## Resolved decisions (Lavish finalization)

| # | Topic | Decision |
| --- | --- | --- |
| Q1 | PDF renderer | **Upgrade to pdf.js in this phase** for page bookmarks and text highlights (recommended default; not explicitly overridden) |
| Q2 | Palette | **5 semantic colors:** yellow, green, blue, pink, purple → map UI to warning / success / info / secondary / primary tokens |
| Q3 | Bulk export | **Single concatenated Markdown** with `## Book Title` sections |
| Q4 | Note markup | **Plain text only**; escape all HTML on display |
| Q5 | Cap | **Soft cap 5,000** annotations per user per book; API returns 400 when exceeded |
| Q6 | EPUB sort key | **Percent from `book.locations` at save time** stored in `sort_key`; fallback to `created_at` |
| Q7 | Audio titles | **Manual notes only**; default label is human time `h:mm:ss` |
| Q8 | Mobile follow-up | **PWA / responsive polish first**; token API when a native client is actually needed |

**PRD status:** LOCKED for implementation (Lavish review, finalize-and-build).

---

## Appendix A: Glossary

| Term | Meaning |
| --- | --- |
| **Highlight** | Colored mark over selected text (EPUB/PDF) with optional note |
| **Bookmark** | Location marker without requiring a text selection |
| **Note** | Annotation whose primary content is user text; may still have an anchor |
| **Anchor** | Format-specific pointer (CFI, page, seconds) stored as JSON |
| **CFI** | EPUB Canonical Fragment Identifier |
| **Quote** | Snapshot of selected text at creation time |

## Appendix B: Product decisions log

| Topic | Decision |
| --- | --- |
| Ambition | Exceed competitors (not MVP-only) |
| Formats | EPUB + PDF + CBZ + audio timestamps |
| Visibility | Private only |
| Export | Markdown / Obsidian |
| Device sync | Explicitly out of scope for this PRD |
| Mobile apps | Deferred; same store/API, later PWA → token API → optional native |
| PDF | pdf.js upgrade in this phase |
| Palette | yellow, green, blue, pink, purple |
| Bulk export | Single concatenated .md |
| Notes | Plain text, escaped |
| Cap | 5,000 per user per book |
| EPUB sort | percent at save → sort_key |
| Audio labels | time h:mm:ss + manual note |
| After ship | PWA before token API |
