# Phase 3 — Metadata pipeline

## Scope
- EPUB/CBZ embedded metadata extraction
- PDF extraction (pdfcpu): Info dict title/author/subject/keywords, page
  count, and an ISBN scan over the first pages' content streams
- EPUB cover resolution: EPUB3 `properties="cover-image"`, then EPUB2
  `<meta name="cover">` indirection, then name heuristics — no cover is
  stored when none is declared (providers backfill on enrichment)
- ISBNs are check-digit validated (EPUB identifiers and PDF page scans)
- Cover extraction and storage under data/covers
- Open Library + Google Books lookup; `metadata.Enrich` runs on BookDrop
  intake and direct upload (library rescans stay extraction-only)
- Manual metadata edit UI and FTS

## Primary packages
- `internal/service/metadata`
- Book edit handlers and lookup partials

## Status
Baseline extraction shipped with Phase 1. Hardening (PDF via pdfcpu, spec-order
EPUB covers, ISBN check digits, shared Enrich) is part of Phase 8
(`phase/8-harden`). Unit coverage lives under `internal/service/metadata/*_test.go`.
