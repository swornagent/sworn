# Schema-Constrained Outputs for Every Role and Layer

**Status:** Draft — proposes **ADR-0011** (role/layer output schemas). Extends and
depends on **[ADR-0009](../adr/0009-records-json-prose-markdown.md)** (Records as
JSON, Prose as Markdown).

**Date:** 2026-06-29
**Author:** Coach-directed synthesis (task #21)

---

## 1. Summary

ADR-0009 established the invariant **"the machine parses JSON only — never prose"**
for *persisted records* (board, spec, proof, slice-status, journeys, attestations)
and named the **authoring path** as the keystone still to be built: the binary makes
a **structured-output call** — `response_format: json_schema` where the driver
advertises a `CapStructuredOutput` capability, **tool-call-schema fallback**
otherwise — so a record is *emitted against its schema and validated on the way in*,
never free-text-scraped.

**The gap this document closes:** ADR-0009 stopped at *records*. But the loop is
shot through with **inter-role, inter-layer, and coach messages** that are still
free text scraped by regex:

- the **design TL;DR** (six markdown sections + `hasSixSections` substring check +
  captain `pinLineRe` scrape),
- the **captain review** (`parsePins` prose-scrape; the escalate-halt counts a
  scraped scalar — the #34 miscount class),
- the **verifier verdict** (`parseInterpretResult` does `HasPrefix(upper,"PASS")`
  over model prose — the *one live ADR-0009 invariant breach in the hot path*),
- the **orchestrator/router decisions** (flat `(role,action,reason)` rows; targets,
  model-slot, attempt, and cost dropped at the persistence boundary),
- the **coach page** (`page_coach <title> <body>` string, re-scraped by keyword for
  colour/routing) and the **coach reply** (`approved-ack.md` file-presence + verb
  parsing).

**The thesis:** every one of these is a structured-output call waiting to happen.
ADR-0011 extends the ADR-0009 authoring path from the six persisted records to
**every role/layer/coach message**, deleting the remaining prose parsers and giving
each message a published, validated, portable schema.

A cross-cutting finding sharpens the urgency: **the published schemas are not
actually enforced.** `internal/baton/validator.go` is a hand-rolled required-fields
scanner — "Full JSON Schema validation is deferred to ADR-0007." So the embedded
draft-2020-12 documents are *decorative*: their patterns, nested shapes, item types,
enums, and `minLength`s never run. That is the very "bespoke scanner over a
structured doc" smell ADR-0009 was written to kill, now applied to the schemas' own
enforcement.

---

## 2. Critique of the existing schemas

Ten schemas reviewed: six **record** schemas (one classified leaf + five flow), and
four **config** schemas. Every one returned **needs-work**.

| Schema | Bucket | Verdict | Top finding | Drift vs emitted |
|---|---|---|---|---|
| slice-status-v1 | record (leaf) | needs-work | `additionalProperties:true` + thin `required` ⇒ cannot be a strict structured-output target, and *blesses* drift instead of constraining it | `need_ids` (writer) vs `covers_needs` (schema + RTM gate) — 3-way; `open_deferrals`/`violations` emitted `[]string` vs schema objects; dispatches/routing/model/release_benefit_link emitted but absent |
| board-v1 | record | needs-work | `$schema` const declared but **never emitted** (no committed board.json carries one) | `depends_on:null` on disk vs schema `type:array`; validator is a hand-rolled subset; track-state enum can't represent parked/blocked |
| spec-v1 | record | needs-work | **Authoring path is backwards** — spec.json is a regex *scrape* of spec.md, the exact anti-pattern ADR-0009 kills | checkbox state written into `type`; missing scope/touchpoints/per-AC trace+test_refs; only `covers_needs` ever read |
| proof-v1 | record | needs-work | proof.json is **write-only** — every consumer scrapes proof.md prose | proof.md independently generated, not `render(json)`; no drift guard; no head_commit/verifier linkage |
| journeys-v1 | record | needs-work | Published schema **stricter than** the hand-rolled validator (per-journey/step required fields never enforced) | `version` (int) vs siblings' `schema_version` (const 1); ordering under-specified; DraftTemplate is a Go heuristic, not a structured-output call |
| attestations-v1 | record | needs-work | **No production emitter** — SaveAttestations is test-only ⇒ the only path to populate is a human hand-editing JSON ("that is the bug") | validator skips the entire `attestations[]` item layer; boundary load↔save round-trip fails; status zero-value `""` not in enum |
| architecture-rules-v1 | config | needs-work | **Hard validation break** — scaffold + struct emit `_description` inside `canonical_docs` which is `additionalProperties:false` | schema cites dead `release-audit-design.sh`; `touchpoint_source` dead config; not embedded ⇒ no validation on read |
| architecture-overrides-v1 | config | needs-work | **Fatal contract divergence** — schema = per-release `{suppress_rules,amend_rules}`; the only reader (mock.go) expects per-slice `{mock_overrides[]}` | schema has **zero live consumers**; `reason` optional (Rule-2 hole) |
| design-fidelity-v1 | config | needs-work | **Incomplete vs consumer** — gate reads `tokens[]` (load-bearing colour exemptions) that `additionalProperties:false` forbids | two divergent Go carriers (config.json + design-fidelity.json); Rule-9 `ui_bearing⇒design_system` conditional not encoded |
| design-allowlist-v1 | config | needs-work | **Total structural drift** — schema `allowlist`/`pattern` vs consumer `rules`/`rule_id`+`file`; mutually exclusive | sworn's own test fixture cites the canonical `$id` on a doc that violates it; ack fields optional/unconsumed |

### High-severity findings

1. **The validator is not the schema (all ten).** `baton.Validate` dispatches to
   per-type hand-rolled checks that verify a handful of top-level fields and the
   `$schema` const, then stop. Nested-object shapes, item types, enums, patterns,
   and `minLength`s are never evaluated. Every "schema says X" below survives only
   because nothing enforces X. **This is the single biggest ADR-0009 alignment gap.**

2. **No schema is a strict structured-output target (all six records).** OpenAI-style
   strict `json_schema` mode requires `additionalProperties:false` **and** every
   property listed in `required`, at every object level. All six records use
   `additionalProperties:true` with thin `required` sets. As written **none can be
   handed to the ADR-0009 authoring-path call.** A strict generation profile must be
   produced (distinct from, or projected from, the lenient validation profile).

3. **Authoring direction is inverted or absent for the records that matter.**
   spec.json is scraped *from* spec.md (backwards); proof.json/attestations are
   emitted-or-not but **never consumed as JSON** (consumers scrape the .md); board's
   `$schema` discriminator is declared but never written. The "validated round-trip"
   ADR-0009 promises does not yet exist for any record except status.json.

4. **Field drift, masked by the shallow validator (slice-status).** The headline
   Rule-8 trace link is `need_ids` in the writer, `covers_needs` in the schema *and*
   in the RTM gate, and `covers_needs` (fed from `st.NeedIDs`) in spec.json — the
   writer, schema, and consumer disagree on one field name. `open_deferrals` and
   `violations` are `[]string` on the wire but objects in the schema — a hard interop
   break that also defeats the Rule-2 why/tracking/ack contract the schema claims.

5. **Config schemas: divergent de-facto contracts.** `architecture-overrides-v1`
   and `design-allowlist-v1` each describe a *different file* than their only sworn
   reader — a file valid against the schema is silently ignored, and the file sworn
   honours fails the schema. The config schemas are also **not embedded** in
   `embed.go`, so none are validated on read at all.

### The baton-web publishing gap (cross-cutting, blocks "model A")

ADR-0009's portability story (consumption **model A**: an arbitrary LLM emits a
record against the *published* schema at its `$id`, and the machine routes it)
requires the schema to actually resolve at its canonical URL. **Only the original
five are published at `baton.sawy3r.net/schemas/`.** Every record schema carries a
canonical `$id` (e.g. `https://baton.sawy3r.net/schemas/spec-v1.json`), but the new
record schemas — and *all six proposed message schemas below* — would **404 at their
`$id`**. Until baton-web serves them, the `$schema` const is a dangling pointer: a
self-identifying discriminator that no external emitter can dereference. **Publishing
the schemas is a prerequisite of the authoring path, not an afterthought.**

---

## 3. The six new schemas

Six new schemas extend the authoring path to inter-role / inter-layer / coach
messages. Each is sketched in draft-2020-12 to match the existing family
(`$schema` const = `$id`, `schema_version` const 1, slice-scoped identity triple).

> **House-style tension (applies to all six).** The persisted-record family uses
> `additionalProperties:true` for forward-compat because it validates
> *binary-marshalled* JSON. These six are **emission targets** on the strict
> authoring path, so several sketches set `additionalProperties:false`. This is a
> deliberate divergence — see §8 open decisions. Where a sketch keeps `true`, a
> strict projection must be generated at call time.

---

### 3.1 `design-v1` — the design TL;DR

**Producer → Consumer:** implementer (`internal/design.Generate`, structured-output
call) → captain (`internal/captain.Review` / `design-reviewer.md` six-step review);
also rendered to design.md for the Coach.

**Purpose:** Replace the free-text six-§ markdown + `hasSixSections` check + captain
`pinLineRe` regex with a validated record. §2 is lifted into the Rule-9
`design_decisions` shape (`choice/stake_class/options/rationale`) the captain's
Step-2b design-fit gate already needs.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://baton.sawy3r.net/schemas/design-v1.json",
  "title": "Baton Design TL;DR Record",
  "type": "object",
  "additionalProperties": true,
  "required": ["$schema","schema_version","slice_id","release","user_visible_change","design_decisions","files","not_doing","reachability","open_questions"],
  "properties": {
    "$schema": {"type":"string","const":"https://baton.sawy3r.net/schemas/design-v1.json"},
    "schema_version": {"type":"integer","const":1},
    "slice_id": {"type":"string","minLength":1},
    "release": {"type":"string","minLength":1},
    "user_visible_change": {"type":"string","minLength":1,"description":"§1 — one sentence; observable proof if internal."},
    "design_decisions": {"type":"array","maxItems":5,"items":{"type":"object","additionalProperties":true,"required":["decision","resolution"],"properties":{"decision":{"type":"string","minLength":1},"resolution":{"type":"string","minLength":1},"rationale":{"type":"string"},"stake_class":{"type":"string","enum":["Type-1","Type-2"]},"options":{"type":"array","items":{"type":"string"}},"architecturally_significant":{"type":"boolean"},"memory_refs":{"type":"array","items":{"type":"string"}}}}},
    "files": {"type":"array","items":{"type":"object","additionalProperties":true,"required":["path","purpose"],"properties":{"path":{"type":"string","minLength":1},"purpose":{"type":"string","minLength":1},"group":{"type":"string"}}}},
    "not_doing": {"type":"array","items":{"type":"object","additionalProperties":true,"required":["item"],"properties":{"item":{"type":"string","minLength":1},"spec_ref":{"type":"string"}}}},
    "reachability": {"type":"object","additionalProperties":true,"required":["integration_point","proof"],"properties":{"integration_point":{"type":"string","minLength":1},"proof":{"type":"string","minLength":1}}},
    "open_questions": {"type":"array","maxItems":3,"items":{"type":"object","additionalProperties":true,"required":["question"],"properties":{"question":{"type":"string","minLength":1}}}}
  }
}
```

**Open questions:** (a) `additionalProperties` true vs strict for the emit call;
(b) does `design_decisions` duplicate or *source* status.json's — pick one writer;
(c) protocol surface (Baton) or impl (Sworn) — the captain-split memory argues
protocol; (d) `reachability` as object vs the prompt's free sentence — update the
prompt in lockstep; (e) encode soft caps (≤5/≤3) as `maxItems` or advisory;
(f) the captain's *output* should also become a record (→ `review-v1` below).

---

### 3.2 `review-v1` — the captain review

**Producer → Consumer:** captain (`/design-review`, `captain.md`) → the run loop
(`internal/run/slice.go` escalate-halt + `FormatPinsAsFeedback`) and the Coach.

**Purpose:** Make the escalate-pin halt a **count over a typed array**
(`len(filter(pins, tag=="escalate"))`) instead of a substring scan of a prose
summary line — structurally eliminating the **#34 pin-miscount class**. Carries the
three captain deliverables (pins, review.md content, Coach acknowledgement reply)
plus the routing verdict in one envelope.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://baton.sawy3r.net/schemas/review-v1.json",
  "title": "Baton Captain Review Record",
  "type": "object",
  "additionalProperties": true,
  "required": ["$schema","schema_version","slice_id","release","verdict","pins"],
  "properties": {
    "$schema": {"type":"string","const":"https://baton.sawy3r.net/schemas/review-v1.json"},
    "schema_version": {"type":"integer","const":1},
    "slice_id": {"type":"string","minLength":1},
    "release": {"type":"string","minLength":1},
    "verdict": {"type":"string","enum":["proceed","implementer_fix","needs_coach","blocked"]},
    "design_commit": {"type":"string"},
    "reviewed_at": {"type":"string","format":"date-time"},
    "reviewer": {"type":"string"},
    "cost_usd": {"type":"number"},
    "blocked_reason": {"type":"string"},
    "acknowledgement_reply": {"type":"string"},
    "flags": {"type":"array","items":{"type":"string"}},
    "untracked_findings": {"type":"array","items":{"type":"string"}},
    "pins": {"type":"array","items":{"type":"object","additionalProperties":true,"required":["number","tag","summary"],"properties":{"number":{"type":"integer","minimum":1},"tag":{"type":"string","enum":["mechanical","memory-cited","escalate"]},"section":{"type":"string"},"summary":{"type":"string","minLength":1},"observation":{"type":"string"},"action":{"type":"string"},"citation":{"type":"string"},"critical":{"type":"boolean"}},"allOf":[{"if":{"properties":{"tag":{"const":"memory-cited"}},"required":["tag"]},"then":{"required":["citation"]}}]}}
  },
  "allOf": [
    {"if":{"properties":{"verdict":{"const":"proceed"}},"required":["verdict"]},"then":{"required":["acknowledgement_reply"],"properties":{"pins":{"not":{"contains":{"type":"object","required":["tag"],"properties":{"tag":{"const":"escalate"}}}}}}}},
    {"if":{"properties":{"verdict":{"const":"blocked"}},"required":["verdict"]},"then":{"required":["blocked_reason"]}}
  ]
}
```

