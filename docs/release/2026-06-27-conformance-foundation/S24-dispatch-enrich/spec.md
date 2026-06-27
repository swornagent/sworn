---
title: 'S24 — Enrich Dispatch record: duration, token split, real cost, model-id fix'
description: 'Add DurationMS, InputTokens, OutputTokens to state.Dispatch; populate real_cost_usd from the pricing map (S10); fix the model-id bug where Dispatch.Model records the configured id, not the response-confirmed id.'
---

# Slice: `S24-dispatch-enrich`

## User outcome

After a `sworn run` session, each `state.Dispatch` entry in status.json includes `duration_ms` (how long the dispatch took), `input_tokens` and `output_tokens` (from the model's usage response), `real_cost_usd` computed from actual tokens × pricing map (not always 0), and `model_id_confirmed` (the model ID returned in the response, not the configured/aliased ID).

## Entry point

`internal/state/state.go` `Dispatch` struct (lines ~80-108, DOCUMENTED SHARED: T7 owns this region; T4 S13 owns Write() §184); and the call sites in `internal/run/slice.go` that append to the `dispatches` slice.

## In scope

- `internal/state/state.go` (T7 section ~Dispatch struct): add fields to `Dispatch`:
  - `DurationMS int64 \`json:"duration_ms,omitempty"\``
  - `InputTokens int64 \`json:"input_tokens,omitempty"\``
  - `OutputTokens int64 \`json:"output_tokens,omitempty"\``
  - `ModelIDConfirmed string \`json:"model_id_confirmed,omitempty"\`` — the model ID from the response (not the configured alias)
- Update `internal/run/slice.go` dispatch-append call sites (search for `state.Dispatch{Role: "verifier"` and `state.Dispatch{Role: "implementer"`) to populate the new fields from the model response
- Call the pricing map from S10 (`model.PriceForModel(modelID)`) to compute `CostUSD` and populate the existing `CostUSD` field (not always 0)
- `model.Verifier.Verify()` return signature already returns `(text, costUSD, error)` — update callers to also capture token counts if the model response includes usage; update OAI/Anthropic Verify() to include usage in the return value (or add a new `VerifyWithUsage()` variant)

## Out of scope

- The durable event store (S25)
- Changes to the `sworn telemetry` output (S26)
- Adding new fields to status.json beyond Dispatch (that is schema evolution in S13)
- `model.Chat()` return value enrichment beyond what S10 already adds

## Planned touchpoints

- `internal/state/state.go` (T7 section: Dispatch struct lines ~80-108)
- `internal/run/slice.go` (add DurationMS, InputTokens, OutputTokens, ModelIDConfirmed to dispatch append calls)
- `internal/model/oai.go` (include usage in Verify() response — update signature or add usage capture)
- `internal/model/anthropic.go` (include usage in Verify() response)

## Acceptance checks

- [ ] `state.Dispatch` has `DurationMS`, `InputTokens`, `OutputTokens`, `ModelIDConfirmed` fields
- [ ] WHEN `sworn run` completes a verify dispatch, the written status.json `dispatches` array entry for `role: "verifier"` has `duration_ms > 0`
- [ ] WHEN an OAI dispatch returns `usage.prompt_tokens` and `usage.completion_tokens`, THE SYSTEM SHALL populate `input_tokens` and `output_tokens` respectively in the Dispatch record
- [ ] WHEN the pricing map has an entry for the dispatched model, THE SYSTEM SHALL populate `cost_usd` from `inputTokens * inputPrice + outputTokens * outputPrice` (not always 0)
- [ ] `state_test.go` (extend existing): test that a Dispatch with the new fields marshals and unmarshals correctly; no regression on existing dispatch fields

## Required tests

- **Unit**: `internal/state/state_test.go` extend — round-trip with new Dispatch fields
- **Reachability artefact**: `go test ./internal/state/... ./internal/run/... -v` exits 0

## Risks

- `model.Verifier.Verify()` signature change (adding usage) is a breaking change to the interface; consider adding a `VerifyWithUsage()` method or returning a struct. A backward-compatible approach: `Verify()` returns `(text, costUSD, inputTokens, outputTokens, error)` — callers that ignore extra returns still compile in Go
- `internal/run/slice.go` (T3's documented shared region at §412) may be edited during this work; confine S24 edits to the dispatch-append lines (not the verifier call section at §412)

## Deferrals allowed?

No.
