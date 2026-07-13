---
title: Capability Policy — model-agnostic role eligibility
description: Baton declares what capabilities each role requires; a conformant engine gates model eligibility as the intersection of the role's requirements and the model's provided capabilities. The hard requirement is Baton's; ranking among the eligible is the engine's.
---

# Capability Policy

Baton owns the role ontology — planner, implementer, verifier, captain. Which **capabilities** each role requires is therefore a Baton contract, not an operator's config detail. This doc defines that contract; the schema is `capability-policy-v1` (`https://baton.sawy3r.net/schemas/capability-policy-v1.json`).

## The problem it fixes

Role→model requirements are usually implicit — encoded as **model pins** in each operator's config: "verifier = opus, for the large context." That is unportable and stale by construction: it names a *model* where it means a *capability*, and it breaks every time the frontier moves. A pin cannot be audited ("why opus?"), cannot be satisfied by a newer model without a config change, and differs silently between engines.

Making role→capability requirements an explicit, model-agnostic Baton contract means: any Baton-conformant engine gates eligibility identically; the requirement is auditable against the rule that motivates it; and new models become eligible with **no protocol or config change**. It is the front half of the golden thread — *need → role → required-capability → eligible-model* — that per-slice verification (Rule 7) structurally cannot see.

## The contract: two halves, one vocabulary

1. **The taxonomy** — the shared capability vocabulary. Each entry is a token with a `kind`:
   - `boolean` — present or not (e.g. `tool_calling`, `structured_output`).
   - `quantitative` — a magnitude compared numerically, with a `unit` (e.g. `context_window` in tokens).
   - `ordinal` — a named level compared by its index in `levels` (e.g. `reasoning_effort: [low, medium, high]`).
2. **The role requirements** — for each role, the capabilities it requires. A bare token requires a boolean be true; an object requires a threshold (`{"context_window": ">=200000"}`, `{"reasoning_effort": ">=high"}`).

The taxonomy is the **interop contract**: Baton says a role *requires* a token; the engine's provider registry says a model *provides* the same token; **eligibility is the intersection**. A model may serve a role only if it provides every capability the role requires.

## Example

```json
{
  "$schema": "https://baton.sawy3r.net/schemas/capability-policy-v1.json",
  "version": 1,
  "taxonomy": {
    "structured_output": { "kind": "boolean" },
    "tool_calling":      { "kind": "boolean" },
    "agentic_multiturn": { "kind": "boolean" },
    "context_window":    { "kind": "quantitative", "unit": "tokens" },
    "reasoning_effort":  { "kind": "ordinal", "levels": ["low", "medium", "high"] }
  },
  "roles": {
    "planner":     { "requires": ["structured_output"] },
    "captain":     { "requires": ["structured_output"] },
    "implementer": { "requires": ["tool_calling", "agentic_multiturn", {"context_window": ">=200000"}],
                     "rationale": "builds against the full slice surface across many turns" },
    "verifier":    { "requires": ["structured_output", {"context_window": ">=200000"}, {"reasoning_effort": ">=high"}],
                     "rationale": "Rule 7 reasons over the full diff + tests + gates in one fresh context" }
  }
}
```

The thresholds above are a **starter policy**. The specific requirements — which capabilities, which thresholds — are the operator/Coach's calibrated call, tuned to the project's models and rules; the `rationale` field ties each requirement back to the rule that justifies it, so a reviewer can audit "why does the verifier require large context?" against Rule 7.

## What Baton does NOT do

Baton declares the **hard requirement only**. It does not rank, score, or pick among the eligible models — that is the engine's **eval-based routing**, a soft prior over the eligible set (explore/exploit), deliberately kept engine-side. Baton draws the eligibility line; the engine chooses within it. A reference engine (sworn, ADR-0013) maps each provider's published metadata (e.g. an OpenRouter `/models` `supported_parameters` list) onto the taxonomy tokens, then routes by eval among the eligible; eval is never itself a gate.

## Capability genuinely absent (the override edge case)

With a capability policy in force, a model that cannot produce structured output is simply **not eligible** for a role that requires it — the failure never reaches the gate. The residual question is the **operator-override** path: if a gate that depends on a capability is nonetheless routed to a model lacking it, the gate must have **defined**, not incidental, behaviour. The policy declares it via `on_capability_absent`:

- `prose_fallback_gated` — the gate must still return a verdict via a prose evaluation that **still gates** (it may not silently skip). Weaker signal, but a signal.
- `rule2_deferral` — the gate records a Rule-2 capability-absent deferral (why + tracking + acknowledgement), never a silent pass.

The one thing the contract forbids is the status quo the dogfood surfaced: a capability-dependent gate that **silently degrades to a no-op** when its capability is missing. Absence of the field is the engine default; declaring it makes the behaviour auditable.

## Provenance

Introduced from the first sworn → Baton dogfood-feedback loop (2026-07-12, findings 2/3 → sworn ADR-0013). The engine findings — providers omitting structured-output config, cost re-derived from a hardcoded table — share one durable fix: *read what the provider publishes and match it against a declared requirement*, not *hardcode per model*. The front half of that fix (what each role requires) is this Baton contract; the back half (what each model provides, and picking among the eligible) is the engine's registry and router.
