# Proof bundle — S13-regress-go-module-cwd

## Scope

`sworn regress` runs the Go test suite from the directory containing
go.mod (root or a first-level subdirectory), not always the worktree
root, so a repo whose Go module lives in a subdirectory (e.g. fired's
`<repo>/go`) reports real Go test results instead of a spurious
`directory prefix . does not contain main module` setup failure.

## Files changed

- `internal/gate/regress.go`
- `internal/gate/regress_test.go`
- `docs/release/2026-06-28-driver-contract/S13-regress-go-module-cwd/status.json`
- `docs/release/2026-06-28-driver-contract/S13-regress-go-module-cwd/journal.md`

## Test results

- `go test ./internal/gate/... -v` — PASS (exit 0)
- `go test -timeout 300s ./...` — PASS (exit 0), all packages `ok`
- `gofmt -l internal/gate/regress.go internal/gate/regress_test.go` — clean (exit 0)
- `go vet ./internal/gate/...` — clean (exit 0)

## Reachability artefact

`internal/gate/regress_test.go:TestRunRegress_GoModuleInSubdir` drives
`runRegress` — the same entry point the `sworn regress` CLI calls —
against a fixture with `go.mod` at `<worktree>/go`. The mock only
satisfies the `"go" "test" "./..."` key for `dir == <worktree>/go`, so
the test fails with `ExitCode -1` (unmocked key, the mockRunner
default) unless `runGoSuite` actually dispatches from the discovered
module directory. Asserts `Passed=true`, `Skipped=false` — the fired-shaped
defect fix reproduced end-to-end through the CLI-invoked gate function.

## Delivered

- `findGoModuleDir` discovers the Go module dir: worktree root first,
  else exactly one first-level subdirectory (skipping `.git`, `vendor`,
  `node_modules`, `testdata`, other hidden dirs). Evidence:
  `internal/gate/regress.go` `findGoModuleDir`;
  `TestFindGoModuleDir_IgnoresVendorAndHidden`.
- AC-01: `runGoSuite` runs `go test ./...` with `cmd.Dir` = the
  discovered module dir, not the worktree root. Evidence:
  `internal/gate/regress.go` `runGoSuite`; `TestRunRegress_GoModuleInSubdir`.
- AC-02: root-module case (sworn's own shape) continues to pass;
  `go test ./internal/gate/...` passes. Evidence:
  `TestRunRegress_AllPass`, `TestRunRegress_AllFail`,
  `TestRunRegress_Mixed`, `TestRunRegress_NoPackageJSON` (each now
  carries a root `go.mod` fixture — the design's required consequential
  edit).
- AC-03: no go.mod anywhere under the worktree -> Go suite reported
  Skipped with a scan-bound-honest reason, not a hard FAIL. Evidence:
  `TestRunRegress_NoGoMod`; skip reason `"no go.mod at worktree root or
  in a first-level subdirectory"` (design-review pin 1 applied).
- D1: multiple first-level Go modules -> skipped with a distinct
  multi-module reason, not an arbitrary pick. Evidence:
  `TestRunRegress_MultipleGoModules`.
- D1/D2 recorded as Type-2 noted defaults in `status.json` per the
  Coach's pin disposition. Evidence: `status.json` `design_decisions[]`.
- Full-suite regression sweep (pin 3) confirms no cross-package fixture
  breakage, including `cmd/sworn`'s real-`RunRegress` integration tests
  tolerating the FAIL->Skipped flip predicted in review.md. Evidence:
  `go test -timeout 300s ./...` — all packages `ok`, including
  `cmd/sworn`.

## Not delivered

None.

## Divergence from plan

None. Implemented exactly as scoped in design.md; both design-review
pins (skip-reason wording, D1/D2 recorded in status.json) applied
inline, and the mandatory full-suite sweep (pin 3) run and green.