**Open questions:** (a) strict-mode projection; (b) **no** echoed scalar count — the
array is the source of truth (re-introducing a count is the #34 footgun); (c) first
baton schema to use `if/then`+`contains` — confirm the Go validator supports
draft-2020-12 conditionals or they silently no-op (fail-open); (d) protocol vs
Sworn; (e) count is the hard gate, `verdict` the human-routing label, invariant ties
them; (f) review.md becomes `render(review.json)` — preserve the ack-reply delimiters
drivers grep for; (g) stable pin ids (`P-01`) vs positional `number`.

---

### 3.3 `verifier-verdict-v1` — the verifier verdict

**Producer → Consumer:** agentic Rule-7 verifier / interpreter
(`internal/verify`, `orchestrator.Interpret`) → status.json verification block +
loop triage (`orchestrator.Decide`, oracle/router); rendered for human review.

**Purpose:** Close the **live ADR-0009 breach** — `parseInterpretResult`'s
`HasPrefix` scan of model prose. The fail-closed merge gate keys off a typed `verdict`
enum; `violations` becomes the slice-status object shape; an invalid emission is
treated as `INCONCLUSIVE` (fail-closed), never scraped.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://baton.sawy3r.net/schemas/verifier-verdict-v1.json",
  "title": "Baton Verifier Verdict",
  "type": "object",
  "additionalProperties": false,
  "required": ["schema_version","slice_id","release","verdict","rationale"],
  "properties": {
    "$schema": {"type":"string","const":"https://baton.sawy3r.net/schemas/verifier-verdict-v1.json"},
    "schema_version": {"type":"integer","const":1},
    "slice_id": {"type":"string","minLength":1},
    "release": {"type":"string","minLength":1},
    "verdict": {"type":"string","enum":["PASS","FAIL","BLOCKED","INCONCLUSIVE"]},
    "rationale": {"type":"string","minLength":1},
    "failed_gate": {"type":"string"},
    "violations": {"type":"array","items":{"type":"object","additionalProperties":false,"required":["gate","description"],"properties":{"gate":{"type":"string","minLength":1},"description":{"type":"string","minLength":1},"evidence":{"type":"string"},"proposed_amendment":{"type":"string"}}}},
    "routing": {"type":"string","enum":["needs_planner","needs_human","needs_implementer"]},
    "model_id_confirmed": {"type":"string"},
    "verifier_was_fresh_context": {"type":"boolean"},
    "verifier_session_id": {"type":"string"},
    "verdict_at": {"type":"string","format":"date-time"},
    "structured_output_mode": {"type":"string","enum":["json_schema","tool_call","text_fallback"]},
    "cost_usd": {"type":"number"},
    "input_tokens": {"type":"integer"},
    "output_tokens": {"type":"integer"},
    "duration_ms": {"type":"integer"}
  },
  "allOf": [{"if":{"properties":{"verdict":{"enum":["FAIL","BLOCKED"]}}},"then":{"required":["violations"],"properties":{"violations":{"minItems":1}}}}]
}
```

**Open questions:** (a) `additionalProperties:false` divergence (this is an emission
target); (b) **two views** — a model-emitted subset handed to `response_format` vs
this fuller persisted record the binary completes with harness telemetry (cost,
tokens, duration, session id, fresh-context, verdict_at are *harness-attached*, not
model-authored); (c) **case + enum mismatch** — this uses UPPERCASE incl.
`INCONCLUSIVE`; slice-status `verification.result` is lowercase `[pending,pass,fail,
blocked,null]` with no inconclusive — lowercase on merge and decide where
INCONCLUSIVE lands (recommend a re-verify path that never persists a result, or
extend the leaf enum); (d) migrate `state.Verification.Violations` to objects;
(e) **`CapStructuredOutput` does not exist yet** (`model/client.go` has only
`CapVerify`/`CapChat`) — hard dependency; (f) should a `text_fallback` PASS be
treated as not-yet-trusted (re-verify/PAGE)?; (g) binary stamps the identity triple
post-emission so the model payload is judgement-only.

---

### 3.4 `orchestrator-decision-v1` — routing / triage decisions

**Producer → Consumer:** router (`router.Route` → `router.Decision`) and triage
(`orchestrator.Decide` → `Output`) → the worker (`scheduler/worker.go` dispatch
switch) **and** the decision-log (`supervisor.RecordDecision/RecordTriage` →
`decisions` table). See §5 for the recommended envelope refinement.

**Purpose:** One validated record is (a) the structured-output target the router
emits, (b) the dispatch instruction the worker consumes, and (c) the durable audit
row — so a routing decision is auditable, replayable, and portable across model A/B.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://baton.sawy3r.net/schemas/orchestrator-decision-v1.json",
  "title": "Baton Orchestrator Decision",
  "type": "object",
  "additionalProperties": false,
  "required": ["$schema","schema_version","release","slice_id","role","action","reason"],
  "properties": {
    "$schema": {"type":"string","const":"https://baton.sawy3r.net/schemas/orchestrator-decision-v1.json"},
    "schema_version": {"type":"integer","const":1},
    "release": {"type":"string","minLength":1},
    "slice_id": {"type":"string","minLength":1},
    "track": {"type":["string","null"]},
    "role": {"type":"string","enum":["router","triage","orchestrator","coach"]},
    "action": {"type":"string","enum":["implement","review","verify","redesign","merge-track","merge-release","replan-release","coach_decision","none","resolve_in_place","escalate_model","halt"]},
    "reason": {"type":"string","minLength":1},
    "command": {"type":["string","null"]},
    "target_slice": {"type":["string","null"]},
    "target_track": {"type":["string","null"]},
    "disposition": {"type":["string","null"],"enum":["dispatch","pause","terminal",null]},
    "violations": {"type":["array","null"],"items":{"type":"string"}},
    "recorded_at": {"type":["string","null"],"format":"date-time"}
  },
  "allOf": [{"if":{"properties":{"role":{"const":"triage"}}},"then":{"properties":{"action":{"enum":["resolve_in_place","escalate_model","halt"]}}}}]
}
```

