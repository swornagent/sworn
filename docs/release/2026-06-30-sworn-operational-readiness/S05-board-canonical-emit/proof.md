---
title: 'Proof bundle — S05-board-canonical-emit'
description: 'sworn emits + validates the canonical object form for board release (strict), reader stays tolerant; kills the S04 schema vendor-drift. Implemented, not yet Rule-7 verified.'
date: 2026-07-01
---

# Proof bundle — S05-board-canonical-emit

## Scope
Make sworn EMIT and VALIDATE the canonical baton object form for a board's `release` (strict,
object-only — removing S04's `oneOf` string-tolerance divergence from canonical baton), while
the READER stays tolerant of a legacy string. Existing string boards self-heal to canonical on
next write.

## Files changed
`git diff --name-only 3847df0 HEAD` (production; on S04's verified base):
```
internal/board/board.go            (Release.MarshalJSON emits canonical object {name} for name-only)
internal/baton/schemas/board-v1.json (release: object-only, required name; oneOf removed)
internal/baton/validator.go        (board-v1 release must be object-with-name; string-accept removed)
internal/board/board_release_test.go (string-round-trip test -> string read emits canonical object)
```
Reader (`Release.UnmarshalJSON`) unchanged — still tolerant of string OR object.

## Test results
- `go build ./...` → exit 0
- `go test ./internal/board/... ./internal/baton/...` → ok
- `go test ./...` (full suite) → all green (no WriteBoard/migrate regression on the canonical-emit change)

## Reachability artefact
S05 binary (`/tmp/sworn-s05`) run from `~/projects/fired`:
```
$ sworn board --release 2026-06-28-yearSnapshot-schema-cleanup
Release board: 2026-06-28-yearSnapshot-schema-cleanup
Track T1-schema-cleanup — in_progress
    S01-networth-hierarchy-remap — planned [human]
    ...
```
Proves the tolerant reader survives the strict-emit change — the real coach object board still
reads (exit 0). The self-heal direction is proven by `TestRelease_StringReadEmitsCanonicalObject`
(read `"legacy"` → marshal `{"name":"legacy"}`).

## Delivered
- AC-01 (canonical emit): `Release.MarshalJSON` object branch for name-only; raw preserved for object reads.
- AC-02 (string self-heals on write): `TestRelease_StringReadEmitsCanonicalObject`.
- AC-03 (schema + validator object-only, reader tolerant): board-v1.json release object-only; validator requires object; `UnmarshalJSON` unchanged + `TestRelease_StringForm` still passes.
- AC-04 (fired still reads): the board output above.
- AC-05 (build/full suite): green.

## Not delivered (Rule 2)
- **Full canonical board-v1 parity** (nested `release.worktree`, enumerated props,
  `additionalProperties:false`). Why: that restructures the worktree representation the oracle
  reads as flat fields — the broader baton re-vendor (FT-6), not this slice. Tracking:
  baton vendor-pin work. Acknowledged 2026-07-01.
- **Migrating existing on-disk string boards**: not a separate task — the canonical writer
  self-heals them on next write (AC-02). They read fine until then (tolerant reader).

## Divergence from plan
- None from the S05 spec. Note: S05 exists because S04 was verified (immutable, Rule 7) before
  this refinement was decided, so the right-moving-forward change is a new slice appended to T4
  rather than an edit to S04 — which reopens T4 into the verify→merge flow.
