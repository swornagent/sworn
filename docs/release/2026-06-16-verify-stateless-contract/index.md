---
title: Release board — 2026-06-16-verify-stateless-contract
description: Patch on v0.1 — stateless prompt/parser contract for the verify gate. 3 slices, one track.
release_worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-16-verify-stateless-contract
release_worktree_branch: release-wt/2026-06-16-verify-stateless-contract
tracks:
  - id: T1-verify-contract
    slices: [S01-stateless-verify-prompt, S02-tolerant-verdict-parser, S03-run-loop-verify-reachability]
    depends_on: null
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-16-verify-stateless-contract-T1-verify-contract
    worktree_branch: track/2026-06-16-verify-stateless-contract/T1-verify-contract
    state: in_progress
    e2e_specs: []
---

# Release Board: `2026-06-16-verify-stateless-contract`

## Release summary

- **Goal**: give the stateless `verify` path its own prompt/parser contract so the
  verification gate returns parseable verdicts instead of spurious
  `BLOCKED / unparseable_verdict`. Cite `intake.md` for the long form.
- **Target version / integration branch**: `release/v0.1.0` — pre-release bug fix
  folded in before first ship (nothing released yet; owner-confirmed 2026-06-16).
- **Started**: 2026-06-16
- **Target ship**: uncommitted
- **Intake**: `intake.md`
- **Stakeholder**: repo owner

## Tracks

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|
| `T1-verify-contract` | S01 → S02 → S03 | — | `track/2026-06-16-verify-stateless-contract/T1-verify-contract` | planned |

Track state: `planned` → `in_progress` → `merged`.

### Touchpoint matrix

> One track this release, so disjointness is trivially satisfied — the matrix is
> recorded for completeness and to license a future split if S03 grows.

| File / surface | T1 |
|---|---|
| `internal/prompt/verify-stateless.md` (new) | ✓ |
| `internal/prompt/prompt.go` | ✓ |
| `internal/verify/verify.go` | ✓ |
| `internal/verify/verify_test.go` | ✓ |
| `internal/run/run_test.go` | ✓ |

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|
| `S01-stateless-verify-prompt` | T1 | The verify path is told "judge from SPEC+DIFF only, verdict-leading, no tools" instead of the agentic role prompt | planned | human | [spec](./S01-stateless-verify-prompt/spec.md) | — |
| `S02-tolerant-verdict-parser` | T1 | A reply with markdown emphasis / leading prose / a leaked tool-call line still resolves to the intended verdict, fail-closed on ambiguity | planned | human | [spec](./S02-tolerant-verdict-parser/spec.md) | — |
| `S03-run-loop-verify-reachability` | T1 | `sworn run`'s verify gate lands a parseable verdict end-to-end instead of always BLOCKED | planned | human | [spec](./S03-run-loop-verify-reachability/spec.md) | — |

### State legend

| State | Meaning | Who can move out of it |
|---|---|---|
| `planned` | Spec written, awaiting implementation | Implementer |
| `in_progress` | Implementer session active | Implementer |
| `implemented` | Implementer claims done; awaiting fresh-context verification | Verifier |
| `verified` | Fresh-context verifier returned PASS | Human (`/merge-track`) |
| `failed_verification` | Verifier returned FAIL; fix and re-submit | Implementer |
| `deferred` | Slice carved out per Rule 2; not in this release | Human |
| `shipped` | Slice is live in production | — (terminal) |

## Aggregate state

- Planned: 3
- In progress: 0
- Implemented (awaiting verification): 0
- Verified (awaiting merge): 0
- Failed verification: 0
- Deferred: 0
- Shipped: 0

**Tracks:** Planned: 1 / In progress: 0 / Merged: 0

## Recent activity

### 2026-06-16 — release planned

- **Actor**: planner
- **Note**: 3 slices, one track. Specs written; board initialised.

## Decisions deferred (Rule 2)

- **Structured-output verdict** (`response_format` / tool-call schema): deferred —
  uneven provider support, trades provider neutrality. Roadmap. Ack 2026-06-16.
- **Agentic tool-using verifier** (a real `verifier.md` consumer): deferred to a
  later release.
- **Live ≥3-provider conformance as a committed test**: deferred — needs
  network/keys; runs as a manual reachability step in S03, committed test uses
  synthetic fixtures.

## Cross-slice / cross-track notes

- All three slices share `internal/verify/verify.go` lineage → single track,
  strictly sequential.
- **`verifier.md` orphaning**: after S01, the embedded `verifier.md` is no longer
  the verify-path system prompt and has no code consumer. It is **intentionally
  retained** in the embed set as the vendored Baton protocol artefact (provenance
  / `sworn version`). Any future move to drop it is a separate, tracked decision.
- **Public-safe fixtures**: S02's regression fixtures must be synthetic spec+diff,
  never the private dogfood slice used as evidence in the findings note.
