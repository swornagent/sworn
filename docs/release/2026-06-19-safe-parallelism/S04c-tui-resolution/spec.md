---
title: 'S04c-tui-resolution — blocked TL;DR panel + open in AI action'
description: 'When a slice is blocked or failed, the sworn TUI surfaces a TL;DR of violations extracted from proof.md and a structured options menu including one-keypress launch in Claude Code or Codex.'
---

# Slice: `S04c-tui-resolution`

## User outcome

A developer sees a blocked slice in the `sworn` TUI, presses `Enter`, and is shown a
panel summarising the violations from the proof bundle with five resolution options —
including launching Claude Code or Codex pre-loaded with context — without needing to
open a terminal, navigate worktrees, or assemble context manually.

## Entry point

The board view or live view (S04a/S04b): user selects a slice in `failed_verification`
or `BLOCKED` state and presses `Enter`.

## In scope

- `internal/tui/blocked.go`: Bubble Tea component showing:
  - Slice ID, track, release, worktree path
  - Violation list extracted from `proof.md` — the "## Violations" or "## Not delivered"
    section (plain text parse; no model call)
  - Structured options menu:
    - `[1]` auto-fix + rerun: resets slice state to `in_progress` and shells out to
      `sworn run --slice <id> --release <name>` as a subprocess
    - `[2]` open in Claude Code: writes context file + opens worktree in CC
    - `[3]` open in Codex: same, opens in Codex
    - `[4]` view full proof bundle (scrollable text panel)
    - `[5]` defer slice: prompts for a one-line reason, writes state `deferred` to
      `status.json`, appends Rule 2 deferral to `intake.md`
- `internal/tui/open_ai.go`:
  - `WriteContextFile(worktreePath, spec, violations, diff string) (path string, err error)`:
    writes `<worktreePath>/.sworn-context.md` with assembled content
  - `LaunchClaudeCode(worktreePath string) error`: exec `code <worktreePath>` (VS Code
    with Claude extension) or configured `SWORN_CLAUDE_CODE_CMD`; returns error if
    command not found, does not crash the TUI
  - `LaunchCodex(worktreePath string) error`: exec `codex <worktreePath>` or configured
    `SWORN_CODEX_CMD`; same error handling
- Integration into `internal/tui/model.go`: board/live view gains `viewBlocked` state;
  pressing `Enter` on a blocked/failed slice transitions to it; `Esc` returns

## Out of scope

- Embedded AI chat interface — no model calls from blocked.go
- Image capture or rich markdown in the context file (plain text only)
- AI tool list beyond Claude Code + Codex (configurable post-R3 via env)
- Resolving the violation automatically without user confirmation

## Planned touchpoints

- `internal/tui/blocked.go` (new)
- `internal/tui/open_ai.go` (new)
- `internal/tui/model.go` (touch — add viewBlocked state, Enter key on blocked slice)
- `internal/tui/tui_test.go` (touch — add blocked panel tests)

## Acceptance checks

- [ ] Selecting a `failed_verification` slice in the board view and pressing `Enter`
  transitions to the blocked panel (verified by model state test)
- [ ] The blocked panel shows the violations list extracted from a fixture `proof.md`
  containing a known "## Violations" section with 2 entries
- [ ] Pressing `[2]` on the blocked panel writes `.sworn-context.md` to the worktree
  path (assert file exists and contains spec excerpt + violations)
- [ ] If `code` is not in PATH, `[2]` shows an inline message "Claude Code not found —
  context written to <path>" and does not crash the TUI
- [ ] Pressing `[5]` (defer) prompts for a reason; after input + confirm, the slice's
  `status.json` has `state: deferred` and `intake.md` contains a new deferral entry
  with the provided reason, a "Why:" line, and an "Acknowledged" timestamp
- [ ] Pressing `[4]` (view proof) opens a scrollable panel showing the raw proof.md
  content; `Esc` returns to the blocked options panel
- [ ] `go test ./internal/tui/...` covers all above paths (model state tests)

## Required tests

- **Unit**: `internal/tui/tui_test.go`
  — `TestBlockedPanelExtractsViolations`: fixture proof.md with 2 known violation
    strings; assert blocked model contains exactly those strings
  — `TestOpenAIWritesContextFile`: call WriteContextFile with known inputs; assert
    file written with expected content
  — `TestLaunchMissingTool`: LaunchClaudeCode when `code` not in PATH; assert error
    returned (no panic); TUI model shows graceful message
  — `TestDeferWritesRuleTwo`: select defer option, provide reason; assert status.json
    state == "deferred" and intake.md contains the three required Rule 2 components
    (why, tracking, acknowledged)
- **Reachability artefact**: smoke step — with a fixture slice in `failed_verification`
  state; run `sworn`; navigate to the slice; press Enter; observe the blocked panel with
  violations; press `[4]` to view the full proof. Document in proof.md.

## Risks

- `proof.md` format is not strictly machine-readable — violations are extracted by
  section heading heuristic. If the proof.md format varies (e.g. violations listed
  under "## Not delivered" vs "## Violations"), the parser must handle both. Check
  actual proof.md format from any verified R2 slice before implementing the parser.
- The defer action writes to `intake.md`. If `intake.md` does not have the expected
  section heading, the append may land in the wrong place. Mitigation: append to the
  end of the "Adjacent / out of scope" section, creating it if absent.

## Deferrals allowed?

Yes, with Rule 2 compliance:
- AI tool list beyond CC + Codex: configurable via `SWORN_AI_TOOLS` env post-R3.
  Why: two tools cover the immediate use cases. Tracking: TBD. Ack: now.
- Auto-fix [1] action may be stubbed to a log message if rerunning from within the
  TUI requires complex subprocess management. Why: subprocess management from Bubble Tea
  is non-trivial (it captures stdout). Tracking: TBD. Ack: now.
