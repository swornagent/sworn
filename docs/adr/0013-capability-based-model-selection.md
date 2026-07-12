# ADR 0013: Capability-based model selection

## Status

**Accepted (2026-07-12) — ratified by the Coach (Brad).** Drafted by the
orchestrator during the `2026-07-12` resume-recovery dogfood session and ratified
by the Coach the same day (the model proposes, the human ratifies — Baton Rule 9).
**Type-1 (architecturally significant, hard to reverse):** this sets the seam every
model-selection decision crosses — the router, every gate that needs a model
capability, and the config surface all resolve against it. Changing its shape
after consumers implement means unwinding several implementations, not one.

Grounded in live evidence from the 2026-07-12 dogfood (see
`docs/captures/2026-07-12-resume-recovery-dogfood-findings.md`, Findings 2 and 3).

## Context

`sworn` today constrains model use by **specific model identity**, in two stale-prone ways:

- **Capability by hardcoded proxy.** ADR-0012 gave drivers a declared `RoleSet`
  (coarse capability: "I can serve implementer/verifier/captain"), but finer
  capabilities are set per-provider as static fields. The `openrouter`, `groq`,
  `mistral`, and `cloudflare` providers omit the structured-output field entirely,
  so any capable model routed through them silently loses structured gating —
  the dogfood hit this as Finding 2 (`driver for "x-ai/grok-4.5" does not support
  structured output` — a *config omission*, not a model limitation; grok, `*OAI`,
  and the native `xai/` provider all support it).
- **Cost by hardcoded table.** Cost is re-derived as `tokens × an in-binary price
  table` (`oai.go:137` comment: "gets a zero cost. Expand as needed"). Any
  un-tabled model records `$0` / `CostSource=unknown` — Finding 3. The served id
  (`x-ai/grok-4.5-20260708`) differs from the requested alias, so even adding a
  row wouldn't reliably match.

Both are the same anti-pattern: **hardcoding, per model id, a fact the provider
already publishes.** In a market where new models ship weekly, any model-pinned
or table-keyed constraint is stale on arrival, and the maintenance is a
never-ending code-edit-plus-release treadmill. Worse, a system whose *value* is
choosing the right model is structurally one step behind the frontier.

## Decision

**Constrain on capabilities, not models. Rank on eval, never gate on it.**

### 1. Two signals, structurally distinct — never conflated

- **Capabilities = eligibility.** A HARD, forward-looking, binary gate. The only
  thing that makes a model *not a candidate* is a capability mismatch (or an
  explicit human denylist/override). A model released today is eligible the moment
  the registry sees it.
- **Eval = preference.** A SOFT, backward-looking, continuous rank over the
  *eligible* set by past per-role performance. It informs and weights; it
  **never excludes**. Treating eval as a gate causes rich-get-richer lock-in — a
  new model has no track record → is never selected → never earns one → the
  router freezes to whatever won early ("eval-driven" silently becomes
  "eval-frozen"). Eval is therefore a **prior with uncertainty** (explore/exploit):
  established models exploited; new eligible models get exploratory traffic
  (low-stakes quadrants first) to earn their data — optimism-under-uncertainty,
  not "no history = don't use."

### 2. Two schemas, split by ownership (the Baton/sworn seam)

- **Capability policy → Baton (`capability-policy-v1`).** The capability
  *taxonomy* plus each role's *required capabilities* ("a verifier requires
  {structured-output, context≥200k, high-reasoning}"). Model-agnostic,
  eval-agnostic, portable, auditable. It belongs to Baton because Baton owns the
  roles (ADR-0010: Baton is pure spec; sworn implements). The taxonomy is the
  **interop contract** — the shared vocabulary both sides must speak — so it is
  Baton-owned.
- **Routing preferences → sworn (`config.json` / routing-preferences).**
  eval-based ranking + cost/velocity + explore/exploit budget. This is the
  engine's value-add. It is **already seeded in code**: `config.Config.OptimizeMode`
  (quality/cost/balanced) and `PassRateFloor` are routing preferences and are
  already sworn-side.

### 3. Provider registry — sworn's mapping layer

sworn syncs each provider's model metadata into a per-model
`{capabilities, pricing}` cache under `~/.config/sworn/`, refreshed on a policy
(TTL / `sworn models sync` / refresh-on-miss). **Proactive-first**: OpenRouter's
`/api/v1/models` returns `supported_parameters` AND `pricing` for the whole
catalog in one call. A **reactive** attempt-and-degrade path (try strict
`json_schema` response_format; on the unsupported-parameter error, fall back to
forced-tool-call; cache the result) is the fallback for sparse-metadata providers.
The registry **translates each provider's idiosyncratic metadata onto the Baton
taxonomy**, so providers stay pluggable without leaking their quirks into the
contract.

### 4. The eligibility → routing pipeline (with ownership)

```
Baton  : role → required capabilities            (capability-policy-v1)
   ↓
sworn  : models → provided capabilities           (registry maps provider metadata → taxonomy)
   ↓
         eligibility = requirements ∩ provided     (HARD gate; sworn enforces Baton's contract)
   ↓
sworn  : route by eval + cost/velocity + explore   (routing-preferences — the moat)
   ↓
         dispatch
```

The audit trail reads: **"eligible *because* capabilities, chosen *because*
eval-score + exploration budget"** — adaptive AND auditable, the opposite of a
frozen benchmark-max snapshot.

## Consequences

**Positive**
- New models are eligible the instant the registry sees them — zero config edits,
  no release. Kills the static-table treadmill.
- Findings 2 and 3 stop being one-off patches and become the *first two consumers*
  of one registry (capability discovery and price discovery are the same query).
- The router (sworn's positioned moat — eval-derived auditable routing) gets its
  input filter: capability-match first, eval/cost route within. Capabilities are
  the audit trail for "why was this model even a candidate."
- Model pins survive only as an explicit override/escape hatch — legible, not the
  default.

**Costs / risks**
- Requires a capability taxonomy (Baton), a registry + cache + refresh policy
  (sworn), and a mapping layer per provider. Provider metadata is heterogeneous —
  the reactive fallback is mandatory, not optional.
- The exploration budget deliberately spends some dispatches on unproven models;
  it must be bounded and quadrant-aware (cheap/low-stakes first) so exploration
  never risks a high-stakes slice.
- Cache staleness is a new failure mode: without refresh, "proactive" decays to
  "static, fetched once."

**Relationships**
- Extends **ADR-0012** (coarse `RoleSet` → fine-grained per-capability matching)
  and honours **ADR-0010** (capability policy is spec → Baton; matching + registry
  + routing are engine → sworn).
- `capability-policy-v1` is a **Baton upstream contribution** and rides the
  formalised sworn→Baton feedback loop (see the 2026-07-12 Baton handoff).

## Alternatives considered

- **Static per-provider capability field** (set `Structured` on the 5 missing
  providers). Rejected: a smaller static table with the identical staleness
  problem — re-opens the silent gap the next time a backend changes.
- **Reactive-only capability discovery.** Rejected as *primary*: learns one model
  at a time, only after a live failure; kept as the *fallback* for sparse-metadata
  providers.
- **Eval as an eligibility gate.** Rejected: rich-get-richer freeze — the router
  ossifies around early winners and never adopts the frontier.
