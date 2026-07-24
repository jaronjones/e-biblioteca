# Phase 5 — Shelves & magic shelves

## Scope
- Per-user static shelves
- Add/remove books from shelves
- Magic shelves with JSONB rules (author, category, series, format, status, query)

## Primary packages
- Store shelf/magic shelf APIs
- Handlers: `/shelves`, `/magic-shelves`

## Status
Implemented as part of Phase 1. Shelf mutations are scoped by `user_id` (no IDOR).
