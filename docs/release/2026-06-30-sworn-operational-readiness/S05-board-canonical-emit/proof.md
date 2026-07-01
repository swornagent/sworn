---
title: 'Proof bundle — S05-board-canonical-emit'
description: 'sworn EMITS, VALIDATES, and now READS only the canonical baton object form for a board release (strict); a bare-string release fails closed on read; operator string boards are migrated at cutover (AC-06). Implemented, not yet Rule-7 verified.'
date: 2026-07-01
---

# Proof bundle — S05-board-canonical-emit

> Rendered from `proof.json` (proof-v1). The JSON is the source of truth.

## Scope
sworn EMITS, VALIDATES, and now READS only the canonical baton object form for a board's
`release` (strict) — a legacy bare-string release fails closed on read — removing S04's
string-tolerance vendor drift from canonical baton. Operator-owned string boards are MIGRATED at
cutover (AC-06), not read-tolerated. This session landed the remaining strict-reader delta on top
of the first cut (`565f909`, which had already made schema + validator + writer object-only): the
reader flip (`Release.UnmarshalJSON` object-only) plus the inverted string-read tests.

## Files changed
`git diff --name-only 0d22f65 -- internal/ cmd/` (start_commit = parent of the S05 feat commit):
```
internal/board/board.go              Release.UnmarshalJSON -> object-only (strict reader); MarshalJSON/Release doc comments -> strict-read world
internal/baton/schemas/board-v1.json release object-only (from first cut); description corrected to strict-read
internal/baton/validator.go          release object-only (from first cut); comment corrected (reader is strict, not tolerant)
internal/board/board_release_test.go inverted: bare string now fails closed (TestRelease_StringForm_FailsClosed, TestRelease_BareStringRead_FailsClosed); StringRelease emit kept (TestStringRelease_EmitsCanonicalObject); AC-07->AC-01 label fix
internal/board/board_test.go         FIXTURE: TestOracleReadBoard_BoardJSONFirst board.json release "test-release" -> {"name":"test-release"} (beyond declared touchpoints — see Divergence)
cmd/sworn/merge_test.go              FIXTURE: setupMergeFixture board.json release %q -> {"name":%q} (beyond declared touchpoints — see Divergence)
```

## Test results
- `go build ./...` → exit 0
- `go vet ./internal/board/... ./internal/baton/...` → exit 0
- `go test ./internal/board/... ./internal/baton/...` → ok
- `go test ./... -timeout 300s` → all green (full suite; AC-05). The strict reader regressed two
  test board.json fixtures (board_test.go, merge_test.go) that used the legacy string form; both
  migrated to canonical object form, then the full suite is green.

## Reachability artefact
**type: cli-run.** S05 strict-reader binary (built from
`track/2026-06-30-sworn-operational-readiness/T4-board-record-reconciliation` HEAD) run in
`~/projects/fired`:
```
$ sworn board --release 2026-06-28-yearSnapshot-schema-cleanup --json
{
  "release": "2026-06-28-yearSnapshot-schema-cleanup",
  "tracks": [ { "id": "T1-schema-cleanup", "state": "in_progress",
    "slices": [ {"id":"S01-networth-hierarchy-remap","state":"planned",...}, ... ] } ]
}
```
The coach board.json on ref `release-wt/2026-06-28-yearSnapshot-schema-cleanup`
(`apps/docs/content/docs/release/.../board.json`) carries the canonical OBJECT release form
`{"name": ..., "target_version": "v0.5.0", "integration_branch": ..., "vertical_trace": {...}}`
— NOT a bare string. The strict reader (object-only, `additionalProperties:true`) reads it and
returns the board (exit 0), proving the tightened reader still accepts a real coach object board.

## Delivered
- **AC-01** (canonical emit): `Release.MarshalJSON` emits `{"name":...}` for a name-only release
  (`StringRelease` path) and re-emits an object read verbatim.
  Evidence: `internal/board/board.go`; `TestStringRelease_EmitsCanonicalObject`,
  `TestRelease_RoundTripPreservesObjectFields`.
- **AC-03** (object-only everywhere, strict reader): schema `release` object-only + validator
  object-only + `Release.UnmarshalJSON` object-only; a bare string fails closed.
  Evidence: `board-v1.json`, `validator.go`, `board.go`; `TestRelease_StringForm_FailsClosed`,
  `TestRelease_BareStringRead_FailsClosed`, `TestRelease_ObjectMissingName_FailsClosed`.
- **AC-04** (fired still reads): the `sworn board` output above (real coach object board).
- **AC-05** (build + full suite + inverted string tests): test results above; the old lenient
  `TestRelease_StringForm` / `TestRelease_StringReadEmitsCanonicalObject` are replaced by their
  fail-closed inversions.

## Not delivered
- **AC-06 — migrating operator on-disk string boards** (op-readiness, conformance-foundation,
  release-hygiene) to the object form. **Why:** AC-06 is an `unwanted`-type criterion describing a
  one-time CUTOVER step, explicitly NOT sworn production code in this slice; it may run only once
  every active session is on a canonical (S04/S05) binary (a strict-reader binary fails on a string
  board; a pre-S04 binary fails on an object board), so it cannot run mid-flight. The op-readiness
  board itself is among the boards to migrate. **Tracking:** spec.json AC-06; the operational-readiness
  cutover. **Ack:** Coach-ratified in spec.json AC-06 (2026-07-01).
- **Full canonical board-v1 parity** (nested `release.worktree`, enumerated props,
  `additionalProperties:false`). **Why:** broader baton board-v1 re-vendor, out of scope per the
  spec rationale. **Tracking:** baton vendor-pin (FT-6). **Ack:** spec.json rationale, 2026-07-01.

## Divergence from plan
- **Two test files beyond the four declared touchpoints were edited:** `internal/board/board_test.go`
  (`TestOracleReadBoard_BoardJSONFirst` fixture) and `cmd/sworn/merge_test.go` (`setupMergeFixture`
  board.json builder). Both encoded the legacy string-form `release` in their board.json fixtures;
  under the strict reader these fixtures fail closed (a real regression AC-05's full-suite gate
  surfaced), so each was migrated to the canonical object form `{"name": ...}`. Both are test
  fixtures in package surfaces owned by T4 (`internal/board`, `cmd/sworn`) — not a cross-track
  collision; the change is a one-line fixture migration, not production logic. Recorded as a Rule-2
  transparency note in `journal.md`.

## First-pass verdict (deterministic gate)
`~/.claude/bin/release-verify.sh S05-board-canonical-emit 2026-06-30-sworn-operational-readiness`:
18 checks pass, 1 residual FAIL — **`spec.md missing`** — which is a script/format-version mismatch,
NOT a slice gap: this release uses the schema-constrained `spec.json` (spec-v1), and the verified
sibling **S04 reached `verified` with only `spec.json`** (no `spec.md`). The canonical model-backed
gate `sworn verify --spec spec.json --diff <diff> --proof proof.json` requires `SWORN_ANTHROPIC_API_KEY`
(not set in this environment) and is the verifier session's to run. (The earlier `state in_progress`
FAIL clears with the `implemented` transition.)
