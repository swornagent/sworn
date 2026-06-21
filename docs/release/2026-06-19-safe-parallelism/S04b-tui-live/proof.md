---
title: Proof Bundle — S04b-tui-live
description: Concurrent status view + credit balance for sworn TUI. Generated from live repo state.
---

# Proof Bundle: S04b-tui-live

## Scope

A developer navigating to an active release in the `sworn` TUI sees a live concurrent
status view: each running track, its current slice, elapsed time, and credits consumed -
updating every second from the SQLite DB without the developer refreshing manually.

## Files changed

```
$ git diff --name-only 33ff7ad
docs/release/2026-06-19-safe-parallelism/S04b-tui-live/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S04b-tui-live/journal.md
docs/release/2026-06-19-safe-parallelism/S04b-tui-live/proof.md
docs/release/2026-06-19-safe-parallelism/S04b-tui-live/status.json
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/journal.md
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/spec.md
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/status.json
docs/release/2026-06-19-safe-parallelism/S27-public-readiness-scrub/journal.md
docs/release/2026-06-19-safe-parallelism/S27-public-readiness-scrub/spec.md
docs/release/2026-06-19-safe-parallelism/S27-public-readiness-scrub/status.json
docs/release/2026-06-19-safe-parallelism/S28-git-dir-guard/journal.md
docs/release/2026-06-19-safe-parallelism/S28-git-dir-guard/spec.md
docs/release/2026-06-19-safe-parallelism/S28-git-dir-guard/status.json
docs/release/2026-06-19-safe-parallelism/index.md
internal/adopt/baton/rules/10-customer-journey-validation.md
internal/prompt/implementer.md
internal/tui/concurrent.go
internal/tui/model.go
internal/tui/styles.go
internal/tui/tui_test.go
```
## Test results

```
$ go test ./internal/tui/... -v -count=1
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
--- PASS: TestAutoTransitionToLive (0.03s)
=== RUN   TestAutoTransitionNoTracks
--- PASS: TestAutoTransitionNoTracks (0.00s)
=== RUN   TestLiveBoardToggle
--- PASS: TestLiveBoardToggle (0.03s)
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
--- PASS: TestHasInProgressTracks (0.04s)
PASS
ok  	github.com/swornagent/sworn/internal/tui	0.226s

$ go build ./...
=== BUILD OK ===

$ go vet ./...
=== VET OK ===
```

## Reachability artefact

**Smoke step** (requires a running scheduler to produce live output - documented for verifier):
1. In a fixture repo (e.g. a test release with a run-loop), run `sworn run --parallel` to create `.sworn/sworn.db` with a track in `in_progress` state.
2. In a separate terminal, run `sworn` (no args) to launch the TUI.
3. Navigate to the release (press Enter). If the release has in-progress tracks in the DB, the TUI auto-transitions to the concurrent status view.
4. Observe each running track: Track ID, current slice, state badge (`running`/`blocked`/`done`), elapsed time updating every ~1 second.
5. Press `b` to return to board view; press `l` to toggle back.
6. Verify credit balance shown in header (or `--` if not logged in).

Reachability verified by:
- `TestAutoTransitionToLive` - auto-transitions when in-progress tracks exist
- `TestLiveBoardToggle` - `l`/`b` toggle works correctly
- `TestConcurrentStatusPoll` - LiveView polls DB and populates rows with elapsed time on tick
- `TestModelTickForwarding` - tickMsg forwarded through Model.Update(), not just LiveView.Update() directly
- `TestCreditBalanceDisplayed` - credits file parsed correctly
- `TestCreditBalanceAbsent` - missing credits file shows `--`

## Delivered

- **LiveView component** (`internal/tui/concurrent.go`): Bubble Tea component that polls SQLite `tracks` table every second. Query: `SELECT id, current_slice, state, started_at FROM tracks WHERE release = ? AND state != 'planned' AND state != 'verified'`. Elapsed time computed from `tracks.started_at`. Confirmed:
  - `started_at` read from `tracks` table (not `events` table) per Coach approved-ack pin 1.
  - WAL confirmed at `internal/db/db.go:69` (PRAGMA journal_mode=WAL) per Coach approved-ack pin 2.
  - DB connection uses `db.Open()` as-is (option a) per Coach approved-ack pin 3 - read-write connection with WAL safety; no `OpenReadOnly()` needed.
