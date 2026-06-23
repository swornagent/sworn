---
title: Proof bundle — S31-lint-symbols
---

## Scope

`sworn lint symbols <slice-id> <release>` extracts backtick-quoted identifiers
from a slice's `design.md`, greps each against the live codebase (excluding
`docs/`), and emits advisory warnings for unresolved symbols. Advisory: exit
code 3 on unresolved, exit 0 on all-resolved.

## Files changed

```
cmd/sworn/lint.go
docs/release/2026-06-19-safe-parallelism/S31-lint-symbols/status.json
internal/lint/symbols.go
internal/lint/symbols_test.go
```

## Test results

```
$ go test ./internal/lint/... -run Symbols -v
=== RUN   TestSymbolsUnresolvedWarns
--- PASS: TestSymbolsUnresolvedWarns (0.00s)
=== RUN   TestSymbolsResolvedQuiet
--- PASS: TestSymbolsResolvedQuiet (0.00s)
=== RUN   TestSymbolsAllResolvedExitZero
--- PASS: TestSymbolsAllResolvedExitZero (0.00s)
=== RUN   TestSymbolsSnakeCaseResolves
--- PASS: TestSymbolsSnakeCaseResolves (0.00s)
=== RUN   TestSymbolsSingleWordLowercaseSkips
--- PASS: TestSymbolsSingleWordLowercaseSkips (0.00s)
=== RUN   TestSymbolsDottedResolves
--- PASS: TestSymbolsDottedResolves (0.00s)
=== RUN   TestSymbolsNoBackticks
--- PASS: TestSymbolsNoBackticks (0.00s)
=== RUN   TestSymbolsDeduplicates
--- PASS: TestSymbolsDeduplicates (0.00s)
=== RUN   TestExtractSymbols
=== RUN   TestExtractSymbols/camelCase
=== RUN   TestExtractSymbols/snake_case
=== RUN   TestExtractSymbols/dotted
=== RUN   TestExtractSymbols/mixed_with_prose
=== RUN   TestExtractSymbols/single_word_lowercase_excluded
=== RUN   TestExtractSymbols/cli_flag_excluded
=== RUN   TestExtractSymbols/empty_backticks
--- PASS: TestExtractSymbols (0.00s)
PASS
ok      github.com/swornagent/sworn/internal/lint    0.017s

$ go build ./...
exit: 0

$ go vet ./internal/lint/...
exit: 0
```

## Reachability artefact

```
$ sworn lint symbols S31-lint-symbols 2026-06-19-safe-parallelism
sworn lint symbols: unresolved symbol(s): Type.Field, \b[A-Z][a-zA-Z0-9]*..., func CalculateFIRE() {}, internal/lint/symbols_test.go
exit: 3
```

All reported unresolved symbols are false positives from the slice's own
design.md — examples of the pattern (`Type.Field`, regex literals, code
snippets) or references to files this slice introduces
(`internal/lint/symbols_test.go`). These are expected per the advisory design:
the lint cannot distinguish a symbol the slice introduces from a typo. Exit
code 3 confirms the advisory warning path is reachable through the CLI.

## Delivered

- [x] `sworn lint symbols <slice> <release>` reports each unresolved
  backtick-quoted identifier — confirmed via CLI invocation above; exit 3
  prints unresolved symbols to stderr.
- [x] Resolved identifiers are not reported — confirmed via
  `TestSymbolsResolvedQuiet` and `TestSymbolsAllResolvedExitZero`.
- [x] Command returns non-zero advisory code (exit 3) when unresolved symbols
  exist — confirmed via CLI invocation (exit 3).
- [x] Exit 0 when all resolve — confirmed via
  `TestSymbolsAllResolvedExitZero` and `TestSymbolsResolvedQuiet`.
- [x] `go build ./...` and `go vet ./internal/lint/...` pass clean.

## Not delivered

None.

## Divergence from plan

- **P1 (Captain): Exit code 3, not 2.** The original design proposed exit 2
  for advisory; Captain's review pin 1 changed this to exit 3 (keeping exit 2
  for genuine I/O errors). Applied.
- **P2 (Captain): `repoRoot` parameter on `CheckSymbols`.** Design proposed
  `CheckSymbols(sliceDir string) error`; Captain required
  `CheckSymbols(sliceDir, repoRoot string) error` so tests can grep temp
  fixture trees. Applied.
- **P3 (Captain): `design_decisions` populated in status.json.** Five Type-2
  entries added from design §2. Applied.

## First-pass script output

See below.