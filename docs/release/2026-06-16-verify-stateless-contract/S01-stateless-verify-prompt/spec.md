---
title: S01-stateless-verify-prompt
description: Give the stateless verify path its own judge prompt instead of the agentic verifier.md role prompt.
---

# Slice: `S01-stateless-verify-prompt`

## User outcome

A developer runs `sworn verify --spec <p> --diff <p> --verifier-model <m>` and the
model is instructed as a **stateless judge** — "you have only the SPEC and DIFF,
no tools, no repo, no test execution; reply with a verdict as the very first
characters" — instead of being told (by `verifier.md`) that it is an investigating
agent that should walk a worktree and run tests it cannot reach. The model stops
emitting tool-call leakage / investigative preamble that the parser rejects.

## Entry point

CLI: `sworn verify` (`cmd/sworn`), via `internal/verify/verify.go` →
`v.Verify(ctx, systemPrompt, payload)`. The same `verify.Run` is the verify gate
inside `sworn run` (`internal/run/run.go:232`), so this slice fixes both.

## In scope

- A new sworn-authored stateless judge prompt embedded via `go:embed`
  (e.g. `internal/prompt/verify-stateless.md`) plus an accessor on the `prompt`
  package (e.g. `prompt.VerifyStateless()`), mirroring the existing
  `prompt.Verifier()` / `prompt.Implementer()` shape.
- Switching the verify path's system prompt source: `internal/verify/verify.go`
  `var systemPrompt` no longer reads `prompt.Verifier()`; it reads the new
  stateless accessor.
- The new prompt MUST:
  - State that ONLY the SPEC and DIFF (and optional PROOF) below are available —
    no tools, no repo, no test execution, no file reads.
  - Require the reply to **begin with exactly one** of `PASS`, `FAIL: <…>`,
    `BLOCKED: <…>`, `INCONCLUSIVE: <…>` as the very first characters — no preamble,
    no markdown, no tool calls, no code fences.
  - Preserve the four-verdict semantics and the **BLOCKED-vs-INCONCLUSIVE
    distinction** from `verifier.md` (BLOCKED = the slice's contract is the
    problem; INCONCLUSIVE = a determinate PASS/FAIL could not be reached).

## Out of scope

- The tolerant parser (`S02`) — this slice changes the prompt only; the parser
  stays as-is until S02.
- End-to-end `sworn run` proof (`S03`).
- Editing or removing `verifier.md` — it stays vendored and embedded (see intake
  decision). It simply stops being the verify-path system prompt.
- Structured output / `response_format` (deferred — see intake).

## Planned touchpoints

- `internal/prompt/verify-stateless.md` (new)
- `internal/prompt/prompt.go` (embed directive + accessor)
- `internal/verify/verify.go` (point `systemPrompt` at the new accessor)

## Acceptance checks

- [ ] `internal/verify/verify.go` no longer references `prompt.Verifier()`; its
      `systemPrompt` is sourced from the new stateless accessor.
- [ ] The new prompt is embedded (`go:embed`) and the binary still builds with
      zero added dependencies (`go build ./...`).
- [ ] The new prompt text explicitly states "no tools / no repo / SPEC+DIFF only"
      and "reply MUST begin with one of PASS/FAIL/BLOCKED/INCONCLUSIVE as the first
      characters".
- [ ] The four verdict tokens and the BLOCKED-vs-INCONCLUSIVE distinction are
      retained in the prompt wording.
- [ ] `prompt.Verifier()` still returns `verifier.md` verbatim (no mutation of the
      vendored artefact); a `prompt` package test asserts it is non-empty and
      unchanged in shape.

## Required tests

- **Unit**: `internal/prompt/prompt_test.go` — asserts the new accessor returns a
  non-empty embedded string and that `prompt.Verifier()` is still the vendored
  role prompt (unchanged).
- **Integration**: `internal/verify/verify_test.go` — a fake `model.Verifier`
  captures the `systemPrompt` it is handed by `verify.Run` and asserts it is the
  stateless prompt (contains the "SPEC+DIFF only / verdict-leading" marker), NOT
  the agentic `verifier.md` (does not contain its worktree/tool instructions).
  This exercises the real `verify.Run` entry point per Rule 1.
- **Reachability artefact**: `go test ./internal/prompt/... ./internal/verify/...`
  green, plus a one-line smoke note: build the binary and run
  `sworn verify --spec <synthetic> --diff <synthetic>` with the `Unconfigured`
  verifier — confirm it still reaches dispatch (BLOCKED on no model, not a build
  or wiring panic).
- **E2E gate type**: N/A (no Playwright).

## Risks

- **Orphaning `verifier.md` silently.** Mitigated: it stays embedded and a
  `prompt` test pins it; the orphaning is a recorded intake decision, not an
  inline deferral.
- **Prompt that pins format but loses verdict fidelity.** Mitigated: acceptance
  check requires the BLOCKED-vs-INCONCLUSIVE distinction to survive in wording.

## Deferrals allowed?

No.
