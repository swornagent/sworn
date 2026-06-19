---
title: 'Release board — 2026-06-19-safe-parallelism'
description: 'R3 — safe parallelism: concurrent multi-track delivery, fail-closed verify gate under concurrency, sworn TUI cockpit, overclaim benchmark, sworn login credits on-ramp, webhook paging, and MCP server for AI-driven planning + resolution.'
release_worktree_path:
release_worktree_branch: release-wt/2026-06-19-safe-parallelism
tracks:
  - id: T1-concurrency-core
    slices: [S01-process-ownership, S02-concurrent-scheduler, S03-verify-under-concurrency]
    depends_on: null
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T1-concurrency-core
    state: planned
  - id: T2-monitoring
    slices: [S04-sworn-tui, S05-overclaim-benchmark]
    depends_on: T1-concurrency-core
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T2-monitoring
    state: planned
  - id: T3-commercial
    slices: [S06-sworn-login, S07-paging]
    depends_on: T1-concurrency-core
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T3-commercial
    state: planned
  - id: T4-mcp
    slices: [S08-sworn-mcp]
    depends_on: T1-concurrency-core
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T4-mcp
    state: planned
---

# Release Board: `2026-06-19-safe-parallelism`

> Frontmatter is the machine-readable registry; the tables below mirror it. Keep them in sync.
> **Prerequisite**: `2026-06-16-fidelity-layer` must be fully merged before R3 implementation begins.

## Release summary

- **Goal**: concurrent multi-track delivery with the fail-closed verify gate provably
  intact under concurrency (overclaim rate flat 1→N); `sworn` TUI cockpit with
  blocked-slice TL;DR and resolution options; `sworn login` credits on-ramp; webhook
  paging; `sworn mcp` as a universal AI planning + operations interface.
- **Target version / integration branch**: `release/v0.1.0`
- **Prerequisite release**: `2026-06-16-fidelity-layer` — fully merged before implementation
- **Started**: 2026-06-19
- **Target ship**: uncommitted
- **Intake**: `intake.md`
- **Stakeholder**: Brad (maintainer)
- **Tracking issue**: TBD (create before first implementation session — Rule 5)

## Tracks

> T1 goes first. T2, T3, T4 all `depends_on T1` and run in parallel after T1 merges.

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|
| `T1-concurrency-core` | S01 → S02 → S03 | — | `track/2026-06-19-safe-parallelism/T1-concurrency-core` | planned |
| `T2-monitoring` | S04 → S05 | T1 | `track/2026-06-19-safe-parallelism/T2-monitoring` | planned |
| `T3-commercial` | S06 → S07 | T1 | `track/2026-06-19-safe-parallelism/T3-commercial` | planned |
| `T4-mcp` | S08 | T1 | `track/2026-06-19-safe-parallelism/T4-mcp` | planned |

### Touchpoint matrix

> T1 owns the shared concurrency core. T2/T3/T4 are mutually disjoint.
> No row carries a `✓` in more than one column of the parallel set {T2, T3, T4}.
> `cmd/sworn/main.go` is a **documented shared file** (additive dispatch only — each
> track adds its own `case`; no overlapping regions).

| File / surface | T1 | T2 | T3 | T4 |
|---|---|---|---|---|
| `docs/adr/0003-sqlite-orchestration-state.md` | ✓ | | | |
| `internal/db/` (new) | ✓ | | | |
| `internal/supervisor/` (new) | ✓ | | | |
| `internal/scheduler/` (new) | ✓ | | | |
| `internal/run/run.go` | ✓ | | (T3 via dep) | |
| `internal/run/parallel.go` (new) | ✓ | | | |
| `internal/verify/verify.go` | ✓ | | | |
| `internal/verify/concurrent_test.go` (new) | ✓ | | | |
| `internal/verdict/verdict.go` | ✓ | | | |
| `internal/model/oai.go` | ✓ | | | |
| `internal/model/client.go` | ✓ | | (T3 via dep) | |
| `cmd/sworn/run.go` | ✓ | | | |
| `go.mod`, `go.sum` | ✓ | | | |
| `cmd/sworn/main.go` (DOCUMENTED SHARED — additive dispatch) | ✓ | ✓ | ✓ | ✓ |
| `cmd/sworn/top.go` | | ✓ | | |
| `internal/tui/` (new) | | ✓ | | |
| `internal/bench/overclaim.go` (new) | | ✓ | | |
| `internal/bench/overclaim_test.go` (new) | | ✓ | | |
| `cmd/sworn/bench.go` | | ✓ | | |
| `docs/benchmark/overclaim-concurrent-1to4.md` (new) | | ✓ | | |
| `internal/account/account.go` (new) | | | ✓ | |
| `internal/account/proxy.go` (new) | | | ✓ | |
| `internal/account/notify.go` (new) | | | ✓ | |
| `internal/scheduler/worker.go` | | | (T3 via dep) | |
| `cmd/sworn/login.go` (new) | | | ✓ | |
| `cmd/sworn/account.go` (new) | | | ✓ | |
| `internal/config/config.go` | | | ✓ | |
| `internal/mcp/` (new) | | | | ✓ |
| `cmd/sworn/mcp.go` (new) | | | | ✓ |
| `docs/mcp-setup.md` (new) | | | | ✓ |