**Open questions:** (a) unify (role discriminator, chosen) vs split router/triage;
(b) strict vs permissive `additionalProperties`; (c) encode role→action coupling in
schema (`if/then`) or Go; (d) does ADR-0009 mean even the *deterministic* router
output round-trips through validate-on-write?; (e) coach messages as `role:coach`
records (→ §6); (f) `disposition` derived vs emitted; (g) `command` is Sworn-specific
— protocol surface is `(action+targets)`; (h) generate the `action` enum from the Go
consts to prevent drift.

> §5 argues this should be refined into an `orchestrator-event-v1` *envelope*
> (discriminated `kind: route|triage|escalation|lifecycle`) plus a separate
> `verdict-v1`. The flat record above is the minimum; the envelope is the target.

---

### 3.5 `coach-call-v1` — orchestrator → Coach escalation

See **§6** for the full interaction model. The orchestrator holds **no authority**;
it emits a coach-call when it must escalate. Replaces the free-text
`page_coach <title> <body>` string and its keyword re-scrape.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://baton.sawy3r.net/schemas/coach-call-v1.json",
  "title": "Baton Coach Call",
  "type": "object",
  "additionalProperties": true,
  "required": ["$schema","schema_version","call_id","created_at","release","decision_kind","title","allowed_actions"],
  "properties": {
    "$schema": {"type":"string","const":"https://baton.sawy3r.net/schemas/coach-call-v1.json"},
    "schema_version": {"type":"integer","const":1},
    "call_id": {"type":"string","format":"uuid","description":"Correlation key. coach-response.in_reply_to MUST equal this."},
    "created_at": {"type":"string","format":"date-time"},
    "release": {"type":"string","minLength":1},
    "slice_id": {"type":"string"},
    "track_id": {"type":"string"},
    "decision_kind": {"type":"string","enum":["design_review","verification_failed","blocked","stuck","merge_ready","route_failed","generic"]},
    "title": {"type":"string","minLength":1},
    "body": {"type":"string"},
    "render": {"type":"object","additionalProperties":true,"description":"Adapter render hints (proof refs, suggested-ack text, links). Never machine-parsed."},
    "allowed_actions": {"type":"array","minItems":1,"uniqueItems":true,"items":{"type":"string","enum":["approve","reject","note","defer","replan","proceed"]}},
    "requires_note": {"type":"array","items":{"type":"string","enum":["approve","reject","note","defer","replan","proceed"]}},
    "timeout": {"type":"object","additionalProperties":false,"required":["seconds","on_timeout"],"properties":{"seconds":{"type":"integer","minimum":0},"on_timeout":{"type":"string","enum":["none","default_action","repage","escalate"]},"default_action":{"type":"string","enum":["approve","reject","note","defer","replan","proceed"]}}},
    "origin": {"type":"object","properties":{"instance_id":{"type":"string"},"host":{"type":"string"}}}
  }
}
```

---

### 3.6 `coach-response-v1` — Coach gesture → JSON

See **§6**. A channel adapter (ntfy/telegram/slack/web/cli/mcp) **echoes a human
gesture** into JSON — the human never authors it. Replaces `approved-ack.md`
file-presence + verb parsing.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://baton.sawy3r.net/schemas/coach-response-v1.json",
  "title": "Baton Coach Response",
  "type": "object",
  "additionalProperties": true,
  "required": ["$schema","schema_version","response_id","in_reply_to","action","responder","responded_at","idempotency_key"],
  "properties": {
    "$schema": {"type":"string","const":"https://baton.sawy3r.net/schemas/coach-response-v1.json"},
    "schema_version": {"type":"integer","const":1},
    "response_id": {"type":"string","format":"uuid"},
    "in_reply_to": {"type":"string","format":"uuid","description":"MUST equal the coach-call call_id."},
    "action": {"type":"string","enum":["approve","reject","note","defer","replan","proceed"]},
    "note": {"type":"string","description":"Human prose, wrapped by the adapter. Required when action is in the call's requires_note."},
    "responder": {"type":"object","required":["id","via","auth"],"properties":{"id":{"type":"string"},"via":{"type":"string","enum":["cli","ntfy","telegram","slack","web","mcp"]},"auth":{"type":"string","enum":["local_shell","topic_secret","chat_allowlist","signing_secret","session"]}}},
    "responded_at": {"type":"string","format":"date-time"},
    "idempotency_key": {"type":"string","minLength":1},
    "channel_ref": {"type":"object","additionalProperties":true}
  }
}
```