- **Model integration** (`internal/tui/model.go`): `viewLive` state added, `Live` field on Model, auto-transition when release with in-progress tracks is selected, `l`/`b` toggle between board and live views, `esc` from live returns to releases. **CRITICAL FIX**: Added `tickMsg` case to `Model.Update()` that forwards to `m.Live.Update(msg)` and chains the returned `tea.Cmd`. Without this, the tick chain terminates after first fire (verifier Violation 1).
- **Credit balance** (`internal/tui/concurrent.go: CreditFileBalance`): Loaded once at TUI startup (static per spec "cache" language). Reads `~/.config/sworn/credits.json`. Displays `--` when absent. Shows inline error when malformed.
- **Design decisions** (`status.json.design_decisions`): 5 Type-2 entries per Coach pin 4.
- **15 tests pass**: 5 existing + 10 new covering concurrent poll, auto-transition, toggle, credits, elapsed time formatting, `HasInProgressTracks`, Model-level tick forwarding.

## Not delivered

- Blocked-slice TL;DR panel - deferred to S04c (spec Out of scope)
- Notification of state changes - deferred to S07 (spec Out of scope)
- Credits purchase flow - deferred to S06b (spec Out of scope)
- Web dashboard - deferred (spec Out of scope)
- Mouse support - deferred (same pattern as S04a)

All deferrals acknowledged by Coach in design review (approved-ack.md, 2026-06-21).

## First-pass script output

```
FIRST-PASS PASS - 23/23 checks green
release-verify.sh S04b-tui-live 2026-06-19-safe-parallelism
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section
  PASS  status.json is valid JSON
  PASS  state is 'implemented' (eligible for verifier review)
  PASS  worktree branch is current with release/v0.1.0 (no drift)
  PASS  20 file(s) changed vs diff base (start_commit 33ff7ad)
  PASS  no dark-code markers in changed source files
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs
  PASS  proof.md 'Files changed' count (~20) consistent with diff vs start_commit (20)
  PASS  spec.md frontmatter is strict-YAML safe
  PASS  Test results section contains no Playwright runner output```

## Divergence from plan

- **DB connection language**: Design Section 2 Decision 1 claimed "read-only connection." Changed to use `db.Open()` as-is (option a per Coach pin 3). Connection is read-write but only issues SELECT queries. WAL mode ensures safety.
- **Query table**: Design Section 1 cited `events` table for `started_at`; corrected to `tracks` table (Coach pin 1, CRITICAL).
- **Poll mechanism**: Design referenced `time.Tick`; implemented via `tea.Tick` chained messages - stays within Bubble Tea's single-goroutine model.
- **Additional tests**: Added 10 tests (up from spec-required 4) for full coverage including edge cases (no-track auto-transition, toggle round-trip, `HasInProgressTracks`, elapsed formatting, LiveView close, Model-level tick forwarding).
- **styles.go (not in planned touchpoints)**: `LiveTitle`, `LiveRow`, `DividerLine` styles added in `internal/tui/styles.go` for the live view table rendering. Spec planned touchpoints listed only `concurrent.go`, `model.go`, `tui_test.go`. This is a legitimate addition (styles belong in the styles file, not in concurrent.go) but was undocumented in the first-pass proof bundle and is now disclosed here.
- **Forward-merge files**: The diff from `start_commit 33ff7ad` includes 14 files from other slices (S21, S27, S28, index.md, internal/adopt/baton/rules/10-customer-journey-validation.md, internal/prompt/implementer.md) that were forward-merged from `release-wt/2026-06-19-safe-parallelism`. These are not changes from this slice. The 6 slice-local files are: `concurrent.go`, `model.go`, `styles.go`, `tui_test.go`, `journal.md`, `proof.md` (status.json and approved-ack.md are board state, not production code).