**T3 via dep notes:**
- `internal/run/run.go`: S07 (T3) adds notification calls; serialised by `depends_on T1`
  (T3 worktree branches from release-wt after T1 merges, so T3 sees T1's run.go changes)
- `internal/model/client.go`: S06 (T3) adds proxy routing; T1 may modify client.go for
  goroutine safety (S03); serialised by `depends_on T1`
- `internal/scheduler/worker.go`: S07 (T3) adds notify call; S02 (T1) creates the file;
  serialised by `depends_on T1`

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|
| `S01-process-ownership` | T1 | Reap-on-restart + single-owner identity; SQLite registry | planned | human | [spec](./S01-process-ownership/spec.md) | — |
| `S02-concurrent-scheduler` | T1 | `sworn run --parallel` launches all independent tracks concurrently | planned | human | [spec](./S02-concurrent-scheduler/spec.md) | — |
| `S03-verify-under-concurrency` | T1 | Verify gate goroutine-safe and fail-closed at N>1 | planned | human | [spec](./S03-verify-under-concurrency/spec.md) | — |
| `S04-sworn-tui` | T2 | `sworn` (no args) opens the management cockpit with live status + blocked TL;DR | planned | human | [spec](./S04-sworn-tui/spec.md) | — |
| `S05-overclaim-benchmark` | T2 | Overclaim rate provably flat at N=1/2/4; published artefact | planned | human | [spec](./S05-overclaim-benchmark/spec.md) | — |
| `S06-sworn-login` | T3 | `sworn login` authenticates + routes model calls via SwornAgent proxy (credits) | planned | human | [spec](./S06-sworn-login/spec.md) | — |
| `S07-paging` | T3 | FAIL/BLOCKED fires webhook + email so developer is paged without watching terminal | planned | human | [spec](./S07-paging/spec.md) | — |
| `S08-sworn-mcp` | T4 | `sworn mcp` exposes all board state + planning tools to any MCP-compatible AI | planned | human | [spec](./S08-sworn-mcp/spec.md) | — |

### State legend

| State | Meaning | Who can move out of it |
|---|---|---|
| `planned` | Spec written, awaiting implementation | Implementer |
| `in_progress` | Implementer session active | Implementer |
| `implemented` | Implementer claims done; awaiting fresh-context verification | Verifier |
| `verified` | Fresh-context verifier returned PASS | Human (`/merge-track`) |
| `failed_verification` | Verifier returned FAIL; fix and re-submit | Implementer |
| `deferred` | Slice carved out per Rule 2 | Human |
| `shipped` | Live in production | — (terminal) |

## Aggregate state

- Planned: 8
- In progress: 0
- Implemented: 0
- Verified: 0
- Failed verification: 0
- Deferred: 0

**Tracks:** Planned: 4 / In progress: 0 / Merged: 0

## Recent activity

### 2026-06-19 — release planned; 2026-06-20 — specs written

- **Actor**: planner (human + Claude)
- **Note**: Discovery session 2026-06-19. 8 slices, 4 tracks specced to `planned`.
  T1 (concurrency core) goes first; T2/T3/T4 depend on T1 and run in parallel after
  it merges. Handed off for implementation pending R2 merge.

## Decisions deferred (Rule 2)

See `intake.md` "Adjacent / out of scope" for full deferral cards.

- Full SaaS billing infrastructure (post-R3) — implementation on SwornAgent backend
- GitHub Action / Marketplace integration (post-R3)
- Compliance ledger / signed attestation (post-launch)
- Team/shared credit pools (post-R3)
- MCP HTTP/SSE transport (post-R3)
- sworn mcp daemon mode (post-R3)
- TUI mouse support (post-R3)
- AI tool list beyond Claude Code + Codex in TUI (post-R3, env-configurable)
- Windows OS support for supervisor PID liveness (post-R3; Unix only in R3)
- Email notification via SwornAgent API (if backend not ready at implementation time,
  stub with log line; backend build timeline is independent)

## Cross-slice / cross-track notes

- **S01 is T1's keystone.** S02 and S03 both depend on the DB schema and supervisor
  API S01 creates. S01 must be implemented and verified before S02 starts.
- **T3's two `depends_on T1` touchpoints** (`run.go`, `worker.go`) are safe because
  T3's worktree branches from release-wt AFTER T1 merges. No merge conflict expected;
  T3's additions are additive (notification calls, not structural rewrites).
- **`cmd/sworn/main.go`** is a documented shared file. Each track adds one or more
  `case` statements to the dispatch switch. Convention from R1: keep each command's
  implementation in its own `cmd/sworn/<cmd>.go` file. The dispatch case is the only
  shared edit — additive, region-separable.
- **R2 S15 (`sworn top`) coordination**: T2's S04 absorbs/replaces `cmd/sworn/top.go`.
  Because R3 implementation gates on R2 being merged first, there is no parallel-edit
  risk. The S04 implementer should check the final state of `top.go` from R2's merge.
- **SwornAgent backend dependency** (S06, S07, S08 partially): auth endpoint, proxy
  endpoint, `/api/notify` endpoint. These are not in this repo. Each dependent slice
  uses mock servers in tests and staging endpoints in smoke steps. See individual specs.
