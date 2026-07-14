---
title: 'Release intake: local and cloud providers'
description: 'Planning record for OpenAI-compatible local endpoints, Ollama Cloud, loop dispatch, and live dialect discovery.'
---

# Release Intake: `2026-07-14-local-cloud-providers`

## Release goal

Enable a SwornAgent operator to use Ollama Cloud or a locally hosted OpenAI-compatible model through the full `sworn run` loop, with provider endpoints declared as data, per-provider base URLs configurable in `config.json`, keyless local services reported available through a real reachability probe, and endpoint wire quirks re-derived by a nightly/on-demand live conformance suite. Shipped means Ollama Cloud and the major local aggregators are first-class loop-capable choices without duplicating provider factories or silently assuming that every OpenAI-compatible endpoint has the same dialect.

## Source of truth

- **Human stakeholder**: Repository owner / Coach
- **Tracking issue / epic**: [sworn#15](https://github.com/swornagent/sworn/issues/15), reopened and expanded on 2026-07-14 after capture commit `2980b98` accidentally auto-closed it before implementation
- **Related captures**:
  - `docs/captures/2026-07-14-local-and-cloud-providers-brief.md`
  - `docs/captures/2026-07-14-outstanding-work-catalogue.md`
  - `docs/captures/2026-07-14-architecture-review-brief.md`
- **Related release**: `docs/release/2026-07-11-contract-edge-gates/S04-provider-registry/` on `release-wt/2026-07-11-contract-edge-gates`; capability and pricing metadata remain owned there
- **Related memory entries**: none

## Users and their gestures

- **SwornAgent operator with an Ollama Cloud subscription**: selects an `ollama-cloud/<model>:cloud` model for a role and runs the normal SwornAgent commands using `OLLAMA_API_KEY` from the canonical credential path.
- **Local-model operator**: points a role at `ollama/`, `llamacpp/`, `lmstudio/`, `vllm/`, or `localai/`, optionally overrides that provider's base URL in `config.json`, and runs the full loop without an API key.
- **Provider maintainer**: adds another OpenAI-compatible endpoint as one declared endpoint-table row rather than editing multiple switches.
- **SwornAgent maintainer**: runs or inspects the live endpoint-conformance suite and sees an observed dialect record per configured endpoint.

## What's currently broken or missing

- Native local Ollama exists on the one-shot utility path but is deliberately absent from the loop driver registry, so it cannot implement a slice through `sworn run`.
- Ollama Cloud has no provider prefix or endpoint declaration.
- llama.cpp, LM Studio, vLLM, and LocalAI have no provider prefixes or endpoint declarations.
- OpenAI-compatible provider construction is duplicated across `internal/model/provider.go`; adding more switch arms repeats the same struct assembly and preserves a known extension-point collision.
- Registry availability for in-process providers is key-driven. A keyless local provider placed behind `keyProbe` is permanently unavailable even when its endpoint is live.
- `config.json` has no provider endpoint override map, so local server locations cannot be declared per provider.
- The project has driver-interface conformance tests but no live endpoint-level probe matrix that observes OpenAI-compatible dialect differences.

## What the human wants

- Run SwornAgent against Ollama Cloud using the existing canonical `OLLAMA_API_KEY` credential path.
- Run the complete implement-verify loop against local Ollama and the popular local aggregators llama.cpp, LM Studio, vLLM, and LocalAI.
- Replace repeated OpenAI-compatible switch arms with one declared endpoint table so a new compatible provider is a data-only addition.
- Allow each provider's base URL to be overridden in `config.json`.
- Determine keyless local availability with a bounded reachability probe rather than an API-key check.
- Keep the genuinely different native Ollama utility driver while also offering loop-capable OpenAI-compatible local dispatch.
- Derive endpoint dialect records from live behaviour, including the two known wire failures and the determinism assumption around `temperature: 0`.
- Keep live provider calls in `.github/workflows/live.yml`, nightly and workflow-dispatch only, never in the pull-request gate.

## Constraints and non-negotiables

- Native Go single binary; standard library only and no new runtime dependency.
- Fail closed for unknown providers, unreachable configured endpoints, unsupported roles, and unparseable model output.
- Never log API keys, model payloads, or request bodies.
- Reuse `model.ProviderKey`; do not introduce another credential-resolution path.
- Local endpoints are untrusted network boundaries even on loopback: use bounded timeouts, do not send credentials unless the endpoint descriptor requires them, and do not infer safety from a `localhost` hostname.
- Preserve explicit prefix resolution and fail-fast role checks in `internal/driver/registry`; no smart fallback to a different provider.
- Capability and pricing discovery belong to `S04-provider-registry`; this release owns endpoint declarations and dialect observation.
- Live tests may spend tokens and depend on external services, so they run nightly/on demand and emit no secret payloads.
- This is a CLI/backend release; WCAG/UI accessibility is not applicable because it adds no interactive visual surface.
- Endpoint lookup and registry resolution must remain bounded by the small declared provider table; no user-controlled quadratic scan.

## Adjacent / out of scope

- **Capability and pricing cache**: model capability taxonomy, OpenRouter metadata sync, TTL refresh, and attempt-and-degrade capability selection remain in `2026-07-11-contract-edge-gates/S04-provider-registry`. **Why deferred**: it is already specced and conflating endpoint dialect with model capability would duplicate an in-flight contract. **Tracking**: `S04-provider-registry`. **Acknowledged**: repository owner, 2026-07-14, in the commissioned brief.
- **Capability eligibility and eval-based routing**: choosing a model by capability or eval score is not part of declaring or probing endpoints. **Why deferred**: those consumers depend on S04 and are already sequenced separately. **Tracking**: `S05-capability-eligibility` and `S06-routing-preferences` in `2026-07-11-contract-edge-gates`. **Acknowledged**: repository owner, 2026-07-14, in the commissioned brief.
- **New credential store or prefixed environment variables**: credentials were unified in sworn#107. **Why deferred**: a second key path would recreate the resolved drift defect. **Tracking**: sworn#107 is the landed authority. **Acknowledged**: repository owner, 2026-07-14, in the commissioned brief.

## Decisions made during planning

### 2026-07-14 — Release identity and starting contract

- **Context**: Convert the commissioned local/cloud provider brief into a Baton release before production code changes.
- **Options considered**: continue as an untracked implementation; fold it into the already-specced capability registry; plan a separate issue-backed release.
- **Decision**: Plan `2026-07-14-local-cloud-providers` as a separate release based on `main`, preserving the explicit seam with S04.
- **Why**: The endpoint/dialect work has a distinct user outcome and touchpoints, while the repo requires non-trivial work to be issue-backed and sliced before implementation.

### 2026-07-14 — Release goal ratified

- **Context**: Confirm the user-reachable outcome before decomposing the commissioned brief.
- **Options considered**: revise the goal; proceed with the drafted goal.
- **Decision**: Proceed with the drafted goal: full-loop Ollama Cloud and local OpenAI-compatible dispatch, data-driven endpoint declarations, configurable endpoint overrides, keyless reachability, and live dialect conformance, kept separate from S04 capability metadata.
- **Why**: The repository owner confirmed that this matches the intended release on 2026-07-14.

### 2026-07-14 — Reopen and expand sworn#15 as the release anchor

- **Context**: Capture commit `2980b98` auto-closed #15 because its commissioned brief used the phrase “closes #15”, although no implementation had landed.
- **Options considered**: reopen and expand #15; reopen #15 plus create a separate epic; leave #15 closed and create a replacement.
- **Decision**: Reopen #15 and expand its contract to anchor the complete local/cloud provider release.
- **Why**: This corrects the false delivered state, preserves the original provider-factory history, and avoids unnecessary issue-management duplication while making the broader endpoint and dialect scope explicit.

### 2026-07-14 — Give `ollama/` one loop-capable meaning

- **Context**: Today `model.NewClient("ollama/...")` returns the native verify-only `/api/chat` client, while loop dispatch requires an `agent.Agent` multi-turn chat client. Registering that prefix unchanged would advertise loop support and then fail closed at dispatch.
- **Options considered**: make `ollama/` the OAI shim everywhere and move native dispatch to `ollama-native/`; keep native `ollama/` and add `ollama-local/`; make one prefix role-dependent.
- **Decision**: `ollama/` resolves through Ollama's OpenAI-compatible `/v1` endpoint on every path; `ollama-native/` preserves the existing native `/api/chat` utility driver.
- **Why**: The natural prefix becomes full-loop capable, every model ID retains one meaning across registry and utility paths, and the genuinely different native implementation remains available without violating the single-resolution-authority invariant.
- **Migration obligation**: The release must document the prefix change and prove both `ollama/` OAI dispatch and `ollama-native/` native dispatch through their owning integration points.

### 2026-07-14 — Use extensible provider objects in `config.json`

- **Context**: Local servers can run at arbitrary hosts and ports, but `config.json` has no per-provider endpoint contract. The shape must not create another credential or capability source of truth.
- **Options considered**: a top-level `provider_endpoints` string map; a top-level `providers` object keyed by prefix; a `base_url` inside each role's model setting.
- **Decision**: Add a top-level `providers` object keyed by canonical prefix, with each provider value containing `base_url` (for example, `"providers": {"ollama": {"base_url": "http://localhost:11434/v1"}}`).
- **Why**: Provider objects leave room for future endpoint/wire settings without duplicating URLs across roles. Credentials remain exclusively in canonical environment variables plus `credentials.json`, and capability/pricing data remains exclusively owned by S04.
- **Boundary**: This release must validate provider keys and absolute HTTP(S) URLs fail-closed; provider objects must not accept or persist API keys, model capability claims, or pricing data.

### 2026-07-14 — Make `config.json` the only endpoint-override source

- **Context**: Existing OAI clients can read `SWORN_<PROVIDER>_BASE_URL`, while the new public contract adds `providers.<prefix>.base_url` to `config.json`. Keeping both would preserve two endpoint sources with precedence rules.
- **Options considered**: legacy environment variable over config; config over the legacy environment variable; remove the legacy environment-variable override.
- **Decision**: Remove `SWORN_<PROVIDER>_BASE_URL` support. Resolve endpoints from `config.json` first and the declared endpoint-table default second.
- **Why**: Endpoint selection becomes visible and durable in one configuration record instead of depending on ambient process state. This is an intentional breaking migration, not a backward-compatible alias.
- **Migration obligation**: Documentation and `sworn doctor` must identify the removed variables and direct operators to `providers.<prefix>.base_url`; runtime handling of a still-set legacy variable must be decided before decomposition so it cannot be silently ignored.

### 2026-07-14 — Warn and ignore removed endpoint environment variables

- **Context**: Removing `SWORN_<PROVIDER>_BASE_URL` creates a choice between failing closed, warning and ignoring, or honouring it for a deprecation window. Ignoring without any signal could connect to a different endpoint.
- **Options considered**: fail closed with migration instructions; warn and ignore; honour for one deprecated release.
- **Decision**: When a removed base-URL environment variable is present, emit a value-free migration warning, ignore it, and continue with `providers.<prefix>.base_url` or the declared default. `sworn doctor` reports the same migration condition.
- **Why**: The repository owner is currently the only person who has built the binary and is not yet using it operationally, so a reusable hard-failure migration mechanism would add machinery without a real deployed-user benefit. The warning preserves visibility without retaining the second endpoint source.
- **Security constraint**: Never print the environment variable's value; name only the variable and the replacement config path.

### 2026-07-14 — Probe keyless reachability during enumeration only

- **Context**: Keyless local providers cannot use `keyProbe`. Reachability can be tested when enumerating capabilities, before every dispatch, or lazily and cached.
- **Options considered**: probe during enumeration only; probe during enumeration and every dispatch; probe on first dispatch and cache.
- **Decision**: `sworn capabilities` and provider model enumeration perform a bounded `GET <base_url>/models` reachability probe for keyless OpenAI-compatible endpoints; dispatch sends the real model request without a preliminary probe and classifies that request's actual error.
- **Why**: Enumeration becomes truthful without adding a redundant request and time-of-check/time-of-use promise to every model call. Dispatch remains fail-closed on the authoritative operation.
- **Probe contract**: Short fixed timeout, no retries, no credentials for keyless providers, and available only on a 2xx response with a parseable OpenAI-style models payload. Errors report provider and endpoint without response bodies or payloads.

## Schema-vs-spec audit notes

- `internal/model.ProviderConfig` currently uses dedicated fields and has only `OllamaHost`; it has no generic per-provider endpoint override representation.
- `internal/config.Config` currently holds role model selections and project settings; it has no provider endpoint map.
- `internal/driver/registry.Default` groups all chat prefixes behind one `keyProbe`; keyless provider availability therefore requires a separate probe/entry design.
- `internal/model.NewClient` deliberately routes `ollama/` to the native `/api/chat` driver today. Planning must choose how to preserve that compatibility while exposing an OpenAI-compatible loop path without assigning one prefix two meanings.

## Proposed slice decomposition (draft)

Not yet decided. Discovery must first resolve prefix compatibility, reachability semantics, endpoint-override shape, and whether live dialect discovery is one slice or a dependent track.

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Whether to reopen #15 or create a replacement issue after it was auto-closed by the capture commit | Rule 5 issue anchor | Resolved 2026-07-14: reopen and expand #15 |
| A-02 | Whether existing native `ollama/` retains its utility-path meaning while the loop registry maps the same prefix through the OAI shim, or a distinct prefix is introduced | Backward compatibility and dispatch consistency | Resolved 2026-07-14: `ollama/` is OAI-compatible everywhere; native moves to `ollama-native/` |
| A-03 | Exact `config.json` shape and precedence for per-provider base URL overrides | Public config contract | Resolved 2026-07-14: `providers.<prefix>.base_url` → declared default; legacy env override removed |
| A-04 | Whether a live reachability probe runs during `sworn capabilities`, on dispatch, or both; timeout and endpoint path are not yet fixed | Latency, availability truth, fail-closed behaviour | Resolved 2026-07-14: enumeration-only bounded `/models` probe; actual dispatch is authoritative |
| A-05 | Which live providers are mandatory in the nightly matrix versus configured/skipped when credentials or daemons are unavailable | Conformance coverage and CI cost | Human decision during discovery |
| A-06 | Whether the conformance suite only reports observed dialect or also generates a checked-in runtime dialect table consumed by dispatch | Runtime architecture and drift semantics | Human decision during discovery |
| A-07 | Whether runtime fails closed or only warns when a removed `SWORN_<PROVIDER>_BASE_URL` variable is still set | Migration safety | Resolved 2026-07-14: value-free warning, ignore, continue from config/default; doctor reports it |

## Screenshots / references

- No screenshots; this release has no visual UI surface.
