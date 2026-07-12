---
title: "Dogfood findings — resume from unclean exit (contract-edge-gates, grok-4.5/OpenRouter)"
date: 2026-07-12
author: Claude (orchestrator) + Brad (Coach)
release: 2026-07-11-contract-edge-gates
driver: openrouter/x-ai/grok-4.5 (implementer + verifier), SWORN_DIRECT=1
---

# Resume-recovery dogfood — 2026-07-12

## Scope

An accidental `sworn loop --parallel --dry-run` (see Finding 0) launched a **real**
parallel loop that ran ~5 min, dispatched grok-4.5, and left the two track
worktrees with uncommitted, un-reviewed, partial implementer output — an
**unclean exit**. Rather than discard it, we used it as a real-world test:
**does `sworn loop --parallel --resume` recover cleanly from a crashed run?**

Command under test (relying on `~/.sworn/.env` only — no coach env sourced, so
this also validated the keys-only env file end to end):

```
sworn loop --release 2026-07-11-contract-edge-gates --parallel --resume \
  --implementer-model openrouter/x-ai/grok-4.5 \
  --verifier-model openrouter/x-ai/grok-4.5 \
  --base release/v0.1.0
```

## Outcome

| Track | Slice | Result |
|-------|-------|--------|
| T1-lint-contracts | S01-lint-contracts-registry | **PAUSED** — max turns exhausted, paged Coach |
| T2-assemble | S03-assemble-command | **FAIL** — first-pass: undeclared boundary mock |

Overall: `RunParallel: 1 track(s) failed: T2-assemble`, exit 1. **Board
uncorrupted — all three slices still `planned`, nothing committed.** The
state machine held; the failure was contained.

## Resume-recovery verdict

**Recovers without corrupting, but does NOT clean the dirty worktree.**

What worked:
- Coordinator **detected the existing release worktree** and did NOT re-bootstrap
  (prior cold runs logged `branch release-wt/... absent — creating it from HEAD
  (cold-start bootstrap)`; the resume run logged only `loaded 2 tracks in 1
  phases`).
- Re-dispatched both slices from committed state (`planned`).
- Board/state-machine integrity preserved — no slice falsely advanced.
- Fail-closed gates fired correctly (T2 first-pass rejected a bad diff).

The gap:
- Resume **re-runs the implementer on top of the still-dirty worktree without
  resetting it** to the committed slice base. The leftover uncommitted code from
  the unclean exit stayed in the tree and **contaminated the resumed attempt's
  diff** — T2's first-pass FAIL was triggered by leftover code (see Finding 4).
- A clean recovery should `git reset --hard <slice-base> && git clean -fd` (or
  equivalent) the track worktree to the committed slice state before
  re-dispatching, so a crashed run's debris cannot pollute the retry.

## Findings

### Finding 0 — `--dry-run` is silently ignored under `--parallel` (sworn bug)
`sworn loop --parallel --dry-run` does NOT dry-run — the `-dry-run` flag only
short-circuits the `--task`/planner path. Under `--parallel` it runs a real loop
(created worktrees, dispatched grok, spent budget). **Never use `--dry-run` as a
`--parallel` safety probe.** Fix: either honour `--dry-run` in parallel mode
(plan-only: materialise nothing, dispatch nothing) or hard-error that the combo
is unsupported.

### Finding 1 — resume does not reset the dirty worktree (recovery gap)
See "Resume-recovery verdict" above. This is the headline product gap: unclean
exit → leftover uncommitted debris → contaminated retry.

### Finding 2 — the `openrouter` provider omits structured-output config (NOT a model limitation)
```
sworn run: design TL;DR: ... captain structured dispatch: model: driver for
"x-ai/grok-4.5" does not support structured output — recording Rule 2 deferral
```
**Corrected root cause (Coach challenged the original framing — rightly):** this is
NOT a grok limitation. grok/xAI supports strict `json_schema` `response_format`,
`*OAI.ChatStructured` is fully implemented (`internal/model/oai.go:318`), and the
NATIVE `xai/` provider IS configured for it (`provider.go:167`,
`Structured: StructuredResponseFormat`). The bug: `provider.go`'s `openrouter`
case constructs its `OAI` client **without setting `Structured`**, so it defaults
to the zero value (unsupported) and `ChatStructured` refuses. Same omission on
`groq`, `mistral`, `cloudflare` — so routing ANY capable model through those
prefixes silently downgrades the structured gate to a Rule-2 deferral.
**Fix scope (Coach directive: all OAI-compat, handled DYNAMICALLY).** 5 providers
omit the mode (`groq`, `mistral`, `openrouter`, `cloudflare`, `github`) — but a
per-provider capability field is just a smaller static table with the same
staleness problem. Handle it dynamically instead:
Coach directive: **proactive-first** — model release velocity means static never
learns and reactive only learns after a live failure; proactive metadata knows
the whole catalog the moment the provider publishes it.
- **PRIMARY — proactive metadata sync:** fetch provider model-metadata into a
  per-model `{capabilities, pricing}` cache under `~/.config/sworn/`, with a
  refresh policy (TTL / `sworn models sync` / refresh-on-miss). OpenRouter
  `/api/v1/models` is the jackpot — `supported_parameters`
  (`response_format`/`structured_outputs`/`tools`) AND `pricing` for the whole
  catalog in one call. Without a refresh policy, "proactive" decays to
  "static, fetched once" — the refresh is the point.
