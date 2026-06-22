---
title: 'S10-provider-foundation — provider router, OAI-compat presets, .env file loading'
description: 'ADR 0007 revises the dep policy to minimal+justified. A provider router dispatches model IDs like openai/gpt-4o, anthropic/claude-sonnet-4-6, groq/llama-3.3-70b to the correct driver. OAI-compat presets cover eight providers without new code. A .env file loader reads per-provider API keys from ~/.sworn/.env. cmd/sworn/run.go switches from direct OAI{} instantiation to the factory.'
---

# Slice: `S10-provider-foundation`

## User outcome

A developer sets `ANTHROPIC_API_KEY=...` in `~/.sworn/.env` and sets
`verifier.model = "anthropic/claude-sonnet-4-6"` in config.json; `sworn run` resolves
the model ID, loads the API key from the .env file, and dispatches to the correct driver.
For OAI-compat providers (Groq, Mistral, DeepSeek, OpenRouter, Ollama, Cloudflare,
GitHub Models), the same works by setting the provider's env var in `.env`.
Native drivers (Anthropic, Google, Bedrock, Azure, OCI) return a clear
`ErrDriverNotRegistered` error until their implementation slice lands (S11-S16).

## Entry point

`sworn run` model resolution path. Verifiable by running `sworn run` with a Groq or
DeepSeek model ID (pointing to an OAI-compat preset) and observing the verifier dispatch
with the correct base URL and API key.

## In scope

- **ADR 0007** at `docs/adr/0007-dep-policy-minimal-justified.md`: supersedes ADR-0001's
  "zero runtime deps" rule; new policy is "minimal, justified deps — each new dependency
  requires an ADR entry". Documents rationale (multi-provider model drivers require their
  providers' Go SDKs; reimplementing auth/streaming/error handling from scratch for AWS
  SigV4, OCI auth, Anthropic SSE is not minimal — using the official SDK is).
- **`CLAUDE.md` update**: replace "zero runtime dependencies — stdlib only" constraint
  with "minimal, justified deps — new dep requires an ADR (see ADR-0004)"
- **`internal/model/env.go`**: `.env` file loader (stdlib only, no new dep):
  - Loads `~/.sworn/.env` first, then `.env` in the current working directory
  (CWD wins on collision — local project keys override global user keys)
  - Skips blank lines and `#` comments; parses `KEY=VALUE` and `KEY="VALUE"` forms
  - Sets values into process env via `os.Setenv` only if the key is not already set
  (explicit env vars always win over .env file; .env is a convenience layer, not an
  override)
  - `LoadDotEnv() error` — called once at process start in main.go or run.go; idempotent
