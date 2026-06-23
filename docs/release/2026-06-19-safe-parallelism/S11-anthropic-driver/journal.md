---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S11-anthropic-driver`

## Session log

### 2026-07-07 — Implementation (session 1)

Entered design_review state with approved-ack.md (PROCEED verdict, 4 pins). Transitioned to in_progress, then implemented the Anthropic driver.

**Decisions made:**
- Pinned `anthropic-sdk-go` to v1.51.1 (latest stable), satisfying Pin 2 + spec Risk 1.
- Used `option.WithAPIKey` to explicitly pass the API key, not relying on env-var credential chain.
- Error extraction (Pin 3): the SDK's error type `*apierror.Error` is in an internal package. Instead of reflection, the status code is parsed from the formatted error string (`'<METHOD> "<URL>": <CODE> <TEXT> ...'`). The approach is documented in a comment naming the internal type.
- Pricing (Pin 2/Decision 2): `anthropicPricing` table mirrors the OAI pattern with three known models (opus-4-8: $15/$75, sonnet-4-6: $3/$15, haiku-4-5: $1/$5 per 1M tokens). Unknown `claude-*` models return zero cost.
- Provider router: replaced the `ErrDriverNotRegistered` stub with `NewAnthropic(model, pcfg.AnthropicKey)`. Updated `TestNewClient_NativeStub` to remove Anthropic from the stub list.

**Trade-offs:**
- String-based status code extraction is fragile if the SDK changes its error format, but it's isolated in `anthropicStatusCode()` and the worst case is the error falls through as KindOther.
- No live integration test run (no ANTHROPIC_API_KEY in this session). This is spec-allowed.

**Coach pins addressed:**
- Pin 1: OAI-import segregation — comment at top of anthropic.go.
- Pin 2: Minor-version pin — v1.51.1 in go.mod.
- Pin 3: SDK error extraction — `anthropicStatusCode()` with comment naming `*apierror.Error`.
- Pin 4: Kind assertion — `TestAnthropicVerify_APIError` asserts `KindRateLimit` via `errors.As`.

**Skeptic panel:** skipped — runtime does not support subagent dispatch.

## Open questions

None.

## Deferrals surfaced

- Live integration test skipped (no `ANTHROPIC_API_KEY`). Spec-allowed deferral ("Deferrals allowed?" section).

## Verifier verdicts received

*(None yet.)*