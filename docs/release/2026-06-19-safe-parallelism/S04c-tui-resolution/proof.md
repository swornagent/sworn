# Proof Bundle — S04c-tui-resolution

## Scope

A developer sees a blocked slice in the `sworn` TUI, presses `Enter`, and is shown a panel summarising the violations from the proof bundle with five resolution options — including launching Claude Code or Codex pre-loaded with context — without needing to open a terminal, navigate worktrees, or assemble context manually.

## Files changed

```
docs/adr/0005-tui-dep-bubbles.md
docs/release/2026-06-19-safe-parallelism/S04c-tui-resolution/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S04c-tui-resolution/journal.md
docs/release/2026-06-19-safe-parallelism/S04c-tui-resolution/proof.md
docs/release/2026-06-19-safe-parallelism/S04c-tui-resolution/status.json
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/spec.md
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/status.json
docs/release/2026-06-19-safe-parallelism/S43-agent-loop-natural-stop/spec.md
docs/release/2026-06-19-safe-parallelism/S43-agent-loop-natural-stop/status.json
docs/release/2026-06-19-safe-parallelism/S44-feedback-driven-retry/spec.md
docs/release/2026-06-19-safe-parallelism/S44-feedback-driven-retry/status.json
docs/release/2026-06-19-safe-parallelism/S45-design-tldr/spec.md
docs/release/2026-06-19-safe-parallelism/S45-design-tldr/status.json
docs/release/2026-06-19-safe-parallelism/S46-captain-review/spec.md
docs/release/2026-06-19-safe-parallelism/S46-captain-review/status.json
docs/release/2026-06-19-safe-parallelism/S47-orchestrator-recovery/spec.md
docs/release/2026-06-19-safe-parallelism/S47-orchestrator-recovery/status.json
docs/release/2026-06-19-safe-parallelism/index.md
go.mod
go.sum
internal/state/state.go
internal/tui/blocked.go
internal/tui/board.go
internal/tui/model.go
internal/tui/open_ai.go
internal/tui/styles.go
internal/tui/tui_test.go
```

