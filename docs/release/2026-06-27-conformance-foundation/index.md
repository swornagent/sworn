---
title: 'Release board — 2026-06-27-conformance-foundation'
description: 'Closes Sworn''s structural Baton-conformance gaps: records-as-JSON, orchestration completeness, agentic verifier, model-layer service, role ontology, contract/re-vendor, telemetry/eval from day 1.'
release_worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation
release_worktree_branch: release-wt/2026-06-27-conformance-foundation
tracks:
  - id: T1-orchestration
    slices: [S01-llm-interpreter, S02-orchestrator-decision-log, S03-crash-recovery, S04-scheduler-dependent-track, S05-merge-gate-oracle, S06-invariant2-enforcement, S07-pause-resume-committed, S27-parallel-dispatch-fix]
    depends_on: null
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T1-orchestration
    worktree_branch: track/2026-06-27-conformance-foundation/T1-orchestration
    state: in_progress
  - id: T2-model-layer
    slices: [S08-capability-descriptor, S09-error-kind-consumption, S10-agentic-chat-anthropic]
    depends_on: null
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T2-model-layer
    worktree_branch: track/2026-06-27-conformance-foundation/T2-model-layer
    state: in_progress
  - id: T3-agentic-verifier
    slices: [S11-agentic-verifier-dispatch, S12-first-pass-demote]
    depends_on: null
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T3-agentic-verifier
    worktree_branch: track/2026-06-27-conformance-foundation/T3-agentic-verifier
    state: in_progress
  - id: T4-records-as-json
    slices: [S13-schema-embed-validate, S14-board-json, S15-spec-proof-records, S16-journeys-attestations-align, S17-journeys-declare]
    depends_on: null
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T4-records-as-json
    worktree_branch: track/2026-06-27-conformance-foundation/T4-records-as-json
    state: in_progress
  - id: T5-role-ontology
    slices: [S18-orchestrator-formalized, S19-captain-split, S20-role-revendor, S21-sworn-run-task]
    depends_on: T6-contract-revendor
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T5-role-ontology
    worktree_branch: track/2026-06-27-conformance-foundation/T5-role-ontology
    state: in_progress
  - id: T6-contract-revendor
    slices: [S22-pin-bump, S23-version-centralise-doctor]
    depends_on: null
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T6-contract-revendor
    worktree_branch: track/2026-06-27-conformance-foundation/T6-contract-revendor
    state: merged
  - id: T7-telemetry-eval
    slices: [S24-dispatch-enrich, S25-event-store-durable, S26-eval-projections]
    depends_on: null
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T7-telemetry-eval
    worktree_branch: track/2026-06-27-conformance-foundation/T7-telemetry-eval
    state: in_progress
---

# Release Board: `2026-06-27-conformance-foundation`

## Release summary

- **Goal**: Close Sworn's structural Baton-conformance gaps across records-as-JSON, orchestration completeness, agentic verifier, model-layer service, role ontology, contract/re-vendor, and telemetry/eval foundations; declare and walk three Rule-10 critical journeys no-mock.
- **Target version / integration branch**: `release/v0.1.0` (sworn version; confirmed 2026-06-27)
- **Started**: 2026-06-27
- **Target ship**: uncommitted
- **Intake**: [intake.md](./intake.md)
- **Stakeholder**: Brad Sawyer
- **Tracking issue**: GH #22 (records-as-JSON anchor; breadth extends beyond #22 to all 7 tracks)

## Tracks

> T5-role-ontology depends_on T6-contract-revendor (prompt re-vendor needs the pin bump to land first, as both touch `internal/prompt/`). All other tracks are parallel-safe.

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|
| `T1-orchestration` | S01 → S02 → S03 → S04 → S05 → S06 → S07 → S27 | — | `track/.../T1-orchestration` | planned |
| `T2-model-layer` | S08 → S09 → S10 | — | `track/.../T2-model-layer` | planned |
| `T3-agentic-verifier` | S11 → S12 | — | `track/.../T3-agentic-verifier` | planned |
| `T4-records-as-json` | S13 → S14 → S15 → S16 → S17 | — | `track/.../T4-records-as-json` | planned |
| `T5-role-ontology` | S18 → S19 → S20 → S21 | T6-contract-revendor | `track/.../T5-role-ontology` | planned |
| `T6-contract-revendor` | S22 → S23 | — | `track/.../T6-contract-revendor` | merged |
| `T7-telemetry-eval` | S24 → S25 → S26 | — | `track/.../T7-telemetry-eval` | planned |

