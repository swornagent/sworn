---
title: 'Release board — 2026-06-30-sworn-operational-readiness'
description: 'Rendered view of board.json. Source of truth is board.json (the oracle reads it preferentially); do not hand-edit track facts here.'
---

# Release: 2026-06-30-sworn-operational-readiness

**Goal:** sworn drives a real coach-produced release to merged unattended, so getfired's
blocked release can land via the autonomous loop. Proven by completing the fired dogfood
release overnight. Parallelism out of scope (serial is acceptable).

**Integration branch:** `release/v0.1.0` · **Target version:** v0.1.0

## Tracks

| Track | Slices | depends_on | State |
|-------|--------|------------|-------|
| `T1-operational-unblock` | `S01-d6-record-reconciliation` | — | planned |

Single track, single slice — no parallelism. The slice is an atomic type migration
(cannot compile half-migrated), so it is one slice despite sitting at the file-count ceiling.

## Slices

| Slice | Track | Outcome | State | E×C |
|-------|-------|---------|-------|-----|
| `S01-d6-record-reconciliation` | T1 | sworn reads + round-trips a real coach status.json (object-form open_deferrals/violations) without unmarshal error or field loss | planned | epic (high/high) |

## Touchpoint matrix (Phase 3b)

Every planned-write file on one axis, tracks on the other. No file may appear in two
tracks. With a single track this is disjoint by construction.

| File | T1 |
|------|----|
| internal/state/state.go | ✓ |
| internal/verify/verify.go | ✓ |
| internal/verify/validate_blocked.go | ✓ |
| internal/run/slice.go | ✓ |
| internal/implement/implement.go | ✓ |
| internal/implement/proof_record.go | ✓ |
| internal/implement/spec_record.go | ✓ |
| internal/mcp/tools_ops.go | ✓ |
| internal/mcp/tools_plan.go | ✓ |
| internal/board/oracle.go | ✓ |
| internal/router/router.go | ✓ |
| internal/ledger/ledger.go | ✓ |
| cmd/sworn/route.go | ✓ |
| cmd/sworn/verify.go | ✓ |
| cmd/sworn/task.go | ✓ |
| internal/baton/schemas/slice-status-v1.json | ✓ |

## Dependency graph

```
T1-operational-unblock (no external deps)
  └─ S01-d6-record-reconciliation
```

## Out of scope (deferred, Rule 2 — see intake.md)

- S02-retry-reset-preserves-work, S03-escalation-honours-config (autonomy hardening)
- Keystone schema-family rollout (review-v1/design-v1/orchestrator/coach)
- Multi-track parallelism proof; baton-web schema publish; T16 capture remainder
