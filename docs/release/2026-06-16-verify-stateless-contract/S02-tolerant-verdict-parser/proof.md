# Proof Bundle: `S02-tolerant-verdict-parser`

## Scope

A developer runs `sworn verify` and a reply that is *substantively* a verdict but not byte-perfect — a markdown-emphasised `**FAIL**`, a leading blank line, or a verdict preceded by a stray fence — still resolves to the intended verdict instead of `BLOCKED / unparseable_verdict`. Genuinely ambiguous or non-verdict replies (including a leaked `<tool_call …>` first line) still fail closed.

## Files changed

```
$ git diff --name-only start_commit..HEAD
internal/verify/verify.go
internal/verify/verify_test.go
```

## Test results

### Go

```
$ go test ./internal/verify/... -v
=== RUN   TestRun_PassExitsZero
--- PASS: TestRun_PassExitsZero (0.00s)
=== RUN   TestRun_MissingSpecBlocks
--- PASS: TestRun_MissingSpecBlocks (0.00s)
=== RUN   TestRun_UnconfiguredModelFailsClosed
--- PASS: TestRun_UnconfiguredModelFailsClosed (0.00s)
=== RUN   TestRun_MissingFileBlocks
--- PASS: TestRun_MissingFileBlocks (0.00s)
=== RUN   TestRun_GarbledVerdictBlocks
--- PASS: TestRun_GarbledVerdictBlocks (0.00s)
=== RUN   TestParseVerdict_MarkdownEmphasis
--- PASS: TestParseVerdict_MarkdownEmphasis (0.00s)
=== RUN   TestParseVerdict_LeadingBlankLines
--- PASS: TestParseVerdict_LeadingBlankLines (0.00s)
=== RUN   TestParseVerdict_LeadingFence
--- PASS: TestParseVerdict_LeadingFence (0.00s)
=== RUN   TestParseVerdict_ToolCallLeakBlocks
--- PASS: TestParseVerdict_ToolCallLeakBlocks (0.00s)
=== RUN   TestParseVerdict_ProsePreambleBlocks
--- PASS: TestParseVerdict_ProsePreambleBlocks (0.00s)
=== RUN   TestRun_SystemPromptIsStateless
--- PASS: TestRun_SystemPromptIsStateless (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/verify	0.007s
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **Path**: `go test ./internal/verify/...`
- **User gesture**: `go test ./internal/verify/...` exercises `verify.Run` → `parseVerdict` with synthetic fake-verifier replies covering each acceptance check shape. Each test name maps to an acceptance check (MarkdownEmphasis, LeadingBlankLines, LeadingFence, ToolCallLeakBlocks, ProsePreambleBlocks, plus the existing GarbledVerdictBlocks regression).

## Delivered

- A reply whose first non-empty line is `**FAIL**` (markdown emphasis) resolves to `FAIL` (exit 1) — evidence: `TestParseVerdict_MarkdownEmphasis` in `internal/verify/verify_test.go:101`
- A reply with one or more leading blank lines before `PASS` resolves to `PASS` (exit 0) — evidence: `TestParseVerdict_LeadingBlankLines` in `internal/verify/verify_test.go:113`
- A reply whose first non-empty line is `` ```\nPASS `` (leading fence) resolves to `PASS` — evidence: `TestParseVerdict_LeadingFence` in `internal/verify/verify_test.go:124`
- A reply whose first non-empty line is `<tool_call name="Bash">` resolves to `BLOCKED` (`unparseable_verdict`) — evidence: `TestParseVerdict_ToolCallLeakBlocks` in `internal/verify/verify_test.go:138`
- A reply whose first non-empty line is investigative prose (e.g. `Verifying slice S0X …`) resolves to `BLOCKED` — evidence: `TestParseVerdict_ProsePreambleBlocks` in `internal/verify/verify_test.go:153`
- No reply shape resolves to `PASS` unless its (stripped) first non-empty line leads with the `PASS` token — the fail-closed property is preserved (existing `TestRun_GarbledVerdictBlocks` still passes) — evidence: `TestRun_GarbledVerdictBlocks` in `internal/verify/verify_test.go:89`

## Not delivered

None — all six acceptance checks are delivered.

## Divergence from plan

None.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S02-tolerant-verdict-parser 2026-06-16-verify-stateless-contract
<paste output here>
```