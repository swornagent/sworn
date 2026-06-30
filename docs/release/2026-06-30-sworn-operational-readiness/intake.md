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
- N-01: sworn reads a real coach-produced status.json whose open_deferrals and verification.violations are arrays of OBJECTS (not strings) without an unmarshal error.
- N-02: Read then Write of such a status.json PRESERVES every field of each deferral/violation object (no silent field drop on write-back) — the no-degrade non-negotiable.
- N-03: the slice-status field carrying intake-need IDs is named covers_needs (schema name), not need_ids, so a real status.json round-trips and the RTM gate reads it.
- N-04: an inconclusive verifier verdict is representable in the slice-status leaf result enum (#37 / deferred D4), so the agentic verifier's inconclusive maps cleanly rather than being forced into fail.
- N-05: the release board's human view (index.md) is deterministically RENDERED from board.json plus the slice records by `sworn render`, never hand-authored by a model or human, so the operator monitoring an unattended run reads a faithful view and the index.md frontmatter-corruption false-ready failure mode is removed.
- N-06: sworn never dirties a consumer repo — when it creates `.sworn/` runtime state in a repo it operates on, that directory is self-ignored (`.sworn/.gitignore` = `*`), so it never appears in the host repo's git status or gets committed.
- N-07: sworn reads a real coach-produced board.json whose `release` is the canonical baton OBJECT form ({name, vertical_trace, ...}) without an unmarshal error (and tolerates the legacy string form), so the oracle/run load a real release instead of failing at board-read.
- N-08: sworn EMITS and VALIDATES the canonical object form for `release` (strict, object-only — no string-tolerance divergence from canonical baton), while the reader stays tolerant of a legacy string; existing string boards self-heal to canonical on next write.

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

### 2026-06-30 — Scope = D6 only tonight; resilience deferred (tracked)

- **Context**: how wide to scope for an autonomous-overnight run tonight (each slice =
  one implement + one verify session ≈ the night's time budget).
- **Options considered**: (A) D6 only, watch the first slice land then walk away;
  (B) D6 + autonomy hardening (retry-reset, escalation-default, turn-cap); (C) + keystone
  family.
- **Decision**: **A — D6 only (S01).** "let's goooo" — fastest path to a running
  autonomous loop. Watch the first slice go implement→verify→merge to confirm the loop
  works end-to-end, then let it run unattended for the rest.
- **Why**: D6 is the only empirical blocker; cold-start + docs-prefix already fixed. The
  resilience items are non-blocking tuning; if the overnight run pages on one, S02/S03 are
  ready follow-ups. Minimises tonight's work to get sworn operational.
- **Deferred (Rule 2)**: `S02-retry-reset-preserves-work` and `S03-escalation-honours-config`
  — why: non-blocking tuning, not a hard stop; tracking: this intake's out-of-scope +
  eval findings 5/6 in `2026-06-30-session-handoff.md`; acknowledged 2026-06-30 (Brad).

### 2026-07-01 — Add S05-board-canonical-emit (T4): right-moving-forward, not permanent back-compat

- **Context**: Brad questioned whether S04's back-compat (oneOf string|object schema + tolerant
  validator) is right vs making it right forward. Test: back-compat earns its keep only against a
  durable, uncontrolled population of old data — here there is none (coach boards are already
  canonical object; the only string boards are our own temporary stopgaps). S04 was already
  verified (immutable, Rule 7), so this is a new slice appended to T4.
- **Decision**: `S05-board-canonical-emit` — Postel: strict EMIT + strict schema/validator
  (canonical object-only — kills the oneOf vendor drift, the #38 class), lenient READ (tolerant
  unmarshaler stays). Writer emits the object form even for a name-only release, so existing
  string boards self-heal on next write (the tight answer to "fix the previous ones too").
- **Registration note (this replan)**: S05 was first implemented directly in the T4 worktree
  (feat 565f909), collapsing the planner→implementer→verifier lifecycle — its board membership +
  start_commit were never planner-established, so the fresh verifier BLOCKED it (registration is
  planner authority). This /replan-release registers S05 under T4 in release-wt's board, sets
  start_commit to 0d22f65 (parent of the S05 feat, isolating its production diff), clears the
  BLOCKED verdict to pending, and forward-syncs to the track. The implementation itself was not
  re-touched.

### 2026-07-01 — Add S04-board-record-reconciliation (T4): oracle reads the canonical board (DIRECTION REVERSAL)

- **Context**: the live fired run also fails at BOARD-read: `sworn board --release
  2026-06-28-yearSnapshot-schema-cleanup` → "cannot unmarshal object into BoardRecord.release
  of type string". Brad asked the key question — is the binary wrong or the release? Checked
  the canonical baton schema (`~/projects/baton/schemas/board-v1.json`): `release` is an
  OBJECT with required `name` (+ vertical_trace, the Rule-8 golden thread). sworn's embedded
  board-v1 + Go BoardRecord.Release are `string` — pinned to an older baton (binary reports
  Baton v0.6.3) and lagging. Every coach-produced board uses the object form.
- **Decision**: the **binary is behind, not the releases.** Add `S04-board-record-reconciliation`
  (track `T4`) — make BoardRecord.Release read the canonical object form (tolerate the legacy
  string), reconcile the embedded board-v1 schema, update the one consumer. The board-level
  companion to D6 (same class: Go types lag baton schemas; migrate UP).
- **Direction reversal (own it)**: an earlier session-step "fixed" THIS release's board.json by
  downgrading its `release` object → string to satisfy the stale oracle. That was wrong-
  direction (conforming the artefact to the buggy binary). **Once S04 lands + the oracle is
  rebuilt, revert this release's board.json back to the canonical nested object** (restore
  vertical_trace). Until then it stays string only to be readable by the current binary
  (chicken-and-egg).
