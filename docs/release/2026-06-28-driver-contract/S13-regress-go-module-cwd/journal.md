# Journal — S13-regress-go-module-cwd

## 2026-07-10 — implemented

Design review acknowledged (captain-proceed.md, Coach: Brad, PROCEED, 3
pins all mechanical/memory-cited, 0 escalate). Implemented against the
acknowledged design with the two pin dispositions applied inline:

1. **Pin 1 (skip-reason wording).** Used
   `"no go.mod at worktree root or in a first-level subdirectory"` for the
   no-go.mod skip path (regress.go `runGoSuite`) instead of the
   design-draft's "no go.mod found under worktree" — states the actual
   scan bound (D2) rather than implying an exhaustive search.
2. **Pin 2 (D1/D2 in status.json).** Recorded D1 (multi-module -> skip
   with reason) and D2 (discovery depth = root + first-level subdirs) as
   Type-2 noted defaults in `status.json.design_decisions` at the
   `in_progress` transition (commit fcb93ad), matching the S01-S03
   convention.
3. **Pin 3 (full-suite sweep).** Ran the full `go test -timeout 300s
   ./...` (not just `./internal/gate/...`) before declaring
   `implemented`. All packages pass, including
   `cmd/sworn/regress_test.go`'s `TestRegressDefaultResolution_BoardJSON`
   / `_LegacyIndexMDFallback`, which the Captain flagged as flipping from
   exercising a real `go test` FAIL to a Skipped result post-change; both
   assert only `exit != 2`, so the flip is tolerated as predicted.

## Implementation

- `internal/gate/regress.go`: added `findGoModuleDir(worktree) (dir
  string, found int)` — root go.mod first, else exactly one first-level
  subdir go.mod (skipping `.git`, `vendor`, `node_modules`, `testdata`,
  and other hidden dirs); `found` is 0 (none), 1 (exactly one), or >1
  (ambiguous / multi-module). `runGoSuite` now branches on this: 0 ->
  skip with the bound-honest reason (AC-03); >1 -> skip with a distinct
  multi-module reason (D1); exactly 1 -> `runner.Run(moduleDir, "go",
  "test", "./...")` (AC-01) — the command stays `go test ./...`, only
  `cmd.Dir` moves, per spec.
- `internal/gate/regress_test.go`: added root `go.mod` fixtures to the
  four pre-existing tests that mock `"go test ./..."` at the worktree
  root (`TestRunRegress_AllPass`, `_AllFail`, `_Mixed`,
  `_NoPackageJSON`) — required consequential edit called out in
  design.md, otherwise they would silently flip to Skipped and break
  their pass/fail tallies. Added five new tests:
  `TestRunRegress_GoModuleInSubdir` (AC-01/AC-02, fired shape — asserts
  the suite is not skipped, passes, and the mock only satisfies the
  module-dir key, so a root-dir dispatch would surface as ExitCode -1),
  `TestRunRegress_NoGoMod` (AC-03), `TestRunRegress_MultipleGoModules`
  (D1), `TestFindGoModuleDir_IgnoresVendorAndHidden` (R-01 — asserts
  `vendor/go.mod` and `.git/go.mod` are ignored and the real subdir
  module is picked).

## Deviations from design.md

None. Implemented exactly as scoped; both pins applied inline as directed.

## Verification run (this session)

- `go test ./internal/gate/... -v` — all tests pass (gate package,
  including 6 new/changed regress tests).
- `go test -timeout 300s ./...` — full suite green, all packages `ok`.
- `gofmt -l internal/gate/regress.go internal/gate/regress_test.go` —
  clean after one auto-fix (`gofmt -w` on the test file for a spacing
  diff introduced by the new fixture map).
- `go vet ./internal/gate/...` — clean.
- Newline-corruption grep
  (`grep -nE '//.*\t+(return|sendRequest|[a-z]+\()'`) over both changed
  `.go` files — no hits.

Never marked `verified` from this session — terminal state is
`implemented`; a fresh-context `/verify-slice` session owns Rule 7.
