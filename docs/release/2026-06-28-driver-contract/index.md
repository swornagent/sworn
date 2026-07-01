---
title: 'Release board — 2026-06-28-driver-contract'
description: 'Re-seam sworn so the orchestrator drives delivery through a stable Driver contract — never reimplementing the agent loop or provider wire format in-process — and validate the Go engine differentially against the coach-loop reference. Keystone of the 2026-06-28 architecture recommendation.'
release_worktree_path: # set by first /implement-slice in this release
release_worktree_branch: release-wt/2026-06-28-driver-contract
tracks:
  - id: T1-driver-contract
    slices: [S01-driver-interface, S02-subprocess-agent-driver, S03-inprocess-oai-driver, S04-driver-registry-resolution]
    depends_on: null
    worktree_path: # set by first /implement-slice in this track
    worktree_branch: track/2026-06-28-driver-contract/T1-driver-contract
    state: planned
  - id: T2-orchestrator-rewire
    slices: [S05-runslice-via-driver, S06-scheduler-driver-dispatch, S07-normalized-result-telemetry]
    depends_on: T1-driver-contract
    worktree_path: # set by first /implement-slice in this track
    worktree_branch: track/2026-06-28-driver-contract/T2-orchestrator-rewire
    state: planned
  - id: T3-validate-against-reference
    slices: [S08-differential-validation, S09-driver-conformance-suite]
    depends_on: T2-orchestrator-rewire
    worktree_path: # set by first /implement-slice in this track
    worktree_branch: track/2026-06-28-driver-contract/T3-validate-against-reference
    state: planned
---

# Release Board: `2026-06-28-driver-contract`

## Release summary

- **Goal**: Make sworn a thin, deterministic orchestrator over a stable **Driver contract** ("one
  orchestrator, N drivers"). The orchestrator must never construct a provider wire message or own the
  agentic tool loop; both live behind a `Driver`. Default to a subprocess agent driver; keep one hardened
  in-process OAI driver as an option. Validate the Go engine **differentially against the coach-loop
  reference** (same release+inputs → same routing/state/verified-set).
- **Why**: the keystone recommendation from `docs/captures/2026-06-28-sworn-architecture-recommendation.md`
  (§1–§3) and the synthesis `docs/captures/2026-06-28-synthesis-and-forward-plan.md`. The three-model
  dogfood proved the harness — not the model — is the decisive variable; the in-process reimplementation of
  the agent loop + per-provider wire format is the root cause of the DOA parallel loop. This release removes
  that bug class by construction and subsumes most of the FT-2 model-layer gaps.
- **Reference model**: the bash coach loop (`docs/captures/2026-06-28-bash-coachloop-learnings.md`) — its
  runtime-driver contract is the architecture this release ports; T3 validates parity against it.
- **Target version / integration branch**: `release/v0.1.0`
- **Started**: 2026-06-28 (planning) — **Stakeholder**: Brad Sawyer

## Tracks

| Track | Slices (in order) | Depends on | State |
|---|---|---|---|
| `T1-driver-contract` | S01 → S02 → S03 → S04 | — | planned |
| `T2-orchestrator-rewire` | S05 → S06 → S07 | T1 | planned |
| `T3-validate-against-reference` | S08 → S09 | T2 | planned |

## Touchpoint matrix (DRAFT)

| File / surface | T1 | T2 | T3 |
|---|---|---|---|
| `internal/driver/*` (new package) | ✓ | | ✓ (conformance) |
| `docs/baton/runtime-drivers.md` | ✓ | | |
| `internal/run/run.go` (resolution) | ✓ | | |
| `internal/run/slice.go` | | ✓ | ✓ (differential) |
| `internal/scheduler/worker.go` | | ✓ | |
| `cmd/sworn/run.go` | | ✓ | |
| `internal/state/state.go`, `internal/supervisor/*` | | ✓ | |
| `internal/run/testdata/coachloop-reference/` (new) | | | ✓ |

## Slices

| ID | Track | User outcome | State |
|---|---|---|---|
| `S01-driver-interface` | T1 | A single `Driver` contract at the process boundary; dispatch is `Driver.Dispatch(spec, worktree) -> Result`, no in-process ChatMessage | planned |
| `S02-subprocess-agent-driver` | T1 | Default driver delegates the agent loop to a real agent CLI (claude-cli/codex); engine never owns the tool loop or wire format | planned |
| `S03-inprocess-oai-driver` | T1 | Existing agent loop + OAI client available as ONE driver behind the contract (with the content-tag fix); an option, not the default | planned |
| `S04-driver-registry-resolution` | T1 | Model selection resolves to a registered driver with fail-fast capability check; replaces the provider×capability matrix | planned |
| `S05-runslice-via-driver` | T2 | RunSlice implements+verifies only through `Driver.Dispatch`; no provider-wire coupling; the nil-factory class is gone by construction | planned |
| `S06-scheduler-driver-dispatch` | T2 | The parallel scheduler dispatches via the Driver contract; per-role/per-model from config; the S27 wiring gap cannot recur | planned |
| `S07-normalized-result-telemetry` | T2 | Every dispatch records duration/tokens/real-cost/confirmed-model-id from the Driver Result (FT-7) | planned |
| `S08-differential-validation` | T3 | The Go engine matches the coach-loop reference on routing/state/verified-set for a fixture release; divergence fails the test | planned |
| `S09-driver-conformance-suite` | T3 | Every Driver impl passes one behavioural conformance suite (content-always, exit-on-no-tools, normalized Result, fail-closed) | planned |

### State legend

`planned` → `in_progress` → `implemented` → [fresh verifier] → `verified` | `failed_verification`

## Aggregate state

- Planned: 9 / In progress: 0 / Implemented: 0 / Verified: 0
- Tracks: Planned 3 / Merged 0

## Relationship to other work

- **Subsumes / supersedes** most of the conformance release's FT-2 model-layer track (capability descriptor,
  agentic-Chat coverage, error-kind consumption) — those become driver-contract concerns. S04 subsumes
  S08-capability-descriptor; the content-tag fix (S27) carries into S03.
- **Parallel, not included here** (separate FT-1 orchestration release): serialized cold-start bootstrap,
  auto-WIP-commit, track-local-failure-not-cascade, the interpreter→responder + three-tier Captain
  escalation with session continuity, and the acceptance spine (Planner Slice-0 harness, SIT gate, pre-merge
  UAT). These pair with this release but are sequenced separately.

## Open planning items (Definition-of-Ready before in_progress)

- Per-slice `covers_needs` are placeholder `N-DRV`; bind to real intake needs + an RTM before DoR.
- Capture coach-loop reference traces for the S08 fixture before T3 begins.
- Type-1 design decision to record (Rule 9): subprocess-default vs in-process-default driver, and the
  `Driver` interface shape — human-owned decision in status.json before T1 starts.
