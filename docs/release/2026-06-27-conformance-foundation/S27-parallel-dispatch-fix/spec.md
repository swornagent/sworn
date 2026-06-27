---
title: 'S27 — Parallel dispatch fix: nil agent/verifier factories + agent-loop content tag'
description: 'Make sworn run --parallel actually dispatch an agentic implementer: default the agent/verifier factories in RunSlice (the parallel path left them nil → SIGSEGV) and stop dropping the required content field on tool-only agent turns (broke the multi-turn loop on every provider). Surfaced by the 2026-06-28 three-model dogfood.'
---

# Slice: `S27-parallel-dispatch-fix`

## User outcome

`sworn run --parallel` can dispatch an agentic implementer and run a multi-turn tool session
end-to-end. Before this slice the parallel path crashed with a nil-pointer panic before any model was
contacted, and — once that was worked around — every provider rejected the multi-turn request because
the assistant's `content` field was dropped on tool-only turns. After this slice the parallel loop
reaches the model, runs tools across turns, and fails only for real reasons (model errors, verdicts),
not engine wiring.

## Background

Found by the 2026-06-28 three-model dogfood (`docs/captures/2026-06-28-sworn-eval-findings.md`). Two
root-cause bugs blocked the entire autonomous loop:
1. `cmd/sworn/run.go` parallel `runSliceFn` constructs `RunSliceOptions` without `NewAgent`/`NewVerifier`.
   `run.Run` (single-slice) defaults them when nil (`run.go:107-111`); `RunSlice` did not, and
   dereferenced them at the design-TL;DR dispatch and the verify step → two SIGSEGVs. The parallel CLI
   path had therefore never worked outside tests (which inject fake factories).
2. The shared request struct `model.ChatMessage.Content` was tagged `json:"content,omitempty"`. On a
   tool-call turn with no prose (`Content == ""`) the field was omitted; DeepSeek rejected
   "missing field content", OpenAI rejected "content: expected a string, got null". This killed every
   multi-turn implement session across all OpenAI-compatible drivers.

## Entry point

`sworn run --parallel --release <name>` → `internal/run/parallel.go` → `internal/scheduler/worker.go`
→ `cmd/sworn/run.go` `runSliceFn` → `internal/run/slice.go` `RunSlice` → `internal/agent/agent.go`
(multi-turn loop) → `internal/model/oai.go` (request serialization).

## In scope

- `internal/run/slice.go` `RunSlice`: when `opts.NewAgent`/`opts.NewVerifier` are nil, default them to
  `newAgentFromModel`/`newVerifierFromModel` (mirroring `run.Run`), before first use.
- `internal/model/oai.go`: change `ChatMessage.Content` tag from `json:"content,omitempty"` to
  `json:"content"` so the field is always emitted (as `""` on tool-only turns).

## Out of scope

- The openai-responses (Responses API) `input[].output` multi-turn bug (separate driver; codex path).
- The 25-turn cap, retry-worktree-reset, openai-only escalation cascade, cold-start bootstrap, and the
  other dogfood findings — each tracked separately (see findings doc / FT-1/FT-2/FT-3).
- Wiring `cmd/sworn/run.go` runSliceFn to pass factories explicitly (the RunSlice default is the
  canonical fix and also protects every other caller).

## Planned touchpoints

- `internal/run/slice.go` (factory defaulting)
- `internal/model/oai.go` (content tag)
- `internal/run/factory_default_test.go` (new — nil-factory no-panic regression test)
- `internal/model/content_tag_test.go` (new — content-always-emitted serialization test)

## Acceptance checks

- [ ] WHEN `RunSlice` is invoked with `RunSliceOptions` whose `NewAgent` or `NewVerifier` is nil, THE
  SYSTEM SHALL default the nil factory to its production constructor before first use, and SHALL NOT
  panic.
- [ ] WHEN the agent serializes an assistant message that carries tool_calls and an empty text content,
  THE SYSTEM SHALL include the `content` field in the request JSON as an empty string (not omit it,
  not emit null).
- [ ] WHEN an assistant message carries non-empty text content, THE SYSTEM SHALL preserve that content
  in the serialized request.
- [ ] The full `internal/model/...` and `internal/run/...` suites SHALL pass (no regression).

## Required tests

- **Unit**: `internal/model/content_tag_test.go` — marshal a tool-only assistant turn, assert
  `"content":""` present; marshal a text turn, assert content preserved.
- **Unit**: `internal/run/factory_default_test.go` — call `RunSlice` with nil factories + an
  unconfigured model (`bogus/none`, errors synchronously, no network); assert it returns an error and
  does not panic (reaching the assertion proves no nil-deref at the design/verify use-sites).
- **Reachability artefact**: the 2026-06-28 dogfood run log — before the fix `sworn run --parallel`
  SIGSEGV'd at `slice.go` design-TL;DR / verify; after the fix it dispatched the implementer and ran
  multi-turn tool sessions (DeepSeek reached `verifying`). `go test ./internal/run/... ./internal/model/... ` exits 0.

## Risks

- Always emitting `content` slightly enlarges request payloads (empty strings); negligible.
- `bogus/none` test depends on `model.FromEnv` erroring synchronously for unknown providers (it does).

## Deferrals allowed?

No.