### Touchpoint matrix (DRAFT — finalised once specs are written)

> Two documented shared files exist. All other rows are single-track. The matrix proves the five parallel tracks (T1/T2/T3/T4/T6/T7) are disjoint; T5 is sequenced after T6.

| File / surface | T1 | T2 | T3 | T4 | T5 | T6 | T7 |
|---|---|---|---|---|---|---|---|
| `internal/orchestrator/interpreter.go` (new) | ✓ | | | | | | |
| `internal/orchestrator/triage.go` | | ✓ | | | | | |
| `internal/scheduler/worker.go` | ✓ | | | | | | |
| `internal/supervisor/decisions.go` (new) | ✓ | | | | | | |
| `internal/run/parallel.go` | ✓ | | | | | | |
| `internal/router/router.go` | ✓ | | | | | | |
| `internal/mcp/tools_ops.go` | ✓ | | | | | | |
| `cmd/sworn/merge.go` (new) | ✓ | | | | | | |
| `cmd/sworn/run.go` | ✓ | | | | | | |
| `internal/model/registry.go` (new) | | ✓ | | | | | |
| `internal/config/config.go` | | ✓ | | | | | |
| `internal/run/run.go` | | ✓ | | | | | |
| `internal/run/slice.go` (DOCUMENTED SHARED) | | ✓ error-halt §321 | ✓ verifier §412 | | | | |
| `internal/verify/verify.go` | | | ✓ | | | | |
| `internal/prompt/verifier.md` | | | ✓ | | | | |
| `internal/state/state.go` (DOCUMENTED SHARED) | | | | ✓ Write() §184 | | | ✓ Dispatch §80 |
| `internal/board/oracle.go` | | | | ✓ | | | |
| `internal/board/board.go` (new) | | | | ✓ | | | |
| `internal/board/index.go` | | | | ✓ | | | |
| `internal/implement/implement.go` | | | | ✓ | | | |
| `internal/journey/journey.go` | | | | ✓ | | | |
| `.sworn/journeys.json` (new) | | | | ✓ | | | |
| `docs/baton/` (new role artefacts) | | | | | ✓ | | |
| `internal/prompt/planner.md` | | | | | ✓ | | |
| `internal/prompt/implementer.md` | | | | | ✓ | | |
| `internal/prompt/captain.md` | | | | | ✓ | | |
| `cmd/sworn/task.go` (new) | | | | | ✓ | | |
| `internal/adopt/baton/VERSION` | | | | | | ✓ | |
| `internal/adopt/baton/source_map.json` | | | | | | ✓ | |
| `cmd/sworn/doctor.go` | | | | | | ✓ | |
| `internal/prompt/VERSION.txt` | | | | | | ✓ | |
| `internal/supervisor/supervisor.go` | | | | | | | ✓ |
| `cmd/sworn/telemetry.go` (new) | | | | | | | ✓ |

## Slices