**Open questions (3.5 + 3.6):** strict `additionalProperties` for the emit target;
`response_options`/`severity` model-emitted vs renderer-derived from
`decision_kind`/`call_type`; richer `pin_verdicts[]` for per-pin reject (v2); loop
control (pause/resume/kick) bundled vs split; how `in_reply_to` is minted + the
open-prompt registry for no-replay; responder auth binding; shared-envelope `$defs`
across the message family.

### 3.7 `effort_complexity` — a slice field on `spec-v1` (not a new schema)

A per-slice rating on **two separate axes** (keeping them apart is the point —
relative "story points" conflate them and lose the signal). Set by the **planner**
during decomposition; **confirmed or revised by the implementer** (best-placed,
about to do the work). A field on `spec-v1`, mirrored confirmed in `status.json`.

**Researched base (kept light):**
- **Complexity → Cynefin** (Snowden): clear → complicated → complex, collapsed to
  low/high — Clear+Complicated = `low` (best/good practice applies), Complex =
  `high` (unknown-unknowns; emergent). "How hard to get *right*."
- **Effort → relative/T-shirt sizing** (S/M/L): touchpoints, LOC, time. "How *much*."

**Quadrants → loop behaviour (the value is the routing, not the label):**

| | low complexity | high complexity |
|---|---|---|
| **high effort** | **Grind** — predictable; batchable; cheaper model ok; watch timeout budget | **Epic** — **decompose gate**: fail closed → planner re-slices (enforces Rule-6 one-vertical-slice) |
| **low effort** | **Chore** — fast path; cheap model; light verify | **Puzzle** — strong model + extra adversarial verify; likely Type-1 → Coach |

