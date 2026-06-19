# Proof Bundle: `S08-design-system-input`

## Scope

When a maintainer of a UI-bearing project declares a design system in project config (the design-token source + the component-library location), `sworn` reads it as the source of truth for design conformance (S09). `sworn` fails closed if a project marked UI-bearing declares no design system; a CLI project explicitly declares none and is exempt.

## Files changed

```
$ git diff --name-only 9b3b637..HEAD -- internal/config/ internal/adopt/baton/rules/09-design-fidelity.md cmd/sworn/init.go
cmd/sworn/init.go
internal/adopt/baton/rules/09-design-fidelity.md
internal/config/config.go
internal/config/config_test.go
internal/config/init.go
```

Also updated: `docs/release/2026-06-16-fidelity-layer/S08-design-system-input/status.json`, `journal.md`, `proof.md`, and `spec.md`.

## Test results

### Go

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

```
$ go test ./...
ok  	github.com/swornagent/sworn/cmd/sworn	0.063s
ok  	github.com/swornagent/sworn/internal/adopt	(cached)
ok  	github.com/swornagent/sworn/internal/config	0.005s
[all packages pass — full output truncated for brevity]
```

## Reachability artefact

- **Type**: manual smoke step (CLI tool — no browser interaction)
- **Path**: internal/config/config.go (the schema) + internal/config/init.go (init prompting) + cmd/sworn/init.go (CLI integration)
- **User gesture**:
  1. Run `sworn init --yes` on a new project — config is created with `DefaultConfig()` (UIBearing: false).
  2. Run `sworn init --ui-bearing --yes` — prompts for design system; if declined, config records `ui_bearing: true` but no `design_system`.
  3. Run `sworn verify` on the resulting config — `Validate()` returns `ErrNoDesignSystem`; the tool fails closed.
  4. Run `sworn init --ui-bearing --force` and provide valid design system paths — config records `design_system` with `token_source` and `component_library`.
  5. Run `sworn verify` — `Validate()` passes; the design system is exposed for the S09 conformance audit.

## Delivered

- **AC1**: WHEN a project declares `ui_bearing: true` with no `design_system`, THE SYSTEM SHALL fail closed — evidence: `TestValidate_uiBearingWithoutDesignSystem/ui_bearing_true_without_design_system_fails_closed` (test passes); `Config.Validate()` returns `ErrNoDesignSystem`.
- **AC2**: WHEN a project declares `ui_bearing: false`, THE SYSTEM SHALL treat the design system as not applicable — evidence: `TestValidate_uiBearingWithoutDesignSystem/ui_bearing_false_without_design_system_succeeds_(exempt)` (test passes); `TestDesignSystem_OmitEmptyOnFalse` (JSON omits fields).
- **AC3**: WHEN a UI-bearing project declares a `design_system`, THE SYSTEM SHALL parse it and expose it — evidence: `TestDesignSystem_JSONRoundTrip` (JSON round-trip preserves TokenSource and ComponentLibrary).
- **AC4**: THE SYSTEM SHALL distinguish the three concepts — evidence: `TestDesignSystem_DistinguishesThreeConcepts`; the `DesignSystem` struct has `TokenSource` (design tokens) and `ComponentLibrary` (coded reusables) as documented fields.

## Not delivered

- None. All four acceptance checks are delivered.

## Divergence from plan

- `cmd/sworn/init.go` was an unplanned file but was necessary for the init prompting integration. The planned touchpoint `internal/config/init.go` was created and contains the `PromptDesignSystem` function.

## First-pass script output

```
$ release-verify.sh S08-design-system-input 2026-06-16-fidelity-layer
(first-pass: PASS — run from track worktree)
```