- **`internal/model/provider.go`**: provider router:
  - `type ProviderConfig struct` — holds per-provider API keys and optional base URL
    overrides
  - `NewClient(modelID string, pcfg ProviderConfig) (Verifier, error)` — dispatches by
    prefix:
    - `openai/*` → `&OAI{BaseURL: "https://api.openai.com/v1", Model: ..., APIKey: pcfg.OpenAIKey}`
    - `deepseek/*` → `&OAI{BaseURL: "https://api.deepseek.com/v1", ...}`
    - `groq/*` → `&OAI{BaseURL: "https://api.groq.com/openai/v1", ...}`
    - `mistral/*` → `&OAI{BaseURL: "https://api.mistral.ai/v1", ...}`
    - `openrouter/*` → `&OAI{BaseURL: "https://openrouter.ai/api/v1", ...}`
    - `ollama/*` → `&OAI{BaseURL: ollama host (default http://localhost:11434/v1), APIKey: "ollama"}`
    - `cloudflare/*` → `&OAI{BaseURL: "https://api.cloudflare.com/client/v4/ai/v1", ...}`
    - `github/*` → `&OAI{BaseURL: "https://models.inference.ai.azure.com", ...}`
    - `anthropic/*`, `google/*`, `bedrock/*`, `azure/*`, `oci/*` → `ErrDriverNotRegistered`
    - unknown prefix → `ErrDriverNotRegistered`
  - `ProviderConfigFromEnv() ProviderConfig` — reads per-provider API keys from env:
    `OPENAI_API_KEY`, `DEEPSEEK_API_KEY`, `GROQ_API_KEY`, `MISTRAL_API_KEY`,
    `OPENROUTER_API_KEY`, `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, `CLOUDFLARE_API_KEY`,
    `GITHUB_TOKEN`, `OLLAMA_HOST` (optional, no key), `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`,
    `AZURE_OPENAI_API_KEY`, `OCI_*` (OCI SDK standard env vars)
  - Backward-compat: also reads `SWORN_OPENAI_API_KEY` as an alias for `OPENAI_API_KEY`
    (existing env var, must not break current users)
- **`cmd/sworn/run.go`**: replace `&model.OAI{...}` instantiation with
  `model.NewClient(modelID, model.ProviderConfigFromEnv())` call (serialised via T1+T3 dep)
- `ErrDriverNotRegistered` sentinel error in `internal/model/provider.go`
- **`internal/model/errors.go`** (new) — typed provider-error taxonomy so callers can
  distinguish terminal from transient failures instead of string-matching opaque errors
  (replan 2026-06-21, Coach decision: see `docs/release/2026-06-19-safe-parallelism/intake.md`):
  - `type ErrorKind int` with `KindOther, KindAuth, KindCredits, KindRateLimit, KindUpstream, KindTransient`
  - `type Error struct { Kind ErrorKind; Status int; Provider, Model, Message string; Err error }`
    implementing `error` and `Unwrap()` (so existing `err != nil` callers are unaffected)
  - `ClassifyHTTP(status int, body []byte) ErrorKind` — 401/403→Auth, 402→Credits,
    429→RateLimit, 5xx→Upstream, else Other; lifts the provider's JSON `error.message` when present
  - `IsTerminal(err) bool` (Auth/Credits — retrying never helps) and
    `IsTransient(err) bool` (RateLimit/Upstream/Transient — backoff may help)
  - `(*Error).UserMessage() string` — actionable per Kind (Credits → "out of credits — run
    `sworn account buy` or top up your provider"; Auth → "provider rejected credentials —
    check the API key for <provider>")
- **`internal/model/oai.go`** (modify) — on non-2xx, return a `*model.Error` built from
  `ClassifyHTTP(resp.StatusCode, body)` instead of the opaque `fmt.Errorf("model: HTTP %d: %s")`.
  Still satisfies `error`, so `Verify`/`Chat` callers that only check `err != nil` keep working.
- **`cmd/sworn/run.go`** (modify) — when a model call fails, unwrap via
  `errors.As(err, &*model.Error)` and print `UserMessage()` so the user sees actionable
  guidance, not raw provider JSON.

## Out of scope

- Native Anthropic / Google / Bedrock / Azure / OCI driver implementations (S11-S16)
- Ollama native API (S16)
- TUI settings screen (S17)
- Adding provider SDK deps to go.mod — each driver slice adds its own dep
- Per-provider base URL config in config.json (env var + default is sufficient for now)
- Run-loop retry policy by error Kind (terminal fail-fast vs transient backoff) — that
  consumes this taxonomy and lands in **S44-feedback-driven-retry** (which now `depends_on` S10)

## Planned touchpoints

- `docs/adr/0007-dep-policy-minimal-justified.md` (new)
- `CLAUDE.md` (modify — dep policy line)
- `internal/model/env.go` (new)
- `internal/model/env_test.go` (new)
- `internal/model/provider.go` (new)
- `internal/model/provider_test.go` (new)
- `internal/model/errors.go` (new — typed error taxonomy)
- `internal/model/errors_test.go` (new)
- `internal/model/oai.go` (modify — return *model.Error on non-2xx)
- `internal/model/config.go` (modify — refactor FromEnv to delegate to NewClient)
- `cmd/sworn/run.go` (modify — LoadDotEnv + print Error.UserMessage; serialised by T1+T3 dep)
## Acceptance checks

- [ ] `docs/adr/0007-dep-policy-minimal-justified.md` is committed; it explicitly names
  ADR-0001 as the predecessor and states the new rule ("each new dep requires an ADR")
- [ ] `CLAUDE.md` no longer contains the phrase "zero runtime dependencies — stdlib only";
  updated text references ADR-0004
- [ ] `LoadDotEnv()` correctly sets env vars from a temp `.env` file; does not overwrite
  a key already present in the environment; skips blank lines and comments
- [ ] `NewClient("openai/gpt-4o", cfg)` returns a non-nil `Verifier` with no error
- [ ] `NewClient("groq/llama-3.3-70b", cfg)` returns a non-nil `Verifier` with no error
  (OAI-compat preset for Groq)
- [ ] `NewClient("deepseek/deepseek-chat", cfg)` returns a non-nil `Verifier` (DeepSeek preset)
- [ ] `NewClient("openrouter/anthropic/claude-sonnet-4-6", cfg)` returns a non-nil Verifier
  (OpenRouter prefix — the model ID after `openrouter/` is passed through as-is to the
  OpenRouter endpoint)
- [ ] `NewClient("anthropic/claude-sonnet-4-6", cfg)` returns `ErrDriverNotRegistered` —
  the native driver is not yet registered; the error message names the slice that adds it
- [ ] `NewClient("unknown/model", cfg)` returns `ErrDriverNotRegistered`
- [ ] `model.ClassifyHTTP` maps 401/403→KindAuth, 402→KindCredits, 429→KindRateLimit,
  503→KindUpstream, 418→KindOther (table-driven)
- [ ] `model.IsTerminal` is true for Auth/Credits and false for RateLimit/Upstream/Transient;
  `model.IsTransient` is the converse
- [ ] `oai.go` returns a `*model.Error` on a non-2xx response (verified via test server
  returning 402 with a JSON error body; assert `errors.As` yields Kind=KindCredits and
  `UserMessage()` mentions `sworn account buy`); a plain `err != nil` check still passes
- [ ] `go test ./internal/model/...` passes with zero failures; no new external deps in
  go.mod (`go build ./...` succeeds without `go get`)
- [ ] A smoke run with `GROQ_API_KEY` set and verifier model `groq/llama-3.3-70b`
  produces a real API response (or, if no live key in CI, the unit test verifies the
  correct base URL is used in the HTTP request via a test server)

## Required tests

- **Unit** `internal/model/env_test.go`:
  - `TestLoadDotEnv_SetsUnsetKeys`: temp .env file with two keys; one already in env;
    assert the unset key is now set; the already-set key is unchanged
  - `TestLoadDotEnv_SkipComments`: comments and blank lines produce no error and no env
  - `TestLoadDotEnv_CWDWins`: ~/.sworn/.env sets KEY=global; .env in CWD sets KEY=local;
    assert local wins (set both before LoadDotEnv is called? No — load order: home first,
    then CWD. CWD wins because it's loaded second and overwrites.)
    *Correction*: since we use `os.Setenv` only if key not already set, home is loaded
    first (sets key), then CWD would not overwrite. Reconsider: load CWD first, then
    home — CWD wins because it's loaded first and sets the key; home sees key already set
    and skips. Implementer: pick one order and document it clearly in a code comment.
- **Unit** `internal/model/provider_test.go`:
  - `TestNewClient_OAICompat`: verify openai, groq, deepseek, mistral, openrouter,
    ollama, cloudflare, github all return non-nil Verifier; use table-driven test
  - `TestNewClient_NativeStub`: anthropic, google, bedrock, azure, oci return
    `ErrDriverNotRegistered`
  - `TestNewClient_Unknown`: unknown prefix returns `ErrDriverNotRegistered`
  - `TestProviderConfigFromEnv`: set env vars for a subset of providers; assert fields
    populated correctly; assert `SWORN_OPENAI_API_KEY` maps to `OpenAIKey`
- **Unit** `internal/model/errors_test.go`:
  - `TestClassifyHTTP`: table-driven status→Kind mapping (401,402,403,429,500,503,418)
  - `TestIsTerminalIsTransient`: Auth/Credits terminal; RateLimit/Upstream/Transient transient
  - `TestErrorUserMessage`: Credits message names `sworn account buy`; Auth names the provider key
  - `TestOAIReturnsTypedError`: test server returns 402 JSON error → `oai.Verify` error
    unwraps (`errors.As`) to `*model.Error{Kind:KindCredits}`
- **Reachability artefact**: smoke step — set `GROQ_API_KEY` in `~/.sworn/.env`; run
  `sworn run` on a fixture release with `verifier.model = "groq/llama-3.3-70b"`; observe
  HTTP request going to `api.groq.com`. Acceptable alternative: test-server integration
  test that verifies the correct base URL is called.

## Risks

- `SWORN_OPENAI_API_KEY` backward-compat alias: if `ProviderConfigFromEnv` checks the
  alias after the canonical key, an existing user who set `SWORN_OPENAI_API_KEY` will
  be silently broken if they also set `OPENAI_API_KEY` to something else. Resolution:
  canonical key wins; alias is fallback only.
- OpenRouter model ID format: OpenRouter uses `provider/model` sub-paths, so a full
  model ID might be `openrouter/anthropic/claude-sonnet-4-6`. The router must strip the
  `openrouter/` prefix and pass the remainder verbatim to the OAI endpoint's `model`
  field. Document this in a code comment in provider.go.
- `.env` file in CWD: if `sworn run` is called from an unrelated directory, `.env`
  files there may inject unexpected keys. Document in ADR-0004 as an acknowledged
  trade-off of convention-based loading.

## Deferrals allowed?

No. Every subsequent provider-driver slice (S11-S16) registers into the factory created
here. S17 (TUI settings) also depends on the ProviderConfig struct. Any deferral here
blocks the entire T5 track.
