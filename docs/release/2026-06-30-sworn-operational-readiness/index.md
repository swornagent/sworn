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

Two independent tracks (touchpoint-disjoint, see matrix). Serial execution is fine —
parallelism is not a goal. **Only T1/D6 is tonight-critical** (the fired overnight run needs
it); T2 is a records-discipline cleanup that can land any time.

## Slices

| Slice | Track | Outcome | State | E×C |
|-------|-------|---------|-------|-----|
| `S01-d6-record-reconciliation` | T1 | sworn reads + round-trips a real coach status.json (object-form open_deferrals/violations) without unmarshal error or field loss | planned | epic (high/high) |
| `S02-board-render` | T2 | `sworn render` deterministically generates index.md from board.json + slice records; no model/human authors the board view | planned | chore (low/low) |

## Touchpoint matrix (Phase 3b)

Every planned-write file on one axis, tracks on the other. No file may appear in two
tracks — disjointness is what licenses the two tracks to run independently.

| File | T1 | T2 |
|------|----|----|
| internal/state/state.go | ✓ | |
| internal/verify/verify.go | ✓ | |
| internal/verify/validate_blocked.go | ✓ | |
| internal/run/slice.go | ✓ | |
| internal/implement/implement.go | ✓ | |
| internal/implement/proof_record.go | ✓ | |
| internal/implement/spec_record.go | ✓ | |
| internal/mcp/tools_ops.go | ✓ | |
| internal/mcp/tools_plan.go | ✓ | |
| internal/board/oracle.go | ✓ | |
| internal/router/router.go | ✓ | |
| internal/ledger/ledger.go | ✓ | |
| cmd/sworn/route.go | ✓ | |
| cmd/sworn/verify.go | ✓ | |
| cmd/sworn/task.go | ✓ | |
| internal/baton/schemas/slice-status-v1.json | ✓ | |
| internal/board/render.go | | ✓ |
| internal/board/render_test.go | | ✓ |
| cmd/sworn/render.go | | ✓ |

No file is marked under two tracks → T1 and T2 are disjoint.

## Dependency graph

```
T1-operational-unblock (no external deps)   [tonight-critical]
  └─ S01-d6-record-reconciliation

T2-board-render (no external deps)           [any time]
  └─ S02-board-render
```

## Out of scope (deferred, Rule 2 — see intake.md)

- S02-retry-reset-preserves-work, S03-escalation-honours-config (autonomy hardening)
- Keystone schema-family rollout (review-v1/design-v1/orchestrator/coach)
- Multi-track parallelism proof; baton-web schema publish; T16 capture remainder
