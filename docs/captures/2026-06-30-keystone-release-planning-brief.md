---
title: 'Planning brief — 2026-06-30-keystone-structured-outputs release'
description: 'Input to /plan-release (NOT a spec). Decomposition, dependency order, and Rule 8/9/10 carry-ins for the remaining unbuilt keystone work, given steps 1-3 + #36 are already on-branch and verified.'
date: 2026-06-30
---

# Planning brief — keystone structured-outputs release

> This is a **planning input** for `/plan-release 2026-06-30-keystone-structured-outputs`,
> not a spec. The Planner role still drives Rule-8 discovery and writes intake.md +
> per-slice specs. Treat the slices below as a starting decomposition to refine, and
> the decisions as items to ratify in conversation — do NOT manufacture spec.json from
> this (that would be the Rule-8 spec-after-the-fact smell).

## Release goal (golden thread — Rule 8)
**Every role/layer message is a schema-validated structured-output record; the free-text
scrapers are deleted** (ADR-0009: "the machine parses JSON only — never prose"). Every
slice must trace up to this. If a slice doesn't move a scraper toward deletion or directly
enable that, it belongs to a different release.

## Foundation already on-branch (NOT re-planned; the release's base)
Base = `keystone/structured-outputs` HEAD. All verified fresh-context against ADR-0011:
- Step 1 @d828ebc — real draft-2020-12 validation (`baton.ValidateSchema`).
- Step 2 @4c8a6ad (verified @65a4a40) — `StructuredOutput` interface + `ChatStructured`
  (OAI native strict `response_format` + deepseek forced-tool; OpenAIResponses `text.format`);
  `strictProjection` (D1).
- Step 3 @869f07c (verified @d41892b) — `verifier-verdict-v1` pilot. `verify.RunAgentic`
  emits the typed verdict; `parseVerdict`/`extractViolations` prose scrapes DELETED; invalid
  emission → INCONCLUSIVE fail-closed. (Did NOT close #32/#34 — #34 lands with review-v1.)
- #36 effort_complexity — substantially built: 763a5c5 field + 1dc1afc dispatch-stamp +
  9338b57 bootstrap. **Planner: confirm what remains** (does routing CONSUME the quadrant
  yet?) and either fold the remainder into Telemetry track or mark done.

## Decomposition — remaining unbuilt work, as tracks
Decisions applied: one release, multiple tracks (your call); Enforcement isolated and
merged last (your call; pilot already PASSed so its precondition is met).

### Track Emit-Baton — design/review schemas (D2: Baton-owned)
Each slice: author+embed schema → role emits via `ChatStructured` → caller validates by
name → delete that role's scraper → baton-web publish at `$id` → re-vendor into sworn.
- **EB1 review-v1** (§3.2) — captain review becomes a record. **Closes #34** (the captain
  pin-count scrape) and finishes the #32 supersede. ADR §8 seq 3 → do this first; it's the
  second-strongest proof after the verdict pilot.
- **EB2 design-v1** (§3.1) — design TL;DR record. D3 makes it the **canonical writer of
  `design_decisions`**; resolve §3.1(b) first (does design-v1 *source* status.json's
  design_decisions, or duplicate them — pick one writer). Type-1, see below.

### Track Emit-Sworn — orchestrator/coach schemas (D2: Sworn-owned)
- **ES1 orchestrator-decision-v1** (§3.4) — routing/triage decision as a record. Co-design
  with the telemetry decision-log (see [[project_telemetry_eval_foundation]]). Type-1:
  flat record now vs the `orchestrator-event-v1` envelope target (§5).
- **ES2 coach-call-v1** (§3.5) + **ES3 coach-response-v1** (§3.6) — escalation + Coach
  gesture→JSON. NOTE: the emit side touches the **private** coach/coach-loop bash
  (open-core split) — the schema + sworn-side consumer are public; the private harness
  emitter is separate work. Flag the boundary at planning.

### Track Enforcement — the rewire (ISOLATED worktree, merged LAST)
- **EN1 step 1b** — rewire `baton.Validate` → `ValidateSchema` + D6 type migration
  (`need_ids`→`covers_needs`; `open_deferrals`/`violations` `[]string`→object; migrate Go
  types UP) + **#37** (add `inconclusive` to the slice-status leaf `result` enum, the
  deferred D4). Expect ~28 test fixes + committed conformance-data churn. Lands after the
  emit tracks so it reconciles against final schema shapes, not moving targets.

### Track Portability — publish + telemetry
- **PT1 baton-web publish** — all new schemas resolve at their `$id` (today only the old 5
  resolve; the rest 404). Prerequisite for model-A portability; lands before cutover.
  (The Go engine validates against *embedded* bytes, so this gates portability, not the
  engine slices.)
- **PT2 telemetry-event-v1** (§3.8) + transport (sworn-internal telemetry-eval-transport doc).

## Carry-ins for the Planner conversation
**Rule 8 (requirements fidelity):** ACs in EARS; each emit slice needs the negative AC —
*"When a role emits a payload failing `ValidateSchema(<schema>)`, the system SHALL
<fail-closed action> and no prose-fallback path SHALL exist."* RTM: each role's need (kill
its scraper) → AC → test. The scraper-deletion is only safe behind that negative AC.

**Rule 9 (design fidelity):** D1-D6 (ADR §8) + the Option-A INCONCLUSIVE decision
(merge gate unchanged; #37 defers the leaf enum) + the D2 ownership split are RATIFIED —
reference, don't re-open. NEW Type-1 choices this release introduces, each needs a recorded
human decision in status.json:
1. `orchestrator-decision-v1` flat record vs `orchestrator-event-v1` envelope (§5).
2. `design_decisions` single-writer resolution (§3.1b).
3. Enforcement track merge ordering (recommended last) and D6 up-migration confirmation.
4. coach-emit public-vs-private-harness boundary (which half is in THIS release).

**Rule 10 (customer journey / no-mock):** the keystone journey = *"one real run emits typed
records across verifier + review + design + orchestrator, zero prose scraping, end to end."*
Must be walked over a **LIVE model dispatch** at cutover — httptest≠live is the standing
watch item from step 2. The no-mock boundary covers both the model dispatch AND baton-web
`$id` resolution (PT1).

**Drift caution (from [[feedback_replan_propagate_by_merge_not_copy]]):** "everything in one
release" = a long branch with parallel tracks. The drift gate counts commit ANCESTRY, not
content — propagate any base-fix forward by `git merge --no-ff release-wt/<rel>`, never
cp-files. Sequence the Enforcement track to merge last to minimise concurrent divergence.
