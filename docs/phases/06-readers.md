# Phase 6 — Readers & progress

## Scope
- EPUB (epub.js), PDF iframe, CBZ page stream, HTML5 audio
- Per-user progress percent + position JSON
- Dashboard continue-reading

## Primary packages
- `web/static/js/readers/*`
- Handlers: `/read/{id}`, `/progress/{id}`, `/stream/{id}`

## Status
Implemented as part of Phase 1. EPUB stream must use `{ openAs: 'epub' }` because
`/stream/{id}` has no file extension. Reader scripts are `defer` so CSRF + progress
JSON are ready before they run. Manual E2E steps: `.agents/skills/testing-e-biblioteca/`.
