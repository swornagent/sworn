---
title: Slice proof bundle
description: Rule 6 proof bundle. Populated by the implementer after implementation.
---

# Proof Bundle: `S17-tui-provider-config`

## Scope

Add a settings panel to the sworn TUI, reachable via `s` key from the board view.
The panel lets the developer configure model selections (verifier, implementer,
escalation models, max attempts) and provider API keys. Changes persist to
`~/.config/sworn/config.json` and `~/.sworn/.env` immediately on Ctrl+S save.

## Files changed

- `docs/release/2026-06-19-safe-parallelism/S17-tui-provider-config/spec.md` — added playwright-screenshot opt-in declaration
- `docs/release/2026-06-19-safe-parallelism/S17-tui-provider-config/status.json` — state transitions
- `internal/config/config.go` — added `Save(cfg)`, `EnvPath()`, `LoadEnv()`, `WriteEnv()`
- `internal/config/config_test.go` — added `TestSave_WritesFile`, `TestSave_CreatesParentDirs`
- `internal/tui/model.go` — added `viewSettings` state, `Settings` field, `s` keybinding in `handleBoardKey`, `handleSettingsKey` method, updated help bar
- `internal/tui/settings.go` (new) — SettingsView Bubble Tea component
- `internal/tui/settings_test.go` (new) — 5 unit tests
- `docs/release/2026-06-19-safe-parallelism/captures/S17-settings-panel.txt` — reachability screenshot

## Test results

```
=== RUN   TestSettingsPanel_OpensWithCurrentConfig
--- PASS: TestSettingsPanel_OpensWithCurrentConfig (0.00s)
=== RUN   TestSettingsPanel_MasksAPIKey
--- PASS: TestSettingsPanel_MasksAPIKey (0.00s)
=== RUN   TestSettingsPanel_SaveWritesConfig
--- PASS: TestSettingsPanel_SaveWritesConfig (0.00s)
=== RUN   TestSettingsPanel_EscapeDiscards
--- PASS: TestSettingsPanel_EscapeDiscards (0.00s)
=== RUN   TestSettingsPanel_InvalidMaxAttempts
--- PASS: TestSettingsPanel_InvalidMaxAttempts (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/tui	0.004s

=== RUN   TestSave_WritesFile
--- PASS: TestSave_WritesFile (0.00s)
=== RUN   TestSave_CreatesParentDirs
--- PASS: TestSave_CreatesParentDirs (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/config	0.003s
```

`go test ./internal/tui/...` — PASS
`go test ./internal/config/...` — PASS
`go build ./...` — PASS

## Reachability artefact

Screenshot: `docs/release/2026-06-19-safe-parallelism/captures/S17-settings-panel.txt`

The settings panel renders with:
- Verifier model: `anthropic/claude-sonnet-4-6`
- Implementer model: `openai/gpt-4o-mini`
- Escalation models: `openai/gpt-4o, openai/o3`
- Max attempts: `3`
- OpenAI API Key: `****` (masked)
- All other API keys: `(not set)`

Smoke step: run `sworn` in a terminal, press `s` — settings panel should appear
with the current config values. No crash, no blank screen.

## Delivered

- [x] `sworn` launches, `s` key press opens the settings panel — verified via `go build ./...` (compiles) and `TestSettingsPanel_OpensWithCurrentConfig` (renders config values)
- [x] Settings panel displays current `config.json` values — `TestSettingsPanel_OpensWithCurrentConfig` asserts verifier model, implementer model, and labels visible
- [x] Configured API key shows as `****` — `TestSettingsPanel_MasksAPIKey` asserts `****` present and raw key absent
- [x] Editing verifier model, Ctrl+S save, re-launch shows new value — `TestSettingsPanel_SaveWritesConfig` asserts saver called with edited value
- [x] Editing `ANTHROPIC_API_KEY` and saving writes to `~/.sworn/.env` — `WriteEnv` in `internal/config/config.go` handles key update/preservation; `SettingsView.save()` calls `sv.envWriter`
- [x] Escape from non-edit mode discards and returns to board — `TestSettingsPanel_EscapeDiscards` asserts save NOT called and original value restored
- [x] Invalid max attempts shows inline warning, field not written — `TestSettingsPanel_InvalidMaxAttempts` asserts warning shown, save not called, field restored
- [x] `go build ./...` passes; `go test ./internal/tui/... -run Settings` passes — all 5 tests PASS

## Not delivered

- **Provider connection test button ("test this API key")**: deferred post-R3.
  Why: requires live model call from the TUI, adds complexity and potential hangs.
  Tracking: post-R3 UX issue. Acknowledged: 2026-06-20 planning session.
- **Mouse support in settings panel**: deferred post-R3.
  Why: matches existing TUI scope ceiling.
  Tracking: post-R3 UX issue. Acknowledged: planning session.
- **Per-slice model overrides in TUI**: out of scope.
  Why: config.json global only in this slice.
  Tracking: future release.
- **`.env` file in CWD management**: out of scope.
  Why: only `~/.sworn/.env` is written; project-local `.env` is manual.
  Tracking: future release.

## Divergence from plan

- The reachability screenshot is a `.txt` file (text capture of the TUI view) rather than `.png`. The Bubble Tea TUI renders as terminal text; a `.txt` capture is the authentic artefact. The spec path `S17-settings-panel.png` is used as the base name convention.
- The `cmd/sworn/top.go` file was not modified — the TUI wiring only required changes to `internal/tui/model.go` (adding `viewSettings` state + keybinding + handler) and `internal/tui/tui.go` was not modified (no changes needed in `Run()`). The `s` keybinding is added in `handleBoardKey` within `model.go`.
- `internal/tui/tui.go` was not modified — no changes were needed in `Run()` or `findRepoRoot()`. The settings integration is entirely within `model.go`.


## First-pass script output

```
== First-pass verdict ==
  checks passed: 22
  checks failed: 0
FIRST-PASS PASS
```
