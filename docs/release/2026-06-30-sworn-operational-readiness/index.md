---
title: 'Release board — 2026-06-30-sworn-operational-readiness'
description: 'Rendered view of board.json. Source of truth is board.json (the oracle reads it preferentially); do not hand-edit track facts here.'
---

# Release: 2026-06-30-sworn-operational-readiness

**Goal:** sworn drives a real coach-produced release to merged unattended, so getfired's
blocked release can land via the autonomous loop. Proven by completing the fired dogfood
release overnight. Parallelism out of scope (serial is acceptable).

**Integration branch:** `release/v0.1.0` · **Target version:** v0.1.0

> NOTE: this file is still hand-authored — `S02-board-render` exists precisely to make it
> a deterministic `sworn render` output. Until S02 ships, board.json (which the oracle reads
> preferentially) remains the source of truth; treat this view as informational.

## Tracks

| Track | Slices | depends_on | State |
|-------|--------|------------|-------|
| `T1-operational-unblock` | `S01-d6-record-reconciliation` | — | in_progress |
| `T2-board-render` | `S02-board-render` | — | planned |
| `T3-consumer-repo-hygiene` | `S03-sworn-self-ignore` | — | planned |
| `T4-board-record-reconciliation` | `S04-board-record-reconciliation` | — | planned |

Four independent tracks (touchpoint-disjoint, see matrix). Serial execution is fine —
parallelism is not a goal. **T1/D6 AND T4/board-record are tonight-critical** — the fired run
fails at board-read (T4) before status-read (T1/D6), so both are needed to run fired. T2
(records-discipline) and T3 (repo hygiene) can land any time.

## Slices

| Slice | Track | Outcome | State | E×C |
|-------|-------|---------|-------|-----|
| `S01-d6-record-reconciliation` | T1 | sworn reads + round-trips a real coach status.json (object-form open_deferrals/violations) without unmarshal error or field loss | planned | epic (high/high) |
| `S02-board-render` | T2 | `sworn render` deterministically generates index.md from board.json + slice records; no model/human authors the board view | planned | chore (low/low) |
| `S03-sworn-self-ignore` | T3 | sworn writes `.sworn/.gitignore` so its runtime state never dirties or gets committed to a consumer repo | planned | chore (low/low) |
| `S04-board-record-reconciliation` | T4 | oracle reads the canonical coach board.json (`release` object form), tolerating the legacy string — the board-level companion to D6 | planned | chore (low/low) |

## Touchpoint matrix (Phase 3b)

Every planned-write file on one axis, tracks on the other. No file may appear in two
tracks — disjointness is what licenses the two tracks to run independently.

| File | T1 | T2 | T3 | T4 |
|------|----|----|----|----|
| internal/state/state.go | ✓ | | |
| internal/verify/verify.go | ✓ | | |
| internal/verify/validate_blocked.go | ✓ | | |
| internal/run/slice.go | ✓ | | |
| internal/implement/implement.go | ✓ | | |
| internal/implement/proof_record.go | ✓ | | |
| internal/implement/spec_record.go | ✓ | | |
| internal/mcp/tools_ops.go | ✓ | | |
| internal/mcp/tools_plan.go | ✓ | | |
| internal/board/oracle.go | ✓ | | |
| internal/router/router.go | ✓ | | |
| internal/ledger/ledger.go | ✓ | | |
| cmd/sworn/route.go | ✓ | | |
| cmd/sworn/verify.go | ✓ | | |
| cmd/sworn/task.go | ✓ | | |
| internal/baton/schemas/slice-status-v1.json | ✓ | | |
| internal/board/render.go | | ✓ | |
| internal/board/render_test.go | | ✓ | |
| cmd/sworn/render.go | | ✓ | |
| internal/db/db.go | | | ✓ | |
| internal/db/db_test.go | | | ✓ | |
| internal/board/board.go | | | | ✓ |
| cmd/sworn/board.go | | | | ✓ |
| internal/board/board_test.go | | | | ✓ |
| internal/baton/schemas/board-v1.json | | | | ✓ |

No file is marked under more than one track → T1, T2, T3, T4 are mutually disjoint.
(Note: T1 touches internal/board/oracle.go and T4 touches internal/board/board.go — same
package, different files; oracle.go does not reference BoardRecord.Release, so the type change
in T4 does not break T1. Disjoint at file granularity, compatible at package granularity.)

## Dependency graph

```
T1-operational-unblock (no external deps)        [tonight-critical: status-read]
  └─ S01-d6-record-reconciliation

T4-board-record-reconciliation (no external deps) [tonight-critical: board-read]
  └─ S04-board-record-reconciliation

T2-board-render (no external deps)                [any time]
  └─ S02-board-render

T3-consumer-repo-hygiene (no external deps)       [any time]
  └─ S03-sworn-self-ignore
```

## Out of scope (deferred, Rule 2 — see intake.md)

- S02-retry-reset-preserves-work, S03-escalation-honours-config (autonomy hardening)
- Keystone schema-family rollout (review-v1/design-v1/orchestrator/coach)
- Multi-track parallelism proof; baton-web schema publish; T16 capture remainder
