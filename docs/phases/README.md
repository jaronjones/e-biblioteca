# Implementation phases

| Phase | Branch | Focus | Status |
| --- | --- | --- | --- |
| 0 | `phase/0-scaffold` | Module, Docker, migrations, themes shell | Done on `main` |
| 1 | `phase/1-auth-and-core` | Auth + full Core+ application | Done on `main` |
| 2 | `phase/2-catalog` | Catalog / libraries coverage | Done on `main` |
| 3 | `phase/3-metadata` | Metadata pipeline (PDF extract, EPUB covers, enrichment) | In progress (`phase/8-harden`) |
| 4 | `phase/4-bookdrop` | BookDrop + upload | Shipped inside Phase 1 |
| 5 | `phase/5-shelves` | Shelves + magic shelves | Shipped inside Phase 1 |
| 6 | `phase/6-readers` | Readers + progress | Shipped inside Phase 1 |
| 7 | `phase/7-opds` | OPDS + admin | Shipped inside Phase 1 |
| 8 | `phase/8-harden` | Docs, tests, CI, offline assets, metadata hardening | In progress |
| 9 | `phase/9-annotations` | Annotations, highlights, bookmarks, Markdown export | In progress |

Phases 3–7 were originally separate slices; Phase 1 delivered the full Core+ surface
(auth, catalog, metadata, BookDrop, shelves, readers, OPDS). Phase 8 hardens that
baseline and deepens metadata extraction. Phase 9 adds private multi-format annotations.

Merge order: 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 into `main`.

Future phases are drawn from the feature tracker in [ROADMAP.md](../../ROADMAP.md).
