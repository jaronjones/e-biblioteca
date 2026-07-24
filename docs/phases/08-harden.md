# Phase 8 — Hardening

## Scope
- Phase docs index reflecting actual delivery order
- Self-hosted vendor JS (no CDN required)
- CI: build, vet, test, gofmt
- Unit tests for auth helpers, themes, magic rules, path containment
- OPDS fail-closed without linked user / download permission
- README polish for air-gapped install

## Primary packages
- `web/static/js/vendor/*`
- `.github/workflows/ci.yml`
- `docs/phases/*`
- Tests under `internal/**/*_test.go`
