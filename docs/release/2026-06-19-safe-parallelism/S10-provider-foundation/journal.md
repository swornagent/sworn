---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S10-provider-foundation`

## Session log

### 2026-06-23 — Implementer session

**State transition: design_review → in_progress → implemented**

Entering with `design_review` state. `approved-ack.md` present — Coach approved with 4 pins (applied before code):

1. ADR renamed 0004→0007 (all references updated in spec.md, design.md, status.json, CLAUDE.md)
2. status.json planned_files extended with errors.go, errors_test.go, oai.go, config.go
3. config.go added to spec.md Planned touchpoints and status.json planned_files
4. ADR-0007 body includes CWD .env acknowledged trade-off section

**Implementation delivered:**

- **ADR-0007** (`docs/adr/0007-dep-policy-minimal-justified.md`): Supersedes ADR-0001 zero-runtime-deps rule with "minimal, justified deps — each new dep requires an ADR entry." Pre-ratifies SDKs for S11-S16. Documents CWD .env trade-off (Coach pin 4).
- **CLAUDE.md**: "zero runtime dependencies" → "minimal, justified deps" with ADR-0007 reference.
- **`internal/model/env.go`**: `.env` loader. Load order: CWD `.env` first, then `~/.sworn/.env` (CWD wins via set-only-if-unset guard — design decision #3). Skips comments, blank lines. Quote-stripping.
- **`internal/model/env_test.go`**: 5 tests covering key collision, comments, CWD-wins, quoted values, idempotent.
- **`internal/model/errors.go`**: `ErrorKind` enum (Auth/Credits/RateLimit/Upstream/Transient/Other), `Error` struct (implements `error`+`Unwrap`), `ClassifyHTTP`, `IsTerminal`, `IsTransient`, `UserMessage`, `AsError`, `NewProviderError`.
- **`internal/model/errors_test.go`**: 11 tests covering status→Kind mapping, terminal/transient classification, Unwrap chain, UserMessage content, AsError direct+wrapped+nil+non-Error, NewProviderError JSON body parsing.
- **`internal/model/provider.go`**: `ProviderConfig` struct, `ProviderConfigFromEnv()` (reads canonical env vars + SWORN_OPENAI_API_KEY alias fallback), `NewClient()` dispatches 8 OAI-compat providers by prefix with preset base URLs, native drivers return `ErrDriverNotRegistered`.
- **`internal/model/provider_test.go`**: 12 tests covering all 8 OAI-compat providers, Ollama default+override, 5 native stubs, unknown prefix, OpenRouter sub-path passthrough, ProviderConfigFromEnv with canonical+alias+canonical-wins, empty/invalid model IDs.
- **`internal/model/oai.go`**: Both Verify and Chat non-2xx paths now return `*model.Error` via `NewProviderError`. 402 wraps `account.ErrInsufficientCredits` inside the typed error. All existing `err != nil` callers unaffected.
- **`internal/model/oai_test.go`**: Updated azure test cases → groq equivalents (azure is now a native driver stub). Preserved all existing test behavior.
- **`internal/model/config.go`**: `FromEnv()` refactored — direct-provider path now delegates to `NewClient()` via `swornProviderConfig()` (reads SWORN_* env vars for backward compat). API key validation preserved. SWORN_*_BASE_URL override applied post-NewClient for backward compat. Proxy routing (S06b) unchanged.
- **`cmd/sworn/run.go`**: `LoadDotEnv()` called at start. `printModelError()` unwraps `*model.Error` via `errors.As` and prints `UserMessage()` for actionable errors.

**Tests: 42 passing, 0 failures.** `go build ./...` and `go vet ./...` clean.

**skeptic_panel: skipped** — runtime does not support subagent dispatch.

## Open questions

None.

## Deferrals surfaced

None — all 14 acceptance checks delivered.

## Verifier verdicts received

*(None yet — slice is implemented, awaiting fresh-context verification.)*