**Routing semantics:** complexity → model choice + verification rigor; effort →
timeout/retry budget; complexity is a strong prior for a **Rule 9 Type-1** check
(≈ blast-radius/uncertainty).

**Schema sketch (added to `spec-v1`):**
```json
"effort_complexity": {
  "type": "object",
  "required": ["effort", "complexity", "quadrant"],
  "properties": {
    "effort":     { "enum": ["low", "high"] },
    "complexity": { "enum": ["low", "high"] },
    "quadrant":   { "enum": ["chore", "grind", "puzzle", "epic"] },
    "rationale":  { "type": "string" },
    "confirmed_by_implementer": { "type": "boolean" }
  }
}
```
(A `low/med/high` 3-point scale is a natural v2; start with the 2×2.)

**Eval tie-in:** the **planned → implementer-confirmed → actual** delta is moat
data — calibrate planner estimates over time, and correlate complexity with
per-model fit and $/verified-slice (feeds model-routing / FinOps). Tracked: **#36**.

**Rating rubric (consistency guidance — lives in the planner + implementer role
prompts).** Subjectivity is unavoidable, so the rubric tames it four ways rather
than pretending to be absolute:

1. **Anchor to countable proxies, not vibes.**
   - *Effort* = `high` if **any**: planned touchpoints > ~3 files · est. LOC delta
     > ~150 · acceptance criteria > ~5 · new test cases > ~6. Else `low`.
     (Thresholds are project-tunable defaults, read from `considerations.md`.)
   - *Complexity* = `high` if **any** (Cynefin "complex" markers): no established
     pattern in the codebase/memory for this (novel) · touches concurrency / shared
     mutable state / cross-track coordination · security- or data-integrity-sensitive
     · irreversible or wide blast-radius (migration, schema/contract/public-API change)
     · carries ≥1 Rule-9 Type-1 decision · the spec's Risks name an unknown-unknown
     ("audit before picking", "needs investigation"). Else `low` (clear/complicated —
     an established pattern applies). It's a **checklist, not a gut feel.**

