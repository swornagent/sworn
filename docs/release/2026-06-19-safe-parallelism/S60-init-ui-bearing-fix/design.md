# Design TL;DR — S60-init-ui-bearing-fix

## §1. User-visible change

A maintainer running `sworn init` in a CLI or library repo (no `--ui-bearing` flag) will never see prompts about "Design tokens source" or "Component library location." The written config records `ui_bearing: false` with no `design_system` key. Passing `--ui-bearing` preserves the existing behaviour: the user is prompted for (or, in `--yes` mode, the config records `ui_bearing: true` without design_system details, triggering fail-closed validation later).

## §2. Design decisions not in spec (max 5)

1. **Gate on `*uiBearer`, not on config existence**: The current code gates the design-system block on `cfgErr == nil && !cfgExisted` (new config) and `cfgErr == config.ErrConfigExists && *uiBearer` (existing config). The fix collapses both into a single `if *uiBearer` block after config creation/loading, simplifying the control flow.
2. **Implementer-model prompt stays in new-config-only path**: The implementer-model prompt (S09) is unrelated to UI-bearing and stays gated on new-config only — it's not collapsed into the unified `*uiBearer` block.
3. **Keep `PromptDesignSystem` call inside the gate**: Rather than modifying `PromptDesignSystem` to be a no-op when `!uiBearer`, we gate at the call site. This keeps the function's contract clean — it always prompts; the caller decides whether to invoke it.
4. **Line 190 `*uiBearer || true` becomes plain `true`**: Since we're already inside the `if *uiBearer` block, `cfg.UIBearing = true` suffices. The old `|| true` was a latent defect that forced `ui_bearing: true` whenever a design system was entered, even without the flag.
5. **No scan-phase changes needed**: The scan phase (lines 67-80) already correctly distinguishes UI-bearing from non-UI-bearing repos in its informational messages. No change required there.

## §3. Files I'll touch grouped by purpose

- **`cmd/sworn/init.go`** — The fix: gate the design-system apply block on `*uiBearer`, remove `|| true`, collapse the two branches into one. This is the only production code change.
- **`cmd/sworn/init_design_system_test.go`** — The existing tests (`TestCmdInit_NonInteractive`, `TestCmdInit_UIBearingFlag`, `TestCmdInit_UIBearing_ValidateFailClosed`) already assert the correct behaviour. They should pass after the fix. No new test additions needed (the spec ACs are already covered), but I'll verify they pass.

## §4. Things I'm NOT doing

- Not changing any colour/formatting/output styling — S61's domain.
- Not modifying `config.PromptDesignSystem`, `config.Validate`, or the config schema.
- Not adding auto-detection of UI-bearing repos.
- Not touching the scan-phase informational messages.
- Not refactoring `init.go` beyond the design-system gate — the file has other areas that could be cleaned up (e.g. repeated `config.Load()` calls) but those are out of scope.

## §5. Reachability plan

**Terminal transcript**: Run `sworn init --yes` in a temp non-UI-bearing repo, capture stdout showing no "Design tokens source" or "Component library location" strings, and verify the written `config.json` has no `design_system` key. Same for interactive mode (pipe "y\n" to stdin, verify no design-system prompts appear). Also run `go test ./cmd/sworn/...` showing existing tests pass.

## §6. Open questions for the Coach

(None — the fix is well-understood and the reference implementation on `wip/cli-styling-reference` confirms the approach.)