- **FALLBACK — reactive attempt-and-degrade:** only for sparse-metadata
  providers (native xAI/groq/mistral list models but expose little capability
  detail). Try strict `json_schema` `response_format`; on "unsupported
  parameter" 400, degrade to `StructuredToolCall`; cache the working mode.
- **LAST RESORT:** the static per-provider `Structured` field, manual override only.

**This is foundational infra, not a bug-fix (scope flag, Rule 2).** A live
provider-metadata registry has THREE consumers: (1) structured-output gating
[Finding 2], (2) cost measurement [Finding 3], (3) the model ROUTER + telemetry
— the eval-derived routing that is sworn's positioned moat. It converges Findings
2+3 into ONE spine (capability discovery and price discovery are the same query),
and the router consumes it later. Decision needed: does `loop-hardening` grow to
hold this, or is it its own foundation release (`provider-registry`) that
loop-hardening's structured-output + cost slices depend on? Lean: separate
foundation release (clean layering).

Note: this run would have kept the structured gate on native `xai/grok-4.5`.
Secondary Baton note: define gate behaviour when a driver *genuinely* cannot do
structured output (defer vs prose-gate).

### Finding 3 — grok-4.5 pricing missing → cost telemetry blind (sworn#99)
```
inprocess: no pricing entry for model "openrouter/x-ai/grok-4.5" — cost recorded
as 0 (CostSource=unknown)
inprocess: no pricing entry for model "x-ai/grok-4.5-20260708" — cost recorded as 0
```
Both the requested id and the resolved dated id (`x-ai/grok-4.5-20260708`) miss
the pricing map. Telemetry records $0 → the model-eval data moat is blind to this
run's cost.

**Crux (not OpenRouter-specific).** Cost is RE-DERIVED, not measured: `tokens ×
a hardcoded static price table`. Token counts are read from the response
(`oai.go:233`), but the price comes from an in-binary stub table
(`oai.go:137` comment: "gets a zero cost. Expand as needed"); a miss →
`ComputeCostFromTokens` returns `{}, false` → `inprocess.go:261` logs
`CostSource=unknown`, cost `$0`. Hits **every un-tabled model**, not just grok.

OpenRouter is the worst case: (1) hundreds of dynamically-priced models a static
table can't track; (2) it serves a dated id (`x-ai/grok-4.5-20260708`) that won't
match the requested alias even if added; (3) **it returns the real $ cost in the
response** (`usage` with `include:true`, or `/generation` → `total_cost`), which
sworn currently discards.

**Fix (architectural, not "add rows"):** prefer **provider-reported cost** where
the API returns it → `CostSource=provider` (authoritative); fall back to
`tokens × price` only where cost is unreported, sourcing the price from a
maintainable registry rather than in-binary constants. This also dissolves the
dated-id mismatch (you read the charged price instead of matching ids to guess
one). Supersedes the shallow "add grok pricing entries" of sworn#99. → engine → sworn.

### Finding 4 — max-turns cap too low for grind/beast slices on grok
Both S01 (grind) and S03 (beast) hit `max turns exhausted` — S01 paged the Coach
mid-implementation (it had reached the `go test` proof stage). The morning
accidental run also hit max-turns on S03. grok needs more turns than the default
cap to carry a full slice (design → code → tests → proof). There is no CLI flag
to raise the turn cap (`--implement-timeout` is wall-clock, not turns). Consider
a `--max-turns` flag and/or a per-quadrant default (beast > grind > quick).

### Finding 5 — first-pass mock-detector false-positive on annotation-string literals
```
first-pass FAIL — Undeclared boundary mock(s):
  - mock: ent := "// @no-mock\n// @mock-boun (boundary: entitlement) at diff:1211
```
S03's assemble code legitimately contains string literals like `// @no-mock` and
`// @mock-boundary (boundary: entitlement)` because it *implements* boundary-mock
parsing (Rule 10). The first-pass dark-code/mock detector matched those string
literals and failed closed. Meta-irony: the boundary-mock-detection code trips
the boundary-mock detector. The detector needs to distinguish real mock code from
string/comment content that merely mentions the annotations (parse Go AST, or
exclude string-literal/comment tokens).

