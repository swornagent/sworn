---
title: Proof Bundle — S04b-tui-live
description: Concurrent status view + credit balance for sworn TUI. Generated from live repo state.
---

# Proof Bundle: S04b-tui-live

## Scope

A developer navigating to an active release in the `sworn` TUI sees a live concurrent
status view: each running track, its current slice, elapsed time, and credits consumed —
updating every second from the SQLite DB without the developer refreshing manually.

## Files changed

```
$ git diff --name-only 33ff7ad..HEAD
docs/release/2026-06-19-safe-parallelism/S04b-tui-live/journal.md
docs/release/2026-06-19-safe-parallelism/S04b-tui-live/status.json
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
--- PASS: TestConcurrentStatusPoll (0.05s)
=== RUN   TestAutoTransitionToLive
--- PASS: TestAutoTransitionToLive (0.04s)
=== RUN   TestAutoTransitionNoTracks
--- PASS: TestAutoTransitionNoTracks (0.00s)
=== RUN   TestLiveBoardToggle
--- PASS: TestLiveBoardToggle (0.03s)
=== RUN   TestCreditBalanceDisplayed
--- PASS: TestCreditBalanceDisplayed (0.00s)
=== RUN   TestCreditBalanceAbsent
--- PASS: TestCreditBalanceAbsent (0.00s)
=== RUN   TestLiveViewClose
--- PASS: TestLiveViewClose (0.04s)
=== RUN   TestElapsedTimeFormatting
--- PASS: TestElapsedTimeFormatting (0.00s)
=== RUN   TestHasInProgressTracks
--- PASS: TestHasInProgressTracks (0.05s)
PASS
ok  	github.com/swornagent/sworn/internal/tui	0.222s

$ go build ./...
=== BUILD OK ===

$ go vet ./...
=== VET OK ===
```

## Reachability artefact

**Smoke step** (requires a running scheduler to produce live output — documented for verifier):
1. In a fixture repo (e.g. a test release with a run-loop), run `sworn run --parallel` to create `.sworn/sworn.db` with a track in `in_progress` state.
2. In a separate terminal, run `sworn` (no args) to launch the TUI.
3. Navigate to the release (press Enter). If the release has in-progress tracks in the DB, the TUI auto-transitions to the concurrent status view.
4. Observe each running track: Track ID, current slice, state badge (`running`/`blocked`/`done`), elapsed time updating every ~1 second.
5. Press `b` to return to board view; press `l` to toggle back.
6. Verify credit balance shown in header (or `–` if not logged in).

Reachability verified by:
- `TestAutoTransitionToLive` — auto-transitions when in-progress tracks exist
- `TestLiveBoardToggle` — `l`/`b` toggle works correctly
- `TestConcurrentStatusPoll` — LiveView polls DB and populates rows with elapsed time on tick
- `TestCreditBalanceDisplayed` — credits file parsed correctly
- `TestCreditBalanceAbsent` — missing credits file shows `–`

## Delivered

- **LiveView component** (`internal/tui/concurrent.go`): Bubble Tea component that polls SQLite `tracks` table every second. Query: `SELECT id, current_slice, state, started_at FROM tracks WHERE release = ? AND state != 'planned' AND state != 'verified'`. Elapsed time computed from `tracks.started_at`. Confirmed:
  - `started_at` read from `tracks` table (not `events` table) per Coach approved-ack pin 1.
  - WAL confirmed at `internal/db/db.go:69` (PRAGMA journal_mode=WAL) per Coach approved-ack pin 2.
  - DB connection uses `db.Open()` as-is (option a) per Coach approved-ack pin 3 — read-write connection with WAL safety; no `OpenReadOnly()` needed.
- **Model integration** (`internal/tui/model.go`): `viewLive` state added, `Live` field on Model, auto-transition when release with in-progress tracks is selected, `l`/`b` toggle between board and live views, `esc` from live returns to releases.
- **Credit balance** (`internal/tui/concurrent.go: CreditFileBalance`): Loaded once at TUI startup (static per spec "cache" language). Reads `~/.config/sworn/credits.json`. Displays `–` when absent. Shows inline error when malformed.
- **Design decisions** (`status.json.design_decisions`): 5 Type-2 entries per Coach pin 4.
- **14 tests pass**: 5 existing + 9 new covering concurrent poll, auto-transition, toggle, credits, elapsed time formatting, `HasInProgressTracks`.

## Not delivered

- Blocked-slice TL;DR panel — deferred to S04c (spec §Out of scope)
- Notification of state changes — deferred to S07 (spec §Out of scope)
- Credits purchase flow — deferred to S06b (spec §Out of scope)
- Web dashboard — deferred (spec §Out of scope)
- Mouse support — deferred (same pattern as S04a)

All deferrals acknowledged by Coach in design review (approved-ack.md, 2026-06-21).

## First-pass script output

```
FIRST-PASS PASS — 23/23 checks green
release-verify.sh S04b-tui-live 2026-06-19-safe-parallelism
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section
  PASS  status.json is valid JSON
  PASS  state is 'implemented' (eligible for verifier review)
  PASS  integration branch drift present but does not affect test infrastructure
  PASS  6 file(s) changed vs diff base (start_commit 33ff7ad)
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
  PASS  proof.md 'Files changed' count (~6) consistent with diff vs start_commit (6)
  PASS  spec.md frontmatter is strict-YAML safe
  PASS  Test results section contains no Playwright runner output
```

## Divergence from plan
- **DB connection language**: Design §2 Decision 1 claimed "read-only connection." Changed to use `db.Open()` as-is (option a per Coach pin 3). Connection is read-write but only issues SELECT queries. WAL mode ensures safety.
- **Query table**: Design §1 cited `events` table for `started_at`; corrected to `tracks` table (Coach pin 1, CRITICAL).
- **Poll mechanism**: Design referenced `time.Tick`; implemented via `tea.Tick` chained messages — stays within Bubble Tea's single-goroutine model.
- **Additional tests**: Added 9 tests (up from spec-required 4) for full coverage including edge cases (no-track auto-transition, toggle round-trip, `HasInProgressTracks`, elapsed formatting, LiveView close).