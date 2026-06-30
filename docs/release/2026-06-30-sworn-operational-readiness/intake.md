---
title: 'Release intake — 2026-06-30-sworn-operational-readiness'
description: 'Make sworn the operational delivery loop: drive a real coach-produced release end-to-end to merged, unattended, on getfired. D6 is the one live blocker; the rest is autonomy hardening.'
---

# Release Intake: `2026-06-30-sworn-operational-readiness`

## Release goal

Make `sworn` operationally able to take a real, coach-produced multi-track release
and drive it autonomously to merged — surfacing to the human only at design gates and
on failure (paging) — proven by completing a real getfired release **unattended
overnight**. The strategic context (handoff 2026-06-30): coach-loop, the private bash
orchestrator, is broken by the baton v0.6.3 JSON-records shift; Brad's call is **do not
backport it — sworn is the operational loop now**. getfired's major release has been
blocked for ~2 months; sworn must be up to land it. "Shipped" = sworn runs a real
getfired release end-to-end without a human babysitting it, and the human wakes to
merged (or a clean page explaining exactly where it stopped).

Parallelism is explicitly **not** a goal of this release — serial track execution is
acceptable. The bar is *autonomous and correct*, not *fast*.

## Source of truth

- **Human stakeholder**: Brad (sworn author / operator).
- **Tracking issue / epic**: #22 (replace string scanners with marshaller round-trips +
  write-time validation — the keystone parent); #37 (D4 leaf-enum, bundled with D6/1b).
  No standalone GH issue exists for D6 itself — create one or anchor to #22.
- **Related captures**: `docs/captures/2026-06-30-session-handoff.md` (the pivot + the
  path-to-operational tiers); `docs/captures/2026-06-30-fired-dogfood-findings.md` (the
  empirical blocker list — D6 is finding 3); `docs/captures/2026-06-30-keystone-release-planning-brief.md`
  (the keystone-remainder decomposition, now subsumed/de-prioritised);
  `docs/captures/2026-06-30-cold-start-smoke-proof.md` (cold-start proven).
- **Related memory**: [[project_parallel_cold_start_broken]] (the eval that found the
  engine is the bottleneck), [[project_loop_verifier_fidelity]] (agentic verifier closed
  the Rule-7 gap), [[project_telemetry_eval_foundation]] (T16 / S07 overlap),
  [[feedback_harness_fix_public_parity]] (public-safe concern).

## Users and their gestures

This is an **engine/infrastructure release** — the "user" is the operator running the
loop and the autonomous loop itself, not an end-user UI. UI/UX, accessibility, and
end-user copy considerations are **not applicable** (no UI surface changes). The floor
considerations that DO bear: correctness/fail-closed, data round-trip fidelity, the
Rule-10 no-mock boundary, and unattended resilience.

- **Operator (Brad)**: runs `sworn run --parallel --release <r> --docs-prefix <p>
  --implementer-model <m> --verifier-model <m>`, then walks away. Before: the run dies
  instantly on a real status.json (D6). After: the run proceeds through implement →
  verify → merge per slice, unattended, and either completes or pages with a reason.
- **The autonomous loop**: reads real coach-produced `status.json` (object-form
  `open_deferrals`/`violations`), dispatches roles, recovers from transient failures
  without discarding committed work, halts+pages on real failure.

## What's currently broken or missing

Empirical, from the live fired dogfood run (`2026-06-30-fired-dogfood-findings.md`):

- **D6 type drift (THE BLOCKER).** `sworn run` on the live fired repo failed at:
  `json: cannot unmarshal object into Go struct field Status.open_deferrals of type
  string`. fired's `open_deferrals` are schema-conformant objects
  (`{id, description, why, tracking, acknowledged_by}`); `state.Status.OpenDeferrals`
  is `[]string`. `Verification.Violations` (`[]string`) almost certainly drifts the
  same way. Cold-start (finding 1) and monorepo docs-prefix (finding 2) are already
  FIXED and proven — D6 is the only remaining hard stop.
- **Unattended resilience — unverified under real load.** The handoff flags three
  "non-blocking tuning" items that become load-bearing for an *unattended* overnight
  run: retry-reset may discard good committed work (eval finding 5, `slice.go:350`);
  escalation default is openai-hardcoded (finding 6, `run.go:29`) so a deepseek run
  can mis-escalate; turn-cap value may be too low for real slices.
- **End-to-end real delivery — UNPROVEN.** Every full-loop success to date used a fake
  agent (smoke) or blocked at D6. No slice has gone implement→verify→merge on a real
  model + real repo via sworn. The design-review gate halt→resume (Rule 9) has never
  been watched live to completion.

## What the human wants

