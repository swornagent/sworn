# Design TL;DR — S09-model-catalog

## User outcome (from spec.json)

Running `sworn models` lists, per linked provider, the models actually
available on the user's account — grouped by the prefix the user would type —
with capability annotations sourced only from wire-reported metadata (tools:
yes/no/unknown), so explicit-prefix resolution (S05) ships with its
discoverability counterpart and unknown never counts as capable.

Acceptance checks: 4 (AC-01..AC-04). Out of scope: active capability probing
via paid test calls, registry/resolution changes (S05 owns those), catalog
caching/auto-refresh.

## Grounding: what's landed that this slice builds on

- **`internal/driver/registry`** (S05, merged in T4): compiled-in table of
  **4** driver entries — `claude-cli`, `codex`, in-process Responses
  (`openai`), in-process chat (`openai-completions`, `deepseek`, `groq`,
  `mistral`, `openrouter`, `cloudflare`, `github`, `anthropic`). Its own
  header comment states plainly that `google`, `vertex`, `bedrock`, `azure`,
  `oci`, `ollama` are **deliberately not registered** — "verify-only
  providers... stay on the one-shot utility path." `registry.Default(cfg)`
  is the authority `cmd/sworn/capabilities.go` renders from, with
  `Registry.Drivers()` returning `Info{Available, Detail}` per entry via
  no-dispatch `Probe` functions (`keyProbe`/`binaryProbe`).
- **`internal/model.ProviderConfig`** (`internal/model/provider.go`): already
  carries a field for every one of this slice's 7 target providers —
  `OpenAIKey`, `GroqKey`, `MistralKey`, `OpenRouterKey`, `AnthropicKey`,
  `GoogleKey`, `OllamaHost` — populated by `ProviderConfigFromEnv()` from the
  canonical env vars (with `SWORN_*` alias fallback).
- **S08 unified pricing registry** (`internal/model/client.go`
  `PriceForModel`): a single cross-provider USD/1M-token lookup aggregating
  `modelPricing` (OAI-compat), `anthropicPricing`, `googlePricing`,
  `bedrockPricing`. Consulted for informational purposes only (see "Pricing
  is not part of this slice's contract" below) — not required by any AC.
- **HTTP client convention**: every provider driver in `internal/model`
  (`oai.go`, `ollama.go`, `anthropic.go`) uses stdlib `net/http` +
  `encoding/json` directly (AGENTS.md: no provider SDK for the model client
  path), with tests built on `httptest.NewServer` (`oai_test.go`
  `fakeServer`). `google.go` is the one exception — it uses the
  `google.golang.org/genai` SDK for **dispatch** (`Verify`), an
  already-justified dependency. This slice does **not** need SDK dispatch
  machinery — `models.list` is a plain authenticated GET — so it uses raw
  `net/http` for all 7 providers, including Google, to stay inside the
  zero-new-deps default and keep every provider's list-client testable the
  same way (`httptest.Server` + a transport recorder, matching AC-04's own
  test requirement).

## Design gap the spec text doesn't resolve: "registry enumeration" can't answer for 2 of the 7 providers

AC-01 says availability is "determined via registry enumeration/availability,
no dispatch." Read literally against `internal/driver/registry`, that
authority has no entries for `google` or `ollama` at all — the registry's own
header comment says so. `internal/model.ProviderConfig` and
`ProviderConfigFromEnv()`, by contrast, already cover all 7 target providers.
Extending `internal/driver/registry` to add Google/Ollama entries is
explicitly out of scope ("Registry/resolution changes (S05)").

**Decision (D1, see below): `catalog.go` runs its own no-dispatch
credential-presence check against `model.ProviderConfig`, for all 7
providers uniformly** — not a call into `internal/driver/registry.Drivers()`.
This satisfies AC-01's *intent* (no-dispatch availability determination
sourced from the same credential surface the registry itself reads) without
touching the registry package, and avoids splitting availability logic
between "5 providers via the registry, 2 providers via a side-channel" inside
the same command.

