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

## Verifier verdicts received

### 2026-07-10T01:54:32Z — PASS (fresh-context Rule 7 verifier)

```
PASS

Slice: `S13-regress-go-module-cwd`
Verified against: `66175a2` (slice's own feat commit; HEAD after forward-merging release-wt/2026-06-28-driver-contract is `4b6c63e`)
Verifier session: fresh, artefact-only
```

Gate-by-gate:
1. User-reachable outcome — `RunRegress` (internal/gate/regress.go:83) is called from `cmd/sworn/regress.go:81`, the real `sworn regress --release <name>` CLI entry point. Not a test-only fixture. PASS.
2. Planned touchpoints vs actual diff — spec touchpoints are exactly `internal/gate/regress.go` and `internal/gate/regress_test.go`; `git diff <start_commit>` (start_commit=fcb93ad) non-merge commits touch exactly those two files plus the slice's own doc scaffolding (status.json, journal.md, proof.json/md) — no undeclared production touchpoints. (The forward-merge of release-wt/2026-06-28-driver-contract added an unrelated S04-inprocess-oai-driver/T3 merge commit and an S06 spec edit as expected drift-gate noise, correctly excluded from scope per the non-merge `feat` commit filter.) PASS.
3. Required tests — re-ran `go test ./internal/gate/... -v` myself: all pass, including the 6 new/changed regress tests (`TestRunRegress_GoModuleInSubdir`, `TestRunRegress_NoGoMod`, `TestRunRegress_MultipleGoModules`, `TestFindGoModuleDir_IgnoresVendorAndHidden`, plus the 4 pre-existing tests now carrying root go.mod fixtures). `TestRunRegress_GoModuleInSubdir` drives `runRegress` (the real entry point), not a leaf-only fixture. PASS.
4. Reachability artefact — `TestRunRegress_GoModuleInSubdir` exists on disk, drives the CLI-invoked `runRegress` entry point, and its mock only satisfies the module-dir dispatch key, so the test provably fails (ExitCode -1) if `runGoSuite` still dispatched at the worktree root. Matches proof.json's description. PASS.
5. No silent deferrals — grepped the slice diff (`internal/gate/regress.go`, `internal/gate/regress_test.go`) for TODO/FIXME/deferred/placeholder/XXX/HACK: no hits. `proof.json.not_delivered` is empty, `status.json` has no unacknowledged `open_deferrals`. PASS.
6. Design conformance — project has no `docs/baton/design-fidelity.json` (non-UI-bearing); gate auto-passes.
7. Claimed scope vs implemented scope — all 7 `proof.json.delivered` items checked against live repo state (function names, test names) and independently re-run; each evidence reference is real and does what it claims, including the full-suite sweep claim (`go test -timeout 300s ./...` — I re-ran it myself, all packages `ok` including `cmd/sworn`, which now tolerates the Go suite's FAIL->Skipped flip via its `exit != 2` assertion in `regress_test.go`). PASS.

Additional checks run: `gofmt -l` and `go vet ./internal/gate/...` both clean; newline-eating-corruption grep over both changed `.go` files — no hits; full `go test -timeout 300s ./...` in the worktree — all packages `ok`.

LLM-backed gates 3b/4b/6b (`sworn llm-check`) skipped — no model provider configured in this environment (non-blocking per contract).
