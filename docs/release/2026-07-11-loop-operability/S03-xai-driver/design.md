# Design TL;DR — S03-xai-driver

**Slice:** S03-xai-driver · **Release:** 2026-07-11-loop-operability · **Track:** T2-xai-driver
**State:** design_review (Rule 9 gate — no production code written yet)
**Covers:** N-03 · **Spec:** `spec.json` (AC-01..AC-04)

## User outcome (restated)

`xai/` becomes a first-class in-process driver prefix. `xai/grok-4.5` (and other
xAI models) resolves natively through the driver registry to xAI's
OpenAI-compatible API at `https://api.x.ai/v1` using `XAI_API_KEY`, declaring
implementer/verifier/captain roles exactly like the sibling in-process prefixes
(deepseek/groq/mistral/openrouter). `sworn capabilities` lists the xai driver as
available when `XAI_API_KEY` is present; `sworn models --provider xai` enumerates
xAI models; grok-4.5 carries a pricing entry so honest cost is a real number. The
existing `openrouter/x-ai/grok-4.5` route is untouched — `xai/` is an *additional*
native path.

## Approach

This is an **additive provider registration**, not a new driver type. xAI is
OpenAI chat/completions-compatible, so it reuses the existing in-process chat
driver — no bespoke xAI SDK (ADR-0007: `net/http` + `encoding/json` only). The
whole slice is: one new `ProviderConfig` field, one `NewClient` prefix case, one
entry in the registry's `chatPrefixes` table + its `keyFor` probe, one catalog
provider def, and one pricing entry — each mirroring an existing sibling prefix.

**Why the chat driver, not the Responses driver (R-01 resolved).** The registry
splits in-process traffic into two shared driver instances:
`inprocess.NewOAIResponses(cfg)` owning `responsesPrefixes = {"openai"}`
(`/v1/responses`) and `inprocess.NewOAIChat(cfg)` owning `chatPrefixes`
(`/v1/chat/completions`) — `internal/driver/registry/registry.go:264-301`. xAI
speaks chat/completions, so `xai` joins **`chatPrefixes`**.

**Structured-output path (R-01, verified at design).** xAI's API accepts the
exact OpenAI strict `json_schema` `response_format` shape
(`{"type":"json_schema","json_schema":{"name",...,"schema","strict":true}}`,
constrained decoding when `strict:true`) — confirmed against
`docs.x.ai/developers/model-capabilities/text/structured-outputs`. That is
identical to the `StructuredResponseFormat` mode `openai-completions` uses
(`internal/model/provider.go:119`, `internal/model/structured.go:29-38`). So
`xai/` gets `Structured: StructuredResponseFormat`. No quirk requiring
containment was found; **if** one surfaces at implementation, the contained
fallback is a one-token mode change to `StructuredToolCall` (the DeepSeek mode,
`provider.go:127`) with no new code path — the driver-contract containment the
spec calls for. This means verifier/captain (which need `ChatStructured`) work on
`xai/`, so the honest declared role set is implementer/verifier/captain.

