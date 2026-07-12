---
title: "Baton handoff — capability policy + protocol updates from sworn dogfood 2026-07-12"
date: 2026-07-12
author: Brad (Coach) + Claude (sworn orchestrator)
target: a fresh Baton session (github.com/sawy3r/baton), PR-up governance
source_of_truth: docs/captures/2026-07-12-resume-recovery-dogfood-findings.md (sworn repo)
related_adr: sworn docs/adr/0013-capability-based-model-selection.md
---

# Baton handoff — protocol updates from the 2026-07-12 sworn dogfood

## 0. How to use this handoff

This is the first formal instance of the **sworn → Baton feedback loop** (see §5).
It is written to be loaded into a fresh Baton session with no other context. It
carries: (a) the empirical grounding — the 7 dogfood findings — so you can judge
each proposed change on evidence, not assertion; (b) the protocol-level changes
sworn is asking Baton to review and land in a new version; (c) concrete schema
sketches and rationale drafted for Baton's own docs. sworn implements against
whatever Baton ratifies and re-vendors on the VERSION-pin bump; nothing here is
imposed — Baton owns the contract (ADR-0010).

Governance: land as Baton issues/PRs labelled `dogfood-feedback`, each linking
back to the sworn capture. Baton triages into a version; sworn re-vendors.

## 1. Context: why this handoff exists

sworn is Baton's reference engine. On 2026-07-12 a resume-from-unclean-exit
dogfood (release `2026-07-11-contract-edge-gates`, driver grok-4.5 via OpenRouter)
surfaced 7 findings. Triage showed several are **not sworn bugs** — they are
feedback on the Baton *protocol* (rules, contracts, schemas). Burying protocol
feedback as engine bugs is exactly the drift this loop exists to prevent, so the
Coach directed that these go upstream, formally, with rationale.

The headline is new: a **capability-based model-selection** architecture (sworn
ADR-0013) whose front half — *what capabilities each role requires* — is a Baton
concern, because Baton owns the roles.

## 2. The 7 dogfood findings (full set, with ownership)

| # | Finding | Owner |
|---|---------|-------|
| 0 | `sworn loop --parallel --dry-run` silently ignores `--dry-run` and runs for real | engine → sworn |
| 1 | Resume does not reset the dirty worktree before re-dispatch — crash debris pollutes the retry | **protocol → Baton** |
| 2 | `openrouter`/`groq`/`mistral`/`cloudflare` omit structured-output config → capable models silently lose the structured gate | engine → sworn (motivates ADR-0013) |
| 3 | Cost re-derived from a hardcoded price table → un-tabled models record `$0` | engine → sworn (motivates ADR-0013) |
| 4 | max-turns cap too low for grind/beast slices on grok | engine → sworn |
| 5 | first-pass mock detector false-positives on annotation *string literals* in mock-parsing code | **protocol → Baton** (definition) + sworn (impl) |
| 6 | Autonomous loop does not halt at the design-review gate for a Coach; gate semantics undefined without a human | **protocol → Baton** |

