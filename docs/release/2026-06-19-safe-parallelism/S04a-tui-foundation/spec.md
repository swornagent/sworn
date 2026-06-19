---
title: 'S04a-tui-foundation — sworn (no args) opens releases list + board view'
description: 'sworn with no arguments launches a Bubble Tea TUI showing a releases list on the left and a board view (tracks + slice states) on the right. Navigation and basic keyboard controls only.'
---

# Slice: `S04a-tui-foundation`

## User outcome

A developer runs `sworn` with no arguments and sees a two-pane TUI: a releases list
on the left (all releases under `docs/release/`) and a board view on the right showing
tracks, slice IDs, states, and last-updated timestamps for the selected release.

## Entry point

`sworn` binary with zero arguments — `cmd/sworn/main.go` detects no subcommand and
launches the Bubble Tea TUI.

## In scope

- No-args detection in `cmd/sworn/main.go`: if `os.Args[1:]` is empty, call `tui.Run()`
- Bubble Tea root model (`internal/tui/model.go`):
  - Two-pane layout via lipgloss
  - Left pane: releases list (scans `docs/release/*/index.md`, shows release name,
    track count, aggregate state derived from slice status.json files)
  - Right pane: board view for selected release — tracks table showing track ID,
    slice list, per-slice state (read from each slice's `status.json` in the primary
    worktree), last-updated-at
  - State machine: `viewReleases` → (select release) → `viewBoard`
- `internal/tui/releases.go`: releases list Bubble Tea component
- `internal/tui/board.go`: board view component; reads `index.md` frontmatter for
  track/slice structure; reads each slice's `status.json` for live state
- `internal/tui/styles.go`: lipgloss colour + layout constants
- Keyboard: `j`/`k` navigate list; `Enter` selects; `Esc` goes back; `q` quits; `?`
  shows a one-line help line at the bottom
- `cmd/sworn/top.go`: if R2's S15 created this file, it is updated to delegate to
  `tui.Run()` so `sworn top` and `sworn` (no args) behave identically; if it does not
  exist yet, leave a TODO comment noting the alias

## Out of scope

- Live concurrent status from SQLite DB (S04b)
- Blocked-slice TL;DR panel and options (S04c)
- Credits display (S04b)
- Mouse support

## Planned touchpoints

- `cmd/sworn/main.go` (touch — no-args detection)
- `cmd/sworn/top.go` (touch — delegate or note alias)
- `internal/tui/model.go` (new)
- `internal/tui/releases.go` (new)
- `internal/tui/board.go` (new)
- `internal/tui/styles.go` (new)
- `internal/tui/tui_test.go` (new)

## Acceptance checks

- [ ] `sworn` (no args) in this repo launches the TUI without error and shows all
  releases under `docs/release/` in the left pane (verified by smoke step)
- [ ] Pressing `j`/`k` moves selection in the releases list; pressing `Enter` switches
  the right pane to the board view for the selected release
- [ ] The board view lists all tracks from the selected release's `index.md` frontmatter
  and shows each slice's current state from its `status.json`
- [ ] Pressing `Esc` returns to the releases list; pressing `q` exits cleanly
- [ ] `go test ./internal/tui/...` passes without a TTY (model state machine tests only)
- [ ] `go build ./...` succeeds; the binary is not significantly larger than before
  (lipgloss is the only new dep; bubble tea if not already present from R2)

## Required tests

- **Unit**: `internal/tui/tui_test.go`
  — `TestReleasesListPopulates`: given a fixture `docs/release/` directory with two
    index.md files, the releases list model contains exactly those two entries
  — `TestBoardViewShowsSlices`: given a fixture release with 3 slices at known states,
    the board view model contains those states after `board.Load()`
  — `TestKeyNavigation`: simulate `j`, `k`, `Enter`, `Esc` keypresses on the model;
    assert correct view transitions
- **Reachability artefact**: run `sworn` (no args) in this repo; navigate to the
  `2026-06-19-safe-parallelism` release; observe board view renders S01–S08 in
  `planned` state. Document terminal screenshot path or describe exact observation
  in proof.md.

## Risks

- Bubble Tea requires a real TTY. Tests must be pure model-state tests with no
  rendering. The smoke step is the reachability artefact.
- lipgloss and bubbletea are new deps if R2's S15 didn't already add them. If they
  did, reuse the same versions — do not introduce a duplicate or upgraded copy.

## Deferrals allowed?

No. S04b and S04c both extend this foundation.