2. **Reference anchors = the "shared ruler" (your agile point, made operational).**
   Story points are only meaningful *relative to the team*; here the "team" is the
   **project**. Each project keeps **one exemplar slice per quadrant** (Chore /
   Grind / Puzzle / Epic) in `considerations.md`; raters calibrate against those,
   not an absolute scale. Ship 4 starter exemplars; the project's own verified
   slices replace them over time. This is what makes the rating *project-relative*
   instead of fictionally universal.

3. **Two raters catch drift.** Planner rates from the spec; implementer confirms
   from code reality. Divergence (planner "Chore" vs implementer "Puzzle") is
   recorded with a one-line reason — the planning-poker "discuss on disagreement"
   move — and is itself a signal the spec was thin (Rule 8).

4. **The eval loop is the real calibrator.** The planned→confirmed→actual delta,
   per project, tunes both the thresholds and the exemplars. The rubric is the
   seed; the project's own history is the ruler that tightens it — so consistency
   *grows* with the project rather than being legislated up front.

### 3.8 `telemetry-event-v1` — content-free eval metric (projection of the records)

Not a new role output — a **content-stripped projection** of the records the loop already
emits (Dispatch / `orchestrator-decision` / `verifier-verdict`), forwarded to the local
eval store and (opt-in) the hosted ingest. **One schema'd stream, three uses:**
decision-log, local eval, hosted eval.

**Transport stance (public-safe):** local-first (always written locally; standalone needs
no network), **opt-in** phone-home (config flag + account token, off by default),
**content-free allowlist** (metrics only; ids hashed; never source/spec/proof/raw-paths),
batched + async + idempotent, validated on emit and on ingest. Start-simple ingest:
HTTPS function → managed Postgres. (Data-tier/moat strategy: internal.)

**Schema sketch:**
```json
{
  "$id": "https://baton.sawy3r.net/schemas/telemetry-event-v1.json",
  "type": "object",
  "required": ["event_id", "ts", "role", "model", "provider", "verdict"],
  "additionalProperties": false,
  "properties": {
    "event_id":     { "type": "string", "description": "idempotency key" },
    "ts":           { "type": "string", "format": "date-time" },
    "project_hash": { "type": "string" }, "release_hash": { "type": "string" },
    "slice_hash":   { "type": "string" },
    "role":         { "enum": ["planner", "implementer", "captain", "verifier", "orchestrator", "merge"] },
    "model":        { "type": "string" }, "provider": { "type": "string" },
    "tokens_in":    { "type": "integer" }, "tokens_out": { "type": "integer" },
    "cost_usd":     { "type": "number", "description": "real, not nominal" },
    "duration_ms":  { "type": "integer" },
    "attempt":      { "type": "integer" },
    "verdict":      { "enum": ["pass", "fail", "blocked", "inconclusive", "n/a"] },
    "effort":       { "enum": ["low", "high"] }, "complexity": { "enum": ["low", "high"] },
    "quadrant":     { "enum": ["chore", "grind", "puzzle", "epic"] },
    "rework_count": { "type": "integer" },
    "gate":         { "type": "string" }, "gate_result": { "enum": ["pass", "fail", "skip"] }
  }
}
```
No source, spec, proof, or raw file names — ever. Full transport + moat architecture:
`sworn-internal/docs/strategy/2026-06-30-telemetry-eval-transport.md` (draft ADR-0012
extracts the public-safe transport stance when built). Closes the FT-7 gaps (duration,
real cost, token split, model-id bug, cross-run durability).

---

## 4. (folded into §3)

The six schemas above are the deliverable for "every role and layer." §5–§6 refine
two of them (orchestrator, coach) into their target shapes.

---

## 5. Orchestrator outputs & consumers — recommended schema set

The orchestrator emits **five semantically distinct messages**, not one: route
decision, triage action, interpreted verdict, escalation/page, lifecycle transition.
They share a who/when/which-slice/why envelope but diverge in payload and consumer —
and today land in three different persistence paths (`decisions` table, `pages`
table, BLOCKED/coach `reason` strings). Two stand out against ADR-0009: the
**interpreted verdict is the live prose-scrape** (`parseInterpretResult`), and
**route/triage are lossy** — targets, `model_idx`, `attempt`, and cost are dropped at
the flat `(role,action,reason)` boundary, so the decision-log shows *what* but cannot
reconstruct *why* (which model slot, which attempt, what it cost — the eval moat).

**Recommendation: not one flat record, not a sprawling family — one envelope with a
discriminated `kind`, plus a separate verdict schema.**

**A. `orchestrator-event-v1`** (envelope, discriminated on `kind`):