### Finding 6 — autonomous loop does not halt at design-review for the Coach
The router logs `the Design TL;DR gate (Step 4) will halt for Coach review before
any code lands`, but the autonomous loop generated the design TL;DR and proceeded
straight to `attempt 1 — implementing`. In autonomous `sworn loop` there is no
human-Coach halt; the structured captain dispatch that would gate it deferred out
(Finding 2). Rule 9 question: in autonomous mode, who plays Captain, and should
the design gate auto-proceed, self-review, or hard-pause for async Coach ack?

## Follow-up candidates

| # | Item | Home |
|---|------|------|
| 0 | `--dry-run` honoured/errored under `--parallel` | new issue |
| 1 | resume resets dirty worktree to committed base before re-dispatch | new issue (recovery) |
| 2 | structured-output on OAI chat path, or gating prose fallback | new issue (driver capability) |
| 3 | grok-4.5 + `x-ai/grok-4.5-20260708` pricing entries | sworn#99 |
| 4 | `--max-turns` flag / per-quadrant turn default | new issue |
| 5 | first-pass mock detector: exclude string/comment tokens | new issue |
| 6 | autonomous design-review gate semantics (Rule 9) | design decision |
| R | **Config-layer consolidation** — sworn is already JSON-on-XDG everywhere (`config.json` = settings, `credentials.json` = login token, both under `~/.config/sworn/`) EXCEPT the legacy dotenv `~/.sworn/.env` for provider keys. Target: (a) provider keys → a `providers` section in `credentials.json` (JSON, XDG, plain names, no `SWORN_` prefix); (b) plain env vars (`OPENROUTER_API_KEY`, …) remain as ephemeral overrides for CI/containers/AgentCore; (c) `SWORN_DIRECT` → explicit `config.json` `"routing": "direct"\|"proxy"`; (d) surface all of it (incl. routing) in the TUI settings screen; (e) retire `~/.sworn/.env` with a one-release read-only back-compat shim + `sworn doctor` migration nudge. Kills three warts at once (SWORN_ prefix split, `~/.sworn/` clutter, opaque `SWORN_DIRECT`). | new slice, `loop-cli-ux` |

## Organizing principle: capability-based constraints (Coach directive 2026-07-12)

The keystone that reframes the whole registry/router direction: **sworn's
constraints are on capabilities, not specific models.** A model id is an
implementation of capabilities and an override at most — never the constraint.

- **Inversion:** FROM "role/gate uses model X" (a pin that goes stale every model
  launch) TO "role/gate REQUIRES capabilities {C1, C2, …}" → the registry returns
  every model that satisfies them → the router picks by cost/velocity/eval.
- **Concrete:** the verifier is pinned `opus` today "for 1M context" — a model pin
  hiding a capability need. Capability-based: verifier requires `{context ≥ 200k,
  structured-output, high-reasoning}`; any model meeting the bar is eligible, and
  a newly-released model that meets it is eligible with zero config change.
- **Key new artifact — a capability taxonomy:** the shared vocabulary between
  task-needs and model-provides (`structured-output/json_schema`, `tool-calling`,
  `context-window`, `vision`, `reasoning-effort`, `agentic-multiturn`, …). sworn
  already seeds it (`CapStructuredOutput`, `sworn capabilities` per-driver roles);
  this promotes it from driver-level to model-level and to *the* constraint layer.
- **Moat linkage:** capability-match is the INPUT FILTER to the eval-derived
  routing function `f(effort_complexity, cost/velocity) → model` — capability-match
  first (eligible set), then eval/cost-route within it. Capabilities are also the
  audit trail for "why was this model a candidate."