## Files to touch

- `internal/model/catalog.go` (new) — `CatalogModel`, `ToolSupport`,
  `CatalogResult`, `ListCatalog`, and one unexported `list<Provider>`
  function per provider (7).
- `internal/model/catalog_test.go` (new) — table-driven
  `TestCatalogAnnotations` (one canned fixture per provider class: OpenRouter
  `supported_parameters`, Mistral `capabilities`, Ollama `/api/show`,
  Google `supportedGenerationMethods`, bare-list OpenAI/Groq/Anthropic) plus
  a shared transport-recorder helper (AC-04).
- `cmd/sworn/models.go` (new) — `sworn models [--provider <prefix>]`,
  self-registers via `init()` + `command.Register` (per
  `cmd/sworn/main.go`'s own header: "Adding a new CLI command never edits
  this file" — `main.go` is **not** touched, despite being named in
  `spec.json` touchpoints; noted as a divergence below).
- `cmd/sworn/models_test.go` (new) — `TestModelsCommand`: canned-fixture
  end-to-end run, `--provider` filter, one-provider-fails-rest-continue
  (AC-03), all-attempted-fail exit code, grouped-by-prefix output shape
  (AC-01).

## Data shapes

```go
// internal/model/catalog.go

// ToolSupport is a fail-closed tri-state capability annotation. Unknown is
// never treated as capable (AC-02) — callers must not coerce it to false or
// true.
type ToolSupport string

const (
    ToolSupportYes     ToolSupport = "yes"
    ToolSupportNo      ToolSupport = "no"
    ToolSupportUnknown ToolSupport = "unknown"
)

// CatalogModel is one model entry from a provider's models/list endpoint,
// normalised across heterogeneous wire shapes. ID is the bare model name as
// the provider reports it (no resolution prefix — the caller prepends it).
type CatalogModel struct {
    ID    string
    Tools ToolSupport
}

// CatalogResult is one provider's outcome from ListCatalog: either a model
// list or a per-provider error (AC-03 — a provider failure never blocks the
// others).
type CatalogResult struct {
    Provider string // canonical resolution prefix, e.g. "openrouter"
    Models   []CatalogModel
    Err      error
}

// ListCatalog queries the models/list endpoint of every provider in cfg
// that has credentials configured (Ollama always attempted — see D3), plus
// an optional single-provider filter. No completion/dispatch/probe calls are
// made (AC-04). client defaults to http.DefaultClient when nil.
func ListCatalog(ctx context.Context, cfg ProviderConfig, client *http.Client, filter string) []CatalogResult
```

Iteration order is the fixed alphabetical list `anthropic, google, groq,
mistral, ollama, openai, openrouter` — diff-stable output, mirroring
`capabilities.go`'s `sort.Slice` discipline.

## Per-provider capability source (AC-02's table, made concrete)

| Provider | List endpoint | Auth | Tools signal | Annotation rule |
|---|---|---|---|---|
| OpenRouter | `GET {base}/api/v1/models` | `Authorization: Bearer <key>` | `supported_parameters: []string` per model | field present & contains `"tools"` -> Yes; field present & absent `"tools"` -> No; field missing -> Unknown |
| Mistral | `GET {base}/v1/models` | `Authorization: Bearer <key>` | `capabilities.function_calling: bool` per model | field present -> Yes/No from the bool; `capabilities` object missing -> Unknown |
| Ollama | `GET {host}/api/tags` for names, then `GET {host}/api/show` per model (D2) | none (local) | `capabilities: []string` per `/api/show` response | field present & contains `"tools"` -> Yes; present & absent -> No; missing (older daemon) -> Unknown |
| Google | `GET {base}/v1beta/models?key=<key>` | key in query string | `supportedGenerationMethods: []string` | **always Unknown for tools** (D4 — the rationale in spec.json confirms this field never carries an explicit tool-support signal; no wire-derivable Yes/No exists) |
| OpenAI | `GET {base}/v1/models` | `Authorization: Bearer <key>` | none (bare ID list) | always Unknown |
| Groq | `GET {base}/openai/v1/models` | `Authorization: Bearer <key>` | none (bare ID list) | always Unknown |
| Anthropic | `GET {base}/v1/models` | `x-api-key: <key>` + `anthropic-version` header | none (bare ID list) | always Unknown |

