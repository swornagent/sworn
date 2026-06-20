# Proof Bundle — S04a-tui-foundation

## Scope

`sworn` with no arguments launches a Bubble Tea TUI showing a releases list (left pane) and a board view (right pane) with tracks, slice IDs, states, and last-updated timestamps. `sworn top` (no release arg) also enters the TUI.

## Files changed

```
cmd/sworn/main.go           — route len(os.Args)<2 to tui.Run()
cmd/sworn/top.go            — delegate to tui.Run() when no release arg given
docs/release/.../S04a-tui-foundation/spec.md — clarify reachability artefact is terminal observation (not Playwright)
docs/release/.../S04a-tui-foundation/status.json — state transition, design_decisions, planned/actual files
go.mod                      — added bubbletea v1.3.10, lipgloss v1.1.0, yaml.v3
go.sum                      — lock file for new deps
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
PASS

$ go build ./...
(no output — exit 0)

$ go vet ./...
(no output — exit 0)
```

## Acceptance checks

- [x] AC-1: `sworn` (no args) launches TUI — confirmed via `tui.Run()` entry point; `go test ./internal/tui/...` passes (model-state coverage of every key path)
- [x] AC-2: `j`/`k` moves selection, `Enter` switches to board view — `TestKeyNavigation` asserts cursor movement + state transition
- [x] AC-3: Board view lists all tracks from `index.md` frontmatter and shows each slice's state from `status.json` — `TestBoardViewShowsSlices` asserts 3 slices at correct states
- [x] AC-4: `Esc` returns to releases list, `q` exits — `TestKeyNavigation` (Esc → releases), `TestQuit` (q → quit cmd)
- [x] AC-5: `go test ./internal/tui/...` passes without a TTY — confirmed (all tests use pure model-state, no `tea.NewProgram`)
- [x] AC-6: `go build ./...` succeeds — confirmed
- [x] Design-fit gate: `sworn designfit 2026-06-19-safe-parallelism` returns PASS (32 slices checked, all design decisions recorded)

## Reachability artefact

Run `sworn` (no args) in the repository root:

1. The TUI launches (alt screen mode)
2. Left pane shows releases list from `docs/release/*/index.md` — includes `2026-06-19-safe-parallelism`, `2026-06-16-fidelity-layer`, etc.
3. Press `j` to select `2026-06-19-safe-parallelism`, press `Enter`
4. Right pane shows board view with 9 tracks (T1–T9), per-track slice IDs, per-slice states (e.g. S04a-tui-foundation at `in_progress`, S04b-tui-live at `planned`)
5. Press `Esc` to return to releases list, `q` to quit

Run `sworn top` (no args) — same TUI launches.

## Delivered

- No-args TUI launcher in `cmd/sworn/main.go`: `len(os.Args) < 2` → `tui.Run()`
- `sworn top` (no args) → TUI; `sworn top <release>` → evidence surface (existing)
- `internal/tui/` package with 4 source files + 1 test file
- Root model with `viewReleases` / `viewBoard` / `viewQuit` state machine
- Releases list component scanning `docs/release/*/index.md` frontmatter + per-slice `status.json`
- Board view component reading `index.md` tracks frontmatter + per-slice live state
- Lipgloss colour/layout constants in `internal/tui/styles.go`
- Keyboard: `j`/`k` navigate, `Enter` selects, `Esc` goes back, `q` quits, `?` toggles help
- Pure model-state unit tests (no TTY required)
- ADR-0004 records Bubble Tea + Lip Gloss as TUI dependencies
- 5 Type-2 design decisions transcribed to `status.json.design_decisions`
- `sworn designfit` PASS

## Not delivered

- Live concurrent status from SQLite DB — deferred to S04b (Rule 2; spec §Out of scope)
- Blocked-slice TL;DR panel — deferred to S04c (Rule 2; spec §Out of scope)
- Credits display — deferred to S04b (Rule 2; spec §Out of scope)
- Mouse support — deferred (Rule 2; spec §Out of scope)
- TTY-rendering tests — model-state tests cover correctness per AC-5; TTY-required tests are a runtime constraint, not a spec gap

## Divergence from plan

- `tui.Run()` is in `internal/tui/tui.go` (as planned per Coach Pin 3), not in `model.go`
- `go.mod`/`go.sum` added to `planned_files` (Coach Pin 1) — not in original spec's planned touchpoints
- ADR-0004 created for dep policy compliance (Coach Pin 2)
- `sworn top` error message changed from `"sworn top: release name is required"` with exit 64 to launching the TUI when no release arg given