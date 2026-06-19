# Proof Bundle: `S08-design-system-input`

## Scope

When a maintainer of a UI-bearing project declares a design system in project config (the design-token source + the component-library location), `sworn` reads it as the source of truth for design conformance (S09). `sworn` fails closed if a project marked UI-bearing declares no design system; a CLI project explicitly declares none and is exempt.

## Files changed

```
$ git diff --name-only 9b3b637..HEAD
bin/spec-quality.sh
cmd/sworn/init.go
cmd/sworn/init_design_system_test.go
cmd/sworn/journeys_regen_test.go
cmd/sworn/main.go
cmd/sworn/specquality.go
cmd/sworn/specquality_test.go
docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/journal.md
docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/proof.md
docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/spec.md
docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/status.json
docs/release/2026-06-16-fidelity-layer/S08-design-system-input/journal.md
docs/release/2026-06-16-fidelity-layer/S08-design-system-input/proof.md
docs/release/2026-06-16-fidelity-layer/S08-design-system-input/spec.md
docs/release/2026-06-16-fidelity-layer/S08-design-system-input/status.json
docs/release/2026-06-16-fidelity-layer/index.md
internal/adopt/baton/rules/08-requirements-fidelity.md
internal/adopt/baton/rules/09-design-fidelity.md
internal/config/config.go
internal/config/config_test.go
internal/config/init.go
internal/prompt/planner.md
internal/specquality/specquality.go
internal/specquality/specquality_test.go
```

(S08-specific files: `cmd/sworn/init.go`, `cmd/sworn/init_design_system_test.go`, `internal/adopt/baton/rules/09-design-fidelity.md`, `internal/config/config.go`, `internal/config/config_test.go`, `internal/config/init.go`. The remaining files are from earlier S03 work on the same track.)

## Test results

### Go — unit tests

```
$ go test ./internal/config/... -v -count=1
=== RUN   TestDefaultConfig
--- PASS: TestDefaultConfig (0.00s)
=== RUN   TestPath
--- PASS: TestPath (0.00s)
=== RUN   TestLoadNotExistReturnsDefault
--- PASS: TestLoadNotExistReturnsDefault (0.00s)
=== RUN   TestResolveVerifierModel
=== RUN   TestResolveVerifierModel/flag_wins
=== RUN   TestResolveVerifierModel/env_wins_over_config
=== RUN   TestResolveVerifierModel/config_fallback
--- PASS: TestResolveVerifierModel (0.00s)
    --- PASS: TestResolveVerifierModel/flag_wins (0.00s)
    --- PASS: TestResolveVerifierModel/env_wins_over_config (0.00s)
    --- PASS: TestResolveVerifierModel/config_fallback (0.00s)
=== RUN   TestResolveVerifierModelMissingKey
--- PASS: TestResolveVerifierModelMissingKey (0.00s)
=== RUN   TestScaffoldIdempotent
--- PASS: TestScaffoldIdempotent (0.00s)
=== RUN   TestScaffoldWithForce
--- PASS: TestScaffoldWithForce (0.00s)
=== RUN   TestValidate_uiBearingWithoutDesignSystem
=== RUN   TestValidate_uiBearingWithoutDesignSystem/ui_bearing_true_without_design_system_fails_closed
=== RUN   TestValidate_uiBearingWithoutDesignSystem/ui_bearing_true_with_design_system_succeeds
=== RUN   TestValidate_uiBearingWithoutDesignSystem/ui_bearing_false_without_design_system_succeeds_(exempt)
=== RUN   TestValidate_uiBearingWithoutDesignSystem/default_config_(not_ui-bearing)_succeeds
--- PASS: TestValidate_uiBearingWithoutDesignSystem (0.00s)
    --- PASS: TestValidate_uiBearingWithoutDesignSystem/ui_bearing_true_without_design_system_fails_closed (0.00s)
    --- PASS: TestValidate_uiBearingWithoutDesignSystem/ui_bearing_true_with_design_system_succeeds (0.00s)
    --- PASS: TestValidate_uiBearingWithoutDesignSystem/ui_bearing_false_without_design_system_succeeds_(exempt) (0.00s)
    --- PASS: TestValidate_uiBearingWithoutDesignSystem/default_config_(not_ui-bearing)_succeeds (0.00s)
=== RUN   TestValidate_uiBearingErrorText
--- PASS: TestValidate_uiBearingErrorText (0.00s)
=== RUN   TestDesignSystem_DistinguishesThreeConcepts
--- PASS: TestDesignSystem_DistinguishesThreeConcepts (0.00s)
=== RUN   TestDesignSystem_JSONRoundTrip
--- PASS: TestDesignSystem_JSONRoundTrip (0.00s)
=== RUN   TestDefaultConfig_NotUIBearing
--- PASS: TestDefaultConfig_NotUIBearing (0.00s)
=== RUN   TestDesignSystem_OmitEmptyOnFalse
--- PASS: TestDesignSystem_OmitEmptyOnFalse (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/config	0.006s
```

