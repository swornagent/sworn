---
title: 'Proof bundle — S30-lint-touchpoints'
description: 'Generated from live repo state at implementation completion.'
---

# Proof: `S30-lint-touchpoints`

## Scope

`sworn lint touchpoints <slice-id> <release>` reconciles a slice's spec file/package references against its `planned_files` AND the release `index.md` touchpoint matrix, flagging undeclared touchpoints, unacknowledged cross-slice file collisions, and duplicate migration numbers. Fails closed (exit 1) on any violation.

## Files changed

From this implementation session (vs HEAD at session start, b2c25f2):

```
cmd/sworn/lint.go                                              (modified — added touchpoints target + cmdLintTouchpoints)
internal/lint/touchpoints.go                                   (new)
internal/lint/touchpoints_test.go                              (new)
docs/release/2026-06-19-safe-parallelism/S30-lint-touchpoints/status.json  (modified — design_decisions + state)
```

## Test results

```
=== RUN   TestTouchpointUndeclaredFails
--- PASS: TestTouchpointUndeclaredFails (0.00s)
=== RUN   TestTouchpointCollisionFails
--- PASS: TestTouchpointCollisionFails (0.00s)
=== RUN   TestTouchpointDocumentedSharedIsNoteNotViolation
--- PASS: TestTouchpointDocumentedSharedIsNoteNotViolation (0.00s)
=== RUN   TestTouchpointCleanPasses
--- PASS: TestTouchpointCleanPasses (0.00s)
=== RUN   TestTouchpointSectionScopingExcludesRiskAndTests
--- PASS: TestTouchpointSectionScopingExcludesRiskAndTests (0.00s)
=== RUN   TestTouchpointPackagePrefixMatch
--- PASS: TestTouchpointPackagePrefixMatch (0.00s)
=== RUN   TestMigrationCollisionFails
--- PASS: TestMigrationCollisionFails (0.00s)
=== RUN   TestNoTouchpointMatrixPasses
--- PASS: TestNoTouchpointMatrixPasses (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/lint	0.072s
```

`go vet ./internal/lint/... ./cmd/sworn/` — clean.

`go build ./...` — clean.

## Reachability artefact

### Fixture: undeclared touchpoint

```
$ sworn lint touchpoints S01-test test-undeclared
sworn lint touchpoints: undeclared: internal/missing.go referenced in spec but not in planned_files
exit: 1
```

### Fixture: cross-slice collision

```
$ sworn lint touchpoints S01-test test-collision
sworn lint touchpoints: collision: file internal/shared/file.go claimed by multiple tracks: T1 (this track) + T2
exit: 1
```

### Fixture: clean slice

```
$ sworn lint touchpoints S01-test test-clean
touchpoints: all references declared, no collisions, no duplicate migrations for S01-test
exit: 0
```

### Self-test on S30's own spec

Exit 1 — two unresolved false positives from illustrative prose in the In-scope section (`cmd/sworn/main.go` as an example of the additive-invariant check, `index.md` as a generic reference). Both are real file paths but not in planned_files because S30 doesn't modify them. This is a known edge case of path extraction from descriptive prose.

## Delivered

- [x] `sworn lint touchpoints <slice> <release>` exits non-zero on undeclared file/package reference — **evidence**: TestTouchpointUndeclaredFails PASS + fixture reachability test
- [x] `sworn lint touchpoints <slice> <release>` exits non-zero on cross-slice collision — **evidence**: TestTouchpointCollisionFails PASS + fixture reachability test
- [x] `sworn lint touchpoints <slice> <release>` exits non-zero on duplicate migration number — **evidence**: TestMigrationCollisionFails PASS
- [x] `sworn lint touchpoints <slice> <release>` exits 0 on clean slice — **evidence**: TestTouchpointCleanPasses PASS + fixture reachability test
- [x] Section-scoped extraction (`## In scope` + `## Planned touchpoints` only) — **evidence**: TestTouchpointSectionScopingExcludesRiskAndTests PASS
- [x] Package prefix matching (`internal/lint` matches `internal/lint/touchpoints.go`) — **evidence**: TestTouchpointPackagePrefixMatch PASS
- [x] DOCUMENTED SHARED files reported as informational notes, not violations — **evidence**: TestTouchpointDocumentedSharedIsNoteNotViolation PASS
- [x] Release without touchpoint matrix doesn't error — **evidence**: TestNoTouchpointMatrixPasses PASS
- [x] `design_decisions` added to status.json — 5 entries, all Type-2
- [x] Spec Risk audit: confirmed `## In scope` (line 18) and `## Planned touchpoints` (line 43) headings exist in S02b-concurrent-scheduler/spec.md with exact casing
- [x] `cmdLint` usage strings updated to include `touchpoints`
- [x] `go build ./...` and `go vet ./internal/lint/...` pass

## Not delivered

- **Prose-based non-additive edit detection** — Rule 2 deferral. The spec prescribed "Flag a slice whose design implies a non-additive (restructuring) edit" to DOCUMENTED SHARED files. This requires fuzzy prose heuristics ("restructuring vs appending") that are too unreliable for a fail-closed gate. Coach accepted the substitution (informational note when any DOCUMENTED SHARED file appears in planned_files) + Rule 2 deferral for prose-based non-additive detection. **Acknowledged**: Brad, 2026-06-22 (approved-ack.md).

## Divergence from plan

- **Reachability self-test exits 1 on S30's own spec** due to two remaining false positives:
  - `cmd/sworn/main.go` — referenced in In-scope as an illustrative example of DOCUMENTED SHARED file detection
  - `index.md` — referenced in In-scope as "cross-checks the release `index.md` touchpoint matrix"
  
  Both are real file paths the spec names for descriptive purposes but S30 doesn't plan to modify. This is an inherent limitation of prose-path extraction from the In-scope section. Fixture-based reachability tests all exit correctly.

- **Section-scoped extraction** (Pin #1 fix) applied: only `## In scope` and `## Planned touchpoints` sections are parsed, excluding Risk/Required-tests/Out-of-scope. Additional filters: bare extension tokens (`.go`, `.ts`), Go package patterns (`internal/...`), and template placeholders (`<release>`) are excluded. Suffix-matching for bare filenames without `/` handles `touchpoints.go` → `internal/lint/touchpoints.go`.