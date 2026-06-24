---
title: 'S17-tui-provider-config — TUI settings screen for provider, model, and escalation config'
description: 'Adds a settings panel to the sworn TUI reachable via the ''s'' key. The panel lets the developer configure provider API keys (written to ~/.sworn/.env), model per role (verifier and implementer), escalation list, and max attempts. Changes are written to ~/.config/sworn/config.json immediately on save.'
---

# Slice: `S17-tui-provider-config`

## User outcome

A developer presses `s` from the sworn TUI board view; a settings panel opens showing
the current verifier model, implementer model, escalation list, max attempts, and a
masked display of configured API keys. The developer navigates fields with arrow keys,
edits values inline, and presses Enter to save — changes are persisted to
`~/.config/sworn/config.json` and `~/.sworn/.env` immediately, no manual file editing
required.

## Entry point

`sworn` (no args) → TUI board view → press `s` → settings panel. Verifiable by running
`sworn`, pressing `s`, confirming the panel renders with current config values.

## In scope

- `internal/tui/settings.go` (new) — Bubble Tea `Model` for the settings panel:
  - Fields: Verifier Model, Implementer Model, Escalation Models (comma-separated),
    Max Attempts, and one API key field per provider:
    `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, `GROQ_API_KEY`,
    `MISTRAL_API_KEY`, `DEEPSEEK_API_KEY`, `OPENROUTER_API_KEY`,
    `AZURE_OPENAI_API_KEY`, `AZURE_OPENAI_ENDPOINT`, `OCI_COMPARTMENT_ID`,
    `OLLAMA_HOST` (optional, no masking)
  - API key fields display as masked (`****`) when a value exists; navigating to the
    field and typing replaces the value
  - Navigation: Tab/Shift-Tab or arrow keys between fields; Enter on a field enters edit
    mode; Escape or Enter again exits edit mode; `Ctrl+S` saves and returns to board;
    `Escape` from non-edit mode discards changes and returns to board
  - Validation: model fields must be non-empty on save (warn, do not block); escalation
    list is split on comma, trimmed; max attempts must be a positive integer
- `internal/tui/settings.go` save logic:
  - Model fields and max attempts → written to `config.json` via `config.Save(cfg)`
    (new function in `internal/config/config.go` — `Save(cfg Config) error` writes
    JSON to `Path()`, creating parent dirs if needed; serialised by T3 dep)
  - API keys → written to `~/.sworn/.env` (create if missing, update existing keys
    in-place, preserve other lines/comments)
- `internal/tui/tui.go` (or the root TUI model in T2's files) — add `s` key binding
  to transition to settings panel; add return transition (Escape/Ctrl+S) back to board
  (T2 dep — modifying T2's file after T2 merges)
- `cmd/sworn/top.go` — if necessary, wire the new TUI state into the top-level Bubble
  Tea program (T2 dep)
- `internal/config/config.go` — add `Save(cfg Config) error` function (T3 dep —
  modifying T3's file after T3 merges, via T6's dep on T5 which depends on T3)

## Out of scope

- Mouse support in the settings panel (deferred post-R3 — matches existing TUI scope
  ceiling)
- Provider connection test button ("test this API key") — post-R3
- Per-slice model overrides in the TUI — config.json global only in this slice
- `.env` file in CWD management (only `~/.sworn/.env` is written; project-local `.env`
  is manual)
- Colour/theme customisation
- Multi-account switching

## Planned touchpoints

- `internal/tui/settings.go` (new)
- `internal/tui/settings_test.go` (new)
- `internal/tui/tui.go` (modify — add `s` keybinding; T2 dep)
- `cmd/sworn/top.go` (modify if needed — wire settings state; T2 dep)
- `internal/config/config.go` (modify — add Save() function; T3 dep)
- `internal/config/config_test.go` (modify — test Save())

## Acceptance checks

- [ ] `sworn` launches, `s` key press opens the settings panel (no crash, no blank screen)
- [ ] Settings panel displays the current `config.json` values for verifier model,
  implementer model, escalation models, and max attempts
- [ ] A configured API key (e.g. `OPENAI_API_KEY` set in `~/.sworn/.env`) shows as
  `****` in the panel; an unconfigured key shows as empty
- [ ] Editing the verifier model field, pressing Ctrl+S, then exiting and re-running
  `sworn` shows the new model value in the panel (persistence verified via second launch)
- [ ] Editing `ANTHROPIC_API_KEY` in the panel and saving writes the key to
  `~/.sworn/.env`; the file is created if it does not exist
- [ ] Pressing Escape from non-edit mode discards unsaved changes and returns to the
  board view
- [ ] Invalid max attempts (non-integer) shows an inline warning; save is not blocked
  but the field value is not written (existing value preserved)
- [ ] `go build ./...` passes; `go test ./internal/tui/... -run Settings` passes

## Required tests

- **Unit** `internal/tui/settings_test.go` (using Bubble Tea test helpers):
  - `TestSettingsPanel_OpensWithCurrentConfig`: construct settings model with known cfg;
    assert rendered view contains the verifier model string
  - `TestSettingsPanel_MasksAPIKey`: settings model with non-empty OPENAI_API_KEY in env;
    assert rendered view shows `****`
  - `TestSettingsPanel_SaveWritesConfig`: send Ctrl+S key msg; assert `config.Save` was
    called with the edited model field (use a fake saver func injected via settings
    constructor)
  - `TestSettingsPanel_EscapeDiscards`: edit a field; press Escape; assert board view
    returned and no save called
  - `TestSettingsPanel_InvalidMaxAttempts`: type "abc" in max attempts field; press
    Ctrl+S; assert warning shown, saver not called with bad value
- **Unit** `internal/config/config_test.go`:
  - `TestSave_WritesFile`: call Save() with a cfg; read back the file; assert round-trip
  - `TestSave_CreatesParentDirs`: point `SWORN_CONFIG_PATH` at a non-existent dir; call
    Save(); assert file created
- **Reachability artefact**: smoke step — run `sworn` in a terminal (or via PTY in test);
  press `s`; confirm settings panel appears (no crash). Document exact commands in
  proof.md. Text capture at `docs/release/2026-06-19-safe-parallelism/captures/S17-settings-panel.txt`
  is the canonical artefact.

## Risks

- `internal/tui/tui.go` (T2's file): T6 modifies it after T2 merges. If T2's S04a/S04b
  add significant complexity to the root TUI model, T6's addition of the `s` binding
  must not break any of T2's existing key bindings. The implementer must read T2's
  merged code before touching tui.go.
- `.env` file in-place update: overwriting a key in-place while preserving surrounding
  content requires careful line-by-line rewrite. A naive approach (`os.WriteFile` with
  all keys) would erase unrecognised keys. The implementer must preserve lines whose key
  is not being updated.
- PTY-based UI testing is flaky. The `settings_test.go` tests must use Bubble Tea's
  programmatic `tea.NewProgram` test mode, not a real PTY. Manual captures suffice
  or driven by a separate PTY test helper.

## Deferrals allowed?

"Test this API key" button: deferred post-R3. Why: requires a live model call from the
TUI, adds complexity and potential hangs. Tracking: post-R3 UX issue. Acknowledged:
2026-06-20 planning session. Document as a Rule 2 deferral in proof.md.