| ID | Track | User outcome | State | Spec | Proof |
|---|---|---|---|---|---|
| `S01-llm-interpreter` | T1 | Non-typed implementer/verifier outcomes route through a bounded cheap-model decision step; the loop never stalls on routine ambiguity | planned | [spec](./S01-llm-interpreter/spec.md) | — |
| `S02-orchestrator-decision-log` | T1 | Every routing decision and triage output is persisted to the supervisor SQLite; the Coach can query the decision trail after a run | planned | [spec](./S02-orchestrator-decision-log/spec.md) | — |
| `S03-crash-recovery` | T1 | A slice that hits error_max_turns PAGEs the Coach instead of looping; the cross-run circuit breaker halts a fingerprinted repeated failure | planned | [spec](./S03-crash-recovery/spec.md) | — |
| `S04-scheduler-dependent-track` | T1 | A dependent track's worktree branches from the dependency tip after finishTrack auto-merges to release-wt, so it always starts with the dependency's code | planned | [spec](./S04-scheduler-dependent-track/spec.md) | — |
| `S05-merge-gate-oracle` | T1 | `sworn merge-track` and `sworn merge-release` route verified-check through board.Oracle; invariant-4 conflict detected and reported; CLI merge commands available | planned | [spec](./S05-merge-gate-oracle/spec.md) | — |
| `S06-invariant2-enforcement` | T1 | The loop enforces track-mode invariant-2 at dispatch time; an attempted concurrent dispatch of two tracks with overlapping touchpoints is blocked with a named report | planned | [spec](./S06-invariant2-enforcement/spec.md) | — |
| `S07-pause-resume-committed` | T1 | `sworn run --resume` correctly identifies the first non-terminal slice by reading committed status.json (not working-tree); resumes from the right slice after a crash | planned | [spec](./S07-pause-resume-committed/spec.md) | — |
| `S08-capability-descriptor` | T2 | Every model driver exposes Capabilities(); implementer-model resolution fails fast at startup with a descriptive error if the selected driver does not support agentic Chat | planned | [spec](./S08-capability-descriptor/spec.md) | — |
| `S09-error-kind-consumption` | T2 | KindAuth, KindCredits, and other terminal Error{Kind}s halt the loop immediately without retry; the factory sentinel is correctly named | planned | [spec](./S09-error-kind-consumption/spec.md) | — |
| `S10-agentic-chat-anthropic` | T2 | The native Anthropic driver supports agentic Chat; a keyless run via claude-cli is a valid implementer path; cost is populated from real token counts (not always 0) | planned | [spec](./S10-agentic-chat-anthropic/spec.md) | — |
| `S11-agentic-verifier-dispatch` | T3 | The engine dispatches the agentic verifier.md role (test-re-running, live-repo) for the verify step; verifier_was_fresh_context is set honestly; Verification.Model records the actual model used | planned | [spec](./S11-agentic-verifier-dispatch/spec.md) | — |
| `S12-first-pass-demote` | T3 | The stateless LLM judge is demoted to a labelled deterministic first-pass (structure/mock/dark-code checks only); it no longer drives the slice to `verified`; verifier.md is re-vendored from canonical | planned | [spec](./S12-first-pass-demote/spec.md) | — |
| `S13-schema-embed-validate` | T4 | All baton schemas (*-v1.json) are embedded in the binary; every record write validates against its schema; missing/invalid records fail closed; example.com $schema placeholder replaced | planned | [spec](./S13-schema-embed-validate/spec.md) | — |
| `S14-board-json` | T4 | board.json is the oracle's source of truth; the oracle renders/drifts index.md from board.json; existing releases auto-migrate board.json from index.md frontmatter on first oracle read | planned | [spec](./S14-board-json/spec.md) | — |
| `S15-spec-proof-records` | T4 | spec.json (spec-v1) and proof.json (proof-v1) records are emitted and validated; proof sections (delivered, not_delivered, divergence, reachability) are derived from live ACs and state, not constant boilerplate | planned | [spec](./S15-spec-proof-records/spec.md) | — |
| `S16-journeys-attestations-align` | T4 | journeys-v1 and attestations-v1 records align to canonical nested shapes; $schema field populated; validate-on-write enabled; both writers fail closed on invalid data | planned | [spec](./S16-journeys-attestations-align/spec.md) | — |
| `S17-journeys-declare` | T4 | Three Rule-10 critical journeys (keyless-full-loop, loop-verifier-negative, ship-a-release) are declared in .sworn/journeys.json and human-ratified; entitlement/credits no-mock boundary declared | planned | [spec](./S17-journeys-declare/spec.md) | — |
| `S18-orchestrator-formalized` | T5 | The Orchestrator role is formally specified as a Sworn-side artefact in docs/baton/; the deterministic-vs-agentic design choice is recorded as a Type-1 decision in status.json | planned | [spec](./S18-orchestrator-formalized/spec.md) | — |
| `S19-captain-split` | T5 | captain.md is split: design-reviewer.md (Baton Rule-9 surface) and orchestrator-notes.md (Sworn engine mapping); each file references the correct owner | planned | [spec](./S19-captain-split/spec.md) | — |
| `S20-role-revendor` | T5 | planner.md, implementer.md, captain.md are re-vendored from canonical post-records-as-JSON; VERSION.txt is bumped to match; run after T6 merges | planned | [spec](./S20-role-revendor/spec.md) | — |
| `S21-sworn-run-task` | T5 | `sworn run --task "<description>"` dispatches the planner role to draft a concrete-AC spec, then runs implement+verify over that spec; direction C (planner-assist quickstart) | planned | [spec](./S21-sworn-run-task/spec.md) | — |
| `S22-pin-bump` | T6 | The vendor pin references a canonical HEAD containing the baton/ layout (≥ records-as-JSON); source map coherent with the new pin; re-vendor would succeed | verified | [spec](./S22-pin-bump/spec.md) | [proof](./S22-pin-bump/proof.md) || `S23-version-centralise-doctor` | T6 | VERSION is centralised to a single source; doctor detects SHA-vs-HEAD drift and pre-JSON-prompt pin staleness; both checks fail closed | verified | [spec](./S23-version-centralise-doctor/spec.md) | [proof](./S23-version-centralise-doctor/proof.md) || `S24-dispatch-enrich` | T7 | Dispatch record captures duration_ms, input_tokens, output_tokens, real_cost_usd (from model pricing map), and the model-id confirmed in the response | planned | [spec](./S24-dispatch-enrich/spec.md) | — |
| `S25-event-store-durable` | T7 | The supervisor SQLite event store survives process restart; events written during a run are queryable after a new `sworn run` starts against the same release | planned | [spec](./S25-event-store-durable/spec.md) | — |
| `S26-eval-projections` | T7 | `sworn telemetry` reports per-model rework rate, mean tokens-per-turn, mean latency_ms, and estimated cost; output is machine-readable JSON and human-readable table | planned | [spec](./S26-eval-projections/spec.md) | — |
| `S27-parallel-dispatch-fix` | T1 | `sworn run --parallel` can dispatch an agentic implementer and run a multi-turn tool session (nil agent/verifier factories defaulted; tool-only turns no longer drop the required `content` field). Surfaced by the 2026-06-28 dogfood | implemented | [spec](./S27-parallel-dispatch-fix/spec.md) | [proof](./S27-parallel-dispatch-fix/proof.md) |

