---
title: 'Release intake — 2026-06-27-conformance-foundation'
description: 'Closes Sworn''s structural Baton-conformance gaps: records-as-JSON, orchestration engine completeness, agentic verifier, model-layer service layer, role ontology, contract/re-vendor, and telemetry/eval from day 1.'
---

# Release Intake: `2026-06-27-conformance-foundation`

## Release goal

SwornAgent carries a genuine, load-bearing deterministic core — oracle reader, slice router,
topological scheduler, fail-closed state machine, Rule 9 design gates — but has structural
conformance gaps concentrated in the CRITICAL rules. This release closes those gaps across
seven dimensions: records-as-JSON (board/spec/proof as machine-readable, schema-validated
records), orchestration completeness (LLM interpreter, decision-log, crash recovery, invariant-2,
merge-gate-oracle), the agentic loop verifier (dispatching the real test-re-running verifier.md
role), model-layer service (capability descriptor, Error{Kind} consumption, Anthropic Chat),
role ontology (Orchestrator formalised, Captain split, prompts re-vendored, sworn run --task),
contract/re-vendor (pin currency, VERSION centralisation, doctor drift), and telemetry/eval
foundations. Three Rule-10 critical journeys (keyless-full-loop, loop-verifier-negative,
ship-a-release) are declared and human-ratified. Shipped = every conformance gap in the
2026-06-27 audit resolved or formally deferred, and the three journeys walk no-mock.

## Source of truth

- **Human stakeholder**: Brad Sawyer
- **Tracking issue / epic**: GH #22 (records-as-JSON + write-time validation anchor; breadth of this release extends beyond #22's scope to cover all 7 conformance tracks — recommend creating a wider epic in a follow-up)
- **Related captures**:
  - `docs/captures/2026-06-27-baton-conformance-audit.md` — full per-dimension audit + adversarial verdicts; Section 4 = foundation-track scope
  - `docs/captures/2026-06-27-surface-seam.md` — Driver 1/2/3 three-surface model + sworn run --task direction C ratification
- **Related memory entries**:
  - `project_loop_verifier_fidelity.md` — loop verifier goes agentic; stateless judge demoted to deterministic first-pass
  - `project_model_layer_service_refactor.md` — wire-vs-usage service layer; capability matrix sparse; FT-2 foundation-track item
  - `project_telemetry_eval_foundation.md` — capture model-eval telemetry from day 1; Dispatch struct + event store; FT-7 track
  - `project_orchestrator_role.md` — Orchestrator as distinct role (Sworn Layer-2); split from Captain; Baton owns contract; FT-5 track
  - `project_task_quickstart_direction.md` — sworn run --task = real single-slice planner-assist (direction C, RATIFIED)
  - `project_baton_v040_publish.md` — vendor pin v0.5.0 target; pin SHA 9ae08fb; FT-6 track

## Users and their gestures

- **Coach (Brad)**: plans a release via `/plan-release`; kicks off `sworn run --release`; observes via TUI or MCP; is paged on escalation; resolves and resumes
- **Coach keyless**: runs the full loop (plan→implement→verify→merge) without a provider API key, using the credits/subscription boundary
- **Unattended engine (sworn run)**: dispatches roles, interprets outcomes, drives state machine, reads oracle, writes records, wires merge gate
- **External agent / UI (Driver 2)**: drives the same core via MCP run-control + tools
- **Future auditor / eval**: queries per-model telemetry for rework rate, token cost, latency — feeds model selection decisions

## What's currently broken or missing

From the 2026-06-27-baton-conformance-audit.md (Section 3, critical-first):