- **Scope note**: tonight-critical (fired's board is unreadable without it — the run fails at
  board-read BEFORE status-read/D6). Touchpoint-disjoint from T1 (oracle.go does not read
  .Release), T2, T3. Full baton re-vendor (all schemas) is a larger follow-up; this is the
  board-read unblock only.

### 2026-07-01 — Add S03-sworn-self-ignore (T3): sworn must not dirty consumer repos

- **Context**: during operational prep for the fired overnight run, `~/projects/fired/.sworn/`
  (the run DB + supervisor DB) showed as `?? .sworn/` — untracked, not ignored. sworn writes
  `.sworn/` into every repo it runs on but never self-ignores it, though sworn's OWN repo
  gitignores `.sworn/` (.gitignore:26). Patched fired by hand for tonight (`.sworn/.gitignore`
  = `*`); the engine fix folds in here (replan of an in-flight release).
- **Decision**: add `S03-sworn-self-ignore` as independent track `T3-consumer-repo-hygiene` —
  sworn writes `.sworn/.gitignore` (`*`) at the `.sworn/` creation site (internal/db/db.go),
  idempotent (never overwrite an existing one) and best-effort (a write failure never fails
  the run).
- **Why**: an untracked `.sworn/` reads the worktree dirty (a dominant loop-failure mode) and
  risks a binary DB being swept into a consumer's release branch by any broad auto-stage.
  sworn's value is running on OTHER repos, so it must leave no trace — directly the
  operational-readiness golden thread ("run cleanly on real repos, unattended").
- **Scope note**: independent of T1/D6 (state/verify/run) and T2 (board render) — touches only
  internal/db; does not disrupt the in-flight T1 work. NOT a hard blocker for tonight (fired
  is hand-patched), but the durable fix belongs in the engine.

### 2026-06-30 — Add S02-board-render: index.md is rendered, never authored

- **Context**: during this very planning session the planner had to HAND-AUTHOR index.md
  because no renderer exists, despite the planner contract saying "rendered from board.json,
  never hand-authored." Brad: "there should be no need for a model to output md."
- **Decision**: add `S02-board-render` as an independent track `T2-board-render` — a
  deterministic `sworn render <release>` that generates index.md from board.json + the slice
  records. The model emits the RECORD (board.json); code renders the VIEW (index.md).
- **Why**: records-vs-prose principle — index.md is a view of a record, not prose, so it must
  be rendered, not authored. It also removes a real operational failure: a hand-authored
  index.md frontmatter fusion previously produced a false merge-ready
  ([[project_index_frontmatter_corruption_false_ready]]). So it traces to operational
  reliability, not just tidiness. intake.md and journal.md remain prose (correctly authored).
- **Scope note**: NOT a blocker for tonight's overnight run — the oracle reads board.json
  preferentially, so the fired run does not need the renderer. T1/D6 stays the only
  tonight-critical path. T2 is touchpoint-disjoint from T1 (new render files vs D6's
  state/verify/run files) and can be done any time. Updating the planner skill/prompt to
  CALL `sworn render` is a separate private-harness change (out of scope for the Go slice).

### 2026-06-30 — Overnight target = the fired dogfood release

- **Context**: what sworn actually drives overnight (A-01).
- **Decision**: the **fired dogfood release** `2026-06-28-yearSnapshot-schema-cleanup`
  (`~/projects/fired`, 3 slices / 1 track, deepseek-v4-pro, provider keys seeded in
  `~/.sworn/.env`). It is the run that blocked at D6 and is the smallest real end-to-end
  operational proof.
- **Why**: already planned + seeded; ready the moment D6 lands. Planning/running the big
  getfired v0.5.0 release is a separate downstream effort — prove the loop on the dogfood
  first. (Landing the existing ~1M-line v0.5.0 branch is a different engine shape —
  reviewer/merger not builder — and is NOT this release.)

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
