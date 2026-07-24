# Phase 4 — BookDrop & upload

## Scope
- fsnotify watcher on BOOKDROP_DIR
- Extract + enrich inbound files
- Review queue UI (import / reject)
- Multipart upload into a chosen library

## Primary packages
- `internal/service/bookdrop`
- Handlers: `/bookdrop`, `/upload`

## Status
Implemented as part of Phase 1. Uploads are capped with `MaxBytesReader`, stream to
disk via temp+rename, and reject paths outside `BOOKS_DIR`.
