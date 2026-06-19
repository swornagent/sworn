---
title: 'Release board — 2026-06-19-safe-parallelism'
description: 'R3 — safe parallelism: concurrent multi-track delivery with fail-closed verify gate under concurrency, sworn top concurrency monitor, overclaim benchmark, and sworn account credits on-ramp.'
release_worktree_path:
release_worktree_branch: release-wt/2026-06-19-safe-parallelism
tracks: []
---

# Release Board: `2026-06-19-safe-parallelism`

> Frontmatter is the machine-readable registry; the tables below mirror it. Keep them in sync.
> Track grouping and touchpoint matrix will be filled in during Phase 3b of planning.

## Release summary

- **Goal**: concurrent multi-track delivery with the fail-closed verify gate intact under
  concurrency; `sworn top` concurrency monitor; formal overclaim benchmark; commercial
  on-ramp (`sworn account` / Credits tier). See `intake.md` for the full form.
- **Target version / integration branch**: `release/v0.1.0`
- **Prerequisite**: `2026-06-16-fidelity-layer` fully merged before implementation begins
- **Started**: 2026-06-19
- **Target ship**: uncommitted
- **Intake**: `intake.md`
- **Stakeholder**: Brad (maintainer)
- **Tracking issue**: TBD (create before first implementation session)

## Tracks

> To be filled in during Phase 3b — slice decomposition not yet finalised.

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|
| *(TBD)* | | | | |

### Touchpoint matrix

> To be filled in once slices and tracks are confirmed.

## Slices

> To be filled in once decomposition is confirmed.

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|
| *(TBD)* | | | | | | |

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

- Planned: 0
- In progress: 0
- Implemented: 0
- Verified: 0
- Failed verification: 0
- Deferred: 0

**Tracks:** TBD

## Recent activity

### 2026-06-19 — release folder initialised; intake written

- **Actor**: planner (human + Claude)
- **Note**: Discovery complete; commercialisation model captured; slice decomposition
  in progress. Tracks and touchpoint matrix TBD.

## Decisions deferred (Rule 2)

See `intake.md` "Adjacent / out of scope" for full deferral cards.

- Full SaaS billing infrastructure (post-R3)
- GitHub Action / Marketplace integration (post-R3)
- Compliance ledger (post-launch)
- Team collaboration features (post-R3)
- Async paging/notifications (TBD during decomposition)

## Cross-slice / cross-track notes

> To be filled in during Phase 3.

- S01 (process-ownership) is likely a hard prerequisite for S02 (concurrent scheduler) —
  cannot run concurrent workers safely without the ownership registry.
- `sworn top` extension (S04) touches `cmd/sworn/top.go` — same file as R2's S15.
  R3 must `depends_on` R2 fully merged; no collision risk if sequenced correctly.
- `sworn account` (S06 TBD) introduces a new backend dependency (SwornAgent cloud API).
  This is the first network call to a SwornAgent-owned endpoint; architecture decision
  needed (separate ADR or inline in spec).
