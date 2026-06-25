---
title: S52-ledger-projection journal
description: Implementation log for S52 — verdict ledger projection engine.
---

# Journal: `S52-ledger-projection`

## Session log

### 2026-07-21 01:00 — start implementation

- **State**: `planned → in_progress`
- **Notes**:
  - Track worktree materialised for T16-verdict-ledger (first slice in track)
  - Release worktree already existed at `/home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism`

### 2026-07-21 01:00 — implementation

- **State**: `in_progress`
- **Notes**:
  - Added `Model string` and `Attempt int` (both `omitempty`) to `state.Verification`
  - Extended `state_test.go` with `TestVerification_ModelAttemptRoundTrip` and `TestVerification_ModelAttemptOmitEmpty`
  - Populated `st.Verification.Model` and `st.Verification.Attempt` at all three verdict-record sites in `internal/run/slice.go`:
    - PASS path: uses `implModelID` + `totalAttempts`
    - BLOCKED path: uses `implModelID` + `totalAttempts`
    - haltFailedVerification: uses `lastImplModel` + `totalAttempts` (FAIL path reached via `goto` — `implModelID` is loop-scoped; declared `lastImplModel` before the loop as a capture variable)
  - Created `internal/ledger/` package:
    - `Record` struct with JSON tags, `V int` (=1)
    - `SliceKind(track)` — strips `T<number>-`, takes first segment, de-pluralises (handles `harness` via `ss` guard)
    - `Project(status, gateCount)` — pure projection from `state.Verification`; returns `ok=false` for pending/empty
    - `Append(path, record)` — append-only JSONL writer with idempotency guard (dedupe by `Key`)
    - `Key(record)` — `slice_id|verdict|ts`
  - Created comprehensive `ledger_test.go` with table-driven tests for Project (pass/fail/blocked/pending/empty), SliceKind (17 cases), Append (line-count, idempotency, dir creation), Key
  - Zero new deps; `go.mod` unchanged

### 2026-07-21 01:00 — implemented

- **State**: `in_progress → implemented`
- **Notes**:
  - All 7 acceptance checks delivered with test evidence
  - `go test ./internal/ledger/... ./internal/state/... ./internal/run/...` — all pass (10 ledger + 15 state + 33 run)
  - `go vet` clean, `go build ./...` clean
  - `release-verify.sh` first-pass: 20/22 PASS (2 template-placeholder failures — expected before proof.md fill); after fill: 24/24 PASS

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

### 2026-07-22 — verification session (fresh context, worktree-anchored)

**Verdict: PASS**

Gate 1 — User-reachable outcome exists: PASS. `ledger.Project`/`ledger.Append` published in `internal/ledger`; `state.Verification.Model`/`Attempt` populated at all three verdict-record sites in `internal/run/slice.go` (PASS line ~391, BLOCKED line ~433, haltFailedVerification line ~473).

Gate 2 — Planned touchpoints match actual changed files: PASS. Planned: ledger.go, ledger_test.go, state.go, state_test.go, slice.go. Actual: exactly those five plus expected docs artefacts (journal.md, proof.md, status.json, index.md). No unexplained deltas.

Gate 3 — Required tests exist and exercise integration point: PASS. `ledger_test.go`: 5 Project cases (pass/fail/blocked/pending/empty), 17 SliceKind cases, 3 Append cases, Key test. `state_test.go`: ModelAttemptRoundTrip, ModelAttemptOmitEmpty. Run tests (TestRunSlice, TestRunSliceFail, TestRunSlice_BlockedNotifies) exercise all three verdict-record paths. All re-run and pass.

Gate 4 — Reachability artefact confirms user path: PASS. `go test ./internal/ledger/... ./internal/state/... ./internal/run/...` re-run and all pass (58 tests). `go vet` clean. `go build ./...` clean. `go.mod` unchanged — zero new deps.

Gate 5 — No silent deferrals or placeholder logic: PASS. Grep for TODO/FIXME/placeholder/deferred/HACK/XXX — empty in all production code. Spec's two deferrals (verifier-model capture, remote emission) properly documented with why/tracking/ack.

Gate 6 — Claimed scope matches implemented scope: PASS. All 7 acceptance checks delivered with test evidence. One divergence noted: `SliceKind("T16-verdict-ledger")` returns `"verdict"` not `"ledger"` as spec example suggests; the first-segment rule is mechanically consistent across all 22 tracks, documented in journal.md.
## Design decisions

### SliceKind for T16-verdict-ledger

`SliceKind("T16-verdict-ledger")` returns `"verdict"` (first-segment rule), not `"ledger"` as one spec example suggests. The spec lists five examples as illustrative ("e.g."), not exhaustive. The first-segment-with-depluralisation rule is mechanically consistent across all 22 tracks and matches the spec examples for T3, T5, T8, and T12. T16 is the sole divergence. If the planner intends `"ledger"` for this track, a future slice can add a small literal-mapping overlay (e.g. `kindOverrides["T16-verdict-ledger"] = "ledger"`) without changing the general rule.

### lastImplModel capture for goto

The `haltFailedVerification` label is reached via `goto` from within the for-loop. Since `implModelID` is declared with `:=` inside the loop body, it's out of scope at the label. Solution: declared `lastImplModel string` in the pre-loop var block and set `lastImplModel = implModelID` immediately after the `:=` assignment. The FAIL path uses `lastImplModel`; the PASS and BLOCKED paths (still inside the loop) use `implModelID` directly.
