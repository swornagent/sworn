---
title: S02-tolerant-verdict-parser
description: Parse the verdict from the first non-empty line (markdown-tolerant), still fail-closed on ambiguity. Belt-and-braces behind S01.
---

# Slice: `S02-tolerant-verdict-parser`

## User outcome

A developer runs `sworn verify` and a reply that is *substantively* a verdict but
not byte-perfect — a markdown-emphasised `**FAIL**`, a leading blank line, or a
verdict preceded by a stray fence — still resolves to the intended verdict instead
of `BLOCKED / unparseable_verdict`. Genuinely ambiguous or non-verdict replies
(including a leaked `<tool_call …>` first line) still fail closed.

## Entry point

CLI: `sworn verify` (`cmd/sworn`) → `internal/verify/verify.go` `parseVerdict`.
Shared by `sworn run`'s verify gate (`internal/run/run.go:232`).

## In scope

- Rework `parseVerdict` (`internal/verify/verify.go:74-90`) to:
  - Operate on the **first non-empty line** of the reply (skip leading
    whitespace-only lines), not the literal first byte of the whole string.
  - Strip surrounding markdown emphasis (`*`, `_`, backticks) and a leading code
    fence before matching the verdict token.
  - Match a leading verdict token case-insensitively against
    `PASS` / `FAIL` / `BLOCKED` / `INCONCLUSIVE`.
  - **Fail closed** on anything else: a first non-empty line that is not a verdict
    token (e.g. `<tool_call …>`, prose preamble, JSON) → `BLOCKED`
    (`unparseable_verdict`). Never widen toward a false `PASS`.
- Keep the existing verdict→`Result` mapping and exit codes unchanged
  (PASS→0, FAIL→1, BLOCKED→2, INCONCLUSIVE→3; see `internal/verdict/verdict.go`
  `ExitCode()`).

## Out of scope

- The prompt change (`S01`) — this slice assumes S01 has landed.
- Structured output / schema-forced verdicts (deferred — see intake).
- Any change to verdict semantics or exit-code mapping.

## Planned touchpoints

- `internal/verify/verify.go` (`parseVerdict`)
- `internal/verify/verify_test.go` (regression cases)

## Acceptance checks

- [ ] A reply whose first non-empty line is `**FAIL**` (markdown emphasis)
      resolves to `FAIL` (exit 1).
- [ ] A reply with one or more leading blank lines before `PASS` resolves to
      `PASS` (exit 0).
- [ ] A reply whose first non-empty line is `` ```\nPASS `` (leading fence)
      resolves to `PASS`.
- [ ] A reply whose first non-empty line is `<tool_call name="Bash">` resolves to
      `BLOCKED` (`unparseable_verdict`) — tool-call leakage never parses as a
      verdict.
- [ ] A reply whose first non-empty line is investigative prose
      (e.g. `Verifying slice S0X …`) resolves to `BLOCKED`.
- [ ] No reply shape resolves to `PASS` unless its (stripped) first non-empty line
      leads with the `PASS` token — the fail-closed property is preserved
      (existing `TestRun_GarbledVerdictBlocks` still passes).

## Required tests

- **Unit**: `internal/verify/verify_test.go` — table-driven cases covering each
  acceptance check above, asserting `verdict.Verdict` AND `ExitCode()` for each.
  Reuse the existing `Run` entry point with a fake `model.Verifier` that returns
  the canned reply (per Rule 1, drive through `verify.Run`, not a private helper).
- **Public-safe fixtures (mandatory)**: the canned spec+diff handed to `Run` MUST
  be **synthetic** (a few hand-written lines). Do NOT import or paste the private
  dogfood slice spec/diff used as evidence in the findings note — this test lands
  in the public repo. Only the reply *shapes* (tool-call leak, markdown emphasis,
  prose preamble) are reproduced from the variance observation.
- **Reachability artefact**: `go test ./internal/verify/...` green; the new cases
  named so each maps to an acceptance check (e.g.
  `TestParseVerdict_MarkdownEmphasis`, `TestParseVerdict_ToolCallLeakBlocks`).
- **E2E gate type**: N/A.

## Risks

- **Tolerance widening into a false PASS.** The headline fail-closed risk.
  Mitigated by the dedicated acceptance check that only a `PASS`-leading stripped
  first line passes, plus retaining the existing garbled-verdict block test.
- **Over-stripping** (e.g. eating a `FAIL` that is part of prose like
  "I will not FAIL this"). Mitigated by matching only the *leading* token of the
  first non-empty line, not a substring search.

## Deferrals allowed?

No.
