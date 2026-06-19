---
title: 'Release board — 2026-06-19-safe-parallelism'
description: 'R3 — safe parallelism: concurrent multi-track delivery, fail-closed verify gate under concurrency, sworn TUI cockpit, overclaim benchmark, sworn login credits on-ramp, webhook paging, and MCP server for AI-driven planning + resolution.'
release_worktree_path:
release_worktree_branch: release-wt/2026-06-19-safe-parallelism
tracks:
  - id: T1-concurrency-core
    slices: [S01-process-ownership, S02a-run-refactor, S02b-concurrent-scheduler, S03-verify-under-concurrency]
    depends_on: null
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T1-concurrency-core
    state: planned
  - id: T2-monitoring
    slices: [S04a-tui-foundation, S04b-tui-live, S04c-tui-resolution, S05-overclaim-benchmark]
    depends_on: T1-concurrency-core
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T2-monitoring
    state: planned
  - id: T3-commercial
    slices: [S06a-sworn-login-auth, S06b-sworn-proxy-credits, S07-paging]
    depends_on: T1-concurrency-core
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T3-commercial
    state: planned
  - id: T4-mcp
    slices: [S08a-mcp-transport, S08b-mcp-ops-tools, S08c-mcp-plan-tools]
    depends_on: T1-concurrency-core
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T4-mcp
    state: planned
---

# Release Board: `2026-06-19-safe-parallelism`

> Frontmatter is the machine-readable registry; the tables below mirror it.
> **Prerequisite**: `2026-06-16-fidelity-layer` must be fully merged before R3 implementation begins.

## Release summary

- **Goal**: concurrent multi-track delivery with fail-closed verify gate provably intact
  under concurrency; `sworn` TUI cockpit with blocked-slice TL;DR; `sworn login` credits
  on-ramp; webhook paging; `sworn mcp` as a universal AI planning + operations interface.
- **Target version / integration branch**: `release/v0.1.0`
- **Prerequisite release**: `2026-06-16-fidelity-layer` — fully merged before implementation
- **Started**: 2026-06-19
- **Target ship**: uncommitted
- **Intake**: `intake.md`
- **Stakeholder**: Brad (maintainer)
- **Tracking issue**: TBD (create before first implementation session)

## Tracks

> T1 goes first. T2, T3, T4 all `depends_on T1` and run in parallel after T1 merges.

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|
| `T1-concurrency-core` | S01 → S02a → S02b → S03 | — | `track/.../T1-concurrency-core` | planned |
| `T2-monitoring` | S04a → S04b → S04c → S05 | T1 | `track/.../T2-monitoring` | planned |
| `T3-commercial` | S06a → S06b → S07 | T1 | `track/.../T3-commercial` | planned |
| `T4-mcp` | S08a → S08b → S08c | T1 | `track/.../T4-mcp` | planned |

### Touchpoint matrix

> No row may carry `✓` in more than one column of the parallel set {T2, T3, T4}.
> `cmd/sworn/main.go` is a **documented shared file** (additive dispatch only).

| File / surface | T1 | T2 | T3 | T4 |
|---|---|---|---|---|
| `docs/adr/0003-sqlite-orchestration-state.md` | ✓ | | | |
| `internal/db/` (new) | ✓ | | | |
| `internal/supervisor/` (new) | ✓ | | | |
| `internal/run/run.go` | ✓ | | (T1 dep) | |
| `internal/run/slice.go` (new) | ✓ | | | |
| `internal/run/parallel.go` (new) | ✓ | | | |
| `internal/run/run_test.go` | ✓ | | | |
| `internal/scheduler/` (new) | ✓ | | | |
| `internal/verify/verify.go` | ✓ | | | |
| `internal/verify/concurrent_test.go` (new) | ✓ | | | |
| `internal/verdict/verdict.go` | ✓ | | | |
| `internal/model/oai.go` | ✓ | | | |
| `internal/model/client.go` | ✓ | | (T1 dep) | |
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
| `internal/account/account_test.go` (new) | | | ✓ | |
| `internal/account/proxy_test.go` (new) | | | ✓ | |
| `internal/account/notify_test.go` (new) | | | ✓ | |
| `internal/scheduler/worker.go` (new, in T1) | ✓ | | (T1 dep) | |
| `cmd/sworn/login.go` (new) | | | ✓ | |
| `cmd/sworn/account.go` (new) | | | ✓ | |
| `internal/config/config.go` | | | ✓ | |
| `internal/mcp/` (new) | | | | ✓ |
| `cmd/sworn/mcp.go` (new) | | | | ✓ |
| `docs/mcp-setup.md` (new) | | | | ✓ |

**T3 `depends_on T1` notes:**
- `internal/run/run.go`: S07 adds notification calls; serialised by dep edge
- `internal/model/client.go`: S06b adds proxy routing; S03 may audit it; serialised by dep
- `internal/scheduler/worker.go`: S07 adds notify call; S02b creates it; serialised by dep

## Slices