```
$id: https://baton.sawy3r.net/schemas/orchestrator-event-v1.json
required: [schema_version, release, kind, actor, emitted_at, reason]
common:  schema_version const 1; release; slice_id?; track_id?;
         actor const "orchestrator"; emitted_at date-time;
         kind enum [route, triage, escalation, lifecycle]; reason; cost_usd?
oneOf (by kind):
  route:      next_type enum[implement,review,verify,redesign,merge-track,
                             merge-release,replan-release,coach_decision,none],
              command, target_slice, target_track
  triage:     action enum[resolve_in_place,escalate_model,halt],
              verdict enum[PASS,FAIL,BLOCKED,INCONCLUSIVE],
              model_idx, attempt_on_model, escalation_len
  escalation: page_reason enum[max_turns,circuit_breaker,interpreter_inconclusive,
                              blocked_replan,design_review_gate,merge_gate],
              fingerprint, target enum[coach]
  lifecycle:  from_state, to_state enum[starting,running,paused,done,
                                        failed,merged,skipped]
```

Persist by widening the `decisions` table `role`→`kind` + payload columns (or a
`payload JSON` column); `RecordDecision`/`RecordTriage`/`RecordPage` collapse into one
`RecordEvent(envelope)`. This gives the Coach a **single typed inbox** instead of
three shapes (matching the "Oracle blind to BLOCKED" memory) and recovers the
dropped eval fields.

**B. `verdict-v1`** (separate role-output schema — *not* in the orchestrator family;
this is `verifier-verdict-v1` from §3.3). It is the verifier's verdict *normalized*
by the interpreter; the orchestrator emits *against* it, it does not own it. Shared by
the agentic verifier role and the interpreter, gated by `CapStructuredOutput`.

This is tracked-adjacent to ADR-0011; the envelope + `verdict-v1` are the
orchestrator's slice of it.

---

## 6. Coach call / response layer

An escalation is a **record exchange**, not a text scrape. The orchestrator emits
`coach-call-v1`; **exactly one** `coach-response-v1` resolves it, joined by `call_id`.

**Core model:**

- The call is **persisted** (`.sworn/calls/<call_id>.json`) *before* any push — so a
  late/after-restart response can be matched, validated against `allowed_actions`, and
  audited (Rule 3/4). Today correlation is implicit (slice id + `approved-ack.md`
  presence); `call_id` makes it explicit and survives concurrent / release-scoped
  calls.
- The **gesture is decoupled from the side effect.** One handler maps `approve` → its
  effect (write `approved-ack.md` / clear the BLOCK / advance state). Adapters never
  perform the effect; they only emit the response.
- **Fail closed.** No verified human gate auto-resolves on silence. Unvalidated
  responder or response → dropped + logged, never an optimistic approve.

**The six actions ↔ today's coach verbs:**

| Action | Meaning | Valid `decision_kind` | Today's verb / effect |
|---|---|---|---|
| `approve` | Greenlight the proposed thing | design_review | `coach ack` → `approved-ack.md` |
| `reject` | Send back with reason (note required) | design_review, verification_failed | `coach decline "<reason>"` → `decline.md` |
| `note` | Answer a paged worker; does **not** resolve the gate | blocked, stuck, generic | `coach note` (+`--resume`=`note!`) |
| `defer` | Snooze; slice stays parked, call re-raised | any | (new) — formalizes "ignore for now" |
| `replan` | Trigger release revision | blocked, route_failed | `/replan-release` |
| `proceed` | Greenlight a queued release action | merge_ready | the "Ready to /merge-release" page |

`allowed_actions` is set per `decision_kind` (e.g. `merge_ready` → `[proceed,defer]`;
`design_review` → `[approve,reject,note]`). The adapter renders **only** those.

**Channel-adapter contract.** Every adapter MUST: (1) **render** the call into
channel-native affordances derived strictly from `allowed_actions`; (2) **capture**
the human gesture and **map** it to exactly one `action` (+`note` where
`requires_note` demands); (3) **emit** a `coach-response-v1` that *validates before
transmission* — on failure, re-prompt, do not send; (4) **stamp** `responder.id`/
`responder.auth` from the channel's authenticated principal + a stable
`idempotency_key` from the channel callback id; (5) **echo** the orchestrator's result
back. The adapter is the *only* place a gesture becomes JSON; the human stays in taps
and prose.

| Channel | No-text actions | Text actions | `idempotency_key` | `auth` |
|---|---|---|---|---|
| **ntfy.sh** | `http` action buttons → POST verb to `<topic>-reply` (today's bridge) | `view` → web compose; bridge parses verb+text | ntfy message id | `topic_secret` |
| **Telegram** | inline keyboard, `callback_data=<action>:<call_id>` | `force_reply` → note | `callback_query.id` | `chat_allowlist` (`from.id`) |
| **Slack** | Block Kit buttons, `action_id=<action>`, `value=call_id` | modal via `trigger_id` | `action_ts`/view hash | `signing_secret` + allowlist |
| **CLI** | `coach approve\|proceed\|defer\|replan <id>` | `coach reject/note <id> "<text>"` | invocation hash | `local_shell` |
| **web / MCP** | form buttons / tool call | form field / tool arg | request id | `session` |

The existing `coach-ntfy-bridge.sh` **is** the ntfy adapter — its verb grammar
(`ack`/`decline`/`note`/`note!`/`resume`) becomes the gesture→action map; the only
change is it now emits a `coach-response-v1` keyed on `call_id` rather than firing a
verb blind.

**Idempotency:** the orchestrator keeps a per-`call_id` seen-set of
`idempotency_key`. **First valid response wins.** Duplicate tap = no-op re-echo; a
different action after resolution is rejected. Kills the double-tap and ntfy
stream-replay hazards the bridge guards against today.

**Timeout** (`timeout.on_timeout`): `none` (default for human gates — never
auto-resolves); `repage`; `escalate`; `default_action` (only for explicitly
low-stakes calls; **never** auto-`approve` a Type-1 gate).

---

## 7. The keystone slice

ADR-0009's authoring path, implemented in sworn. **One vertical slice** (Rule-6
bounded): wire the structured-output call end-to-end for **one** role emitter and
delete its parser, proving the pattern; the rest follow as sibling slices.

