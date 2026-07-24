# Implementation phases

| Phase | Branch | Focus | Status |
| --- | --- | --- | --- |
| 0 | `phase/0-scaffold` | Module, Docker, migrations, themes shell | Done on `main` |
| 1 | `phase/1-auth-and-core` | Auth + full Core+ application | Done on `main` |
| 2 | `phase/2-catalog` | Catalog / libraries coverage | Done on `main` |
| 3 | `phase/3-metadata` | Metadata pipeline | Shipped inside Phase 1 |
| 4 | `phase/4-bookdrop` | BookDrop + upload | Shipped inside Phase 1 |
| 5 | `phase/5-shelves` | Shelves + magic shelves | Shipped inside Phase 1 |
| 6 | `phase/6-readers` | Readers + progress | Shipped inside Phase 1 |
| 7 | `phase/7-opds` | OPDS + admin | Shipped inside Phase 1 |
| 8 | `phase/8-harden` | Docs, tests, CI, offline assets, polish | In progress |

Phases 3–7 were originally separate slices; Phase 1 delivered the full Core+ surface
(auth, catalog, metadata, BookDrop, shelves, readers, OPDS). Phase 8 hardens that
baseline for self-hosted use.
