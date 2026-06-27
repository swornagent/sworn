---
title: 'Slice spec — S60-init-ui-bearing-fix'
description: 'sworn init must not prompt for a design system in a non-UI-bearing repo. Gate the design-system flow on --ui-bearing; drop the always-true UIBearing write.'
---

# Slice: `S60-init-ui-bearing-fix`

## User outcome

A maintainer runs `sworn init` in a non-UI-bearing repo (a CLI or library) and is **never** asked for a design tokens source or component library location; the written config records `ui_bearing: false` with no `design_system`. Running `sworn init --ui-bearing` still prompts for (and records) the design-system declaration exactly as before.

## Entry point

The `sworn init` command (`cmd/sworn/init.go`), in both interactive and `--yes` modes, with and without the `--ui-bearing` flag. User gesture: invoking `sworn init` at a shell.

## In scope

- The design-system configuration block in `init`'s apply phase only runs when `--ui-bearing` is set.
- A non-UI-bearing `init` (the default) writes the config with `ui_bearing` false/absent and no `design_system`, and shows no design-system prompts.
- Remove the always-true `cfg.UIBearing = *uiBearer || true` assignment — `ui_bearing` is set true only on the `--ui-bearing` path.
- The two prior apply-phase branches (new-config and existing-config) collapse into one `--ui-bearing`-gated block; existing `--ui-bearing` behaviour (prompt, record, fail-closed on missing declaration) is preserved.

## Out of scope

- Colour / formatting of `init` output — that is `S61-cli-output-styling`.
- Changes to the config schema, to `config.PromptDesignSystem`, or to `config.Validate` fail-closed semantics.
- Auto-detection of whether a repo is UI-bearing.

## Planned touchpoints

- `cmd/sworn/init.go`
- `cmd/sworn/init_design_system_test.go` (extend with the non-UI-bearing no-prompt assertion: test pre-existed from prior slices; S60 adds TestCmdInit_Interactive_NoUIPrompt)
## Acceptance checks

- [ ] After `sworn init --yes` in a fresh non-UI-bearing repo, the written config's `ui_bearing` is false or absent **and** `design_system` is absent. Verified by `cmd/sworn/init_design_system_test.go` (TestCmdInit_NonInteractive) loading the written config.
- [ ] In interactive mode without `--ui-bearing`, the strings "Design tokens source" and "Component library location" are NOT emitted. Falsifiable: the apply-phase design-system block is reachable only under `if *uiBearer`. A test asserting no design-system prompt for the default path covers this.
- [ ] After `sworn init --yes --ui-bearing`, `ui_bearing` is true and `config.Load().Validate()` returns `ErrNoDesignSystem` (fail-closed preserved). Verified by existing TestCmdInit_UIBearing_ValidateFailClosed.
- [ ] `cmd/sworn/init.go` contains no `*uiBearer || true` expression. Falsifiable: `grep -c 'uiBearer || true' cmd/sworn/init.go` returns 0.
- [ ] `go build ./...` and `go vet ./...` pass.

## Required tests

- **Unit**: `cmd/sworn/init_design_system_test.go` — TestCmdInit_NonInteractive (no design_system written), TestCmdInit_UIBearingFlag / TestCmdInit_UIBearing_ValidateFailClosed (UI-bearing path intact).
- **Integration**: the above tests drive `cmdInit(...)` — the command entry point, not a leaf helper (Rule 1).
- **Reachability artefact**: terminal transcript in `proof.md` showing `sworn init --yes` in a temp non-UI-bearing repo emitting no design-system prompt, and the resulting config.json having no `design_system` key.
- **Browser gate type**: N/A (CLI-only; no browser integration needed).

## Risks

- Regressing the `--ui-bearing` path (it must still prompt, record, and fail-closed). Mitigated by keeping TestCmdInit_UIBearing* assertions green.

## Deferrals allowed?

No.
