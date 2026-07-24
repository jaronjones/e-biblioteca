# Phase 3 — Metadata pipeline

## Scope
- EPUB/CBZ embedded metadata extraction
- Cover extraction and storage under data/covers
- Open Library + Google Books lookup
- Manual metadata edit UI and FTS

## Primary packages
- `internal/service/metadata`
- Book edit handlers and lookup partials

## Status
Implemented as part of Phase 1 (`phase/1-auth-and-core`). Unit coverage for format
detection, natural CBZ page order, and cover selection lives under
`internal/service/metadata/*_test.go`.
