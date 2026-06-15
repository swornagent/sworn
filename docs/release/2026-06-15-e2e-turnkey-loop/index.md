---
title: Release board — 2026-06-15-e2e-turnkey-loop
description: sworn v0.1 — native-Go end-to-end loop. Machine-readable track registry + human-readable board.
release_index: 1
release_worktree_path: /home/brad/projects/swornagent-worktrees/release-2026-06-15-e2e-turnkey-loop
release_worktree_branch: release-wt/2026-06-15-e2e-turnkey-loop
tracks:
  - id: T1-engine
    slices: [S01-verifier-core, S02-oai-model-client, S03-agentic-tool-loop, S04-embed-baton-prompts]
    depends_on: null
    worktree_path:
    worktree_branch: track/2026-06-15-e2e-turnkey-loop/T1-engine
    state: planned
    e2e_specs: []
  - id: T2-orchestration
    slices: [S05-state-and-git, S06-implementer, S07-run-loop]
    depends_on: T1-engine
    worktree_path:
    worktree_branch: track/2026-06-15-e2e-turnkey-loop/T2-orchestration
    state: planned
    e2e_specs: []
  - id: T3-turnkey-ux
    slices: [S08-init-config, S09-distribution]
    depends_on: T1-engine
    worktree_path:
    worktree_branch: track/2026-06-15-e2e-turnkey-loop/T3-turnkey-ux
    state: planned
    e2e_specs: []
  - id: T4-proof
    slices: [S10-benchmark-dogfood]
    depends_on: T2-orchestration
    worktree_path:
    worktree_branch: track/2026-06-15-e2e-turnkey-loop/T4-proof
    state: planned
    e2e_specs: []
---

# Release Board: `2026-06-15-e2e-turnkey-loop`

## Release summary

- **Goal**: `sworn` v0.1 — one native-Go binary that runs implement→verify→
  (retry/escalate)→gated-merge end-to-end, turnkey self-serve, zero deps.
- **Target version / integration branch**: `main` (this repo).
- **Started**: 2026-06-15
- **Target ship**: uncommitted (multi-month native build)
- **Intake**: `intake.md`
- **Stakeholder**: repo owner

## Tracks

| Track | Slices (in order) | Depends on | Branch |
|---|---|---|---|
| `T1-engine` | S01 → S02 → S03 → S04 | — | `track/2026-06-15-e2e-turnkey-loop/T1-engine` |
| `T2-orchestration` | S05 → S06 → S07 | `T1-engine` | `track/2026-06-15-e2e-turnkey-loop/T2-orchestration` |
| `T3-turnkey-ux` | S08 → S09 | `T1-engine` | `track/2026-06-15-e2e-turnkey-loop/T3-turnkey-ux` |
| `T4-proof` | S10 | `T2-orchestration` | `track/2026-06-15-e2e-turnkey-loop/T4-proof` |

### Touchpoint matrix

> No row may carry a `✓` in more than one track column. `cmd/sworn/main.go` is a
> **documented shared file** (each subcommand lives in its own file; only the
> dispatch switch is the shared edit — additive, region-separable).

| File / surface | T1 | T2 | T3 | T4 |
|---|---|---|---|---|
| `internal/model/`, `internal/agent/`, `internal/prompt/` | ✓ | | | |
| `internal/verify/`, `internal/verdict/` | ✓ | | | |
| `cmd/sworn/verify.go` | ✓ | | | |
| `internal/state/`, `internal/git/` | | ✓ | | |
| `internal/implement/`, `internal/run/` | | ✓ | | |
| `cmd/sworn/run.go` | | ✓ | | |
| `internal/config/`, `cmd/sworn/init.go` | | | ✓ | |
| `.goreleaser.yaml`, `.github/workflows/`, `Dockerfile`, `packaging/` | | | ✓ | |
| `internal/bench/`, `cmd/sworn/bench.go`, `docs/benchmark/` | | | | ✓ |
| `cmd/sworn/main.go` (dispatch — documented shared) | ✓ | ✓ | ✓ | ✓ |

## Slices

| ID | Track | User outcome | Spec |
|---|---|---|---|
| `S01-verifier-core` | T1 | A dev runs `sworn verify` and gets a fail-closed JSON verdict (DONE) | [spec](./S01-verifier-core/spec.md) |
| `S02-oai-model-client` | T1 | With a key, `sworn verify` produces a real verdict from a chosen model | [spec](./S02-oai-model-client/spec.md) |
| `S03-agentic-tool-loop` | T1 | The engine can read/write/edit files + run commands via a model | [spec](./S03-agentic-tool-loop/spec.md) |
| `S04-embed-baton-prompts` | T1 | Planner/implementer/verifier prompts are embedded in the binary | [spec](./S04-embed-baton-prompts/spec.md) |
| `S05-state-and-git` | T2 | Slice state + git branch/commit ops are driven natively | [spec](./S05-state-and-git/spec.md) |
| `S06-implementer` | T2 | The engine implements a spec and writes a proof bundle | [spec](./S06-implementer/spec.md) |
| `S07-run-loop` | T2 | `sworn run` does implement→verify→retry→gated-merge end-to-end | [spec](./S07-run-loop/spec.md) |
| `S08-init-config` | T3 | `sworn init` gives a turnkey zero-config start (BYO-key) | [spec](./S08-init-config/spec.md) |
| `S09-distribution` | T3 | The binary installs via Homebrew / go install / container | [spec](./S09-distribution/spec.md) |
| `S10-benchmark-dogfood` | T4 | A model×jurisdiction×cost×pass-rate benchmark + a real E2E run | [spec](./S10-benchmark-dogfood/spec.md) |

### State legend

`planned` → `in_progress` → `implemented` → `verified` → (`/merge-track` →
`/merge-release`) → `shipped`. `failed_verification` returns to the implementer.
Live state lives in each slice's `status.json` (not mirrored here).

## Cross-slice / cross-track notes

- `cmd/sworn/main.go` dispatch is the only shared file; keep each subcommand in
  its own file (`verify.go`, `run.go`, `init.go`, `bench.go`) so the only shared
  edit is the additive dispatch entry.
- T2 and T4 depend on T1 (the model client + tool loop + verify core). T3 depends
  on T1 for model config but is otherwise parallel to T2.
- No web E2E (Playwright) — this is a CLI; the "E2E" proof is the S10 dogfood run.
- Safe-hosted default model is chosen by the S10 benchmark; until then, config
  requires an explicit model.

## Decisions deferred (Rule 2)

- TUI (`sworn top`), full planner decomposition, enterprise tier, other git
  providers, provenance gate, telemetry — see `intake.md` "Adjacent / out of scope".
