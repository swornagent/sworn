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

### 2026-06-23 — Verifier session (fresh context)

**Verdict: BLOCKED**

Gates 1–5 passed. Gate 6 blocked on a spec internal inconsistency.

**Finding:** The spec acceptance check at line 115 reads:
> `CLAUDE.md` no longer contains the phrase "zero runtime dependencies — stdlib only"; updated text references ADR-0004

ADR-0004 (`0004-tui-deps-bubbletea-lipgloss.md`) is the TUI-dependency ADR, not the dep-policy ADR. The spec body (lines 31–33 and the planned touchpoints list) correctly says "ADR 0007" and "docs/adr/0007-dep-policy-minimal-justified.md". The Coach pin 1 (noted in the start_commit and journal) updated the spec description and body references but missed this acceptance check. The implementer followed the spec body correctly — CLAUDE.md references ADR-0007, the actual dep-policy ADR created by this slice.

Remediation via the implementer is not possible: changing CLAUDE.md to reference ADR-0004 would point to the TUI-deps ADR, which is factually wrong.

**Proposed spec.md amendment (for planner to ratify):**
In the Acceptance checks section, change:
```
updated text references ADR-0004
```
to:
```
updated text references ADR-0007
```

**Evidence summary (for completeness):**
- `go test ./internal/model/...` → 42 tests, 0 failures (run live)
- `go build ./...` + `go vet ./...` → clean
- All 8 OAI-compat presets dispatch correctly (TestNewClient_OAICompat)
- Native drivers return ErrDriverNotRegistered (TestNewClient_NativeStub)
- `LoadDotEnv()` sets, skips, and does not overwrite as specified (TestLoadDotEnv_* suite)
- `oai.go` returns `*model.Error` on non-2xx; `errors.As` yields KindCredits for 402 (TestNewProviderError)
- `cmd/sworn/run.go` calls `LoadDotEnv()` and `printModelError(err)` as specified
- No silent deferrals in changed source files

## Planner correction — 2026-06-23 (replan resolving the BLOCKED verdict)

**Actor**: planner (`/replan-release`)

The verifier BLOCKED on a spec acceptance-check inconsistency: the AC read "updated text
references ADR-0004", but ADR-0004 is the TUI-deps ADR (`0004-tui-deps-bubbletea-lipgloss.md`);
the dep-policy ADR is `0007-dep-policy-minimal-justified.md`. Ground truth (`docs/adr/`) confirms
0004 = tui-deps, 0007 = dep-policy, 0008 = canonical-baton. The implementation correctly cites
ADR-0007, so there was no legal implementer fix — a spec defect, owned by the planner.

This was the tail of the ADR-number-collision flagged 2026-06-21 (S10's planned `0004-dep-policy`
collided with the merged TUI ADR-0004; the implementation renumbered to 0007 but the spec AC + two
other refs lagged — partially fixed by Coach at start_commit, leaving the AC stale).

**Resolution**: all dep-policy ADR references in this spec corrected to 0007 (description, body,
CLAUDE.md text, the blocking AC, the env note, planned-touchpoint, AC-committed line); index.md
matrix rows + S10 `planned_files` corrected to 0007/0008; the collision note marked RESOLVED.
`verification.result` cleared to `pending`, `state` kept `implemented` (gates 1-5 already passed).
`start_commit` preserved. Next: a fresh `/verify-slice S10-provider-foundation` — no code change
required, the implementation already satisfies the corrected spec.