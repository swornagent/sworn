# Design TL;DR: S04c-tui-resolution

## §1. User-visible change
When a user selects a blocked or failed slice in the TUI and presses Enter, they will see a new panel displaying the violations extracted from the slice's `proof.md`. This panel provides a menu of options to resolve the issue: auto-fix and rerun, open in Claude Code or Codex with pre-loaded context, view the full proof bundle, or defer the slice.

## §2. Design decisions not in spec (max 5)
1. **Violation extraction heuristic**: We will look for `## Violations` or `## Not delivered` in `proof.md` and extract the text until the next `## ` heading or EOF.
2. **Context file format**: `.sworn-context.md` will contain the slice spec and the extracted violations, formatted clearly for the AI tool to read.
3. **Subprocess execution**: For `[1]` (auto-fix) and `[2]/[3]` (launch AI), we will use `tea.ExecProcess` to suspend the TUI, run the command, and resume the TUI afterward, ensuring stdout/stderr are handled correctly.
4. **Deferral input**: We will use a simple text input component (e.g., `bubbles/textinput`) for the deferral reason prompt when `[5]` is pressed.
5. **Intake.md append**: We will append the deferral to `docs/release/<release>/intake.md` under `## Adjacent / out of scope`, creating the section if it doesn't exist.

## §3. Files I'll touch grouped by purpose
- `internal/tui/blocked.go`: The new Bubble Tea component for the blocked panel, handling the display of violations and the options menu.
- `internal/tui/open_ai.go`: Functions to write the context file and launch Claude Code or Codex.
- `internal/tui/model.go`: Integrate the blocked panel into the main TUI model, handling the transition when Enter is pressed on a blocked/failed slice.
- `internal/tui/tui_test.go`: Unit tests for the new functionality.

## §4. Things I'm NOT doing
- I am not building an embedded AI chat interface.
- I am not supporting AI tools beyond Claude Code and Codex (configurable via env post-R3).
- I am not resolving the violation automatically without user confirmation.

## §5. Reachability plan
- Smoke step: Run `sworn top`, navigate to a fixture slice in `failed_verification` state, press Enter, observe the blocked panel, press `[4]` to view the full proof, and press `[2]` to verify the context file is written. Document in `proof.md`.

## §6. Open questions for the Coach
- For the auto-fix option `[1]`, the spec mentions it "may be stubbed to a log message if rerunning from within the TUI requires complex subprocess management". Given `tea.ExecProcess` is available, should we attempt the actual subprocess execution or stick to the stub for now?