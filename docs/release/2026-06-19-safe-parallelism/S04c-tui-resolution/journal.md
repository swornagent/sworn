# Journal — S04c-tui-resolution

## Session: 2026-06-21 (initial implementation)

### Design review outcome
Coach approved the design with 5 pins (approved-ack.md). All pins addressed:
- **Pin 1 (BLOCKED-state detection):** Fixed `handleBoardKey` to check BOTH `state == "failed_verification"` AND `state == "implemented" && verification.result == "blocked"`. Added `VerificationResult` field to `SliceBoardInfo`, populated from `status.json`. Added `SliceStateColor` helper for correct colour rendering.
- **Pin 2 (Board cursor):** Already implemented — `Cursor` field on `BoardView`, up/down navigation in `handleBoardKey`, visual selection indicator in `board.go` `View()`.
- **Pin 3 (bubbles/textinput dep):** ADR-0005 written, `charmbracelet/bubbles` added to `go.mod`, `go.mod`/`go.sum` in `planned_files`.
- **Pin 4 (proof.md format audit):** `grep -r "^## Violations" docs/release/` returned zero results. `ExtractViolations` handles both `## Violations` and `## Not delivered` headings for forward-compatibility.
- **Pin 5 (auto-fix [1] stub):** Implemented as spec-permitted stub — shows inline message with the `sworn run` command, no `tea.ExecProcess`.

### Decisions
1. **Violation extraction heuristic:** Parse `## Violations` or `## Not delivered` sections, extract bullet points (`- ` or `* `). Both headings checked for forward-compat even though `## Violations` never appears in practice.
2. **Context file format:** `.sworn-context.md` with spec, violations, and git diff sections.
3. **Subprocess execution:** `tea.ExecProcess` for `[2]`/`[3]` (AI tool launch); stub for `[1]` (auto-fix).
4. **Deferral input:** `bubbles/textinput` per Coach Pin 3 decision.
5. **Intake.md append:** Append under `## Adjacent / out of scope (Rule 2 deferrals)`, creating section if absent.
6. **State changes:** Added `Deferred` state to `internal/state/state.go` with appropriate transitions.

### Deferrals (Rule 2)
1. **AI tool list beyond CC + Codex:** Configurable via `SWORN_AI_TOOLS` env post-R3. **Why:** Two tools cover the immediate use cases. **Tracking:** TBD. **Acknowledged:** Coach, 2026-06-21 (spec "Deferrals allowed?").
2. **Auto-fix [1] stubbed:** Shows inline message instead of running `sworn run` subprocess. **Why:** Subprocess management from Bubble Tea is non-trivial (captures stdout). **Tracking:** TBD. **Acknowledged:** Coach, 2026-06-21 (spec "Deferrals allowed?", approved-ack.md Pin 5).

### Session: re-entry (this session)
- Fixed Pin 1 (BLOCKED-state detection) — `handleBoardKey` was checking `si.State == "blocked"` which is never a state value. Now checks both conditions.
- Added `VerificationResult` to `SliceBoardInfo` and `SliceStateColor` helper.
- Added `TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict` and `TestBlockedPanelViewProof` tests.
- Ran gofmt on all touched files.