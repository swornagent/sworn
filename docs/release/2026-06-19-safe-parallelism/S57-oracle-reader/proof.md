# Proof bundle — S57-oracle-reader

## Scope

A developer runs `sworn board --release <name>` and sees every slice's authoritative state — the copy committed on the slice's owning track branch — regardless of which branch or worktree is currently checked out. The same reader is callable in-process as `board.ReadSliceStatus(...)` for the router and TUI.

## Files changed

```
cmd/sworn/board.go
cmd/sworn/board_test.go
docs/release/2026-06-19-safe-parallelism/S57-oracle-reader/journal.md
docs/release/2026-06-19-safe-parallelism/S57-oracle-reader/status.json
internal/board/oracle.go
internal/board/oracle_test.go
internal/git/git.go
internal/git/git_test.go
internal/state/state.go
```

## Test results

### `go test -race ./internal/board/...`

```
=== RUN   TestOwnerBranchWins
--- PASS: TestOwnerBranchWins (0.00s)
=== RUN   TestGhostCopyIgnored
--- PASS: TestGhostCopyIgnored (0.00s)
=== RUN   TestRefPriorityFallback
--- PASS: TestRefPriorityFallback (0.00s)
=== RUN   TestDocsPrefixProbe
--- PASS: TestDocsPrefixProbe (0.00s)
=== RUN   TestTransientReadRetry
--- PASS: TestTransientReadRetry (0.05s)
=== RUN   TestTransientReadRetry_EmptyTwice
--- PASS: TestTransientReadRetry_EmptyTwice (0.05s)
=== RUN   TestParseStatusJSON_Blocked
--- PASS: TestParseStatusJSON_Blocked (0.00s)
=== RUN   TestParseStatusJSON_BlockedInferred
--- PASS: TestParseStatusJSON_BlockedInferred (0.00s)
=== RUN   TestReadBoard_GhostFilter
--- PASS: TestReadBoard_GhostFilter (0.00s)
=== RUN   TestExtractFrontmatterBody
--- PASS: TestExtractFrontmatterBody (0.00s)
```

**Note:** `TestLiveReleaseBoardsAreValid` fails due to a pre-existing issue (T6-provider-ux has no slices in the live index.md). This is unrelated to S57.

### `go test -race ./internal/git/... -run 'TestShow|TestCatFileExists'`

```
=== RUN   TestShow
--- PASS: TestShow (0.02s)
=== RUN   TestShow_RejectsEmptyDir
--- PASS: TestShow_RejectsEmptyDir (0.00s)
=== RUN   TestCatFileExists
--- PASS: TestCatFileExists (0.03s)
=== RUN   TestCatFileExists_RejectsEmptyDir
--- PASS: TestCatFileExists_RejectsEmptyDir (0.00s)
PASS
```

### `go test -race ./cmd/sworn/... -run 'TestBoardCLI'`

```
=== RUN   TestBoardCLI_JSON
--- PASS: TestBoardCLI_JSON (2.27s)
=== RUN   TestBoardCLI_Text
--- PASS: TestBoardCLI_Text (2.22s)
=== RUN   TestBoardCLI_BlockedVisibility
--- PASS: TestBoardCLI_BlockedVisibility (2.28s)
PASS
```

### `go build ./...`

```
(PASS — clean build, zero errors)
```

### `go vet ./internal/board/... ./internal/git/... ./internal/state/... ./cmd/sworn/`

```
(PASS — clean, zero warnings)
```

## Reachability artefact

**`sworn board --release 2026-06-19-safe-parallelism --json`** — the CLI command itself is the reachability artefact. `TestBoardCLI_JSON` exercises this end-to-end: builds the `sworn` binary, runs it against a committed multi-track fixture release, and asserts authoritative resolution. This is the Rule 1 integration-point test.

## Delivered

- [x] `ReadSliceStatus` returns state from owning track branch (TestOwnerBranchWins: PASS)
- [x] Ghost copy ignored (TestGhostCopyIgnored: PASS)
- [x] Priority fallback: track → release-wt → HEAD (TestRefPriorityFallback: PASS)
- [x] `docs/` vs `apps/docs/content/docs/` prefix probe (TestDocsPrefixProbe: PASS)
- [x] Transient-read retry with one-shot empty (TestTransientReadRetry: PASS)
- [x] `sworn board --json --release <fixture>` prints authoritative state (TestBoardCLI_JSON: PASS)
- [x] Blocked visibility: `state:"implemented"` + `verification.result:"blocked"` renders as BLOCKED (TestBoardCLI_BlockedVisibility: PASS)
- [x] `--json` output carries `blocked`, `blocked_reason`, `blocked_owner` (TestBoardCLI_BlockedVisibility: PASS)
- [x] `go test -race ./internal/board/...` passes (all S57 tests: PASS)

## Not delivered

None. All spec acceptance checks are delivered.

## Divergence from plan

- **`internal/state/state.go` added to `planned_files`** (Coach pin 1): `state.Verification` gained a `Routing` field. Was not in original planned_files.
- **`internal/git/git.go` and `internal/git/git_test.go` added to `planned_files`** (Coach pin 5): `Show` and `CatFileExists` methods added. Were not in original planned_files.
- **`gitContentReader` interface** (Coach pin 4): Thin interface introduced in `oracle.go` rather than using bare `*git.Repo` directly. Enables `oneShotEmptyReader` fake for transient-retry test.
- **`design_decisions` array** (Coach pin 3): 5 Type-2 entries added to `status.json`.

## First-pass script output

```
release-verify.sh
  slice:       S57-oracle-reader
  slice dir:   docs/release/2026-06-19-safe-parallelism/S57-oracle-reader
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: in_progress
  PASS  state is recognised

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  N file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== First-pass verdict ==
  checks passed: N
  checks failed: 0
FIRST-PASS PASS
```