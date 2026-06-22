# Proof Bundle: `S34-tui-merge-actor`

## Scope

A developer watching an active release in the `sworn` TUI sees merge activity as its own distinct, highlighted row labelled `merge:<track>` in the live concurrent-status view and the release board.

## Files changed

```
$ git diff --name-only ba6ee06609adc53bbf1e8fc2b12d40c945a7c47a
internal/tui/board.go
internal/tui/concurrent.go
internal/tui/releases.go
internal/tui/styles.go
internal/tui/tui.go
internal/tui/tui_test.go
```

Note: `releases.go` and `tui.go` appear in the diff due to `gofmt -w` normalisation (whitespace only); no functional changes were made to those files.

## Test results

### Go

```
$ go test ./internal/tui/... -v
=== RUN   TestReleasesListPopulates
--- PASS: TestReleasesListPopulates (0.00s)
=== RUN   TestBoardViewShowsSlices
--- PASS: TestBoardViewShowsSlices (0.00s)
=== RUN   TestKeyNavigation
--- PASS: TestKeyNavigation (0.00s)
=== RUN   TestHelpToggle
--- PASS: TestHelpToggle (0.00s)
=== RUN   TestQuit
--- PASS: TestQuit (0.00s)
=== RUN   TestConcurrentStatusPoll
--- PASS: TestConcurrentStatusPoll (0.04s)
=== RUN   TestAutoTransitionToLive
--- PASS: TestAutoTransitionToLive (0.04s)
=== RUN   TestAutoTransitionNoTracks
--- PASS: TestAutoTransitionNoTracks (0.00s)
=== RUN   TestLiveBoardToggle
--- PASS: TestLiveBoardToggle (0.04s)
=== RUN   TestCreditBalanceDisplayed
--- PASS: TestCreditBalanceDisplayed (0.00s)
=== RUN   TestCreditBalanceAbsent
--- PASS: TestCreditBalanceAbsent (0.00s)
=== RUN   TestModelTickForwarding
--- PASS: TestModelTickForwarding (0.05s)
=== RUN   TestLiveViewClose
--- PASS: TestLiveViewClose (0.04s)
=== RUN   TestElapsedTimeFormatting
--- PASS: TestElapsedTimeFormatting (0.00s)
=== RUN   TestHasInProgressTracks
--- PASS: TestHasInProgressTracks (0.05s)
=== RUN   TestBlockedPanelExtractsViolations
--- PASS: TestBlockedPanelExtractsViolations (0.00s)
=== RUN   TestOpenAIWritesContextFile
--- PASS: TestOpenAIWritesContextFile (0.00s)
=== RUN   TestLaunchMissingTool
--- PASS: TestLaunchMissingTool (0.00s)
=== RUN   TestDeferWritesRuleTwo
--- PASS: TestDeferWritesRuleTwo (0.00s)
=== RUN   TestBoardEnterTransitionsToBlocked
--- PASS: TestBoardEnterTransitionsToBlocked (0.00s)
=== RUN   TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict
--- PASS: TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict (0.00s)
=== RUN   TestBlockedPanelViewProof
--- PASS: TestBlockedPanelViewProof (0.00s)
=== RUN   TestLiveViewRendersMergeActorRow
--- PASS: TestLiveViewRendersMergeActorRow (0.04s)
=== RUN   TestLiveViewNoMergeActorNoRow
--- PASS: TestLiveViewNoMergeActorNoRow (0.04s)
=== RUN   TestLiveViewNoMergeActorAfterRelease
--- PASS: TestLiveViewNoMergeActorAfterRelease (0.04s)
=== RUN   TestBoardViewShowsMergeBadge
--- PASS: TestBoardViewShowsMergeBadge (0.04s)
=== RUN   TestBoardViewNoMergeBadge
--- PASS: TestBoardViewNoMergeBadge (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/tui	0.431s
```

```
$ go build ./...
(exit 0, no output)

$ go vet ./internal/tui/...
(exit 0, no output)
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **Path**: `internal/tui/tui_test.go` — `TestLiveViewRendersMergeActorRow` and `TestBoardViewShowsMergeBadge`
- **User gesture**: "Developer opens `sworn` TUI, selects a release with an active merge in flight, observes a distinct amber-bold `merge:<track>` row in the live view and a `⟪merge⟫` badge on the track header in the board view."

The reachability artefact is the TUI render test that feeds a SQLite DB with a `merge:T1-engine` acquired event, calls `StartLiveView`, and asserts `lv.View()` contains a distinct `merge:T1-engine` row rendered with `MergeRowStyle` (amber, bold). The board view test (`TestBoardViewShowsMergeBadge`) sets up both a filesystem fixture (index.md + status.json) and a SQLite DB with a `merge:T1-core` acquired event, then asserts the rendered board view contains the merge badge.

## Delivered

- **AC1: Live view renders distinct, highlighted `merge:<track>` row** — evidence: `TestLiveViewRendersMergeActorRow` in `internal/tui/tui_test.go`; `MergeRowStyle` in `internal/tui/styles.go`; `poll()` merge-actor query in `internal/tui/concurrent.go`; `View()` conditional `MergeRowStyle` rendering in `internal/tui/concurrent.go`
- **AC2: Board view shows merge activity as its own highlighted row/indicator** — evidence: `TestBoardViewShowsMergeBadge` in `internal/tui/tui_test.go`; `MergeBadge` style in `internal/tui/styles.go`; `MergeActive` field + `ActiveMerges()` call in `internal/tui/board.go`; `⟪merge⟫` badge rendering in `BoardView.View()`
- **AC3: Snapshot with no merge actor renders unchanged (no spurious merge row)** — evidence: `TestLiveViewNoMergeActorNoRow` and `TestBoardViewNoMergeBadge` in `internal/tui/tui_test.go`
- **AC4: `go test ./internal/tui/...` passes** — evidence: test output above (27/27 PASS)
- **AC5: `go build ./...` passes** — evidence: build output above (exit 0)

## Not delivered

None. All acceptance checks are delivered.

## Divergence from plan

- `internal/tui/releases.go` and `internal/tui/tui.go` appear in the diff due to `gofmt -w` normalisation (whitespace-only changes). No functional changes were made to these files. They are not in `planned_files` but the changes are mechanical formatting, not logic.
- The `TestLiveViewNoMergeActorAfterRelease` test (Pin 2 from design review) was added as a third live-view test beyond the two named in the spec's Required tests section. This is additive coverage, not a divergence from the spec's intent.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S34-tui-merge-actor 2026-06-19-safe-parallelism
release-verify.sh
  slice:       S34-tui-merge-actor
  slice dir:   docs/release/2026-06-19-safe-parallelism/S34-tui-merge-actor
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
  state: implemented

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  6 file(s) changed vs diff base
  (first 20)
    internal/tui/board.go
    internal/tui/concurrent.go
    internal/tui/releases.go
    internal/tui/styles.go
    internal/tui/tui.go
    internal/tui/tui_test.go

== Dark-code markers in changed files ==
  PASS  no dark-code markers found

== Proof bundle structural checks ==
  PASS  proof.md has Scope section
  PASS  proof.md has Files changed section
  PASS  proof.md has Test results section
  PASS  proof.md has Reachability artefact section
  PASS  proof.md has Delivered section
  PASS  proof.md has Not delivered section
  PASS  proof.md has Divergence from plan section

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  proof.md test results section contains slice-relevant test commands
exit_code:0
```