```
$ go vet ./...
(no output — clean)
```

### Go — integration tests (Gate 3 / Rule 1)

```
$ go test ./cmd/sworn/... -run TestCmdInit -v -count=1
=== RUN   TestCmdInit_NonInteractive
--- PASS: TestCmdInit_NonInteractive (0.00s)
=== RUN   TestCmdInit_UIBearingFlag
--- PASS: TestCmdInit_UIBearingFlag (0.01s)
=== RUN   TestCmdInit_UIBearingOutput
--- PASS: TestCmdInit_UIBearingOutput (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.024s
```

The `TestCmdInit_UIBearingFlag` test calls `cmdInit([]string{"--yes", "--ui-bearing"})` in a temp directory and verifies the config file is written with `ui_bearing: true`. This exercises the real `cmdInit` entry point (Rule 1 Reachability Gate) — the exact Gate 3 gap identified by the verifier.

## Reachability artefact

- **Type**: automated test (Rule 1 integration test via cmdInit entry point) + manual smoke step
- **Path**: `cmd/sworn/init_design_system_test.go` (the integration test) + `cmd/sworn/init.go` (CLI integration) + `internal/config/config.go` (the schema) + `internal/config/init.go` (init prompting)
- **Automated smoke step** (`TestCmdInit_UIBearingFlag`):
  1. Sets `SWORN_CONFIG_PATH` to a temp dir.
  2. Calls `cmdInit([]string{"--yes", "--ui-bearing"})`.
  3. Verifies exit 0.
  4. Verifies the config file contains `ui_bearing: true`.
  5. Verifies `Validate()` returns `ErrNoDesignSystem` (fail-closed, no design_system declared).
- **Manual smoke step**: Run `sworn init --ui-bearing --yes` — config records `ui_bearing: true` without `design_system`; subsequent `sworn verify` fails closed via `Validate()`.

## Delivered

- **AC1**: WHEN a project declares `ui_bearing: true` with no `design_system`, THE SYSTEM SHALL fail closed — evidence: `TestValidate_uiBearingWithoutDesignSystem/ui_bearing_true_without_design_system_fails_closed` (unit test); `TestCmdInit_UIBearingFlag` confirms `cmdInit --ui-bearing` pipeline records `ui_bearing: true` and `Validate()` fails closed; `Config.Validate()` returns `ErrNoDesignSystem`.
- **AC2**: WHEN a project declares `ui_bearing: false`, THE SYSTEM SHALL treat the design system as not applicable — evidence: `TestValidate_uiBearingWithoutDesignSystem/ui_bearing_false_without_design_system_succeeds_(exempt)` (unit test); `TestDesignSystem_OmitEmptyOnFalse` (JSON omits fields); `TestCmdInit_NonInteractive` confirms default init produces non-UI-bearing config.
- **AC3**: WHEN a UI-bearing project declares a `design_system`, THE SYSTEM SHALL parse it and expose it — evidence: `TestDesignSystem_JSONRoundTrip` (JSON round-trip preserves TokenSource and ComponentLibrary); `TestCmdInit_UIBearingFlag` confirms config file is written with `ui_bearing: true` for subsequent parsing.
- **AC4**: THE SYSTEM SHALL distinguish the three concepts — evidence: `TestDesignSystem_DistinguishesThreeConcepts`; the `DesignSystem` struct has `TokenSource` (design tokens) and `ComponentLibrary` (coded reusables) as documented fields.

## Not delivered

- None. All four acceptance checks are delivered. The previous verifier's Gate 3 gap (missing integration test via cmdInit entry point) has been addressed with `cmd/sworn/init_design_system_test.go`.

## Divergence from plan

- `cmd/sworn/init.go` was an unplanned file but was necessary for the init prompting integration. The planned touchpoint `internal/config/init.go` was created and contains the `PromptDesignSystem` function.
- `cmd/sworn/init_design_system_test.go` was added to address the verifier's Gate 3 finding.

## First-pass script output

```
$ release-verify.sh S08-design-system-input 2026-06-16-fidelity-layer
(first-pass: PASS — run from track worktree)
```