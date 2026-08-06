# Phase 8 — Hardening

## Scope
- README and .env.example
- MIT license
- Docker healthchecks
- Graceful shutdown
- Phase docs index reflecting actual delivery order
- Self-hosted vendor JS (no CDN required)
- CI: build, vet, test, gofmt
- Unit tests for auth helpers, themes, magic rules, path containment
- OPDS fail-closed without linked user / download permission
- README polish for air-gapped install
- PDF metadata extraction (pdfcpu) and ISBN page scan
- Spec-order EPUB cover selection and container.xml OPF resolution
- Shared `metadata.Enrich` helper (BookDrop + upload, bounded timeout)

## Primary packages
- `web/static/vendor/*`
- `.github/workflows/ci.yml`
- `docs/phases/*`
- `internal/service/metadata`
- Tests under `internal/**/*_test.go`
