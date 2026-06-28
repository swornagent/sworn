---
title: 'Release board — 2026-06-27-conformance-foundation'
description: 'Closes Sworn''s structural Baton-conformance gaps: records-as-JSON, orchestration completeness, agentic verifier, model-layer service, role ontology, contract/re-vendor, telemetry/eval from day 1.'
release_worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation
release_worktree_branch: release-wt/2026-06-27-conformance-foundation
tracks:
  - id: T1-orchestration
    slices: [S01-llm-interpreter, S02-orchestrator-decision-log, S03-crash-recovery, S04-scheduler-dependent-track, S05-merge-gate-oracle, S06-invariant2-enforcement, S07-pause-resume-committed, S27-parallel-dispatch-fix]
    depends_on: [T2-model-layer, T3-agentic-verifier]
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T1-orchestration
    worktree_branch: track/2026-06-27-conformance-foundation/T1-orchestration
    state: in_progress
  - id: T2-model-layer
    slices: [S08-capability-descriptor, S09-error-kind-consumption, S10-agentic-chat-anthropic]
    depends_on: null
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T2-model-layer
    worktree_branch: track/2026-06-27-conformance-foundation/T2-model-layer
    state: merged
  - id: T3-agentic-verifier
    slices: [S11-agentic-verifier-dispatch, S12-first-pass-demote]
    depends_on: T2-model-layer
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T3-agentic-verifier
    worktree_branch: track/2026-06-27-conformance-foundation/T3-agentic-verifier
    state: merged
  - id: T4-records-as-json
    slices: [S13-schema-embed-validate, S14-board-json, S15-spec-proof-records, S16-journeys-attestations-align, S17-journeys-declare]
    depends_on: null
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T4-records-as-json
    worktree_branch: track/2026-06-27-conformance-foundation/T4-records-as-json
    state: merged
  - id: T5-role-ontology
    slices: [S18-orchestrator-formalized, S19-captain-split, S20-role-revendor, S21-sworn-run-task]
    depends_on: T6-contract-revendor
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T5-role-ontology
    worktree_branch: track/2026-06-27-conformance-foundation/T5-role-ontology
    state: merged
  - id: T6-contract-revendor
    slices: [S22-pin-bump, S23-version-centralise-doctor]
    depends_on: null
    worktree_path: /home/brad/sworn-eval-coach-deepseek-worktrees/release-2026-06-27-conformance-foundation-T6-contract-revendor
    worktree_branch: track/2026-06-27-conformance-foundation/T6-contract-revendor
    state: merged
  - id: T7-telemetry-eval
    slices: [S24-dispatch-enrich, S25-event-store-durable, S26-eval-projections]
    depends_on: [T2-model-layer, T3-agentic-verifier]
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

> T5-role-ontology depends_on T6-contract-revendor (prompt re-vendor needs the pin bump to land first, as both touch `internal/prompt/`). T3-agentic-verifier and T7-telemetry-eval depend_on T2-model-layer (all three touch `internal/model/oai.go` + drivers). The genuinely-parallel set is T1/T2/T4/T6; T3/T7 follow T2, T5 follows T6.

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|
| `T1-orchestration` | S01 → S02 → S03 → S04 → S05 → S06 → S07 → S27 | — | `track/.../T1-orchestration` | planned |
| `T2-model-layer` | S08 → S09 → S10 | — | `track/.../T2-model-layer` | merged |
| `T3-agentic-verifier` | S11 → S12 | T2-model-layer | `track/.../T3-agentic-verifier` | planned |
| `T4-records-as-json` | S13 → S14 → S15 → S16 → S17 | — | `track/.../T4-records-as-json` | merged |
| `T5-role-ontology` | S18 → S19 → S20 → S21 | T6-contract-revendor | `track/.../T5-role-ontology` | merged || `T6-contract-revendor` | S22 → S23 | — | `track/.../T6-contract-revendor` | merged |
| `T7-telemetry-eval` | S24 → S25 → S26 | T2-model-layer | `track/.../T7-telemetry-eval` | planned |

### Touchpoint matrix (DRAFT — finalised once specs are written)

