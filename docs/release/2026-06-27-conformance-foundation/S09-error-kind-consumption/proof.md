# Proof Bundle — S09-error-kind-consumption

## Scope

Fix the slice runner to detect `model.IsTerminal()` errors and surface them as a BLOCKED verdict before the triage retry loop, preventing KindAuth/KindCredits from being retried+escalated like transient failures. Also rename `ErrDriverNotRegistered` → `ErrDriverNotImplemented`.

## Files changed

```
internal/model/bedrock_test.go      |   8 +-
internal/model/cli.go               |   2 +-
internal/model/cli_test.go          |   4 +-
internal/model/config.go            |   4 +-
internal/model/provider.go          |   9 +-
internal/model/provider_test.go     |  10 +-
internal/run/slice.go               |  15 +++
internal/run/slice_terminal_test.go | 257 ++++++++++++++++++++++++++++++++++++
8 files changed, 290 insertions(+), 19 deletions(-)
```

## Test results

### Terminal error tests (`go test ./internal/run/... -v -run TestTerminalError`)

```
=== RUN   TestTerminalError_KindAuth_Halts
--- PASS: TestTerminalError_KindAuth_Halts (0.02s)
=== RUN   TestTerminalError_KindCredits_Halts
--- PASS: TestTerminalError_KindCredits_Halts (0.02s)
=== RUN   TestTerminalError_KindRateLimit_DoesNotHalt
--- PASS: TestTerminalError_KindRateLimit_DoesNotHalt (0.03s)
=== RUN   TestTerminalError_NilError_Continues
--- PASS: TestTerminalError_NilError_Continues (0.04s)
=== RUN   TestTerminalError_UntypedTerminal
--- PASS: TestTerminalError_UntypedTerminal (0.03s)
=== RUN   TestTerminalError_AllKinds
=== RUN   TestTerminalError_AllKinds/Kind=auth
=== RUN   TestTerminalError_AllKinds/Kind=credits
=== RUN   TestTerminalError_AllKinds/Kind=rate_limit
=== RUN   TestTerminalError_AllKinds/Kind=upstream
=== RUN   TestTerminalError_AllKinds/Kind=transient
=== RUN   TestTerminalError_AllKinds/Kind=other
--- PASS: TestTerminalError_AllKinds (0.17s)
PASS
ok  	github.com/swornagent/sworn/internal/run	0.319s
```

### Full run package tests (`go test ./internal/run/...`)

All 17 tests pass (7 new + 10 existing), no regressions.

### Model package tests (`go test ./internal/model/...`)

All tests pass, no regressions from sentinel rename.

### `go vet`

Clean — zero warnings in `./internal/run/...` and `./internal/model/...`.

## Reachability artefact

`go test ./internal/run/... -v -run TestTerminalError` exits 0.

The tests exercise the integration point: `RunSlice()` → `implement.Run()` → error return → `model.IsTerminal()` guard → verdict. Each test drives through the full `RunSlice` pipeline, not a leaf function.

## Delivered

- [x] AC1: KindAuth and KindCredits dispatch errors return BLOCKED verdict before triage
  - Evidence: `TestTerminalError_KindAuth_Halts` PASS, `TestTerminalError_KindCredits_Halts` PASS
  - Evidence: `TestTerminalError_AllKinds/Kind=auth` PASS, `TestTerminalError_AllKinds/Kind=credits` PASS
  - Implementation: `internal/run/slice.go` lines 326-337 (terminal error guard)

- [x] AC2: KindRateLimit (non-terminal) does NOT trigger terminal halt
  - Evidence: `TestTerminalError_KindRateLimit_DoesNotHalt` PASS — `IsBlocked(err)==false`
  - Evidence: `TestTerminalError_AllKinds/Kind=rate_limit` PASS

- [x] AC3: nil errors (successful dispatch) not affected
  - Evidence: `TestTerminalError_NilError_Continues` PASS — happy path reaches PASS verdict

- [x] AC4: `ErrDriverNotImplemented` used consistently — zero `ErrDriverNotRegistered` in Go files
  - Evidence: `grep -rn 'ErrDriverNotRegistered' --include='*.go' .` returns no results
  - Evidence: `grep -rn 'ErrDriverNotImplemented' --include='*.go' .` returns 18 consistent references

- [x] AC5: `slice_terminal_test.go` covers KindAuth→BLOCKED, KindCredits→BLOCKED, KindRateLimit→not blocked
  - Evidence: `TestTerminalError_AllKinds` table covers all 6 ErrorKind values
  - Evidence: Individual named tests for the three specific acceptance-check cases

## Not delivered

None — all acceptance checks satisfied.

## Divergence from plan

- **Sentinel rename location**: spec.md listed `internal/model/errors.go` as the planned touchpoint for the rename, but `ErrDriverNotRegistered` is defined in `internal/model/provider.go`. The rename was applied to the correct file. `errors.go` was not modified — it already contained the correct `IsTerminal()` and `ErrorKind` taxonomy.
- **Dark-code markers**: the first-pass script flags `S63-deferral-1` comments in `cli.go` and `provider.go` as dark-code. These are pre-existing Rule 2 deferrals from S63 (subscription CLI driver), not introduced by this slice. The rename touched the surrounding lines but did not change the deferral status or the deferred code paths.

## First-pass script output

See below (run after proof.md created).
## First-pass script output

```
== First-pass verdict ==
  checks passed: 21
  checks failed: 1

FIRST-PASS FAIL

  FAIL  dark-code markers found in changed source files (must be Rule 2 deferrals)
  hits:
    internal/model/cli.go:
    1:+	return nil, fmt.Errorf("%w: codex support deferred (S63-deferral-1)", ErrDriverNotImplemented)
    internal/model/provider.go:
    3:+		return nil, fmt.Errorf("%w: codex support deferred (S63-deferral-1)", ErrDriverNotImplemented)
```

The single FAIL is dark-code markers from S63-deferral-1 — pre-existing Rule 2 deferrals (why: "codex support deferred", tracking: "S63-deferral-1", acknowledged: S63 spec). The sentinel rename (`ErrDriverNotRegistered` → `ErrDriverNotImplemented`) touched the surrounding lines but did not introduce or change the deferral. These deferrals existed before this slice and are not in scope for S09. See `journal.md` "Out-of-scope deferrals" and proof.md "Divergence from plan".