- D6 fixed so sworn reads real coach status.json (round-trips the object form — no
  flatten-to-string hack, which would degrade fired's real data on write-back).
- sworn running a real getfired release **autonomously, overnight, tonight**.
- Minimise churn and tech debt — nail it properly, no half-measures that need redoing.
- Serial track execution is fine; parallelism is not required for this milestone.

## Constraints and non-negotiables

- **No flatten-on-read hack for D6.** sworn rewrites status.json every transition;
  flattening objects→strings silently degrades real coach data. Must round-trip the
  object form (the proper struct migration). Non-negotiable.
- **Rule-10 no-mock boundary intact.** D6 touches `CheckBoundaryMocks` (verify.go) —
  the migration must not weaken the no-mock-boundary enforcement. The overnight run is
  the live no-mock journey walk (real model, real repo), not a fake.
- **Fail-closed.** Exit 0 only on PASS; invalid/unparseable status → halt+page, never
  silent pass. Migrate Go types UP to the schema (D6 ratified direction), never
  downgrade the schema.
- **Public-safe.** This release runs sworn against the PRIVATE getfired/fired repo.
  Captures committed to the public sworn repo must not leak private business content
  (handoff flagged existing fired-path leakage → S27-public-readiness-scrub). Any new
  capture from the overnight run is private-by-default.
- **Minimal justified deps** (ADR-0007); single Go binary; stdlib-preferred.

## Adjacent / out of scope

- **Multi-track parallelism on real content.** **Why deferred**: serial is acceptable
  for this milestone (operator's explicit call); parallel proof is a separate concern.
  **Tracking**: handoff tier-3 + the `2026-06-29-engine-readiness` 2-track test bed in
  `~/sworn-eval-engine-readiness`. **Acknowledged**: 2026-06-30 (Brad).
- **Keystone schema-family rollout** (review-v1, design-v1, orchestrator/coach schemas).
  **Why deferred**: completeness, not an operational blocker; sworn runs without it.
  **Tracking**: `docs/captures/2026-06-30-keystone-release-planning-brief.md` (Emit-Baton
  / Emit-Sworn tracks). **Acknowledged**: 2026-06-30 (Brad).
- **baton-web schema publish** (new schemas resolve at $id). **Why deferred**: engine
  validates against embedded bytes; publish gates external portability only.
  **Tracking**: handoff open items. **Acknowledged**: 2026-06-30.
- **T16 capture remainder** (durable cross-run store, token enrichment, real cost).
  **Why deferred**: routing-moat data, not an operational blocker. **Tracking**: #26 /
  driver-contract S07 (≡ T16). **Acknowledged**: 2026-06-30.

## Decisions made during planning

`<Captured below as each AskUserQuestion is answered.>`

## Schema-vs-spec audit notes

- `slice-status-v1` (`internal/baton/schemas/slice-status-v1.json`) defines
  `open_deferrals` and `violations` as **arrays of objects**, not strings. The Go
  carriers (`state.Status.OpenDeferrals []string`, `Verification.Violations []string`)
  are the drift. Migration direction is UP (Go → schema), confirmed by D6 and by the
  fired real data being the object form already.
- `need_ids` (writer) vs `covers_needs` (schema + RTM gate) — 3-way drift flagged in
  the role-layer schema audit; reconcile as part of D6.
- #37: `inconclusive` is NOT yet in the slice-status-v1 leaf `result` enum; the agentic
  verifier can already PRODUCE inconclusive (folded into FAIL today). Bundled with D6.

## Proposed slice decomposition (draft)

`<Phase 3 — confirmed via Scope-Ceiling Bar + Dependency Graph below.>`

- `S01-d6-record-reconciliation` — sworn reads real coach status.json: migrate
  `OpenDeferrals`/`Violations` `[]string`→object round-trip, reconcile
  `need_ids`→`covers_needs`, add `inconclusive` leaf-enum (#37), update the ~6 consumers.
- `S02-retry-reset-preserves-work` (candidate) — a transient failure retry does not
  discard already-committed slice work.
- `S03-escalation-honours-config` (candidate) — escalation uses the configured models,
  not a hardcoded openai default; sane turn-cap for real slices.

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | What does sworn drive overnight — the 3-slice fired dogfood release (`2026-06-28-yearSnapshot-schema-cleanup`), or a getfired v0.5.0 release that must be planned first, or LANDING the existing ~1M-line `release/v0.5.0` branch (review/merge of code that already exists)? These are different operational shapes. | The overnight run target + whether a downstream `/plan-release` is needed | human will provide tonight |
| A-02 | Does the unattended overnight run need the S02/S03 resilience hardening, or is D6-only + a watched first run acceptable for tonight? | Release scope boundary | resolved by scope-boundary decision (below) |
| A-03 | D6 blast radius — exact consumer list and whether the object types are new Go structs or reuse an existing baton type. | S01 spec precision | requires grep during spec authoring (Phase 4) |

## Screenshots / references

- (none yet — engine release, no UI)
