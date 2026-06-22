# Design TL;DR — S04a-tui-foundation

## §1. User-visible change

A developer runs `sworn` with **no arguments** and sees a two-pane Bubble Tea TUI:
left pane lists all releases (scanned from `docs/release/*/index.md`); selecting a
release fills the right pane with a board view showing tracks, slices, states, and
last-updated-at timestamps. Navigation via `j`/`k`/`Enter`/`Esc`/`q`. Running
`sworn` (no args) and `sworn top` (no release arg) both launch the TUI.

## §2. Design decisions not in spec (max 5)

1. **No-args replaces current `usage()` + `exit(64)`** — `main.go` line 27 currently
   calls `usage()` + `os.Exit(64)` for `len(os.Args) < 2`. This slice routes
   `len(os.Args) < 2` to `tui.Run()` instead. The change is minimal and additive.

2. **`sworn top` (no release arg) also enters the TUI** — the existing `cmdTop()` in
   `top.go` renders an evidence surface and requires a release name argument. We
   update it so `sworn top` with zero remaining args (after flag parse) delegates
   to `tui.Run()`. `sworn top <release>` continues to render the evidence surface
   as before — the existing behaviour is preserved.

3. **Releases list reads from on-disk `docs/release/*/index.md`** — the TUI scans
   the filesystem with `filepath.Glob("docs/release/*/index.md")` relative to the
   repo root (detected via `git rev-parse --show-toplevel`). Each release's board
   view reads its `index.md` frontmatter and each slice's `status.json` for live
   state. This is a read-only operation — no DB, no IPC.

4. **Model state machine uses Bubble Tea's built-in `tea.Model` pattern** — a
   root `struct` with a `viewState` enum (`viewReleases`, `viewBoard`, `viewQuit`).
   The `Update` method switches on view state to dispatch keyboard events. This is
   the standard BT pattern; no custom event bus or middleware.

5. **Unit tests are pure model-state, no TTY** — Bubble Tea's `tea.NewProgram`
   requires a real TTY. Tests construct the model directly, send `tea.KeyMsg`
   values via `model.Update(msg)`, and assert on the returned model's state fields
   and view string. No `tea.NewProgram` in unit tests. The spec's AC-5 explicitly
   allows this ("pass without a TTY").

## §3. Files I'll touch grouped by purpose

- **Entry point wiring** (touch existing):
  - `cmd/sworn/main.go` — add TUI import + route `len(os.Args) < 2` to `tui.Run()`
  - `cmd/sworn/top.go` — add delegation to `tui.Run()` when no release arg given

- **New TUI package** (new files):
  - `internal/tui/model.go` — root model, state machine, `Init`/`Update`/`View`
  - `internal/tui/releases.go` — releases list component (reads `docs/release/*/index.md`)
  - `internal/tui/board.go` — board view component (reads `index.md` + `status.json`)
  - `internal/tui/styles.go` — lipgloss colour/layout constants

- **Tests** (new):
  - `internal/tui/tui_test.go` — model state machine unit tests

## §4. Things I'm NOT doing

- Live concurrent status from SQLite DB — deferred to S04b (Rule 2: spec §Out of scope)
- Blocked-slice TL;DR panel — deferred to S04c (Rule 2)
- Credits display — deferred to S04b (Rule 2)
- Mouse support — deferred (Rule 2: spec §Out of scope)
- TTY-rendering tests — TTY-unavailable is a runtime constraint; model-state tests cover correctness per AC-5

## §5. Reachability plan

1. `go build ./...` — build succeeds, binary size is comparable (lipgloss + bubbletea ~2-3MB added)
2. `go test ./internal/tui/...` — model state machine tests pass without a TTY
3. Run `sworn` (no args) in repo root → observe releases list, navigate to `2026-06-19-safe-parallelism`, press Enter → board view shows S04a at `planned` alongside all other slices
4. Verify `j`/`k`/`Enter`/`Esc`/`q` all behave correctly
5. Run `sworn top` (no args) → same TUI opens

## §6. Open questions for the Coach

- **top.go delegation boundary**: Should `sworn top <release>` also open the TUI
  pre-navigated to that release's board? Or preserve its existing evidence-surface
  behaviour when a release arg is given? My design keeps the existing behaviour
  when a release arg is given.
- **Repo root detection**: Should we `git rev-parse --show-toplevel` or assume CWD
  is the repo root? The spec doesn't specify; I'll use `git rev-parse
  --show-toplevel` with a fallback to `os.Getwd()`.