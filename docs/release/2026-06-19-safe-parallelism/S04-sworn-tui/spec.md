---
title: 'S04-sworn-tui — management cockpit (no args)'
description: 'sworn with no arguments opens a Bubble Tea TUI showing releases, live concurrent track status, blocked-slice TL;DR with structured resolution options, and credits.'
---

# Slice: `S04-sworn-tui`

## User outcome

A developer runs `sworn` with no arguments and sees a live management cockpit: a
releases list on the left, board/track detail on the right, live concurrent status
from the DB, and — when a slice is blocked — a TL;DR of violations with one-keypress
options to auto-fix, open in Claude Code, open in Codex, view the full proof, or defer.

## Entry point

`sworn` binary invoked with zero arguments — `cmd/sworn/main.go` detects no subcommand
and launches the Bubble Tea TUI instead of printing help.

## In scope

- No-args detection in `cmd/sworn/main.go`
- Bubble Tea root model with two-pane layout:
  - **Left pane — Releases list**: scans `docs/release/*/index.md`, shows release name,
    track count, aggregate state (all planned / N verified / blocked)
  - **Right pane — Detail**: switches between views based on selection
- **Board view** (selected release): tracks table + slices with live state; state read
  from `status.json` in the primary worktree for planned slices, from SQLite DB for
  in-flight slices
- **Concurrent status view** (active release): live per-track progress polled from
  SQLite DB every 1 second via `time.Tick`; shows track ID, current slice, started_at,
  elapsed time; credits consumed (from `~/.config/sworn/credits.json` cache)
- **Blocked-slice TL;DR panel**: triggered when user selects a `failed_verification`
  or `BLOCKED` slice; violations extracted from `proof.md` (no model call — violations
  are already structured text in the proof bundle); shows:
  - Slice ID, track, release
  - Violation list (numbered, from proof.md)
  - Structured options menu:
    - `[1]` auto-fix + rerun: calls `sworn run` on the specific slice
    - `[2]` open in Claude Code: assembles context file (spec + violations + diff)
      at `<worktree>/.sworn-context.md`; opens worktree in Claude Code via
      `code <worktree>` or configured `SWORN_CLAUDE_CODE_CMD`
    - `[3]` open in Codex: same, via `codex <worktree>` or `SWORN_CODEX_CMD`
    - `[4]` view full proof bundle (scrollable panel)
    - `[5]` defer with reason (prompts for text input, writes Rule 2 deferral)
- Keyboard navigation: `j`/`k` up/down, `Enter` select, `q` quit, `?` help overlay
- Absorbs R2's `sworn top` (S15): `cmd/sworn/top.go` is refactored to delegate to the
  new TUI; `sworn top` becomes an alias for `sworn` with no args (or is removed if R2's
  top.go is the sole consumer)

## Out of scope

- Embedded AI chat interface — no model calls from the TUI (except the auto-fix action
  which shells out to `sworn run`)
- Image capture or rich markdown rendering
- Web dashboard (swornagent.com — separate commercial infrastructure)
- Configurable AI tool list beyond Claude Code + Codex (extend post-R3)
- The MCP server (S08)
- Mouse support (keyboard only for R3)

## Planned touchpoints

- `cmd/sworn/main.go` (touch — no-args detection + TUI launch)
- `cmd/sworn/top.go` (touch — delegate to internal/tui or remove; coordinate with R2 S15)
- `internal/tui/model.go` (new — root Bubble Tea model, key bindings, view switching)
- `internal/tui/releases.go` (new — releases list component)
- `internal/tui/board.go` (new — board/track/slice detail component)
- `internal/tui/concurrent.go` (new — live DB poll component, 1s tick)
- `internal/tui/blocked.go` (new — TL;DR panel + options menu)
- `internal/tui/open_ai.go` (new — context file assembly + AI tool launch)
- `internal/tui/styles.go` (new — Bubble Tea lipgloss style definitions)
- `internal/tui/tui_test.go` (new — model state machine tests, non-visual)

## Acceptance checks

- [ ] `sworn` (no args) launches the TUI without error in a repo with at least one
  release under `docs/release/`
- [ ] The releases list shows all releases found under `docs/release/*/index.md`
- [ ] Navigating to an in-progress release shows live track status updating every ~1s
  (verified by watching the elapsed time counter increment in the smoke test)
- [ ] Selecting a `failed_verification` slice shows the TL;DR panel with violations
  extracted from `proof.md` (test with a fixture proof.md containing known violations)
- [ ] `[2]` on a blocked slice writes `.sworn-context.md` to the worktree and attempts
  to launch Claude Code (the launch command is logged; no error if Claude Code is absent
  — degrades gracefully with a "Claude Code not found; context written to <path>" message)
- [ ] `[5]` defer prompts for a reason, writes the deferral to `status.json` and appends
  a Rule 2 deferral entry to the release `intake.md`
- [ ] `sworn top` (if retained as an alias) produces the same output as `sworn` with
  no args (or is cleanly removed with a deprecation note in the commit message)
- [ ] `go test ./internal/tui/...` passes (model state machine tests, no TTY required)

## Required tests

- **Unit**: `internal/tui/tui_test.go`
  — `TestBlockedPanelExtractsViolations`: given a fixture `proof.md` with 2 violations,
    assert the blocked panel model contains exactly those 2 violation strings
  — `TestConcurrentStatusPoll`: inject a fake DB with known track state; advance the
    1s tick; assert the concurrent status component model reflects the new state
  — `TestDeferWritesRuleTwo`: select option [5], provide a reason; assert
    `status.json` state is `deferred` and `intake.md` has the deferral appended
- **Reachability artefact**: explicit smoke step — run `sworn` (no args) in this repo;
  navigate to the `2026-06-19-safe-parallelism` release; observe the board view renders
  S01-S08 in `planned` state. Document terminal output (or screen recording path) in
  proof.md.

## Risks

- Bubble Tea requires a real TTY. CI environments (no TTY) will cause the TUI to fail
  to render. Mitigation: the unit tests test the model state machine only, not
  rendering. The reachability artefact is a manual smoke step. A `--no-tui` flag or
  env var fallback to `sworn board` (tabular output) is deferred to post-R3.
- The `open_ai.go` logic for launching Claude Code / Codex is platform-dependent
  (`open -a`, `code`, `codex` CLI). Graceful degradation (log the context file path,
  don't error) is required; the AI launch is best-effort.

## Deferrals allowed?

Yes, with Rule 2 compliance for each:
- Mouse support: deferred post-R3. Why: keyboard-first is sufficient for R3. Tracking: TBD.
- AI tool list beyond CC + Codex: deferred. Why: 2 tools covers the main cases; list
  is config-extensible post-R3 via `SWORN_AI_TOOLS` env. Tracking: TBD.
