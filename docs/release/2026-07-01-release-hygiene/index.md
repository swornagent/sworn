---
title: 'Release board — 2026-07-01-release-hygiene'
description: 'Rendered view of board.json (source of truth). sworn reports its real version from one embedded source; a CI gate blocks merge to main without a version bump.'
---

# Release: 2026-07-01-release-hygiene

**Goal:** `sworn version` reports the real version from a single embedded source for any build
method (not 0.0.0-dev), and a fail-closed CI gate blocks a merge into `main` without a version
bump. Split out of the operational-readiness push so its golden thread stays clean.

**Integration branch:** `release/v0.1.0` · **Target version:** v0.1.0 · Not on the fired
overnight path — independent governance work.

> NOTE: board.json uses the `release: string` form (what the installed oracle reads today).
> Once S04-board-record-reconciliation is the running binary, new boards can use the canonical
> nested object. This file is hand-authored until S02-board-render (other release) ships a
> renderer; board.json is the source of truth.

## Tracks

| Track | Slices | depends_on | State |
|-------|--------|------------|-------|
| `T1-version-hygiene` | `S01-embedded-version` → `S02-version-bump-ci-gate` | — | planned |

One serial track: S02 depends on S01 (the gate compares the version source S01 establishes).

## Slices

| Slice | Track | Outcome | State | E×C |
|-------|-------|---------|-------|-----|
| `S01-embedded-version` | T1 | `sworn version` reports the real version from `internal/version/version.txt` for any build method | planned | chore (low/low) |
| `S02-version-bump-ci-gate` | T1 | a PR to `main` without bumping the version fails CI (fail closed) | planned | chore (low/low) |

## Touchpoint matrix (Phase 3b)

Single track — disjoint by construction.

| File | T1 |
|------|----|
| internal/version/version.txt | ✓ |
| internal/version/version.go | ✓ |
| internal/version/version_test.go | ✓ |
| cmd/sworn/main.go | ✓ |
| Makefile | ✓ |
| .github/workflows/version-bump.yml | ✓ |
| scripts/check-version-bump.sh | ✓ |

## Dependency graph

```
T1-version-hygiene (no external deps)
  └─ S01-embedded-version  →  S02-version-bump-ci-gate   (S02 depends on S01)
```

## Out of scope (deferred, Rule 2 — see intake.md)

- Auto-tagging / release automation; changelog enforcement.
