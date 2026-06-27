---
title: 'S09-per-role-model-config — per-role model config in config.json'
description: 'Config file gains implementer.model, escalation_models, and max_attempts alongside the existing verifier.model. sworn init prompts for all roles. sworn run reads config before falling back to flags and env vars.'
---

# Slice: `S09-per-role-model-config`

## User outcome

A developer can set `implementer.model`, `implementer.escalation_models`, and
`implementer.max_attempts` in `~/.config/sworn/config.json` (alongside the existing
`verifier.model`), and `sworn run` picks them up without requiring any flags or env vars.
`sworn init` prompts for all model fields so a fresh install is fully configured in one
command.

## Entry point

`sworn init` (prompts for both roles) and `sworn run` (reads config during model
resolution). Verifiable by running `sworn init`, inspecting the written `config.json`,
then running `sworn run --dry-run` (or equivalent) and confirming the resolved model IDs
log correctly.

## In scope

- Extend `ModelSetting` struct to add `EscalationModels []string` and `MaxAttempts int`
  fields (JSON: `escalation_models`, `max_attempts`)
- Add `Implementer ModelSetting` field to `Config` struct (JSON: `implementer`)
- `DefaultConfig()` sets `implementer.model = "openai/gpt-4o-mini"`,
  `implementer.escalation_models = ["openai/gpt-4o", "openai/o3"]`,
  `implementer.max_attempts = 3`
- `ResolveImplementerModel(flagModel string, cfg Config) (string, error)`:
  precedence flag → `$SWORN_IMPLEMENTER_MODEL` → `cfg.Implementer.Model` → first
  entry of `cfg.Implementer.EscalationModels` → error
- `ResolveEscalationModels(flagModels []string, cfg Config) []string`:
  precedence `--escalation-models` flag → `$SWORN_ESCALATION_MODELS` (comma-separated)
  → `cfg.Implementer.EscalationModels` → `DefaultEscalationModels`
- `ResolveMaxAttempts(flagN int, cfg Config) int`:
  precedence `--retry-cap` flag (>0) → `cfg.Implementer.MaxAttempts` (>0) → 3
- `cmd/sworn/run.go`: replace direct flag reads with resolver calls (serialised via T1 dep)
- `cmd/sworn/init.go`: prompt for implementer model (default shown), escalation list
  (comma-separated, default shown), and max attempts; write all fields to config.json

## Out of scope

- Provider router or multi-provider support (S10)
- .env file loading for API keys (S10)
- TUI settings screen (S17)
- Verifier escalation models — verifier stays on a single fixed model per-run (no cascade)

## Planned touchpoints

- `internal/config/config.go` (modify — new fields, new resolvers)
- `internal/config/config_test.go` (modify/new — test new fields and resolvers)
- `cmd/sworn/run.go` (modify — use resolver calls; serialised by T1 dep)
- `cmd/sworn/init.go` (modify — prompt for both roles)

## Acceptance checks

- [ ] `Config` JSON round-trips: a config.json with `implementer.model`,
  `implementer.escalation_models`, and `implementer.max_attempts` set loads without
  error and is accessible via the struct fields
- [ ] `ResolveImplementerModel`: flag takes precedence over env var; env var takes
  precedence over config file; config file takes precedence over escalation list default;
  returns error when all are empty
- [ ] `ResolveEscalationModels`: flag slice takes precedence; `$SWORN_ESCALATION_MODELS`
  is parsed comma-separated; config file list used when flags/env absent; falls back
  to `DefaultEscalationModels` when config is empty
- [ ] `ResolveMaxAttempts`: flag >0 takes precedence; config >0 used when no flag;
  falls back to 3 when both are zero/absent
- [ ] `sworn init` prompts for implementer model (with default shown), escalation list
  (comma-separated, default shown), and max attempts; writes all fields to config.json
- [ ] `go test ./internal/config/...` passes with zero failures
- [ ] `go build ./...` succeeds with no new external deps

## Required tests

- **Unit** `internal/config/config_test.go`:
  - `TestResolveImplementerModel_FlagWins`: set flag + env + config; assert flag returned
  - `TestResolveImplementerModel_EnvFallback`: no flag; set env; assert env returned
  - `TestResolveImplementerModel_ConfigFallback`: no flag, no env; set config; assert config
  - `TestResolveImplementerModel_EscalationFallback`: only escalation_models set; assert first entry
  - `TestResolveImplementerModel_Error`: all empty; assert non-nil error
  - `TestResolveEscalationModels_FlagWins`, `_EnvParsed`, `_ConfigUsed`, `_DefaultFallback`
  - `TestResolveMaxAttempts_FlagWins`, `_ConfigUsed`, `_DefaultFallback`
  - `TestConfigRoundTrip`: marshal + unmarshal with all new fields; assert field equality
- **Reachability artefact**: `sworn init` smoke step — run init with piped input providing
  values for both roles; cat the written config.json; assert all four keys present.
  Document in proof.md as explicit smoke step.

## Risks

- `sworn init` is interactive (stdin prompts). Test it with piped stdin rather than a PTY
  to avoid flakiness. If `init.go` uses a PTY-detection guard (as most CLI tools do),
  the test may need to use `SWORN_CONFIG_PATH` to write to a temp path.
- The `Implementer` field name must match `internal/run/run.go`'s existing
  `Options.ImplementerModel` field — reconcile naming in the implementer session.

## Deferrals allowed?

No. S10 (provider foundation) reads the resolved implementer/verifier model IDs. If
S09 is skipped, the config layer is incomplete and S10 tests cannot verify full
resolution precedence.