> Documented shared files: `internal/run/slice.go` (T1/T2/T3), `internal/state/state.go` (T4/T7), `internal/model/oai.go` + drivers (T2/T3/T7), and the **agentic verify surface** `internal/verify/verify.go` + `internal/verify/verify_test.go` + `internal/model/openai_responses.go` (T3 agentic + T7 S24 telemetry). After the 2026-06-28 replan, **T1 and T7 depend_on [T2-model-layer, T3-agentic-verifier]** (they extend the merged model + agentic-verify base) and **T5 depends_on T6** — so the genuinely-parallel set is T2/T4/T6; T3 follows T2; T1/T7 follow T2+T3; T5 follows T6.

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
| `internal/model/oai.go` + drivers (DOCUMENTED SHARED) | | ✓ Capabilities/Chat | ✓ verifier model-calls | | | | ✓ S24 dispatch tokens/cost |
| `internal/run/slice.go` (DOCUMENTED SHARED) | ✓ S01 verdict path | ✓ error-halt §321 | ✓ verifier §412 | | | | |
| `internal/verify/verify.go` (DOCUMENTED SHARED) | | | ✓ agentic Run/RunAgentic/RunFirstPass | | | | ✓ S24 telemetry (tokens/duration in RunAgentic) |
| `internal/model/openai_responses.go` (DOCUMENTED SHARED) | | ✓ | ✓ verifier Verify | | | | ✓ S24 tokens |
| `internal/verify/verify_test.go` (DOCUMENTED SHARED) | | | ✓ | | | | ✓ S24 fake-verifier sig |
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
| `S10-agentic-chat-anthropic` | T2 | The native Anthropic driver supports agentic Chat; a keyless run via claude-cli is a valid implementer path; cost is populated from real token counts (not always 0) | verified | [spec](./S10-agentic-chat-anthropic/spec.md) | [proof](./S10-agentic-chat-anthropic/proof.md) || `S11-agentic-verifier-dispatch` | T3 | The engine dispatches the agentic verifier.md role (test-re-running, live-repo) for the verify step; verifier_was_fresh_context is set honestly; Verification.Model records the actual model used | planned | [spec](./S11-agentic-verifier-dispatch/spec.md) | — |
| `S12-first-pass-demote` | T3 | The stateless LLM judge is demoted to a labelled deterministic first-pass (structure/mock/dark-code checks only); it no longer drives the slice to `verified`; verifier.md is re-vendored from canonical | planned | [spec](./S12-first-pass-demote/spec.md) | — |
| `S13-schema-embed-validate` | T4 | All baton schemas (*-v1.json) are embedded in the binary; every record write validates against its schema; missing/invalid records fail closed; example.com $schema placeholder replaced | verified | [spec](./S13-schema-embed-validate/spec.md) | [proof](./S13-schema-embed-validate/proof.md) |
| `S14-board-json` | T4 | board.json is the oracle's source of truth; the oracle renders/drifts index.md from board.json; existing releases auto-migrate board.json from index.md frontmatter on first oracle read | verified | [spec](./S14-board-json/spec.md) | [proof](./S14-board-json/proof.md) |
| `S15-spec-proof-records` | T4 | spec.json (spec-v1) and proof.json (proof-v1) records are emitted and validated; proof sections (delivered, not_delivered, divergence, reachability) are derived from live ACs and state, not constant boilerplate | verified | [spec](./S15-spec-proof-records/spec.md) | [proof](./S15-spec-proof-records/proof.md) || `S16-journeys-attestations-align` | T4 | journeys-v1 and attestations-v1 records align to canonical nested shapes; $schema field populated; validate-on-write enabled; both writers fail closed on invalid data | verified | [spec](./S16-journeys-attestations-align/spec.md) | [proof](./S16-journeys-attestations-align/proof.md) |
| `S17-journeys-declare` | T4 | Three Rule-10 critical journeys (keyless-full-loop, loop-verifier-negative, ship-a-release) are declared in .sworn/journeys.json and human-ratified; entitlement/credits no-mock boundary declared | verified | [spec](./S17-journeys-declare/spec.md) | [proof](./S17-journeys-declare/proof.json) || `S18-orchestrator-formalized` | T5 | The Orchestrator role is formally specified as a Sworn-side artefact in docs/baton/; the deterministic-vs-agentic design choice is recorded as a Type-1 decision in status.json | verified | [spec](./S18-orchestrator-formalized/spec.md) | [proof](./S18-orchestrator-formalized/proof.md) || `S19-captain-split` | T5 | captain.md is split: design-reviewer.md (Baton Rule-9 surface) and orchestrator-notes.md (Sworn engine mapping); each file references the correct owner | verified | [spec](./S19-captain-split/spec.md) | [proof](./S19-captain-split/proof.md) || `S20-role-revendor` | T5 | planner.md, implementer.md, captain.md are re-vendored from canonical post-records-as-JSON; VERSION.txt is bumped to match; run after T6 merges | verified | [spec](./S20-role-revendor/spec.md) | [proof](./S20-role-revendor/proof.md) || `S21-sworn-run-task` | T5 | `sworn run --task "<description>"` dispatches the planner role to draft a concrete-AC spec, then runs implement+verify over that spec; direction C (planner-assist quickstart) | verified | [spec](./S21-sworn-run-task/spec.md) | [proof](./S21-sworn-run-task/proof.md) || `S22-pin-bump` | T6 | The vendor pin references a canonical HEAD containing the baton/ layout (≥ records-as-JSON); source map coherent with the new pin; re-vendor would succeed | verified | [spec](./S22-pin-bump/spec.md) | [proof](./S22-pin-bump/proof.md) || `S23-version-centralise-doctor` | T6 | VERSION is centralised to a single source; doctor detects SHA-vs-HEAD drift and pre-JSON-prompt pin staleness; both checks fail closed | verified | [spec](./S23-version-centralise-doctor/spec.md) | [proof](./S23-version-centralise-doctor/proof.md) || `S24-dispatch-enrich` | T7 | Dispatch record captures duration_ms, input_tokens, output_tokens, real_cost_usd (from model pricing map), and the model-id confirmed in the response | planned | [spec](./S24-dispatch-enrich/spec.md) | — |
| `S25-event-store-durable` | T7 | The supervisor SQLite event store survives process restart; events written during a run are queryable after a new `sworn run` starts against the same release | planned | [spec](./S25-event-store-durable/spec.md) | — |
| `S26-eval-projections` | T7 | `sworn telemetry` reports per-model rework rate, mean tokens-per-turn, mean latency_ms, and estimated cost; output is machine-readable JSON and human-readable table | verified | [spec](./S26-eval-projections/spec.md) | [proof](./S26-eval-projections/proof.json) || `S27-parallel-dispatch-fix` | T1 | `sworn run --parallel` can dispatch an agentic implementer and run a multi-turn tool session (nil agent/verifier factories defaulted; tool-only turns no longer drop the required `content` field). Surfaced by the 2026-06-28 dogfood | implemented | [spec](./S27-parallel-dispatch-fix/spec.md) | [proof](./S27-parallel-dispatch-fix/proof.md) |

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

