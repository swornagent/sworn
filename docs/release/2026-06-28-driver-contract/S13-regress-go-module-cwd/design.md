# Design TL;DR — S13-regress-go-module-cwd

**Slice:** S13-regress-go-module-cwd · **Release:** 2026-06-28-driver-contract · **Track:** T8-regress-cwd
**State at write:** planned → design_review (Rule 9 gate)

## Problem (from spec)

`internal/gate/regress.go` `runGoSuite` (currently regress.go:120) invokes
`runner.Run(worktree, "go", "test", "./...")` with `cmd.Dir` = the worktree
ROOT. A repo whose Go module lives in a subdirectory (fired's `<repo>/go`)
gets a phantom `directory prefix . does not contain main module` FAIL on a
clean track. sworn's own regress passes only because its module is at the
repo root.

## Approach

One new pure helper + a three-way branch in `runGoSuite`. No signature
changes, no new deps (stdlib `os` / `path/filepath` only — already imported).

### 1. Module discovery helper (new, regress.go)

```go
// findGoModuleDir locates the Go module directory under worktree.
// Returns (dir, true) when found; ("", false) when no go.mod exists.
func findGoModuleDir(worktree string) (string, bool)
```

Resolution order (implements spec R-01 mitigation exactly):

1. `<worktree>/go.mod` exists → return `worktree` (repo-root module; sworn's
   own shape — today's behaviour preserved bit-for-bit).
2. Else scan **first-level** subdirectories only (`os.ReadDir`), skipping
   hidden dirs (`.git`, `.anything`), `vendor`, `node_modules`, `testdata`.
   Collect subdirs containing `go.mod`.
   - Exactly one → return it (fired's `<repo>/go` shape).
   - Zero → `("", false)`.
   - More than one → treated as not-found-for-our-purposes but with a
     distinct skip reason (see Design decision D1 below).

Discovery reads the real filesystem (`os.Stat`/`os.ReadDir`), not the
`testRunner` — fixtures are plain `t.TempDir()` layouts, so tests stay
hermetic without widening the runner interface.

### 2. runGoSuite branch (modified, regress.go)

- Discovery finds a module dir → `runner.Run(moduleDir, "go", "test", "./...")`.
  The command stays `go test ./...`; only `cmd.Dir` moves (spec explicitly
  forbids `go test ./go/...` from the root).
- No go.mod anywhere → `SuiteResult{Name: "Go tests", Skipped: true,
  SkippedReason: "no go.mod found under worktree"}` (AC-03: non-Go project
  degrades to skipped, never a hard FAIL).
- Multiple candidate modules → skipped with reason
  `"multiple Go modules found; multi-module repos unsupported (see spec out_of_scope)"`.

### 3. Tests (regress_test.go)

The existing `mockRunner` keys on `"<dir>/<name> <args...>"`, so **cmd.Dir is
directly assertable** — a subdir-module test that mocks only
`<worktree>/go/go test ./...` fails unless runGoSuite actually runs from the
module dir. New/changed cases:

- **Subdir module (fired shape, AC-01/AC-02):** fixture `<tmp>/go/go.mod`;
  mock `"<tmp>/go/go test ./..."` → exit 0; assert Go suite `Passed`, not
  skipped, and (via the mock's default `-1` for unknown keys) that the root
  dir was NOT used.
- **Repo-root module still passes (AC-02):** existing fixtures
  (`TestRunRegress_AllPass`, `TestRunRegress_AllFail`, etc.) gain a
  `go.mod` file at the fixture root. **Required consequential edit:** today's
  fixtures have no go.mod at all, so after this change they would silently
  flip to Skipped and break their passed/skipped tallies — writing the root
  go.mod keeps them exercising the root-module path unchanged.
- **No go.mod (AC-03):** fixture with no go.mod anywhere; assert Go suite
  `Skipped` with a non-empty `SkippedReason`, report tallies it under
  `Skipped` not `Failed`.
- **Multiple modules (D1):** fixture `<tmp>/a/go.mod` + `<tmp>/b/go.mod`;
  assert skipped with the multi-module reason.
- **vendor/hidden ignored (R-01):** fixture with `<tmp>/vendor/go.mod` +
  `<tmp>/go/go.mod`; assert discovery picks `<tmp>/go`.

## AC traceability

| AC | Planned change | Planned test |
|----|----------------|--------------|
| AC-01 | `findGoModuleDir` + `runGoSuite` uses module dir as cmd.Dir | subdir-module fixture asserts run dir = `<tmp>/go` |
| AC-02 | root-module path preserved (order-of-resolution step 1) | root go.mod added to existing fixtures; new subdir test; `go test ./internal/gate/...` green |
| AC-03 | skip/degrade branch in `runGoSuite` | no-go.mod fixture asserts Skipped + reason |

## Design decisions / pins for the reviewer

- **D1 (Type-2, noted default): multiple first-level modules → skip-with-reason,
  not "pick one".** Spec scopes multi-module out (`out_of_scope[1]`) and R-01
  warns against picking the wrong go.mod. Testing an arbitrary one of N
  modules would report a partial result as if it were the whole Go suite;
  an explicit skip reason is honest degradation and keeps the phantom-FAIL
  fix from introducing a phantom-PASS. Reversible in the follow-up
  multi-module slice.
- **D2 (Type-2, noted default): discovery depth = root + one level.** Matches
  the spec's in_scope wording ("repo-root go.mod, else a single-level subdir
  like `<worktree>/go`"). No recursive walk — cheaper and avoids vendored /
  fixture go.mod false positives below level 1 by construction.
- **Skip-list at level 1:** hidden dirs, `vendor`, `node_modules`, `testdata`
  — vendor is spec-mandated (R-01); node_modules/testdata are the two other
  places a stray go.mod plausibly lives in this codebase's fixtures.

## Files to touch (== spec touchpoints, no additions)

- `internal/gate/regress.go` — `findGoModuleDir` (new) + `runGoSuite` branch
- `internal/gate/regress_test.go` — fixtures + 4 new/extended cases above

## Risks

- **R-01 (from spec):** wrong go.mod picked — mitigated by root-first order,
  level-1-only scan, skip-list, and multi-module → skip (D1).
- **Fixture regression risk:** existing gate tests flip to Skipped if root
  go.mod isn't added to their fixtures — handled as a planned edit, and the
  full `go test -timeout 120s ./...` sweep before `implemented` catches any
  cross-package fixture I miss.

## Verification plan

`go test ./internal/gate/...` (AC-02's named command), then the full
`go test -timeout 120s ./...` in this worktree, `gofmt -l` + `go vet` on
`internal/gate`, and the newline-corruption grep over changed .go files.
Reachability artefact: the subdir-module integration test run (regress is a
CLI-invoked gate; the test drives the same `runRegress` entry the CLI calls).