- **sworn↔Baton seam — TWO schemas, split by ownership:**
  - **Capability policy → Baton (`capability-policy-v1`):** the capability
    taxonomy + per-role required capabilities ("a verifier requires
    {structured-output, context≥200k, high-reasoning}"). HARD eligibility
    contract; model-agnostic, eval-agnostic, portable, auditable. Belongs to
    Baton because Baton owns the roles. → upstream contribution (feedback loop).
  - **Routing preferences → sworn (`config.json` / routing-preferences):**
    eval-based ranking + cost/velocity + explore/exploit budget. SOFT preference,
    the engine's moat. **Already seeded in code** — `config.json` carries
    `OptimizeMode` (quality/cost/balanced) + `PassRateFloor`, which ARE routing
    preferences, already sworn-side. The split names a seam the code already cut.
  - **Taxonomy is the interop contract → Baton-owned.** Baton policy says a role
    *requires* `structured-output`; the registry says a model *provides* it; the
    intersection only works if both speak one vocabulary. So the taxonomy is part
    of `capability-policy-v1`; sworn's registry is a **mapping layer** translating
    each provider's idiosyncratic metadata onto the Baton taxonomy (providers stay
    pluggable without leaking quirks into the contract).
  - **Thread w/ ownership:** Baton `role→required-caps` → sworn registry
    `models→provided-caps` → eligibility `∩` (sworn enforces Baton's contract) →
    sworn routing `eval+cost+explore/exploit` → dispatch.

The `provider-registry` foundation therefore stores per-model **capabilities**
(not just pricing/structured-output flags); Findings 2+3 and the router are its
first three consumers.

### Eval is a soft prior, NOT a hard gate (Coach directive)

Two signals, structurally different roles — never conflate:
- **Capabilities = eligibility** (HARD gate, forward-looking, binary). The only
  thing that makes a model *not a candidate* is a capability mismatch (or an
  explicit human denylist/override). A model released today is eligible the moment
  the registry sees it.
- **Eval = preference** (SOFT rank, backward-looking, continuous). Orders/weights
  the eligible set by past per-role performance. Informs, **never excludes**.

**Failure mode to avoid:** if eval gated eligibility, you get rich-get-richer
lock-in — a new model has no track record → never selected → never accumulates a
track record → the router freezes to whatever won early. "Eval-driven" silently
becomes "eval-frozen." That is the forward restriction to prevent.

**Mechanism:** treat eval as a prior WITH UNCERTAINTY (explore/exploit). Established
model = tight interval, exploited; new eligible model = wide uncertainty, gets
exploratory traffic (naturally on low-stakes / quick quadrants first) to earn its
data — optimism-under-uncertainty (UCB-style), not "no history = don't use." Audit
trail: "eligible *because* capabilities, chosen *because* eval-score + exploration
budget" — adaptive AND auditable, the opposite of a frozen benchmark-max snapshot.

## Routing: sworn engine vs Baton protocol (Coach directive 2026-07-12)

The dogfood findings do not all belong to sworn. Some are feedback on the **Baton
protocol** (rules/contracts/schemas) and must go **upstream to Baton for review
and inclusion in a new version** — not be buried as engine bugs. Triage:

| # | Owner | Note |
|---|-------|------|
| 0 | engine → sworn | pure CLI flag bug |
| 1 | **protocol → Baton** | resilience contract: a resumed loop restores committed state before re-dispatch (strengthen Rule 11 / multi-worktree-resilience). sworn implements the reset. |
| 2 | **engine → sworn** | root cause = `openrouter`/`groq`/`mistral`/`cloudflare` providers omit the `Structured` field (default unsupported); grok + `*OAI` + native `xai/` all support it. Minor Baton note only: gate behaviour when a driver *genuinely* cannot do structured output. |
| 3 | engine → sworn | sworn#99 pricing map |
| 4 | engine → sworn | loop config; per-quadrant minimum-turns is a minor Baton note |
| 5 | mixed | detector impl = sworn; "what counts as a mock at a boundary" (exclude annotation string/comment tokens) refines Rule 10 = Baton |
| 6 | **protocol → Baton** | Rule 9 — autonomous-mode design-gate semantics is a protocol decision |

### Formalise the sworn → Baton feedback loop

Coach (Brad) directive: this feedback path must be **formalised**, not ad hoc. Proposed
mechanism (rides existing Baton PR-up governance + semver-tag vendor pin, ADR-0006):

1. **Triage** — every dogfood/session finding tagged `protocol` or `engine` in its
   capture doc (this doc is the first instance).
2. **Route** — `engine` → sworn issues; `protocol` → Baton issues on
   `github.com/sawy3r/baton`, label `dogfood-feedback`, linking the sworn capture.
   Baton triages into a version; sworn re-vendors on the VERSION-pin bump.
3. **Durability** — the process is stable reference, so it lands as a sworn **ADR**
   ("Dogfood → Baton feedback loop"), not an issue. That ADR is what turns the
   hand-off into a formal loop.

Pending Coach confirmation before any outward-facing Baton issue is filed (public repo).

## Cleanup state (as of writing)

- Loop process exited; board intact (`planned` ×3).
- The two track worktrees still hold the contaminated dirty state (kept for
  inspection / until a clean re-run is decided).
- Monitor tasks stopped.
