# Phase 7 — OPDS & multi-user admin

## Scope
- OPDS 1.2 navigation + acquisition feeds
- Separate OPDS basic-auth users
- Admin user permission management

## Primary packages
- Handlers: `/opds`, `/settings`
- Store: opds_users, user_permissions
## Status
Implemented as part of Phase 1. Covers are served under `/opds/cover/{id}` (basic auth).
OPDS credentials must link to an app user; downloads enforce that user's `CanDownload`.
