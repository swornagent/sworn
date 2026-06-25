# Design TL;DR — S04b-tui-live

## §1. User-visible change

A developer running `sworn` from their repo root navigates to a release in the
existing two-pane TUI (S04a). If that release has active tracks in the SQLite
run-loop database (`.sworn/sworn.db`), the TUI auto-transitions from the board
view to a **live concurrent status view** that polls the database every second.
The live view shows each running track's ID, current slice, state badge
(`running`/`verifying`/`blocked`/`done`), and elapsed time since `started_at`,
all updating in real time without a manual refresh. The user can press `l` to
toggle between board and live views, and `b` to return to the board. The header
bar shows the credit balance from `~/.config/sworn/credits.json` (or `–` when
absent). This is a read-only display — no actions (S04c).

## §2. Design decisions not in spec (max 5)

1. **DB path anchored at repo-root `.sworn/sworn.db`** — The spec says "polls
   the SQLite DB" but doesn't say which one. The `run` package creates it at
   `<workspaceRoot>/.sworn/sworn.db`. Since the TUI already discovers `repoRoot`
   via `git rev-parse --show-toplevel`, the DB path is `filepath.Join(repoRoot,
   ".sworn", "sworn.db")`. The TUI opens its own read-only connection to this
   DB (SQLite WAL mode supports concurrent readers while the scheduler writes).

2. **Polling via `tea.Tick` not `time.Tick` goroutine** — Bubble Tea provides
   `tea.Tick(d, fn)` which returns a `tea.Cmd` that fires once after `d`,
   delivering a message to `Update`. The concurrent component chains ticks:
   every second a `tickMsg` arrives → model re-queries the DB → schedules the
   next tick. This avoids a goroutine and stays within Bubble Tea's single-
   goroutine message model. The spec's `time.Tick` reference is a design
   intent, not a binding implementation choice.

3. **Live view reads from SQLite directly, not status.json** — The S04a board
   view reads `status.json` files from disk (a snapshot at board-load time).
   The concurrent view reads live state from the `tracks` and `events` SQLite
   tables written by the run-loop's scheduler/supervisor. These are two distinct
   data sources with different freshness characteristics — the live view is
   fully DB-backed and does not touch `status.json`.

4. **State machine: `viewLive` is a sub-state of `viewBoard`, not a sibling** —
   The `l`/`b` toggle only works from `viewBoard`/`viewLive`; pressing `Esc`
   from either returns to `viewReleases`. This means `viewLive` is accessible
   only when a release's board is loaded, keeping keyboard navigation simple
   and avoiding edge cases where `l` behaves differently per view.

5. **Credits file path: `os.UserHomeDir()/.config/sworn/credits.json`** — The
   spec says `~/.config/sworn/credits.json` (next to `config.json`). I use
   `os.UserHomeDir()` + `filepath.Join(".config", "sworn", "credits.json")`
   which matches the `config.Path()` pattern. If absent, `–` is shown silently;
   if present but corrupted, `err` is shown inline (not a TUI crash).

## §3. Files I'll touch grouped by purpose

- **New live view component** (new file):
  - `internal/tui/concurrent.go` — `LiveView` struct (Bubble Tea component),
    `tickMsg`, DB polling query for tracks + events, elapsed time rendering,
    credit balance display header, table render, `Init`/`Update`/`View`

- **Root model extension** (touch existing):
  - `internal/tui/model.go` — add `viewLive` to `viewState` enum, add `LiveView`
    field to `Model`, wire `l`/`b` key handling, auto-transition logic on board
    load, credit balance loaded at startup, update `View()` and help bar

- **Tests** (touch existing):
  - `internal/tui/tui_test.go` — `TestConcurrentStatusPoll` (fake DB, advance
    tick), `TestAutoTransitionToLive` (in-progress track in DB triggers view),
    `TestCreditBalanceDisplayed` (fixture credit file), `TestCreditBalanceAbsent`
    (no credit file shows `–`)

## §4. Things I'm NOT doing

- Blocked-slice TL;DR panel — deferred to S04c (Rule 2: spec §Out of scope)
- Notification on track state change — deferred to S07 (Rule 2: spec §Out of scope)
- Credits purchase flow — deferred to S06b (Rule 2: spec §Out of scope)
- Web dashboard — deferred (Rule 2: spec §Out of scope)
- Writing to the DB — this view is read-only per spec §In scope ("no actions")
- Writing any `status.json` files — live view is DB-backed only
- Mouse support — deferred (same pattern as S04a)

## §5. Reachability plan

1. `go build ./...` — build succeeds, TUI links with no new dependencies (uses
   existing `modernc.org/sqlite` via `internal/db`)
2. `go test ./internal/tui/...` — all 4 new unit tests pass without a TTY
3. Run `sworn run --parallel` in a fixture repo (or primary repo) so it creates
   `.sworn/sworn.db` with a running track; in a separate terminal run `sworn`
   (no args), navigate to the release, observe auto-transition to live view
   showing the running track with incrementing elapsed time
4. Press `b` to return to board view; press `l` to toggle back
5. Remove `~/.config/sworn/credits.json` (or stub it with `{"balance": 42}`)
   and verify header updates accordingly

## §6. Open questions for the Coach

- **DB connection lifecycle**: Should the `LiveView` open its own SQLite
  connection on construction (and close on `viewQuit`), or should a single DB
  connection be opened at the `Model` level and shared with the live view? My
  design opens a per-view connection so the DB path can be bound per-release
  (each release may have its own DB) and the connection is cleaned up when the
  view exits.
- **Credit balance refresh**: Should the credit balance be loaded once at
  startup (static), or re-read every N ticks (dynamic)? The spec says "reads
  ... cache" suggesting static load. I'll load on TUI startup (once) and cache.
- **Auto-transition vs offer `l`**: The spec says "transitions to (or offers
  `l` to view)" — my design auto-transitions AND allows `l`/`b` toggle. Is
  auto-transition the right default, or should it always require `l` first?
  (I chose auto-transition since the AC under "Acceptance checks" lists it as
  primary, with `l` as alternative.)