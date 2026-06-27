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
- Ran gofmt on all touched files.- skeptic_panel: skipped -- runtime does not support subagent dispatch (single Claude Code session, no parallel Agent tool).

## Verifier verdicts received

### Verdict: FAIL (2026-06-28)

Slice: `S04c-tui-resolution`

Violations:
1. Gate 2 — `internal/tui/board.go`, `internal/tui/styles.go`, and `internal/state/state.go` changed but not in spec.md "Planned touchpoints" and not explained in proof.md "Divergence from plan" (which states "None")
   Evidence: `git diff --name-only start_commit..HEAD` shows these files; proof.md "Divergence from plan" section

Required to address:
1. Add `internal/tui/board.go`, `internal/tui/styles.go`, and `internal/state/state.go` to spec.md "Planned touchpoints" OR document them in proof.md "Divergence from plan" with rationale for each

All other gates (1, 3–6) pass. Tests: 21/21 PASS. go vet: clean.

## Session: 2026-06-28 (re-entry to address Gate 2 violation)

### Violation addressed
The verifier FAIL was purely a documentation gap: three files changed but not
listed in spec.md "Planned touchpoints" and proof.md "Divergence from plan"
said "None." No code changes were needed — the existing code is correct and
all tests pass (22/22). The fix was to document each divergence with rationale
in proof.md "Divergence from plan":

1. **`internal/state/state.go`** — `Deferred` state constant + transitions needed
   for acceptance check 5 (defer action writes `state: deferred`). Also gofmt fixes.
2. **`internal/tui/board.go`** — `VerificationResult` field, `Cursor`/`orderedSlices`
   for navigation, `SliceStateColor` call. Required by design review Pins 1 & 2.
3. **`internal/tui/styles.go`** — `BoardItemSelected` style, `SliceStateColor`
   function, `deferred` case. Required by Pins 1 & 2 + acceptance check 5.

Also updated proof.md "Files changed" to list all 27 files in the diff (including
forward-merge artefacts from S42–S47 and index.md, which are not this slice's work).

### Decisions
- Documented divergences in proof.md rather than amending spec.md "Planned
  touchpoints" — the spec is the planner's contract and should not be retroactively
  edited by the implementer. The proof bundle's "Divergence from plan" section is
  the correct place for this documentation.
- Carried forward both open deferrals verbatim with their Coach acknowledgements
  intact (Rule 2 compliance).

### Test results
- `go test ./internal/tui/... -v`: 22/22 PASS
- `go vet ./internal/tui/...`: clean
- `go test ./...`: all packages PASS

- skeptic_panel: skipped — runtime does not support subagent dispatch (single Claude Code session, no parallel Agent tool).
## Verifier verdicts received

### Verdict 3 — 2026-06-28 — PASS

**Session:** fresh context, artefact-only.
**Verdict:** PASS

All six gates passed:
- Gate 1: Entry point `cmd/sworn → tui.Run() → Model.handleBoardKey Enter on failed/blocked slice → viewBlocked` fully wired. Both `failed_verification` and `implemented+verification.result=="blocked"` cases tested and working.
- Gate 2: All 4 planned touchpoints in diff. 3 divergences (state.go, board.go, styles.go) documented with rationales. Forward-merge artefacts (S42-S47, index.md) correctly identified as non-S04c scope.
- Gate 3: All 7 tests pass (re-run with `-count=1`). Integration test `TestBoardEnterTransitionsToBlocked` exercises full Model.Update → viewBlocked path.
- Gate 4: Smoke step describes concrete user gesture with specific key presses.
- Gate 5: No TODO/FIXME/HACK markers. All `deferred` hits are protocol-defined state name. Two deferrals documented with Rule 2 compliance.
- Gate 6: All 10 Delivered items have verifiable evidence references to real working code.

Verified against commit: `041382b384d1fe698a7fe95469407ad9d1e126d7`
Verifier session: fresh, artefact-only.
