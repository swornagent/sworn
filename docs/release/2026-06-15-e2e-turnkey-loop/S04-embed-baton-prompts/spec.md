---
title: S04-embed-baton-prompts
description: Embed the Baton planner/implementer/verifier role prompts in the binary via go:embed.
---

# Slice: `S04-embed-baton-prompts`

## User outcome

The planner/implementer/verifier role prompts are embedded in the `sworn` binary
(from the open Baton protocol), so the loop runs with no external prompt files —
the inline placeholder in `verify` is replaced by the real Baton verifier prompt.

## Entry point

Internal `prompt` package (embedded), consumed by verify (S01/S02) and the
implementer (S06).

## In scope

- Vendor the Baton role-prompt content into the repo (an embedded directory).
- `go:embed` the prompts; a small registry (`prompt.Verifier()`, etc.).
- Replace the inline `systemPrompt` placeholder in `internal/verify`.
- Record the vendored Baton protocol version.

## Out of scope

- Authoring/changing the prompts (they live in the Baton protocol).

## Planned touchpoints

- `internal/prompt/` (+ embedded `.md` files), `internal/verify/verify.go`

## Acceptance checks

- [ ] The verifier uses the embedded Baton verifier prompt (not the placeholder).
- [ ] The build embeds the files (no runtime file read for prompts).
- [ ] The vendored Baton version is recorded and surfaced (`sworn version` or a
      build var).

## Required tests

- **Unit**: assert each embedded prompt is non-empty and the verifier prompt
  contains the PASS/FAIL/BLOCKED verdict-contract instruction.

## Risks

- Prompt drift vs the Baton protocol — pin + record the vendored version.

## Deferrals allowed?

No.
