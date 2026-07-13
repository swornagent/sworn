# ADR 0011: Structured-outputs keystone — schema-validated records, prose scrapers deleted

## Status

**Accepted.** Decision made and implemented ~2026-06-30 on branch
`keystone/structured-outputs` (steps 1–3 verified fresh-context at the time).
**This document is a retroactive backfill authored 2026-07-12** — the decision
was implemented and cited by number in ~25 source sites but the ADR file itself
was never written (discovered 2026-07-12 while numbering ADR-0013, which builds
on it). Reconstructed from those code citations and the six keystone captures:
`docs/captures/2026-06-30-keystone-release-planning-brief.md`,
`-keystone-step2-{handoff,proof,verify}.md`, `-keystone-step3-{proof,verify}.md`.
High confidence on the decision, the schema family, and delivery; the D1–D6
lettering and §-numbering are reconstructed from the planning brief and code
citations and may differ in minor detail from the original working notes.

**Type-1 (architecturally significant, hard to reverse):** this is the seam by
which every role/layer message crosses from model to engine. It deleted the prose
scrapers, so reverting would mean re-writing them. ADR-0013 (capability-based
model selection) builds directly on the `StructuredOutput` capability defined here.

## Context (§1)

`sworn`'s loop passes messages between model and engine at many seams — the
verifier's verdict, the captain's review, the design TL;DR, orchestrator routing
decisions, Coach escalations. Historically the engine recovered structure from
these by **scraping free text**: regex/line-splitters like `parseVerdict`,
`extractViolations`, `firstVerdictLine`, `stripMarkdown`, and a stateless
prose-classifier interpreter. Two problems (§2):

- **The scrapers were fragile and load-bearing.** A model that phrased a verdict
  slightly differently, wrapped it in markdown, or added narration could be
  mis-parsed into a scraped/optimistic verdict — a silent correctness hole on the
  Rule-7 path (the very place fail-closed matters most).
- **Schema "validation" was decorative** (§2, finding 1): `baton.Validate` did
  not actually validate against the JSON Schemas — the checks were cosmetic, so a
  malformed record could pass.