**Scope:**

1. **Driver capability — `CapStructuredOutput`.** Add the flag to
   `internal/model/client.go` (today only `CapVerify`/`CapChat`). A driver that
   advertises it gets `response_format: {type:"json_schema", json_schema:{...,
   strict:true}}`; one that does not gets the **tool-call-schema fallback** (a
   single tool whose parameters *are* the schema, `tool_choice` forced). Both paths
   validate the returned object against the embedded schema before the binary accepts
   it. This is the same foundation as the model-layer service refactor and the
   loop-verifier-goes-agentic work.

2. **Real draft-2020-12 validation.** Replace the hand-rolled `baton.Validate`
   subset checks with an actual evaluator over the embedded schemas (a justified dep
   per ADR-0007, or a vetted minimal evaluator). Until this lands, the published
   schemas remain decorative and every drift in §2 stays invisible. Add the four
   config schemas + the six new schemas to `embed.go` `SchemaMap`.

3. **Strict generation profiles.** For each schema used as an emit target, produce
   the strict variant (`additionalProperties:false`, all properties `required`,
   optionals as nullable type-unions) — either a second published artefact or a
   projection generated at call time from the lenient canonical.

4. **First role emitter + parser deletion.** Pick **`verifier-verdict-v1`** as the
   pilot (highest leverage — it closes the live breach): the interpreter/verifier
   emits against the schema; **delete `parseInterpretResult`'s `HasPrefix` scrape and
   `extractViolations`'s prose-splitting**. The merge gate keys off the typed
   `verdict` enum; an invalid emission = `INCONCLUSIVE` (fail-closed).

5. **baton-web publishing.** Serve the new schemas at their canonical `$id` so the
   `$schema` const is dereferenceable (model-A portability + a 404-free authoring
   path). Prerequisite, not afterthought.

**Supersedes:** this keystone **explicitly supersedes #32 and #34** — #34's
pin-miscount class is structurally eliminated once `review-v1`'s typed `pins[]`
replaces the scraped count, and #32's scanner work is subsumed by the
schema-constrained authoring path. Mark both closed-by ADR-0011.

**Reachability artefact (Rule 1):** end-to-end — a real verifier dispatch emits a
schema-validated verdict, the merge gate routes off the typed field, and a
deliberately malformed emission fails closed as INCONCLUSIVE (test + smoke step).
For the coach layer: ntfy-button → `coach-response-v1` JSON → router un-blocks a real
`design_review` slice (the inbound twin of the existing outbound page).

---

## 8. Sequencing + open decisions for the Coach

**Sequencing:**

1. **`CapStructuredOutput` + real draft-2020-12 validation** (the foundation —
   nothing else is trustworthy until the validator *is* the schema).
2. **`verifier-verdict-v1`** emitter + delete the interpreter prose-scrape (closes
   the live ADR-0009 breach; highest leverage).
3. **`review-v1`** + **`design-v1`** (delete `parsePins`/`hasSixSections`; kills #34).
4. **`orchestrator-event-v1`** envelope — fold `decisions`/`pages`/reason-strings
   into one `RecordEvent`; wire `cost_usd`/`model_idx`/`attempt` through (the eval
   moat — capture telemetry from day 1).
5. **`coach-call-v1` + `coach-response-v1`** + the ntfy adapter as the first
   channel echo.
6. **Reconcile the existing record schemas' field drift** (§2 findings) and
   **publish all schemas on baton-web.**

**Type-1 decisions — RATIFIED by Brad (Coach), 2026-06-30.** All six adopted as
recommended. **D3 resolved:** `design-v1` is the canonical writer of
`design_decisions` (produced during design, reviewed by the captain); `status.json`
carries a projection/reference. **D5 resolved:** adopt a justified draft-2020-12
evaluator dependency (ADR-0007 process) — confirmed by the "borrow the validator,
not the framework" call. These move into the keystone slice's `design_decisions`
when it is built.

(Original framing, for provenance — Rule 9: model may propose, may not record:)

- **D1. `additionalProperties` policy.** Strict-everywhere emit targets (false +
  all-required + nullable optionals) vs lenient canonical + generated strict
  projection. Affects every schema and whether the keystone works on OpenAI strict
  mode. *Recommend: lenient canonical for storage, strict projection at call time.*
- **D2. Protocol vs implementation surface** for `design-v1` / `review-v1` /
  `verifier-verdict-v1`. The captain-split memory says design-review is **Baton**
  (argues protocol → `baton.sawy3r.net` `$id` + Baton-repo issue treatment like
  board/spec/proof #45–#48). `orchestrator-event-v1` and the coach layer are Sworn
  Layer-2 (argues impl). Confirm before publishing each `$id`.
- **D3. Single source of truth for `design_decisions`** — design-v1 vs status.json.
  Pick one writer; the other projects.
- **D4. Verdict enum reconciliation** — add `inconclusive` to the slice-status leaf
  enum (and lowercase-on-merge) vs an INCONCLUSIVE→re-verify path that never persists.
- **D5. Dependency** — adopt a draft-2020-12 evaluator dep (ADR-0007 process) vs a
  minimal in-tree evaluator.
- **D6. Field-drift reconciliation direction** — for slice-status `need_ids` vs
  `covers_needs`, and `open_deferrals`/`violations` string-vs-object: migrate the Go
  types to the richer schema shape (recommended; aligns Rule-2/Rule-8) vs downgrade
  the schemas.

**Tracking:** this document is the substance for open **task #21** (ADR-0011 draft:
role/layer output-schema). The keystone slice supersedes **#32** and **#34**.