| ID | Track | User outcome | State | Spec |
|---|---|---|---|---|
| `S01-process-ownership` | T1 | SQLite registry + reap-on-restart; single-owner identity | planned | [spec](./S01-process-ownership/spec.md) |
| `S02a-run-refactor` | T1 | `run.RunSlice()` exported; callable from goroutine; no regression | planned | [spec](./S02a-run-refactor/spec.md) |
| `S02b-concurrent-scheduler` | T1 | `sworn run --parallel` launches all independent tracks concurrently | planned | [spec](./S02b-concurrent-scheduler/spec.md) |
| `S03-verify-under-concurrency` | T1 | Verify gate goroutine-safe and fail-closed at N>1 | planned | [spec](./S03-verify-under-concurrency/spec.md) |
| `S04a-tui-foundation` | T2 | `sworn` (no args) shows releases list + board view with navigation | planned | [spec](./S04a-tui-foundation/spec.md) |
| `S04b-tui-live` | T2 | Live concurrent track status from DB (1s poll) + credit balance in header | planned | [spec](./S04b-tui-live/spec.md) |
| `S04c-tui-resolution` | T2 | Blocked slice TL;DR panel + options + open in Claude Code / Codex | planned | [spec](./S04c-tui-resolution/spec.md) |
| `S05-overclaim-benchmark` | T2 | Overclaim rate flat at N=1/2/4; published benchmark artefact | planned | [spec](./S05-overclaim-benchmark/spec.md) |
| `S06a-sworn-login-auth` | T3 | `sworn login` device-code flow; credentials file; `sworn logout` | planned | [spec](./S06a-sworn-login-auth/spec.md) |
| `S06b-sworn-proxy-credits` | T3 | Model calls route via SwornAgent proxy; `sworn account buy`; credit display | planned | [spec](./S06b-sworn-proxy-credits/spec.md) |
| `S07-paging` | T3 | FAIL/BLOCKED fires webhook + email; developer paged without watching terminal | planned | [spec](./S07-paging/spec.md) |
| `S08a-mcp-transport` | T4 | `sworn mcp` JSON-RPC server; initialize handshake; tools scaffold | planned | [spec](./S08a-mcp-transport/spec.md) |
| `S08b-mcp-ops-tools` | T4 | 9 ops tools: get_board, get_blocked, get_slice_context, rerun, patch, merge, defer | planned | [spec](./S08b-mcp-ops-tools/spec.md) |
| `S08c-mcp-plan-tools` | T4 | 4 planning tools + resources + prompts + mcp-setup.md | planned | [spec](./S08c-mcp-plan-tools/spec.md) |

## Aggregate state

- Planned: 14
- In progress: 0
- Implemented: 0
- Verified: 0
- Failed verification: 0
- Deferred: 0

**Tracks:** Planned: 4 / In progress: 0 / Merged: 0

## Recent activity

### 2026-06-20 — re-decomposed from 8 to 14 slices

- **Actor**: planner (human + Claude)
- **Note**: 4 over-scoped slices split on review: S02→S02a+S02b, S04→S04a+S04b+S04c,
  S06→S06a+S06b, S08→S08a+S08b+S08c. Each split slice is now a genuine
  one-implementer-session + one-verifier-session unit.

### 2026-06-19 — release planned; specs written

- **Actor**: planner (human + Claude)
- **Note**: 14 slices, 4 tracks. T1 first; T2/T3/T4 parallel after T1.

## Decisions deferred (Rule 2)

See `intake.md` "Adjacent / out of scope" for full deferral cards.

- Full SaaS billing infrastructure (post-R3)
- GitHub Action / Marketplace integration (post-R3)
- Compliance ledger (post-launch)
- Team credit pools (post-R3)
- MCP HTTP/SSE transport (post-R3)
- sworn mcp daemon mode (post-R3)
- TUI mouse support (post-R3)
- AI tool list beyond CC + Codex in TUI (post-R3)
- Windows supervisor PID liveness (post-R3)
- Email via SwornAgent API in S07 (stub if backend not ready)
- `resources/list` dynamic scanning (post-R3)
- TUI auto-fix action [1] subprocess management (may be stubbed — see S04c)

## Cross-slice / cross-track notes

- **S01 is T1's keystone.** S02a, S02b, S03 all depend on the DB + supervisor API.
- **S02a before S02b.** The RunSlice() refactor must be verified before the scheduler
  is built on top of it.
- **S04a before S04b before S04c.** Each TUI slice extends the previous foundation.
- **S06a before S06b.** Proxy routing requires credentials from the auth flow.
- **S08a before S08b before S08c.** Transport must work before tools are registered.
- **T3 serialised behind T1** via `depends_on`: S07 touches `run.go` and `worker.go`
  created by T1. The dep edge ensures T3's worktree starts from release-wt after T1's
  changes are merged — no touchpoint conflict.
- **`cmd/sworn/main.go`** is documented shared. Each track adds additive dispatch
  cases only. Each command implementation lives in its own `cmd/sworn/<cmd>.go` file.
- **R2 S15 (`sworn top`) coordination**: S04a absorbs/extends `cmd/sworn/top.go`.
  R3 implementation gates on R2 being merged; no parallel-edit risk.
