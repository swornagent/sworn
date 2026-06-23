# Design TL;DR: S31-lint-symbols

## §1. User-visible change

`sworn lint symbols <slice-id> <release>` parses a slice's `design.md` for every
backtick-quoted identifier matching code-symbol shapes (CamelCase, snake_case,
dotted `Type.Field`, table-like), greps each against the repo's codebase
(excluding `docs/`), and reports unresolved identifiers as advisory warnings.
Exit code 2 (advisory, distinct from the hard-fail exit 1 used by
`deps`/`touchpoints`) when there are unresolved symbols; exit 0 when all
resolve. The command is a warn-only gate — it never blocks, because it cannot
distinguish a symbol the slice itself introduces from a typo.

## §2. Design decisions not in spec (max 5)

1. **Advisory exit code 2** — distinct from the hard-fail exit 1 used by
   `deps`/`touchpoints`/`ac`/`trace`. Rationale: spec requires "advisory code
   (distinct from the hard fail-closed of S29/S30)"; code 2 is unambiguous for
   scripts that check `$?` ranges.
2. **Grep against the repo working tree (not git-tracked-only)** via `grep -r`
   excluding `docs/` — matches the spec's "live codebase" intent. Rationale:
   `git grep` would miss untracked generated files, and the spec explicitly
   says "live codebase", not "committed codebase".
3. **Extract from design.md only, not spec.md** — spec says "slice's design"
   and the touchpoint note says "a design names a function…". Rationale: the
   evidence rows (§3a #3) all involve design-phase symbol errors; extracting
   from spec.md would yield noisy backticks from acceptance checks and file
   paths.
4. **Symbol shape regex: `\b[A-Z][a-zA-Z0-9]*(\.[A-Z][a-zA-Z0-9]*)*\b` for
   CamelCase/dotted + `\b[a-z]+(_[a-z0-9]+)+\b` for snake_case** —
   intentionally conservative. Rationale: the spec warns about
   over-extraction; this regex pair catches the known evidence classes
   (function names, field names, constants, table names) while avoiding CLI
   flags and prose backticks.
5. **`state.Read` for status metadata, `os.ReadFile` for design.md** — follow
   the `CheckDeps`/`CheckTouchpoints` pattern of using the shared state
   package for status.json, direct OS reads for slice documents. Rationale:
   consistency with the existing `internal/lint` package surface.

## §3. Files I'll touch grouped by purpose

- **New lint logic**: `internal/lint/symbols.go` — `CheckSymbols(sliceDir
  string) error`. Extracts backtick identifiers from the slice's `design.md`
  using regex, greps each against the repo root (excluding `docs/`), returns
  nil if all resolved, or an error naming unresolved symbols.
- **Tests**: `internal/lint/symbols_test.go` — table-driven tests with temp
  fixture trees: a fixture `design.md` referencing real and fake symbols,
  verifying that `CheckSymbols` correctly resolves/doesn't-resolve each.
- **Dispatch wiring**: `cmd/sworn/lint.go` — add `"symbols"` to the dispatch
  switch + `cmdLintSymbols` function + update usage strings.

## §4. Things I'm NOT doing

- **Not touching `spec.md` extraction** — only `design.md` is parsed. Spec
  backticks (AC lines, file paths) are not code symbols.
- **Not type-checking or semantic resolution** — a textual grep match anywhere
  in the tree suffices.
- **Not adding a `--strict` mode or hard fail-closed** — advisory by design,
  per spec.
- **Not creating a standalone extractor package** — the symbol extraction
  logic lives inside `symbols.go` in the `lint` package.

## §5. Reachability plan

Integration test: `internal/lint/symbols_test.go` creates a temp fixture tree
with a real Go file containing `func CalculateFIRE() {}` and a `design.md`
referencing both `CalculateFIRE` (real) and `NonExistentFunc` (fake).
`CheckSymbols` is called and the test asserts the returned error names
`NonExistentFunc` but not `CalculateFIRE`. Reachability artefact documented in
`proof.md` as test output plus a manual `sworn lint symbols
S31-lint-symbols 2026-06-19-safe-parallelism` invocation (the slice's own
design.md references real symbols which should all resolve).

## §6. Open questions for the Coach

None.