- Planned: 20
- In progress: 0
- Implemented (awaiting verification): 1 (S27-parallel-dispatch-fix)
- Verified (awaiting merge): 13 (S10-agentic-chat-anthropic, S13-schema-embed-validate, S14-board-json, S15-spec-proof-records, S16-journeys-attestations-align, S17-journeys-declare, S18-orchestrator-formalized, S19-captain-split, S20-role-revendor, S21-sworn-run-task, S22-pin-bump, S23-version-centralise-doctor, S26-eval-projections)
- Failed verification: 0
- Deferred: 0
- Shipped: 0

**Tracks:** Planned: 1 / In progress: 2 / Merged: 4

## Rule-10 journeys to declare (in T4 S17)

| Journey ID | Name | No-mock boundary | Status |
|---|---|---|---|
| J1 | keyless-full-loop | entitlement / credits | declared (verified) |
| J2 | loop-verifier-negative | loop-verifier (real gate, not stateless judge) | declared (verified) |
| J3 | ship-a-release (surface-seam) | Driver 1/2/3 end-to-end, real board + real gates | declared (verified) |

## Decisions deferred (Rule 2)

- **Hosted/SaaS layer**: attestation + credits. Why: private-repo scope. Tracking: sworn-internal. Acknowledged: Brad, 2026-06-27.
- **Codex exec driver**: GH #19. Why: codex environment complexity; not a conformance gap. Acknowledged: Brad, 2026-06-27.
- **sworn init test-capability detection**: proof-visibility theme. Why: FT-4 lays the foundation; extend sworn init in a dedicated release. Tracking: project memory project_proof_visibility_theme. Acknowledged: Brad, 2026-06-27.
- **S01-llm-interpreter verify-side wiring descoped**: the agentic-verifier migration (S11/S12) made the verifier return a typed verdict (BLOCKED on unparseable), superseding S01's cheap-interpreter rescue on the verify path. Why: S01 was built against the stateless judge; with the agentic verifier the two overlap, and S01 is not integrated anywhere yet. The 2026-06-28 S04 forward-merge takes the agentic `lastVerdict = result` base and drops S01's stateless `verify.Run` + interpreter block from `internal/run/slice.go`. Tracking: re-home S01 onto the agentic/implementer path in a future slice if it earns its place (option A). Acknowledged: Brad, 2026-06-28.