### State legend

| State | Meaning |
|---|---|
| `planned` | Spec written, awaiting implementation |
| `in_progress` | Implementer session active |
| `implemented` | Implementer claims done; awaiting fresh-context verification |
| `verified` | Fresh-context verifier returned PASS |
| `failed_verification` | Verifier returned FAIL; fix and re-submit |
| `deferred` | Slice carved out per Rule 2 |
| `shipped` | Live in production |

## Aggregate state

- Planned: 24
- In progress: 0
- Implemented (awaiting verification): 1 (S27-parallel-dispatch-fix)
- Verified (awaiting merge): 2 (S22-pin-bump, S23-version-centralise-doctor)
- Failed verification: 0
- Deferred: 0
- Shipped: 0

**Tracks:** Planned: 1 / In progress: 5 / Merged: 1

## Rule-10 journeys to declare (in T4 S17)

| Journey ID | Name | No-mock boundary | Status |
|---|---|---|---|
| J1 | keyless-full-loop | entitlement / credits | not-yet-declared |
| J2 | loop-verifier-negative | loop-verifier (real gate, not stateless judge) | not-yet-declared |
| J3 | ship-a-release (surface-seam) | Driver 1/2/3 end-to-end, real board + real gates | not-yet-declared |

## Decisions deferred (Rule 2)

- **Hosted/SaaS layer**: attestation + credits. Why: private-repo scope. Tracking: sworn-internal. Acknowledged: Brad, 2026-06-27.
- **Codex exec driver**: GH #19. Why: codex environment complexity; not a conformance gap. Acknowledged: Brad, 2026-06-27.
- **sworn init test-capability detection**: proof-visibility theme. Why: FT-4 lays the foundation; extend sworn init in a dedicated release. Tracking: project memory project_proof_visibility_theme. Acknowledged: Brad, 2026-06-27.

## Cross-slice notes

- T5 depends_on T6: S20-role-revendor requires the pin bump from T6 before copying new canonical prompt files.
- T4 S17 (journeys-declare) requires T4 S16 (journeys shape aligned) to be implemented first — sequential within T4.
- `internal/run/slice.go` is a documented shared file between T2 (error-halt, lines ~321-327) and T3 (verifier dispatch, lines ~412-429). Merge-track for the second track must resolve the conflict on this file.
- `internal/state/state.go` is a documented shared file between T4 (Write() validation, line ~184) and T7 (Dispatch struct, line ~80). Regions are well-separated and non-overlapping.

## Recent activity

### 2026-06-28 — track `T6-contract-revendor` merged to release-wt (commit 0b039d0)

- **Actor**: track integrator (/merge-track)
- **Note**: 2 verified slices merged: S22-pin-bump, S23-version-centralise-doctor. Track state → merged.
