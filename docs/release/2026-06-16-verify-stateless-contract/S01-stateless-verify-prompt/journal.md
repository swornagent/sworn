---
title: S01-stateless-verify-prompt journal
description: Implementation session notes for S01 — stateless verify prompt.
---

# Journal: `S01-stateless-verify-prompt`

## Session 2026-06-16 — Implementer session (initial)

### State transition

`planned → in_progress` (start) → `implemented` (end)

### Decisions

- **New prompt file**: Created `internal/prompt/verify-stateless.md` as a sworn-
  authored stateless judge prompt. It follows the `verifier.md` format (frontmatter
  + sections) for consistency but with the key difference: no tools, no repo, no
  test execution — SPEC+DIFF+PROOF only, verdict-leading reply.
- **Embed + accessor**: Added `verify-stateless.md` to the existing `go:embed`
  directive in `prompt.go` alongside `verifier.md` et al. Added `VerifyStateless()`
  accessor mirroring the existing `Verifier()` / `Implementer()` / `Planner()` /
  `Captain()` shape.
- **verify.go switch**: Changed `systemPrompt` from `prompt.Verifier()` to
  `prompt.VerifyStateless()`. This is a one-line change in the package-level var.
  `verify.Run` is the single consumer of `systemPrompt` — both the `sworn verify`
  CLI path and the `sworn run` verify gate go through it, so this fixes both.
- **`verifier.md` retained**: `prompt.Verifier()` still returns the full vendored
  Baton verifier role prompt. It stays embedded and its accessor is unchanged. It
  is simply no longer the verify-path system prompt. Orcharding is a recorded
  intake decision.
- **`INCONCLUSIVE` preserved**: The new prompt retains the BLOCKED-vs-INCONCLUSIVE
  distinction from `verifier.md`, with clear definitions of each.

### Test design trade-offs

- **`TestVerifyStateless_NotAgenticVerifier`**: Changed the agentic-token list
  from bare `"tool"` (which triggered a false positive on "no tool-call syntax"
  in the prompt) to more specific tokens: `"walk a worktree"`, `"git worktree"`,
  `"git -C"`, `"run tests"`, `"fresh terminal"`, `"Baton verifier"`,
  `"investigating agent"`. These are affirmative agentic instructions, not
  negations.
- **Integration test**: Added `capturingVerifier` in `verify_test.go` that records
  the `systemPrompt` argument it receives. `TestRun_SystemPromptIsStateless`
  asserts the captured prompt contains stateless markers and does NOT contain
  agentic tokens. This exercises the real `verify.Run` entry point (Rule 1).

### Out-of-scope discoveries

None. No track collisions (all files in T1's matrix).