## Cross-slice notes

- T5 depends_on T6: S20-role-revendor requires the pin bump from T6 before copying new canonical prompt files.
- T4 S17 (journeys-declare) requires T4 S16 (journeys shape aligned) to be implemented first — sequential within T4.
- `internal/run/slice.go` is a documented shared file across **T1** (S01 interpreter on the verdict path), T2 (error-halt, lines ~321-327) and T3 (verifier dispatch, lines ~412-429). The original matrix under-declared T1, causing S04's forward-merge BLOCK. The T1 vs T3 sides diverged on the verdict source (T1's stateless `verify.Run` + S01 interpreter vs T3's agentic `lastVerdict = result`); resolved 2026-06-28 in favour of the agentic base (see the S01 Rule-2 deferral below).
- `internal/state/state.go` is a documented shared file between T4 (Write() validation, line ~184) and T7 (Dispatch struct, line ~80). Regions are well-separated and non-overlapping.
- **`internal/model/oai.go` (and the model drivers anthropic/azure/bedrock/cli/google/oci/ollama) is DOCUMENTED SHARED across T2-model-layer (Capability/Chat methods), T3-agentic-verifier (verifier model-call paths), and T7-telemetry-eval (S24 dispatch token/cost enrichment).** T3 and T7 therefore **depend_on T2-model-layer** for their model-touching slices: they must carry T2's merged base before those slices, and merge-track resolves the combine. (Added 2026-06-28 replan: the original matrix under-declared this single most-shared surface, causing recurring merge conflicts on `oai.go` — T7/S25 and T3/S12 both BLOCKED on it. Declaring it shared + sequencing T3/T7 after T2 is the durable fix.)
- **The agentic verify surface — `internal/verify/verify.go`, `internal/verify/verify_test.go`, `internal/model/openai_responses.go` — is DOCUMENTED SHARED across T3-agentic-verifier (the agentic migration: `RunAgentic`/`RunFirstPass`, `prompt.Verifier()`) and T7-telemetry-eval (S24 dispatch-enrich: input/output token + duration capture).** T7 therefore **depend_on T3-agentic-verifier**, and S24's telemetry must be captured in the **agentic `RunAgentic`** path (cost/usage come from `resp.Usage`), NOT via a stateless `Verify`-signature change. (Added 2026-06-28 replan: the original matrix declared `verify.go` T3-only; T7/S24 built telemetry on the pre-agentic stateless `verify.Run`, so T7/S26's forward-merge collided with the merged agentic rewrite — an overlapping (not additive) conflict requiring a one-time design reconciliation, not a clean combine. The durable fix is declaring the surface shared, sequencing T7 after T3, and re-homing S24 telemetry into `RunAgentic`.)

## Recent activity

### 2026-06-28 — slice `S26-eval-projections` verified (PASS)

- **Actor**: verifier (/verify-slice, fresh context)
- **Verdict**: PASS. All 7 gates passed: Gate 1 (user-reachable via `sworn telemetry report`), Gate 2 (touchpoints match), Gate 3 (17 tests pass), Gate 4 (smoke `sworn telemetry report` exits 0), Gate 5 (no silent deferrals), Gate 6 (non-UI exempt), Gate 7 (all 7 delivered items verified).
- **Next**: `/merge-track T7-telemetry-eval 2026-06-27-conformance-foundation` (all 3 slices in T7 are now verified)


### 2026-06-28 — slice `S17-journeys-declare` verified (PASS)

- **Actor**: verifier (/verify-slice, fresh context)
- **Verdict**: PASS. All 7 gates passed: Gate 1 (user-reachable outcome via `.sworn/journeys.json` + `sworn journeys --check`), Gate 2 (planned touchpoints with divergence explained), Gate 3 (tests pass), Gate 4 (reachability via `sworn journeys --check`), Gate 5 (no silent deferrals), Gate 6 (non-UI exempt), Gate 7 (all 9 delivered items verified).
- **Next**: `/merge-track T4-records-as-json 2026-06-27-conformance-foundation` (all 5 slices in T4 are now verified)

### 2026-07-28 — slice `S16-journeys-attestations-align` verified (PASS)
- **Actor**: verifier (/verify-slice)
- **Note**: All 7 gates passed. Nested ratification/boundary shapes, $schema fields, validate-on-write confirmed.