**Role declaration is inherited, not per-prefix (design fact worth a reviewer's
eye).** All `chatPrefixes` share **one** `inprocess.NewOAIChat(cfg)` instance
(one registry `Entry`). Roles are declared by that driver, not per prefix — so
`xai/` inherits implementer/verifier/captain by *joining the prefix slice*, the
same way deepseek/groq/mistral do. There is no separate xai `Entry`. This is the
correct, honest design (the spec's "declaring … roles like the other in-process
prefixes" = inheriting them), and it is why the per-dispatch base-URL + structured
mode must be carried by `NewClient`, not the registry entry.

**Dispatch wiring mechanism (grounded).** The shared chat driver resolves each
dispatch's client via `newClient: model.ResolveLoopClient`
(`internal/driver/inprocess/inprocess.go:76-77`), and `ResolveLoopClient`
(`internal/model/config.go:192-204`) proxy-routes or falls through to
`model.NewClient(modelID, pcfg)`. Therefore the new `case "xai"` in `NewClient`
is what actually stamps `BaseURL: https://api.x.ai/v1`, `APIKey: pcfg.XAIKey`,
and `Structured: StructuredResponseFormat` onto the `*OAI` at dispatch time. The
registry entry only needs `xai` in `chatPrefixes` (for resolution) and a
`keyFor` case (for the no-dispatch availability probe).

## Files to change (each AC traced)

| File | Change | AC |
|------|--------|----|
| `internal/model/provider.go` | Add `XAIKey string` to `ProviderConfig`; populate in `ProviderConfigFromEnv` via `envOrAlias("XAI_API_KEY","SWORN_XAI_API_KEY")`; add `case "xai"` to `NewClient` returning `&OAI{BaseURL:"https://api.x.ai/v1", Model:model, APIKey:pcfg.XAIKey, Structured: StructuredResponseFormat}` | AC-01, AC-03 |
| `internal/model/config.go` | Add `XAIKey: os.Getenv("SWORN_XAI_API_KEY")` to `swornProviderConfig()`; add `case "xai"` to `FromEnv`'s key switch reading `envOrAlias("XAI_API_KEY","SWORN_XAI_API_KEY")` so the one-shot utility path honours the canonical var too (default case reads SWORN_-only) | AC-01 |
| `internal/driver/registry/registry.go` | Add `"xai"` to `chatPrefixes` (`:266-269`); add `case "xai": return cfg.XAIKey` to `keyFor` (`:310-330`) | AC-01, AC-02 |
| `internal/model/client.go` | Add `xaiPricing` lookup to `PriceForModel` (new per-provider map, mirroring `anthropicPricing`/`googlePricing`/`bedrockPricing` at `:72-90`) | AC-04 |
| `internal/model/oai.go` (or new `xai.go`) | Define `xaiPricing` map with a `grok-4.5` entry (published xAI rate, confirmed at implementation); `listXAIModels` calling `GET https://api.x.ai/v1/models` with `Bearer cfg.XAIKey`, fail-closed unknown on absent metadata (mirrors `listGroqModels`/`listOpenRouterModels`, `catalog.go:209/245`) | AC-04 |
| `internal/model/catalog.go` | Add `{"xai", func(cfg) bool { return cfg.XAIKey != "" }, listXAIModels}` to `catalogProviderDefs` (`:86-94`, alphabetical order → between mistral/ollama) | AC-04 |

### Tests (spec touchpoints)

- `internal/driver/registry/registry_test.go` — assert `Resolve("xai/grok-4.5", …)`
  returns the oai chat driver for implementer/verifier/captain roles; **update the
  golden prefix-set assertions** that enumerate the full chat prefix list
  (`:56-57`, `:105-107`, and the `wantChat` string `:300`
  `"anthropic,cloudflare,deepseek,github,groq,mistral,openai-completions,openrouter"`
  → add `xai`). See Risk R-3.
- capabilities test — xai listed available with `XAIKey` set, unavailable when
  absent, no dispatch (AC-02).
- structured httptest — `ChatStructured` through the xai `*OAI`
  (`StructuredResponseFormat`, httptest server as `BaseURL`, no live dispatch)
  returns a valid JSON object (AC-03), reusing the `oai_test.go`/`structured_test.go`
  pattern.
- pricing test — `PriceForModel("grok-4.5")` returns a real, non-zero rate (AC-04).

## Design choices + rationale

- **D1 — reuse the shared oai chat driver (Type-2, noted default).** xAI is
  OpenAI-compatible; joining `chatPrefixes` is the established sibling pattern.
  Reversible and local. No new `Entry`, no new driver type.
- **D2 — `StructuredResponseFormat`, not `StructuredToolCall` (Type-2).** Verified
  xAI supports strict `json_schema` `response_format` today; fallback is a
  one-token change if a wire quirk appears. Local, reversible.
- **D3 — new `xaiPricing` map wired into `PriceForModel`, not an entry in the
  OpenAI `modelPricing` map (Type-2).** Follows the per-provider-map convention
  (`anthropicPricing`/`googlePricing`/`bedrockPricing`); keeps grok rates out of
  the OpenAI table. Local, reversible.
- **D4 — canonical `XAI_API_KEY` on both the registry/loop path AND the one-shot
  `FromEnv` path (Type-2).** `ProviderConfigFromEnv` already uses `envOrAlias`;
  adding a `FromEnv` `case "xai"` keeps the utility path consistent (its default
  case reads `SWORN_XAI_API_KEY` only). Matches the existing google/openai-responses
  carve-outs.

No Type-1 (architecturally-significant / hard-to-reverse) choices: every change
mirrors an in-place sibling pattern within the already-ratified driver-contract
architecture (S05 registry, ADR-0011 structured outputs, S08 pricing registry).

## Risks / pins for the reviewer

- **R-1 (spec R-01) — RESOLVED at design.** xAI structured-output wire format
  matches OpenAI strict `response_format`; declared roles (impl/verify/captain)
  reflect what actually works. Contained fallback documented (D2).
- **R-2 (spec R-02).** Availability is credential-presence only (`keyProbe` on
  `cfg.XAIKey`, no dispatch); the catalog `configured` predicate is `XAIKey != ""`.
  No key ever logged.
- **R-3 — golden-list test churn (mechanical, flagged).** Adding `xai` to
  `chatPrefixes` changes several enumerated expected-set assertions in
  `registry_test.go` (including the `wantChat` CSV string). These edits are
  *expected* and correct, not a regression — calling it out so the fresh verifier
  does not read the golden-list diff as scope creep.
- **R-4 — grok-4.5 pricing accuracy.** The exact per-1M input/output rate must be
  taken from xAI's published pricing at implementation time (not guessed); the
  slice only requires a real non-zero entry so cost is `CostSource=pricing-table`,
  not `unknown`.
- **Hazard note.** Watch for newline-eating edit corruption on the `.go` edits
  (fused `//`+code); `gofmt -l` + `go vet` + full `go test -count=1 -timeout 300s ./...`
  before any state transition.

## Reachability artefact (planned)

The user-facing affordance is `--implementer-model xai/grok-4.5` reaching the loop
via the registry, surfaced by `sworn capabilities`. The first failing test renders
through the **integration point that owns resolution** — `registry.Resolve` /
`registry.Default(cfg).Drivers()` — not a leaf. Smoke step at implementation:
`XAI_API_KEY=sk-… go run ./cmd/sworn capabilities` shows the `xai/` prefix listed
available; `sworn models --provider xai` enumerates xAI models. (No live dispatch
in tests — httptest only.)