1. Fail-closed proof-bundle gate is absent — `sworn verify` PASSes with no proof bundle (`internal/verify/verify.go:34,51-54,106-116`)
2. Schema validation on record write is absent — `$schema` is `example.com` placeholder; no validator (`internal/state/state.go:184-192`)
3. board-v1 unbuilt — oracle parses index.md frontmatter (the ADR-0009 corruption surface) (`internal/board/index.go`, `oracle.go:370-394`)
4. RTM horizontal chain inert on real intake — needs:0 on any real planner output (`internal/gate/trace.go:325-385`)
5. Journey gate not wired into merge loop — exists only as a standalone command; no journeys.json in repo (`internal/run/parallel.go`, `cmd/sworn/journeys.go:73-104`)
6. No-mock detector blind to entitlement; never invoked by loop (`internal/gate/mock.go:118-178`)
7. In-loop verifier not Rule-7-grade — single-shot tool-less judge; no test re-run (`internal/run/slice.go:412-429`)
8. Agentic verifier.md never dispatched by engine; vendored verifier.md is v0.4.2 stale (`internal/agent/agent.go:6-7`)
9. proof.json / spec.json records missing (`internal/implement/implement.go:40`)
10. LLM interpreter absent — non-typed outcomes stall/pause (`internal/scheduler/worker.go:249-261`)
11. Orchestrator decision-log missing — routing/triage reasoning is stderr-ephemeral (`internal/orchestrator/triage.go`)
12. Error{Kind} unconsumed by loop — KindAuth retried + escalated like transient (`internal/orchestrator/triage.go:39-57`)
13. Agentic Chat only on 2 driver types; checked at runtime not resolution (`internal/run/run.go:343-352`)
14. Orchestrator role unformalized — unnamed, no recorded design choice
15. Vendor pin predates records-as-JSON + baton/ layout (`internal/adopt/baton/VERSION`)
16. VERSION strings inconsistent — three coexist (`v0.4.2`/`v0.5.0`/`v1.0.0`)
17. Dispatch struct missing: duration, token split, real cost; model-id bug (`internal/run/run.go` + state.Dispatch)
18. Event store in-memory only; no cross-run durability

## What the human wants

Each need is numbered for RTM tracing. Needs map to acceptance criteria in each slice spec.