### 2026-06-28 — track `T3-agentic-verifier` merged to release-wt (commit 29a1c8a)

- **Actor**: track integrator (/merge-track)
- **Note**: 2 verified slices merged: S11-agentic-verifier-dispatch, S12-first-pass-demote. Track state → merged.
### 2026-06-28 — track `T5-role-ontology` merged to release-wt (commit 605a76c)

- **Actor**: track integrator (/merge-track)
- **Note**: 4 verified slices merged: S18-orchestrator-formalized, S19-captain-split, S20-role-revendor, S21-sworn-run-task. Track state → merged.

### 2026-06-28 — slice `S21-sworn-run-task` verified (PASS)

- **Actor**: verifier (/verify-slice)
- **Verdict**: PASS (all 7 gates). All 9 TestTask* tests pass; go vet + gofmt clean; `sworn run --task 'hello' --dry-run` exits 0; `--help` shows `--task` flag; no silent deferrals; all 6 ACs satisfied.
- **Next**: `/merge-track T5-role-ontology`


### 2026-06-28 — slice `S21-sworn-run-task` FAILED verification
- **Actor**: verifier (/verify-slice)
- **Verdict**: FAIL. Gate 3 — required test (mock planner dispatch, verify spec.md written, no-ACs error path) is absent; delivered tests only exercise leaf helpers and `TestTaskDryRunFlagAccepted` is an empty placeholder (Gate 5). Gate 2 — `internal/git/git.go` changed but undeclared and not surfaced in proof "Divergence". All four changed files fail `gofmt -l` (AGENTS.md). Gate 1 + dry-run reachability hold.
- **Next**: `/implement-slice S21-sworn-run-task 2026-06-27-conformance-foundation` (fresh session)

### 2026-07-28 — slice `S20-role-revendor` verified (PASS)

- **Actor**: verifier (/verify-slice)
- **Verdict**: PASS (all 7 gates). `planner.md`, `implementer.md`, `captain.md` re-vendored from canonical post-records-as-JSON; `implementer.md` has 1-line public-safety scrub; `VERSION.txt` = 42eb48b (matches S22 pin). All prompt tests + go build pass.
- **Next**: `/implement-slice S21-sworn-run-task 2026-06-27-conformance-foundation`### 2026-07-28 — track `T2-model-layer` merged to release-wt (commit 71ec0803)

- **Actor**: track integrator (/merge-track)
- **Note**: 3 verified slices merged: S08-capability-descriptor, S09-error-kind-consumption, S10-agentic-chat-anthropic. Track state → merged.

### 2026-07-24 — slice `S10-agentic-chat-anthropic` verified (PASS)

- **Actor**: verifier (/verify-slice)
- **Note**: All 7 gates passed. Track T2-model-layer is now complete (3/3 slices verified). Next step: /merge-track T2-model-layer.

### 2026-06-28 — track `T6-contract-revendor` merged to release-wt (commit 0b039d0)

- **Actor**: track integrator (/merge-track)
- **Note**: 2 verified slices merged: S22-pin-bump, S23-version-centralise-doctor. Track state → merged.

### 2026-06-27 — S18-orchestrator-formalized verified (commit b6b40e7)

- **Actor**: verifier (/verify-slice)
- **Verdict**: PASS (all 7 gates). Documentation-only slice — `docs/baton/roles/orchestrator.md` and `docs/baton/decisions/orchestrator-model.md` created.
- **Next**: `/implement-slice S19-captain-split 2026-06-27-conformance-foundation`

### 2026-06-28 — S19-captain-split verified (commit 589a233)

- **Actor**: verifier (/verify-slice)
- **Verdict**: PASS (all 7 gates). `design-reviewer.md` (293 lines, self-contained design-review role prompt), `orchestrator-notes.md` (62 lines, states orchestrator is realised by Sworn engine), and `captain.md` (updated with split-notice header). All 24 prompt tests pass.
- **Next**: `/implement-slice S20-role-revendor 2026-06-27-conformance-foundation`


### 2026-07-28 — S12-first-pass-demote verified (PASS)

- **Actor**: verifier (/verify-slice, fresh context)
- **Note**: All 5 acceptance checks satisfied; verifier.md is byte-for-byte canonical; RunFirstPass correctly short-circuits agentic dispatch on FAIL/BLOCKED and never writes state.Verified. T3-agentic-verifier track now complete (S11+S12 both verified). Next: /merge-track T3-agentic-verifier.

