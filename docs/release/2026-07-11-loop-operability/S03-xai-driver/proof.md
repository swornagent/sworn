---
title: Slice proof bundle — S03-xai-driver
description: Rule 6 proof bundle, scoped to one slice. Generated from live repo state, not recollection. Verifier reads this; do not paraphrase.
---

# Proof Bundle: `S03-xai-driver`

Rendered from `proof.json` (proof-v1). First implementation pass.

## Scope

Make `xai/` a first-class native driver prefix: `xai/grok-4.5` resolves through
the driver registry to xAI's OpenAI-compatible API (`https://api.x.ai/v1`) using
`XAI_API_KEY`, declaring implementer/verifier/captain roles like the sibling
in-process prefixes, surfaced by `sworn capabilities` and `sworn models`, with a
real `grok-4.5` pricing entry so cost is honest (not `unknown`).

## Files changed

```
$ git diff --name-only 71b003057abe46483df0bfcdfdace70c6e7582bd
cmd/sworn/capabilities_test.go
docs/release/2026-07-11-loop-operability/S03-xai-driver/journal.md
docs/release/2026-07-11-loop-operability/S03-xai-driver/proof.json
docs/release/2026-07-11-loop-operability/S03-xai-driver/proof.md
docs/release/2026-07-11-loop-operability/S03-xai-driver/status.json
internal/driver/registry/registry.go
internal/driver/registry/registry_test.go
internal/model/catalog.go
internal/model/catalog_test.go
internal/model/client.go
internal/model/config.go
internal/model/pricing_test.go
internal/model/provider.go
internal/model/provider_test.go
internal/model/structured_test.go
internal/model/xai.go
```

(`proof.json` and this `proof.md` land with the bundle commit.)

## Test results

```
$ go build ./...
(no output, exit 0)

$ go vet ./internal/model/... ./internal/driver/... ./cmd/sworn/...
(no output, exit 0)

$ go test -count=1 ./internal/driver/... ./internal/model/... ./cmd/sworn/...
ok  github.com/swornagent/sworn/internal/model            2.147s
ok  github.com/swornagent/sworn/internal/driver           2.030s
ok  github.com/swornagent/sworn/internal/driver/drivertest 0.052s
ok  github.com/swornagent/sworn/internal/driver/inprocess 0.162s
ok  github.com/swornagent/sworn/internal/driver/registry  0.039s
ok  github.com/swornagent/sworn/cmd/sworn                 30.349s

$ go test -count=1 -timeout 300s ./...
ok — all test packages PASS, 0 failures
(cmd/sworn 31.6s, internal/account 10.1s, internal/model 2.1s, internal/driver 2.1s,
 internal/driver/registry 0.03s, ...; only internal/baton/schemas and
 internal/verdict have no test files)
```

## Reachability artefact

- **Type**: cli-run (plus httptest for the structured path — no live xAI dispatch)
- **Smoke**:
  - `XAI_API_KEY=sk-smoke-test sworn capabilities` → the `oai-inprocess` block
    lists `xai/` among its prefixes, `roles: implementer,verifier,captain`, and
    `available: yes — API keys present: xai/`.
  - `sworn capabilities` with no xAI key → `xai/` still listed,
    `available: no — no API keys present; no sworn proxy login`.
  - `sworn models --provider zzz-bad` → `valid providers: anthropic, google,
    groq, mistral, ollama, openai, openrouter, xai`.
- **Structured path**: `TestXAI_ChatStructured_ResponseFormat` drives
  `ChatStructured` through the `NewClient`-resolved xai client against an httptest
  server, asserting a strict `json_schema` `response_format` is emitted and the
  response normalises into `Content` (proves our marshalling/parse; live xAI
  strict-schema acceptance is doc-confirmed only — docs.x.ai structured-outputs).

## Delivered

- **AC-01** — the registry resolves `xai/grok-4.5` to the in-process oai chat
  driver for implementer/verifier/captain, configured for `https://api.x.ai/v1`
  with `XAIKey`. Evidence: `registry.go` (`xai` in `chatPrefixes` + `keyFor`
  case), `provider.go` (`NewClient` `case "xai"`). Tests: `TestResolveXAIRoles`,
  `TestDefaultRegistryTable`, `TestRegistryNewClientConsistency`,
  `TestNewClient_OAICompat` (xai row), `TestNewClient_XAIStructured` — all pass.
- **AC-02** — `sworn capabilities` lists `xai/` available with `XAI_API_KEY`
  present and unavailable when absent, no dispatch. Evidence:
  `capabilities_test.go` `TestCapabilitiesListsXAI` (both subtests);
  `clearProviderEnv` extended to blank the XAI vars; smoke above. Availability is
  credential-presence only (`keyProbe`/`keyFor`).
- **AC-03** — `xai/` emits schema-constrained structured output
  (`StructuredResponseFormat`) that validates via the `ChatStructured` path
  (httptest, no live dispatch). Evidence: `provider.go` (`Structured:
  StructuredResponseFormat`), `TestXAI_ChatStructured_ResponseFormat`,
  `TestNewClient_XAIStructured`. Contained fallback (D2 `StructuredToolCall`)
  documented if a live quirk ever surfaces.
- **AC-04** — `sworn models --provider xai` enumerates xAI models (fail-closed
  Unknown), `grok-4.5` carries a real non-zero pricing entry, build + targeted
  tests pass. Evidence: `xai.go` (`listXAIModels`, `xaiPricing`), `catalog.go`
  (`xai` def appended last), `client.go` (`xaiPricing` in `PriceForModel`).
  Tests: `TestCatalogAnnotations` (xai subtest), `TestCatalogProviderNames`
  (8 entries, `xai` last), `TestPricing_Grok45`.

## Not delivered

- **Exact grok-4.5 published pricing confirmation.** The `xaiPricing` entry uses
  xAI's Grok flagship tier ($3/$15 per 1M, 2026-07-12 snapshot); grok-4.5 may
  postdate that rate. AC-04 requires only a real non-zero entry (delivered);
  exact-rate accuracy is a pricing-snapshot maintenance concern beyond this slice
  (spec R-4). **Tracked: sworn#99.**

## Divergence from plan

- **`swornProviderConfig()` XAIKey read.** Design D4 said
  `XAIKey: os.Getenv("SWORN_XAI_API_KEY")` (SWORN_-only). Implemented as
  `envOrAlias("XAI_API_KEY", "SWORN_XAI_API_KEY")`, matching the sibling
  `GoogleKey` line in the same function. Reason: `FromEnv` passes this pcfg into
  `NewClient`, which reads `pcfg.XAIKey`; a SWORN_-only read would let a
  canonical-`XAI_API_KEY`-only user pass the key-presence gate (which uses
  `envOrAlias`) yet dispatch with an empty key. Using `envOrAlias` realises D4's
  stated intent end-to-end; the literal design line would have been a latent bug.