ADR-0009 had already set the direction ("the machine parses JSON only — never
prose"); this ADR is the keystone that makes it real across every role/layer.

## Decision (§3)

**Every role/layer message is a schema-validated structured-output record; the
free-text scrapers are deleted.** Concretely:

1. **Real schema validation.** Replace decorative checks with genuine
   draft-2020-12 validation (`baton.ValidateSchema`). A record that fails its
   schema is rejected, fail-closed — no prose-fallback path exists.
2. **A `StructuredOutput` capability** (additive interface, `internal/model`):
   ```go
   type StructuredOutput interface {
       ChatStructured(ctx, messages, schema []byte) (*ChatResponse, error)
   }
   ```
   A driver opts in by implementing it and advertising `CapStructuredOutput`;
   `Verify`/`Chat` signatures are untouched. Fail-closed at the WIRE level only
   (non-empty, parses as a JSON object); SEMANTIC validation against the named
   canonical schema is the caller's job (`baton.ValidateSchema`), keeping the
   schema layer decoupled from the wire layer.
3. **Two emission mechanisms behind one interface:** native strict `json_schema`
   `response_format` (OpenAI-compatible chat + OpenAIResponses `text.format`), and
   a forced single-tool fallback for models without strict `response_format`
   (e.g. DeepSeek). See ADR-0013 for the later move to discover this per model.

### The schema family (§3.1–§3.8)

Each role/layer gets a typed record schema; emitting it via `ChatStructured` and
validating by name replaces that role's scraper:

- **§3.1 `design-v1`** — the design TL;DR record. (D3: canonical writer of
  `status.json` `design_decisions`.)
- **§3.2 `review-v1`** — the captain review record (closes #34, the pin-count
  scrape; finishes the #32 supersede).
- **§3.3 `verifier-verdict-v1`** — the verifier's verdict, **judgement-only**
  (§3.3 g): PASS/FAIL/BLOCKED/INCONCLUSIVE + typed violations coming off the record,
  not a prose split. **This was the pilot (step 3).**
- **§3.4 `orchestrator-decision-v1`** — routing/triage decision as a record.
- **§3.5 `coach-call-v1` / §3.6 `coach-response-v1`** — escalation + Coach
  gesture → JSON (emit side touches the private coach harness; open-core split).
- **§3.7 `effort_complexity`** (#36) — the two-axis per-slice rating and its
  effort×complexity → quadrant mapping (source of truth for routing).
- **§3.8 `telemetry-event-v1`** — telemetry record + transport.

The envelope target (§5) is a future `orchestrator-event-v1` unifying the
orchestrator/coach records; §3.4 shipped as a flat record first.

## Design decisions (§8 / D1–D6)

- **D1 — strict projection.** The interface takes the LENIENT canonical schema
  (opaque bytes); drivers using strict `response_format` project it to the strict
  profile at call time (`strictProjection`). Documented author constraint: some
  optionals must be made required in the strict profile (§3 limitation).
- **D2 — schema ownership split.** Design/review schemas are **Baton-owned**;
  orchestrator/coach schemas are **Sworn-owned** (the open-core seam; ADR-0010).
- **D3 — single writer.** `design-v1` is the canonical writer of
  `design_decisions` (resolve duplicate-writer ambiguity in favour of one writer).
- **D4 — INCONCLUSIVE, deferred leaf enum (Option A / #37).** Invalid emission →
  INCONCLUSIVE, fail-closed; the merge gate is unchanged; adding `inconclusive`
  to the slice-status leaf `result` enum was deferred (#37).
- **D6 — type migration UP.** `need_ids`→`covers_needs`;
  `open_deferrals`/`violations` `[]string`→object; Go types migrated up to match.
- (The Enforcement rewire `baton.Validate`→`ValidateSchema` lands last so it
  reconciles against final schema shapes, not moving targets.)

## Delivery (steps 1–3, on `keystone/structured-outputs`)

- **Step 1 @d828ebc** — real draft-2020-12 validation (`baton.ValidateSchema`).
- **Step 2 @4c8a6ad (verified @65a4a40)** — `StructuredOutput` + `ChatStructured`
  (OAI strict `response_format` + DeepSeek forced-tool + OpenAIResponses
  `text.format`); `strictProjection` (D1).
- **Step 3 @869f07c (verified @d41892b)** — `verifier-verdict-v1` pilot:
  `verify.RunAgentic` emits the typed verdict; `parseVerdict`/`extractViolations`
  prose scrapes DELETED; invalid emission → INCONCLUSIVE fail-closed. The
  stateless prose-classifier interpreter was removed (Step 3 note in
  `interpreter.go` / `run/slice.go`).

Remaining schema-emit tracks (review-v1, design-v1, orchestrator/coach, telemetry)
were decomposed into the follow-on `2026-06-30-keystone-structured-outputs`
release per the planning brief.

## Consequences

- **Positive:** the correctness hole (scraped/optimistic verdicts) is closed on
  the Rule-7 path; records are machine-validated, portable at their `$id`, and
  driver-neutral. Establishes the `StructuredOutput` capability that ADR-0013
  generalises into capability-based selection.
- **Costs:** every role needs a schema + a negative AC ("emit failing
  `ValidateSchema` → fail-closed, no prose fallback"); the strict-projection D1
  constraint is a real author burden; not all drivers support structured output
  (the gap ADR-0013 addresses via capability matching).
- **Relationships:** realises ADR-0009 (JSON not prose); honours ADR-0010
  (schema ownership split); is the foundation ADR-0012 (driver `RoleSet`) and
  ADR-0013 (capability-based selection) build on.

## Section map (for the ~25 in-code `ADR-0011 §…` citations)

`§2` findings (scrapers decorative) · `§3` the structured-output decision ·
`§3 / D1` strict projection · `§3.3 g` verifier-verdict judgement-only ·
`§3.7` effort×complexity routing rating (#36) · `§8` the D1–D6 decisions & sequence.