No provider's annotation is ever derived from the model ID string (R-02 /
AC-02's explicit heuristic ban) — only from a field present in that
provider's own list-endpoint JSON response.

## Availability / "configured" determination (D1, D3)

- `openai`/`groq`/`mistral`/`openrouter`/`anthropic`: configured iff the
  matching `ProviderConfig` key field is non-empty. Not attempted otherwise
  (no noisy 401s for providers the user never linked).
- `google`: configured iff `GoogleKey` is non-empty. (Vertex ADC routing is
  a distinct prefix, out of this slice's provider list per spec.json
  touchpoints — informational only, not attempted.)
- `ollama`: **always attempted** (D3) — Ollama is a keyless local daemon,
  mirroring the registry's own `claude-cli` treatment (binary-presence, not
  key-presence, gates availability). A connection failure (no local daemon
  running) surfaces as a normal per-provider AC-03 error, not a silent skip.
  This also removes any "zero providers attempted" edge case: there is
  always at least one attempted provider, so AC-03's "exit non-zero only
  when EVERY configured provider failed" has an unambiguous meaning in every
  environment.
- `--provider <prefix>` restricts `ListCatalog` to one provider regardless of
  its configured state; an unsupported/unknown prefix is a usage error (exit
  64, message enumerates the 7 valid prefixes), not a silent empty result.

## Command output shape (AC-01)

One block per attempted provider, sorted by prefix, models listed with their
resolution-prefixed ID and tools annotation — mirroring
`capabilities.go`'s per-driver block style for visual consistency across the
two discoverability verbs:

```
openrouter/ (2 models)
  openrouter/deepseek/deepseek-v3.2   tools: yes
  openrouter/qwen/qwen3-max           tools: unknown

anthropic/ (3 models)
  anthropic/claude-opus-4-6           tools: unknown
  ...

groq/: error: models.list returned 401 Unauthorized
```

Exit code: 0 unless every attempted provider errored (AC-03), in which case
1. `--provider` usage errors (unknown prefix) return 64 before any HTTP call.

## Pricing is not part of this slice's contract

The task brief names the S08 unified pricing registry (`PriceForModel`) as
something to design against. None of S09's 4 ACs mention price. OpenRouter's
`/api/v1/models` response does carry a `pricing` block, which would be a
free, wire-honest annotation to add later — but adding a `$/1M` column now
would be scope creep against an unwritten AC, and `PriceForModel` is keyed
by fully-resolved model ID (`provider/model`) built from `NewClient`'s
routing table, not from catalog's raw per-provider wire IDs, so wiring it in
would need its own normalisation pass. Left out of `catalog.go` entirely;
call out in `journal.md` as a Rule 2 deferral (why: no AC requires it, adding
it un-asked risks a second, unreviewed capability-shaped surface; tracking:
none filed — flag to the Coach at design review whether a follow-on issue is
wanted; acknowledgement: pending this design review).

## AC-04's transport recorder — what counts as an allowed "list" path

Every provider gets exactly one allowed path **except** Ollama, which gets
two (`/api/tags` then `/api/show`) per D2. The test's recorder is a
per-provider allowlist of exact paths; any request outside that allowlist
(in particular anything resembling a completion/chat/generate path) fails
the test immediately. This directly proves AC-04's "no
completion/dispatch/probe calls" for all 7 providers, not just by assertion
on the code but by failing on the wire if violated.

## Design decisions (Rule 9 self-classification — all Type-2)