- N-01: Fail-closed proof-bundle gate — `sworn verify` BLOCKs/FAILs when proof is missing, unreadable, empty, or invalid before model dispatch
- N-02: All record writes validate against embedded JSON schemas, fail closed on invalid data; replace example.com $schema
- N-03: board.json replaces index.md YAML frontmatter as oracle source of truth; oracle still renders/drifts against index.md
- N-04: RTM parser reads real planner-generated intake (needs > 0 on this release's own intake); dropped-need detection fires on a synthetic gap test
- N-05: Journey gate runs automatically as part of merge-release (not opt-in CLI only); gates on journeys.json presence and ratification
- N-06: No-mock detector recognises entitlement/credits keywords; `RunMock` invoked during per-slice verify by the loop
- N-07: Agentic verifier dispatched from engine path; stateless judge demoted to deterministic first-pass only; `verifier_was_fresh_context` set honestly; Verification.Model records verifier model-id correctly
- N-08: spec.json (spec-v1) and proof.json (proof-v1) records emitted and validated; proof sections derived from live state + ACs (not constant boilerplate)
- N-09: Three Rule-10 critical journeys declared in `.sworn/journeys.json`, human-ratified; no-mock boundary covers entitlement; journeys walk no-mock at ship
- N-10: LLM interpreter handles non-typed outcomes via a bounded cheap-model decision step; does not stall/pause for routine routing
- N-11: Orchestrator decision-log persists router Decision + triage Output to durable storage (supervisor SQLite events table)
- N-12: Crash recovery: error_max_turns→PAGE event + cross-run failure-fingerprint circuit breaker
- N-13: Scheduler dependent track branches from dependency tip (finishTrack auto-merges to release-wt before dependent T starts)
- N-14: Merge gate routes verified-check through board.Oracle (not working-tree status.json); invariant-4 conflict classifier present
- N-15: Track-mode invariant-2 (touchpoint disjointness) enforced by loop at slice dispatch time
- N-16: Pause/resume: findFirstNonTerminal reads committed (git-visible) status.json, not working-tree copy
- N-17: All drivers expose Capabilities() descriptor; implementer-model resolution fails fast with a meaningful error at startup if Chat is unavailable
- N-18: Error{Kind} consumed by loop triage: KindAuth/KindCredits/terminal kinds → Halt (not retry); escalation still fires
- N-19: Agentic Chat available for native Anthropic driver and keyless claude-cli path; cost field populated from real token counts (not always 0)
- N-20: Orchestrator role formally specified as Sworn-side artefact; deterministic-vs-agentic design choice recorded as a Type-1 decision in status.json
- N-21: Captain artefact split: design-reviewer.md (Baton Rule-9) vs orchestrator-notes.md (Sworn engine); captain.md description updated
- N-22: planner/implementer/verifier/captain prompts re-vendored from canonical post-records-as-JSON commit; VERSION.txt bumped
- N-23: `sworn run --task "<description>"` dispatches planner to draft a concrete-AC spec, then runs implement+verify over it (direction C, RATIFIED)
- N-24: Vendor pin references a canonical HEAD containing the baton/ layout (≥ records-as-JSON)
- N-25: VERSION centralised to a single source; doctor detects SHA-vs-HEAD drift and pre-JSON-prompt pin staleness; both fail closed
- N-26: Dispatch record captures: duration_ms, input_tokens, output_tokens, real_cost_usd (model pricing map applied), model_id from actual dispatch response
- N-27: Event store writes to supervisor SQLite across runs; persists on restart; queryable by `sworn telemetry`
- N-28: `sworn telemetry` reports per-model: rework rate, mean tokens-per-turn, mean latency, estimated cost; foundation for online eval

## Constraints and non-negotiables

- **Public-safe**: no business/pricing/competitive/strategy content; no references to private repos
- **Minimal justified deps**: stdlib preferred; any new dep requires ADR (ADR-0007); no provider SDKs
- **Fail closed**: exit 0 only on PASS; new gates must fail closed on absence-of-evidence
- **Go binary**: this is a single Go binary; no external build toolchain changes without ADR
- **Baton protocol**: Sworn implements the protocol but does not own it; changes to Baton files go upstream; Sworn-side additions are in `internal/` or new role artefacts clearly labelled Sworn-side
- **No re-vendor from a stale pin**: FT-6 must ship (or at least S22 pin-bump) before FT-5 S20 re-vendor goes to verified

## Adjacent / out of scope

- **Hosted/SaaS layer**: attestation + credits + moat data — out of scope; engineering this is a private release. **Why deferred**: this public-safe repo contains only the open binary. **Tracking**: sworn-internal. **Acknowledged**: Brad, 2026-06-27.
- **Codex exec driver**: S63-deferral-1 carried from R3. **Why deferred**: codex environment complexity; issue #19. **Tracking**: GH #19. **Acknowledged**: Brad, 2026-06-27.
- **Previous_response_id / streaming for OpenAI Responses API**: R3 deferrals. **Why deferred**: GH #16/#17. **Tracking**: GH #16, #17. **Acknowledged**: Brad, 2026-06-27.
- **sworn init test-capability detection** (proof-visibility theme): worth doing but scoped to a later release. **Why deferred**: FT-4 lays the foundation; extending sworn init rides a dedicated release. **Tracking**: memory entry project_proof_visibility_theme. **Acknowledged**: Brad, 2026-06-27.
- **Prompt caching** (GH #8): separate performance concern. **Why deferred**: not a conformance gap. **Tracking**: GH #8. **Acknowledged**: Brad, 2026-06-27.

## Decisions made during planning

### 2026-06-27 — Issue anchor and target integration branch

- **Context**: Rule 5 requires a GitHub issue anchor before first /implement-slice; target integration branch needed for release board
- **Options considered**: new epic vs closest existing issue; v0.6.0 vs release/v0.1.0
- **Decision**: Anchor = GH #22 (records-as-JSON conformance); integration branch = `release/v0.1.0` (sworn is still at v0.1.0)
- **Why**: sworn version numbering is independent of Baton's; Brad confirmed sworn stays on v0.1.0 for this release

### 2026-06-27 — Track structure: use audit Section 4 track grouping as canonical

- **Context**: scope is fully audited and adversarially verified; the 7 tracks and 3 journeys are given
- **Options considered**: single monolithic track vs audit's 7-track decomposition
- **Decision**: adopt audit Section 4 grouping: FT-1 Orchestration, FT-2 Model-layer, FT-3 Agentic verifier, FT-4 Records-as-JSON, FT-5 Role ontology, FT-6 Contract/re-vendor, FT-7 Telemetry/eval
- **Why**: tracks already proven disjoint by the audit analysis; parallelism directly usable

### 2026-06-27 — sworn run --task: direction C (real single-slice planner-assist quickstart)

- **Context**: RATIFIED in project memory (project_task_quickstart_direction.md)
- **Decision**: sworn run --task dispatches the planner role to draft a concrete-AC spec, then runs implement+verify over that spec; replaces the current faked/Rule-8-violating stub
- **Why**: honest demo/on-ramp path; the Rule-8 vague-AC + example.com-schema bugs are fixed as a side effect

### 2026-06-27 — Loop verifier: agentic dispatch; stateless judge demoted to deterministic first-pass

- **Context**: RATIFIED in project memory (project_loop_verifier_fidelity.md)
- **Decision**: FT-3 wires the real test-re-running verifier.md as the engine's verifier; stateless LLM judge becomes a deterministic first-pass (structure/mock/dark-code check) only; REMOVED from the verified-state path
- **Why**: Rule-7 grade requires test re-run + live repo; the stateless judge cannot provide that

### 2026-06-27 — Baton pin target: v0.6.1 / 42eb48b; sworn#23 must merge first

- **Context**: S22 requires the exact canonical Baton commit; Brad confirmed the target
- **Options considered**: n/a — Brad provided the exact version
- **Decision**: Pin to Baton v0.6.1 at SHA 42eb48b; sworn#23 (source map update `claude/baton/` → `baton/`) is a hard pre-requisite; both embed roots (internal/adopt/baton/VERSION and internal/prompt/) bumped to the same SHA
- **Why**: v0.6.1 is the first post-baton/-layout + post-records-as-JSON tag; sworn#23 must land first or the re-vendor resolves old paths against new layout and fails closed

### 2026-06-27 — Three Rule-10 journeys declared in FT-4

- **Context**: journeys need the correct journeys-v1 record shape (FT-4 S16) before they can be declared
- **Decision**: journey declaration (S17) is a slice in FT-4, sequentially after journeys/attestations alignment (S16)
- **Why**: natural dependency; FT-4 owns the records layer; ratification is a content operation on top of the correct record type

## Schema-vs-spec audit notes

- `state.Dispatch` struct (`internal/state/state.go`) — model-id is recorded as the configured id, not the response-confirmed id; `cost` is always 0 (bug in cost calculation) — FT-7 S24 fixes both
- `journeys-v1` record (`internal/journey/journey.go`) — flat ratification struct diverges from canonical nested shape; FT-4 S16 aligns to canonical
- `proof.md` proof bundle — sections `not_delivered` and `divergence` are hardcoded `"None"`; `reachability` and `delivered` are constant self-referential boilerplate — FT-4 S15 derives them from live ACs

## Proposed slice decomposition (draft)

See index.md Tracks section for the authoritative ordered list once approved. Draft below for reference.

**T1-orchestration** (7 slices):
- S01-llm-interpreter — non-typed outcomes handled via bounded cheap-model decision step
- S02-orchestrator-decision-log — routing/triage reasoning persisted to supervisor SQLite
- S03-crash-recovery — error_max_turns→PAGE + cross-run circuit breaker
- S04-scheduler-dependent-track — dependent track branches from dependency tip
- S05-merge-gate-oracle — merge gate reads oracle; invariant-4 classifier; sworn merge CLI
- S06-invariant2-enforcement — touchpoint disjointness enforced at dispatch
- S07-pause-resume-committed — findFirstNonTerminal reads committed status.json

**T2-model-layer** (3 slices):
- S08-capability-descriptor — Capabilities() registry + fail-fast at implementer-model resolution
- S09-error-kind-consumption — KindAuth/terminal kinds → Halt; self-registering factory rename
- S10-agentic-chat-anthropic — native Anthropic Chat driver + keyless claude-cli path; fix cost=0

**T3-agentic-verifier** (2 slices):
- S11-agentic-verifier-dispatch — engine dispatches agentic verifier.md; verifier_was_fresh_context; fix Verification.Model
- S12-first-pass-demote — demote stateless judge to labeled deterministic first-pass; re-vendor verifier.md

**T4-records-as-json** (5 slices):
- S13-schema-embed-validate — embed baton/schemas/*-v1.json; fail-closed validate-on-write; replace example.com $schema
- S14-board-json — build board.json as oracle source; render index.md from board.json + drift guard
- S15-spec-proof-records — spec-v1 spec.json writer; proof-v1 proof.json emitter; derive proof sections from live ACs
- S16-journeys-attestations-align — align journeys-v1/attestations-v1 to canonical nested shapes + $schema + validation
- S17-journeys-declare — declare 3 Rule-10 journeys in .sworn/journeys.json; human-ratified

**T5-role-ontology** (4 slices):
- S18-orchestrator-formalized — Sworn-side Orchestrator spec; deterministic-vs-agentic as Type-1 design decision
- S19-captain-split — split captain.md: design-reviewer.md (Baton) + orchestrator-notes.md (Sworn)
- S20-role-revendor — re-vendor planner/implementer/verifier/captain from canonical post-records-as-JSON; bump VERSION.txt
- S21-sworn-run-task — sworn run --task becomes real single-slice planner-assist quickstart (direction C)

**T6-contract-revendor** (2 slices):
- S22-pin-bump — bump vendor pin to canonical HEAD containing baton/ layout (≥ records-as-JSON)
- S23-version-centralise-doctor — centralise VERSION; doctor: SHA-vs-HEAD + pre-JSON-prompt drift check

**T7-telemetry-eval** (3 slices):
- S24-dispatch-enrich — Dispatch captures duration_ms, token split, real cost, correct model-id
- S25-event-store-durable — supervisor SQLite event store durable across runs
- S26-eval-projections — sworn telemetry: per-model rework rate, tokens-per-turn, latency, cost projections

**Rule-10 journey declarations** (in T4 S17):
- J1: keyless-full-loop (no-mock: real entitlement/credits boundary)
- J2: loop-verifier-negative (no-mock: real gate, not stateless judge)
- J3: ship-a-release / surface-seam (no-mock: Driver 1/2/3 end-to-end)

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | LLM interpreter decision step: inline async (same goroutine, cheap model) vs separate dispatch (new process, same retry infra)? | S01 ACs | Inline async preferred to avoid process spawn overhead; bounded by max_tokens + timeout; Brad confirms at design-review |
| A-02 | sworn run --task planner dispatch: uses existing model.Generate (OAI-compat only) or the new Chat infra from FT-2? | S21 ACs | Uses existing model.Generate for planner dispatch; no dependency on FT-2; can upgrade later |
| A-03 | Board.json migration: new releases write board.json immediately; existing releases? Auto-migrate on first read or require sworn migrate? | S14 ACs | Auto-generate board.json from index.md frontmatter on first oracle read (lazy migration); requires no user action |
| A-04 | Agentic verifier process model: same binary with `-role=verifier` flag, or a new `sworn verify --agentic` subcommand? | S11 ACs | `sworn verify --agentic` subcommand wires the agentic path; backward-compat with existing MCP/slash-command invocations |
| A-05 | Telemetry event store location: extend `internal/supervisor` SQLite, or new `internal/telemetry` DB? | S25 ACs | Extend supervisor SQLite (existing infra); add telemetry table(s); no new DB file |
| A-06 | Which GitHub issue is the anchor for this release? | Session discipline (Rule 5) | RESOLVED: GH #22 (closest existing anchor; breadth extends beyond its scope) |
| A-07 | Target integration branch / version tag? | index.md Release summary | RESOLVED: `release/v0.1.0` (sworn is still at v0.1.0; this release builds on that branch) |

## Screenshots / references

None yet — this is an infrastructure/conformance release with no UI changes.