Findings 0, 4 stay entirely in sworn. Findings 2, 3 stay in sworn but are the
*motivation* for the capability architecture (their durable fix is "read what the
provider publishes," not "hardcode per model"). Findings 1, 5, 6 and the new
capability policy are the **Baton actionables** below.

## 3. Baton actionables

### 3.1 NEW SCHEMA — `capability-policy-v1` (the headline)

**What.** A Baton schema declaring (a) the **capability taxonomy** — the shared
vocabulary of model capabilities — and (b) each **role's required capabilities**.
This is the hard-eligibility contract for model selection: a model may serve a
role only if it provides every capability the role requires.

**Why (rationale for Baton docs).** Baton already owns the role ontology
(planner/implementer/verifier/captain) and, via ADR-0012 on the sworn side, the
notion that "capability IS a declared set." But role capability *requirements* are
currently implicit — encoded as model pins in each operator's config (e.g.
"verifier = opus, for 1M context"). That is unportable and stale: it names a model
where it means a capability, and it breaks every time the frontier moves. Making
role→capability requirements an explicit, model-agnostic Baton contract means:
any Baton-conformant engine gates eligibility identically; the requirement is
auditable ("a verifier requires large-context because Rule 7 asks it to reason
over the full diff + tests + gates"); and new models become eligible with no
protocol or config change. It is the front half of the golden thread —
need → role → required-capability → eligible-model — that per-slice verification
(Rule 7) cannot see.

**Proposed shape (sketch — Baton to finalise):**
```json
{
  "$id": "capability-policy-v1",
  "taxonomy": {
    "structured_output": { "kind": "boolean" },
    "json_schema_strict": { "kind": "boolean" },
    "tool_calling":      { "kind": "boolean" },
    "vision":            { "kind": "boolean" },
    "agentic_multiturn": { "kind": "boolean" },
    "context_window":    { "kind": "quantitative", "unit": "tokens" },
    "reasoning_effort":  { "kind": "ordinal", "levels": ["low","medium","high"] }
  },
  "roles": {
    "implementer": { "requires": ["tool_calling", "agentic_multiturn",
                                   {"context_window": ">=200000"}] },
    "verifier":    { "requires": ["structured_output",
                                   {"context_window": ">=200000"},
                                   {"reasoning_effort": ">=high"}] },
    "captain":     { "requires": ["structured_output"] },
    "planner":     { "requires": ["structured_output"] }
  }
}
```
The taxonomy is the interop contract: Baton says a role *requires* a token; the
engine's registry says a model *provides* the same token; eligibility is the
intersection. Requirement thresholds (context, reasoning) are part of the policy.
**Non-goal:** Baton does NOT rank or pick models (that is sworn's eval-based
routing-preferences, deliberately kept engine-side). Baton only declares the hard
requirement.

**sworn side (for context, not Baton's work):** a provider registry maps each
provider's metadata (e.g. OpenRouter `/models` `supported_parameters`) onto this
taxonomy; eval-based routing picks among the eligible; eval is a soft prior, never
a gate (explore/exploit). See sworn ADR-0013.

### 3.2 Rule 11 (process-global mutation / multi-worktree resilience) — resume-reset contract

**What.** Strengthen the resilience contract: **a resumed loop must restore each
track worktree to its committed slice state (`reset --hard` + `clean`) before
re-dispatching.** Today resume re-runs on top of whatever a crashed run left
uncommitted.

**Why (rationale for Baton docs).** Finding 1: after an unclean exit, the two track
worktrees held uncommitted implementer output; on resume the loop re-dispatched on
top of it, and the leftover code's content contaminated the new attempt's diff
(Finding 5's false-positive fired on the *leftover* code). The coordinator
correctly avoided re-bootstrapping and the board stayed intact — so "recovers
without corrupting" holds — but "recovers *cleanly*" does not, because crash debris
survives into the retry. Rule 11 already governs process-global mutation and target
assertions; a resumed unit of work restoring to committed state before acting is
the same fail-closed principle applied to the worktree. This belongs in the
protocol because any Baton engine running concurrent worktrees inherits the hazard.

### 3.3 Rule 9 (design fidelity) — autonomous-mode gate semantics

**What.** Define what the design-review gate does when there is **no human Coach in
the loop** (autonomous `sworn loop`). Options to specify: auto-proceed (recorded),
self-review by a captain role, or hard-pause for async Coach acknowledgement.

**Why (rationale for Baton docs).** Finding 6: the router logged "the Design TL;DR
gate (Step 4) will halt for Coach review before any code lands," but the autonomous
loop generated the design TL;DR and proceeded straight to implementing — the
structured captain dispatch that would have gated it deferred out (Finding 2). So
the gate's *human-in-the-loop* semantics are well-defined but its *autonomous*
semantics are not. Rule 9 keeps design human-owned and stakes-calibrated; it should
state explicitly how a Type-1/architecturally-significant design choice is handled
when no human is present at dispatch time (e.g. hard-pause + page; never
auto-proceed on Type-1). Without this, "autonomous" silently downgrades design
review to a no-op — the exact fidelity gap Rule 9 exists to close.

### 3.4 Rule 10 (mock parity) — "mock" must be a code construct, not a string match

**What.** Refine the mock/boundary definition so a detector excludes **string
literals and comments** that merely *mention* the annotations (`// @no-mock`,
`// @mock-boundary`) from counting as real mocks.

**Why (rationale for Baton docs).** Finding 5: the S03 assemble slice — whose job
is to *parse* boundary/mock annotations for Rule 10 — contains string literals like
`ent := "// @no-mock\n// @mock-boundary (boundary: entitlement)"`. The first-pass
detector matched those literals and failed the slice closed. The boundary-mock
detector tripped on the boundary-mock-*parsing* code. The contract should specify
that a "mock at a boundary" is a code construct (a call/binding that substitutes
the boundary), detected against code tokens (AST or non-string/non-comment spans),
not raw text. This keeps the gate fail-closed on real mocks while not penalising
code that legitimately handles the annotation vocabulary.

### 3.5 Rule 8/9 gate contract — behaviour when a capability is genuinely absent

**What.** Specify what a structured-output-dependent gate does when the selected
driver *genuinely* cannot produce structured output: degrade to a gated prose pass,
or record a Rule-2 deferral (capability-absent)?

**Why (rationale for Baton docs).** Finding 2 was a config omission (fixed by
ADR-0013's capability matching — a non-structured model simply isn't eligible for a
role that requires it). But the residual protocol question stands: if capability
policy still routes a gate to a model that cannot do structured output (e.g. an
operator override), the gate must have defined behaviour. Today it records a Rule-2
deferral silently. The contract should state whether that is acceptable or whether
the gate must fall back to a still-gating prose evaluation. Tie-in: with §3.1 in
place, this becomes an edge case (override only), but it should be defined, not
incidental.

## 4. What sworn does on its side (not Baton's work — for coordination)

- ADR-0013 ratified (Coach) → build the provider registry (proactive metadata +
  cache under `~/.config/sworn/`, reactive fallback), the taxonomy mapping layer,
  and eval-based routing-preferences (extending `config.OptimizeMode`/`PassRateFloor`).
- Fix Findings 0, 2, 3, 4 as engine work (`loop-hardening` release); 2 and 3
  collapse into "consume the registry."
- Re-vendor `capability-policy-v1` (and any Rule 9/10/11 text changes) on the Baton
  VERSION-pin bump; implement eligibility = requirements ∩ registry as the hard gate.

## 5. The feedback loop this instantiates (proposal for a Baton CONTRIBUTING note)

Formalise the path so it is repeatable, not ad hoc:
1. **Triage** — every sworn dogfood/session finding is tagged `protocol` or
   `engine` in its capture doc.
2. **Route** — `engine` → sworn issues; `protocol` → Baton issues labelled
   `dogfood-feedback`, linking the sworn capture (this handoff is the template).
3. **Durability** — the process itself is stable reference: a sworn ADR + a Baton
   CONTRIBUTING/feedback note, so the hand-off is a formal loop, not tribal memory.

Baton may wish to add a short "Feedback from reference engines" section to its
docs describing how such findings are received and versioned.