- **D1** — `catalog.go` implements its own no-dispatch credential-presence
  check against `model.ProviderConfig` for all 7 providers, rather than
  reusing `internal/driver/registry.Drivers()`. *Rationale*: the registry
  only enumerates 4 of the 7 target providers (Google/Ollama structurally
  absent by the registry's own design); extending the registry is
  out-of-scope; a single uniform check across all 7 is simpler than splitting
  by provider. *Stake*: narrow (this command only), easily revisited if the
  registry is later widened. Default: proceed as designed.
- **D2** — Ollama capability lookup makes one `/api/show` call per listed
  model (N+1 against a local daemon), the only provider with 2 allowlisted
  list-shaped paths. *Rationale*: spec.json's own rationale names
  "Ollama `/api/show` capabilities" as the wire source; `/api/tags` alone
  carries no capability field. *Stake*: narrow, local daemon, low latency.
  Default: proceed as designed.
- **D3** — Ollama is always attempted (no credential gate); a daemon-down
  failure is a normal AC-03 per-provider error. *Rationale*: mirrors the
  registry's own `claude-cli` keyless-availability precedent. *Stake*:
  narrow. Default: proceed as designed.
- **D4** — Google's `tools` annotation is unconditionally `unknown`, never
  derived from `supportedGenerationMethods`. *Rationale*: spec.json's
  rationale states this field is "partial — tool support not explicit";
  there is no wire-derivable signal to annotate Yes/No from, so AC-02's
  fail-closed rule leaves only Unknown. *Stake*: narrow, matches spec intent
  verbatim. Default: proceed as designed.

None of D1–D4 touch the driver contract, the registry's resolution
semantics, or any other slice's surface — all four are local to this one new
package/command and reversible without a migration. Flagging all four for
the Captain's design review rather than asserting they need a full Type-1
human decision.

## Risks (from spec.json, restated with the concrete mitigation)

- **R-01** (shape drift/account-tier variance): every provider's JSON parse
  is defensive — unknown fields ignored (Go's default `encoding/json`
  behaviour with a struct target), and a shape that fails to parse at all
  degrades that one provider to an `Err` result (AC-03), never panics, never
  fails the whole command.
- **R-02** (name-heuristic temptation): explicitly rejected — the table above
  is the only place capability annotation is decided, and it never inspects
  `CatalogModel.ID`.

## Test plan

- `internal/model/catalog_test.go`:
  - `TestCatalogAnnotations` — table-driven, one canned-fixture case per
    provider (7 cases) asserting the exact `ToolSupport` value the table
    above specifies, including the two Mistral/OpenRouter absent-field ->
    Unknown edge cases and the always-Unknown Google/OpenAI/Groq/Anthropic
    cases.
  - `TestListCatalog_ProviderErrorIsolation` (AC-03) — one provider's server
    returns non-2xx, others succeed; asserts the failing provider's
    `CatalogResult.Err` is set and naming the provider/cause, and every other
    result is still populated.
  - `TestListCatalog_NoDispatchPaths` (AC-04) — shared transport recorder
    across all 7 providers, asserting no path outside each provider's
    allowlist is ever requested.
  - `TestListCatalog_OllamaAlwaysAttempted` (D3) — empty `ProviderConfig`
    still yields an Ollama `CatalogResult` (error, since no fixture server is
    at the default host in test) while the other 6 are absent from the
    result set entirely (not attempted, not errored).
- `cmd/sworn/models_test.go`:
  - `TestModelsCommand` — end-to-end through `cmdModels`, fixture servers
    injected, asserts grouped-by-prefix stdout shape (AC-01).
  - `TestModelsCommand_ProviderFilter` — `--provider mistral` restricts
    output to one provider.
  - `TestModelsCommand_AllFailedExitsNonZero` / `TestModelsCommand_PartialFailureExitsZero` (AC-03).
  - `TestModelsCommand_UnknownProviderFlag` — usage error, exit 64.

Required commands: `go build ./...`; `go test ./cmd/sworn/... ./internal/model/...`.

## Out of scope (restated from spec.json)

- Active capability probing via paid test calls.
- Registry/resolution changes (S05 territory).
- Catalog caching or auto-refresh.
- Pricing display (see "Pricing is not part of this slice's contract" above).
