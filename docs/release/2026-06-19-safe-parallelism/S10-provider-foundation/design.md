---
title: Design TL;DR
description: Implementer design decisions prior to code. Captain reviews this.
---

# Design TL;DR: `S10-provider-foundation`

## §1. User-visible change

A developer sets `OPENAI_API_KEY=sk-...` or `GROQ_API_KEY=gsk_...` in
`~/.sworn/.env` and configures `verifier.model = "groq/llama-3.3-70b"` in
`sworn-config.json`. When `sworn run` executes, it loads the `.env` file,
resolves the model ID prefix against a provider routing table, picks the correct
OAI-compatible base URL (`https://api.groq.com/openai/v1`), and dispatches
verification calls. The existing `SWORN_OPENAI_API_KEY` env var continues to
work as a fallback. Native-driver prefixes (`anthropic/*`, `google/*`,
`bedrock/*`, `azure/*`, `oci/*`) return a clear `ErrDriverNotRegistered` error
naming the future slice that will add them. On credential or credit failures,
the error message is actionable (`sworn account buy`, "check the API key for
<provider>") instead of raw provider JSON.

## §2. Design decisions not in spec (max 5)

1. **`model.FromEnv()` refactoring, not replacement.** The spec says
   `cmd/sworn/run.go` should call `NewClient(modelID, ProviderConfigFromEnv())`,
   but `cmd/sworn/run.go` doesn't instantiate `Verifier` directly — it calls
   `config.ResolveVerifierModel()` (returns a string), and `model.FromEnv()` in
   `internal/run/run.go` (and `cmd/sworn/verify.go`, `cmd/sworn/reqverify.go`)
   creates the `Verifier`. I'm refactoring `FromEnv()` in `config.go` to
   delegate its direct-provider path to `NewClient()` while preserving proxy
   routing (S06b). Zero callers change; the new router is transparent.
   *Rationale*: avoids touching three caller sites, preserves S06b proxy logic.

2. **OpenRouter multi-slash model IDs work without special-casing.** The
   existing `parseModelID()` splits on the first `/`, so
   `openrouter/anthropic/claude-sonnet-4-6` yields `provider="openrouter"`,
   `model="anthropic/claude-sonnet-4-6"` — exactly the sub-path OpenRouter
   expects. No new parsing logic required.
   *Rationale*: spec risk #2 is pre-resolved by the existing parser.

3. **`.env` load order: CWD first, then `~/.sworn/.env`.** Since
   `LoadDotEnv()` only calls `os.Setenv` when the key is unset, loading CWD
   first means CWD keys "stick" and home keys are skipped on collision. This
   achieves the spec's stated goal ("CWD wins — local project keys override
   global user keys") but uses the opposite load order from the spec's
   ambiguous suggestion. *Rationale*: the semantic contract "CWD wins" is
   what matters; the implementation detail of load order follows from the
   `os.Setenv`-only-if-unset guard. Code comment documents this explicitly.

4. **Error taxonomy is additive (still satisfies `error`).** `oai.go` on non-2xx
   returns a `*model.Error` that wraps the HTTP status via `ClassifyHTTP` and
   implements both `error` and `Unwrap()`. Existing `err != nil` callers are
   unaffected; callers that want typed handling opt in via `errors.As`. The 402
   special case (S06b `ErrInsufficientCredits`) is preserved and mapped to
   `KindCredits`. *Rationale*: the spec's "existing `err != nil` callers are
   unaffected" constraint demands backward compatibility.

5. **`ProviderConfig` uses standard env var names, not `SWORN_*` prefix.**
   `ProviderConfigFromEnv()` reads `OPENAI_API_KEY`, `DEEPSEEK_API_KEY`, etc.
   The `SWORN_OPENAI_API_KEY` alias is checked only as a fallback when the
   canonical key is empty. The existing `SWORN_*` prefix system in `config.go`
   is preserved for backward compatibility in the refactored `FromEnv()` path
   (which maps old env vars to the new ProviderConfig before calling
   `NewClient`). *Rationale*: the spec explicitly requires this — canonical
   keys win, alias is fallback only (per spec Risk #1).

## §3. Files I'll touch grouped by purpose

- **ADR + policy update** — `docs/adr/0004-dep-policy-minimal-justified.md`
  (new), `CLAUDE.md` (edit). Supersedes ADR-0001's "zero runtime deps" rule.
  *Why*: the spec requires a formal ADR before any provider SDK dep can be
  added; ADR-0004 establishes the "minimal, justified" rule that S11-S16
  drivers will invoke.

- **`.env` loader** — `internal/model/env.go` (new), `internal/model/env_test.go`
  (new). `LoadDotEnv()` loads `~/.sworn/.env` + CWD `.env`, skips comments and
  blank lines, never overwrites already-set env vars. *Why*: zero new deps
  (stdlib `os`, `bufio`, `strings`); this is the convenience layer the spec
  requires.

- **Provider router** — `internal/model/provider.go` (new),
  `internal/model/provider_test.go` (new). `ProviderConfig` struct,
  `NewClient(modelID, cfg)`, `ProviderConfigFromEnv()`, `ErrDriverNotRegistered`.
  *Why*: replaces the single-provider `FromEnv()` with prefix-based dispatch;
  S11-S16 register here.

- **Error taxonomy** — `internal/model/errors.go` (new),
  `internal/model/errors_test.go` (new). `ErrorKind`, `Error` (implements
  `error`+`Unwrap`), `ClassifyHTTP`, `IsTerminal`, `IsTransient`,
  `UserMessage`. *Why*: callers can distinguish terminal from transient
  failures without string-matching; S44 consumes this.

- **OAI typed errors** — `internal/model/oai.go` (modify). Non-2xx path returns
  `*model.Error` instead of opaque `fmt.Errorf`. 402 special case preserved.
  *Why*: the existing `err != nil` contract is preserved; typed handling is
  opt-in via `errors.As`.

- **`FromEnv()` refactoring** — `internal/model/config.go` (modify). Direct
  provider path delegates to `NewClient()`; proxy path (S06b) unchanged. *Why*:
  transparent to all callers.

- **CLI glue** — `cmd/sworn/run.go` (modify). Call `LoadDotEnv()` at start;
  when a model call fails, unwrap via `errors.As` and print `UserMessage()`.
  *Why*: the integration point for user-visible error guidance.

## §4. Things I'm NOT doing

- **Not adding provider SDK deps to `go.mod`.** Each driver slice (S11-S16)
  adds its own dependency with an ADR entry per ADR-0004.
- **Not modifying `cmd/sworn/verify.go` or `cmd/sworn/reqverify.go`.** They
  call `model.FromEnv()` which delegates internally — no code change needed.
- **Not touching proxy routing (S06b).** The credential-trust boundary in
  `FromEnv()` is preserved as-is.
- **Not adding retry-by-Kind logic.** That's S44-feedback-driven-retry, which
  consumes the error taxonomy built here.
- **Not adding per-provider base URL config in `sworn-config.json`.** Env var
  + default is sufficient per spec.
- **Not implementing native drivers.** Anthropic, Google, Bedrock, Azure, OCI
  return `ErrDriverNotRegistered` until S11-S16.

## §5. Reachability plan

**Primary artefact:** `go test ./internal/model/...` — table-driven tests cover
every provider prefix (openai, groq, deepseek, mistral, openrouter, ollama,
cloudflare, github → non-nil Verifier; anthropic, google, bedrock, azure, oci,
unknown → ErrDriverNotRegistered). The OAI typed-error test uses an
`httptest.Server` returning 402 with JSON body; asserts `errors.As` yields
`KindCredits` and `UserMessage()` mentions `sworn account buy`.

**Smoke test (optional, key-dependent):** set `GROQ_API_KEY` in
`~/.sworn/.env`; run `sworn run --parallel --release <fixture>` with
`verifier.model = "groq/llama-3.3-70b"`; observe HTTP request to
`api.groq.com`. If no live key available (CI), the test-server integration
test verifies the correct base URL is used.

## §6. Open questions for the Coach

None.