**Slice-owned files** (this slice's production code + artefacts):
`docs/adr/0005-tui-dep-bubbles.md`, `go.mod`, `go.sum`, `internal/state/state.go`,
`internal/tui/blocked.go`, `internal/tui/board.go`, `internal/tui/model.go`,
`internal/tui/open_ai.go`, `internal/tui/styles.go`, `internal/tui/tui_test.go`,
plus this slice's own `status.json`, `proof.md`, `journal.md`, `approved-ack.md`.

**Forward-merge artefacts** (not this slice's work — landed via release-wt forward-merges
into the track branch): `docs/release/2026-06-19-safe-parallelism/index.md` and the
`S42`–`S47` spec.md + status.json files. These are planner-added slices from other tracks
that merged into `release-wt/2026-06-19-safe-parallelism` and were forward-merged into
this track branch. They are not part of S04c's implementation.
## Test results

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
--- PASS: TestLiveBoardToggle (0.05s)
=== RUN   TestCreditBalanceDisplayed
--- PASS: TestCreditBalanceDisplayed (0.00s)
=== RUN   TestCreditBalanceAbsent
--- PASS: TestCreditBalanceAbsent (0.00s)
=== RUN   TestModelTickForwarding
--- PASS: TestModelTickForwarding (0.04s)
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
--- PASS: TestLaunchMissingTool (0.01s)
=== RUN   TestDeferWritesRuleTwo
--- PASS: TestDeferWritesRuleTwo (0.00s)
=== RUN   TestBoardEnterTransitionsToBlocked
--- PASS: TestBoardEnterTransitionsToBlocked (0.00s)
=== RUN   TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict
--- PASS: TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict (0.00s)
=== RUN   TestBlockedPanelViewProof
--- PASS: TestBlockedPanelViewProof (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/tui	(cached)
```
```
$ go vet ./internal/tui/...
(clean)
```

```
$ go test ./...
ok  	github.com/swornagent/sworn/cmd/sworn	0.373s
ok  	github.com/swornagent/sworn/internal/adopt	(cached)
ok  	github.com/swornagent/sworn/internal/agent	(cached)
ok  	github.com/swornagent/sworn/internal/bench	(cached)
ok  	github.com/swornagent/sworn/internal/board	0.004s
ok  	github.com/swornagent/sworn/internal/config	(cached)
ok  	github.com/swornagent/sworn/internal/db	(cached)
ok  	github.com/swornagent/sworn/internal/designaudit	(cached)
ok  	github.com/swornagent/sworn/internal/designfit	(cached)
ok  	github.com/swornagent/sworn/internal/ears	(cached)
ok  	github.com/swornagent/sworn/internal/git	(cached)
ok  	github.com/swornagent/sworn/internal/implement	(cached)
ok  	github.com/swornagent/sworn/internal/journey	(cached)
ok  	github.com/swornagent/sworn/internal/model	(cached)
ok  	github.com/swornagent/sworn/internal/prompt	(cached)
ok  	github.com/swornagent/sworn/internal/reqvalidate	(cached)
ok  	github.com/swornagent/sworn/internal/reqverify	(cached)
ok  	github.com/swornagent/sworn/internal/rtm	(cached)
ok  	github.com/swornagent/sworn/internal/run	(cached)
ok  	github.com/swornagent/sworn/internal/scheduler	(cached)
ok  	github.com/swornagent/sworn/internal/specquality	(cached)
ok  	github.com/swornagent/sworn/internal/state	(cached)
ok  	github.com/swornagent/sworn/internal/supervisor	(cached)
ok  	github.com/swornagent/sworn/internal/telemetry	(cached)
ok  	github.com/swornagent/sworn/internal/tui	0.395s
?   	github.com/swornagent/sworn/internal/verdict	[no test files]
ok  	github.com/swornagent/sworn/internal/verify	(cached)
```
## Reachability artefact

Smoke step (manual): with a fixture slice in `failed_verification` state, run `sworn top`, navigate to the slice using j/k, press Enter, observe the blocked panel with violations extracted from proof.md, press `[4]` to view the full proof, press Esc to return, press `[2]` to verify the context file is written to `.sworn-context.md`.

The binary builds successfully:
```
$ go build -o /tmp/sworn-s04c ./cmd/sworn
(success, no errors)
```

The blocked panel is reachable through the TUI integration point (`cmd/sworn` → `internal/tui.Model` → `handleBoardKey` → `viewBlocked` → `BlockedView`). The `TestBoardEnterTransitionsToBlocked` test exercises the full path from `Model.Update()` through the board view Enter key to the blocked panel transition — this is the integration point test, not a leaf-only test.

## Delivered

- **Selecting a `failed_verification` slice and pressing Enter transitions to blocked panel:** `TestBoardEnterTransitionsToBlocked` — model state transitions from `viewBoard` to `viewBlocked` with `Blocked.sliceID` set. Evidence: `internal/tui/model.go` `handleBoardKey` Enter case, `internal/tui/tui_test.go`.
- **Blocked panel shows violations extracted from proof.md:** `TestBlockedPanelExtractsViolations` — fixture proof.md with `## Violations` (2 entries) and `## Not delivered` (1 entry); `ExtractViolations` returns all 3. Evidence: `internal/tui/blocked.go` `ExtractViolations`, `internal/tui/tui_test.go`.
- **Pin 1: BLOCKED-state detection (implemented + verification.result == "blocked"):** `TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict` — slice at `implemented` with `verification.result: "blocked"` transitions to blocked panel on Enter. Evidence: `internal/tui/model.go` `handleBoardKey`, `internal/tui/board.go` `SliceBoardInfo.VerificationResult`, `internal/tui/tui_test.go`.
- **Pressing [2] writes .sworn-context.md:** `TestOpenAIWritesContextFile` — `WriteContextFile` writes file with spec + violations + diff content. Evidence: `internal/tui/open_ai.go`, `internal/tui/tui_test.go`.
- **If `code` not in PATH, [2] shows graceful message:** `TestLaunchMissingTool` — `LaunchClaudeCode` returns error, `BlockedView.Update` shows "Claude Code not found — context written to <path>". Evidence: `internal/tui/blocked.go` case "2", `internal/tui/open_ai.go`, `internal/tui/tui_test.go`.
- **Pressing [5] (defer) writes status.json + intake.md:** `TestDeferWritesRuleTwo` — after defer with reason, `status.json` has `state: deferred`, `intake.md` contains slice ID, reason, "Why", and "Acknowledged". Evidence: `internal/tui/blocked.go` `deferSlice` + `AppendDeferralToIntake`, `internal/tui/tui_test.go`.
- **Pressing [4] (view proof) opens scrollable panel, Esc returns:** `TestBlockedPanelViewProof` — pressing [4] sets `viewingProof=true`, View shows raw proof content, Esc returns to options. Evidence: `internal/tui/blocked.go` case "4" + `View()`, `internal/tui/tui_test.go`.
- **Board cursor navigation (Pin 2):** `Cursor` field on `BoardView`, j/k navigation in `handleBoardKey`, visual selection indicator (`▸`) in `board.go` `View()`. Evidence: `internal/tui/board.go`, `internal/tui/model.go`.
- **ADR-0005 for bubbles dep (Pin 3):** `docs/adr/0005-tui-dep-bubbles.md` written before dep added. `go.mod`/`go.sum` in `planned_files`.
- **Auto-fix [1] stub (Pin 5):** Shows inline message with `sworn run` command, no subprocess. Evidence: `internal/tui/blocked.go` case "1".

## Not delivered

- **AI tool list beyond Claude Code + Codex:** Configurable via `SWORN_AI_TOOLS` env post-R3. **Why:** Two tools cover the immediate use cases. **Tracking:** TBD. **Acknowledged:** Coach, 2026-06-21 (spec "Deferrals allowed?").
- **Auto-fix [1] stubbed to inline message:** Shows `sworn run --slice <id> --release <name>` command instead of running subprocess. **Why:** Subprocess management from Bubble Tea is non-trivial (captures stdout). **Tracking:** TBD. **Acknowledged:** Coach, 2026-06-21 (spec "Deferrals allowed?", approved-ack.md Pin 5).

## Divergence from plan

Three files were changed but not listed in spec.md "Planned touchpoints." Each
change is a necessary consequence of the feature scope and is documented here
per Gate 2:

1. **`internal/state/state.go`** — Added `Deferred` state constant (`State = "deferred"`)
   and corresponding transitions in `allowedTransitions` (from `Planned`, `DesignReview`,
   `InProgress`, `Implemented`, `FailedVerification` → `Deferred`; `Deferred` → `InProgress`).
   **Rationale:** Acceptance check 5 requires pressing `[5]` (defer) to write `state: deferred`
   to `status.json`. The `Deferred` state must exist in the state machine for the transition
   to be legal via `state.Transition()`. Without this, the defer action would write an
   invalid state. Also fixed gofmt issues (misaligned struct fields, missing newline at EOF).

2. **`internal/tui/board.go`** — Added `VerificationResult` field to `SliceBoardInfo`
   (populated from `status.json` `verification.result`), `Cursor` and `orderedSlices`
   fields to `BoardView` for keyboard navigation, and `SliceStateColor` call in `View()`
   to correctly colour slices with `verification.result == "blocked"`.
   **Rationale:** Design review Pin 1 required detecting BLOCKED-state slices (state
   `implemented` + `verification.result == "blocked"`) for the Enter-to-blocked-panel
   transition. Pin 2 required board cursor navigation (j/k selection). Both are necessary
   for the acceptance check "Selecting a `failed_verification` slice and pressing Enter
   transitions to the blocked panel" to work correctly, including the `implemented` +
   `blocked` case. Also fixed gofmt (missing newline at EOF).

3. **`internal/tui/styles.go`** — Added `BoardItemSelected` style for cursor highlight,
   `SliceStateColor` function (considers `verification.result` for colour), `deferred`
   case in `StateColor` switch, and fixed gofmt issues (misaligned variable declarations,
   missing closing paren on `EmptyMessage`, missing newline at EOF).
   **Rationale:** Design review Pin 1 required correct colour rendering of BLOCKED-state
   slices. Pin 2 required a visual selection indicator. The `deferred` state colour is
   needed because acceptance check 5 transitions a slice to `deferred`, which must render
   correctly on the board. The gofmt fixes were necessary because the file had pre-existing
   formatting violations that `go vet` / `gofmt` would flag.

All other acceptance checks addressed. Pin 1 fix applied during re-entry session — the
initial implementation checked `si.State == "blocked"` (never a state value); fixed to
check both `failed_verification` and `implemented` + `verification.result == "blocked"`.

**Forward-merge artefacts:** The diff vs `start_commit` includes `docs/release/.../index.md`
and `S42`–`S47` spec.md + status.json files. These are planner-added slices from other
tracks that merged into `release-wt/2026-06-19-safe-parallelism` and were forward-merged
into this track branch. They are not part of S04c's implementation.

## First-pass script output

```
$ BASE_BRANCH=release-wt/2026-06-19-safe-parallelism release-verify.sh S04c-tui-resolution 2026-06-19-safe-parallelism

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
  PASS  state is implemented (eligible for verifier review)

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  diff base: start_commit 83e38dd14e85460a26cc03970aee731d6aff1abd
  PASS  27 file(s) changed vs diff base

== Dark-code markers in changed files ==
  FAIL  dark-code markers found in changed source files (must be Rule 2 deferrals)
  hits:
    internal/state/state.go: Deferred State = "deferred"
    internal/tui/blocked.go: b.message = "Slice deferred successfully!"
    internal/tui/model.go: // Reload board to reflect any state changes (e.g. deferred)
    internal/tui/styles.go: case "deferred":
    internal/tui/tui_test.go: t.Errorf("expected state 'deferred', got %q", st.State)

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md Not delivered deferrals carry non-placeholder tracking refs
  PASS  proof.md Files changed count (27) consistent with diff vs start_commit (27)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

== First-pass verdict ==
  checks passed: 22
  checks failed: 1

FIRST-PASS FAIL
```

**Dark-code marker false positive analysis:** The single FAIL is a known false
positive. The script DARK_PATTERNS regex includes `deferred`, which matches the
canonical Baton state name `Deferred` / `"deferred"`. All 5 hits are legitimate
uses of the state name in code:
- `internal/state/state.go`: `Deferred State = "deferred"` — the state constant definition
- `internal/tui/blocked.go`: `"Slice deferred successfully!"` — user-facing success message
- `internal/tui/model.go`: comment about state changes — documentation
- `internal/tui/styles.go`: `case "deferred":` — switch case for state colour rendering
- `internal/tui/tui_test.go`: test assertion on state value

None of these are TODO/FIXME/HACK markers. The word "deferred" is the
protocol-defined state name, not a dark-code deferral comment.