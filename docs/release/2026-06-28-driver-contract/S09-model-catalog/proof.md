# Proof bundle — S09-model-catalog

Rendered from `proof.json` (schema `proof-v1`). See that file for the
machine-readable record; this is the human-readable summary.

## Scope

Running `sworn models` lists, per linked provider, the models actually
available on the user's account — grouped by the resolution prefix the user
would type — with capability annotations sourced only from wire-reported
metadata (tools: yes/no/unknown), so explicit-prefix resolution (S05) ships
with its discoverability counterpart and unknown never counts as capable.

## Files changed

- `cmd/sworn/models.go` (new) — the `sworn models [--provider <prefix>]` verb
- `cmd/sworn/models_test.go` (new)
- `internal/model/catalog.go` (new) — `ListCatalog` + 7 per-provider clients
- `internal/model/catalog_test.go` (new)
- `docs/release/2026-06-28-driver-contract/S09-model-catalog/{status.json, design.md, journal.md, review.md, captain-proceed.md}`

## Test results

| Command | Result |
|---|---|
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `gofmt -l` (all 4 new .go files) | PASS (clean) |
| `go test -count=1 ./cmd/sworn/... ./internal/model/...` (spec.json AC-04's named command) | PASS |
| `go test -count=1 -v -run Catalog ./internal/model/...` | PASS — 10 tests/subtests |
| `go test -count=1 -v -run Models ./cmd/sworn/...` | PASS — 6 tests |
| `go test -count=1 -timeout 300s ./...` (full suite) | PASS — 47 packages ok, 0 FAIL, zero regressions |
| `sworn verify` (deterministic first-pass, proof-bundle gate) | PASS, `cost_usd: 0` — see `proof.json` divergence for the verifier-model credential workaround |

## Reachability artefact

`TestModelsCommand` (`cmd/sworn/models_test.go`) — PASS.

Drives the registered `models` command's `Run` function end to end through
`command.Lookup("models").Run(nil)`, the exact integration point
`main.dispatch` resolves at runtime (Rule 1) — not a leaf
`internal/model/catalog.go` unit test. All 7 providers are configured via env
vars; the 6 HTTP-based providers are redirected to a combined fixture server
through a host-routing test `Transport` (the `modelsHTTPClient` seam in
`cmd/sworn/models.go`), while Ollama's fixture is reached directly via
`OLLAMA_HOST`. Asserts the rendered stdout carries every provider's
grouped-by-prefix block with the correct wire-sourced `tools` annotation
(AC-01, AC-02).

Supporting reachability at the same integration point:

- `TestModelsCommand_ProviderFilter` — `--provider mistral` restricts output
  to exactly one provider.
- `TestModelsCommand_AllFailedExitsNonZero` / `TestModelsCommand_PartialFailureExitsZero`
  — AC-03's per-provider isolation and exit-code rule.
- `TestModelsCommand_UnknownProviderFlag` — an unsupported `--provider` value
  is a usage error (exit 64) rejected before any HTTP call.

Manual smoke step (not run this session — no live provider credentials in
this implementer environment): `sworn models --provider mistral` against a
real `MISTRAL_API_KEY` would print the account's actual Mistral models with
real capability annotations.

## Delivered

See `proof.json` `delivered` for the full per-AC breakdown with evidence
citations (AC-01 through AC-04, D1–D4, and all five Coach pin dispositions
from `captain-proceed.md`).

## Not delivered

- **Pricing display** (OpenRouter's wire-honest `pricing` block) — not
  implemented. No AC requires it, and `PriceForModel` is keyed by the
  registry's fully-resolved `provider/model` ID, not catalog's raw
  per-provider wire IDs — wiring it in would need its own normalisation
  pass. Tracking: `sworn#92` (filed and confirmed at design review).
  Acknowledgement: Coach-ratified proceed, `captain-proceed.md` pin 3.
- Active capability probing, registry/resolution changes, catalog
  caching/auto-refresh — all out of scope per `spec.json`'s own list; no
  work attempted.

## Divergence from plan

See `proof.json` `divergence` for the full text. Summary: `cmd/sworn/main.go`
correctly untouched (self-registration precedent, accepted at design review);
`design.md`'s HTTP-client-convention section corrected per Coach pin 1
(anthropic.go uses `anthropic-sdk-go`, an ADR-0007 exception — documentation
fix only, no code impact); the Ollama-always-attempted test points at an
explicit closed port instead of the env-default host, because this dev
machine runs a real local Ollama daemon (discovered during implementation) —
same behaviour under test, environment-independent assertion; Ollama's
`/api/show` call is implemented as the real API's documented POST+body shape
rather than design.md's imprecise "GET" table prose (mechanical correctness
fix, not a design change); `sworn lint coverage` and `sworn llm-check` could
not run to completion in this environment (spec.md false-negative hazard;
zero configured model credentials) — declared, not contorted around; `sworn
verify` required a `--verifier-model` + dummy key workaround since the
deterministic path never dispatches it; this proof bundle's
`delivered`/`not_delivered`/`divergence` arrays use the same `{item,
evidence}` object convention every other slice in this release uses, which a
strict schema check confirms **every** slice's proof.json — including
already-verified S08 — also fails against the embedded `proof-v1.json`
schema's plain-string-array shape (pre-existing repo-wide drift, not
specific to this slice).
