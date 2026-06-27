# Design TL;DR: `S09-per-role-model-config`

## §1. User-visible change

`sworn init` now prompts for the implementer model, escalation path, and retry
limit alongside the existing scaffold, so a single `sworn init` fully configures
both roles. `sworn run` reads those values from `~/.config/sworn/config.json`
when no flags or env vars are set, removing the need to pass `--implementer-model`
and `--escalation-models` on every invocation. The config file gets a new
top-level `implementer` key with `model`, `escalation_models`, and
`max_attempts`; the verifier section is unchanged.

## §2. Design decisions not in spec (max 5)

1. **`ModelSetting` holds escalation and retry for both roles, but verifier ignores them.**
   Adding `EscalationModels` and `MaxAttempts` to the shared `ModelSetting` struct
   means the verifier JSON block will also contain zero-valued `escalation_models: []`
   and `max_attempts: 0` after round-trip. This is harmless — `ResolveVerifierModel`
   doesn't read those fields, and existing config files without them unmarshal
   cleanly (Go zero-values the missing fields). Rationale: avoids two near-identical
   structs for a two-field difference.

2. **Config defaults differ from `run.DefaultEscalationModels`.**
   The spec sets config defaults to `["openai/gpt-4o", "openai/o3"]` (2 entries),
   while `run.DefaultEscalationModels` has 4 entries (`gpt-4o-mini`, `gpt-4o`,
   `o3-mini`, `o3`). The config default is what lands in the user's file; the
   `run` default is the programmatic fallback when config + env + flags are all
   absent. They serve different audiences — the config file is a suggested starting
   point, the programmatic default is a safety net. No reconciliation needed.

3. **`ResolveImplementerModel` escalation fallback uses `cfg.Implementer.EscalationModels[0]`.**
   When model is unset but escalation models are configured, the first entry is used
   as the initial implementer model. This matches the existing `run.Options`
   behaviour where `ImplementerModel` defaults to the first escalation model.

4. **`cmd/sworn/run.go`'s `resolveVerifierModel` gets replaced by the config package's `ResolveVerifierModel`.**
   The current `resolveVerifierModel` in run.go does flag > env only and drops
   through to empty string before falling back in `cmdRun`. The config package
   already has `ResolveVerifierModel(flag, cfg)` that does flag > env > config.
   Swapping to it is a net simplification and completes the config-is-source-of-truth
   pattern.

5. **`sworn init` prompts for implementer settings only on fresh scaffold.**
   When the config file already exists, `init` shows informational messages but
   doesn't re-prompt — the user edits the file or re-runs with `--force`. This
   matches the existing pattern for the API key and design system prompts.

## §3. Files I'll touch grouped by purpose

- **`internal/config/config.go`** — extend `ModelSetting`, add `Implementer` field to `Config`,
  add `ResolveImplementerModel`, `ResolveEscalationModels`, `ResolveMaxAttempts`,
  update `DefaultConfig()`. _Why: all config shape and resolution lives here._
- **`internal/config/config_test.go`** — table-driven tests for each new resolver
  covering all precedence paths, plus a JSON round-trip test. _Why: every
  resolver has a spec-mandated precedence chain._
- **`cmd/sworn/run.go`** — replace ad-hoc `resolveVerifierModel` and
  `resolveEscalationModels` with calls to `config.Resolve*`, load config to
  resolve implementer model and retry cap. _Why: run.go is the integration
  point that wires config resolution into the run loop._
- **`cmd/sworn/init.go`** — add prompts for implementer model, escalation models,
  max attempts during the apply phase when config is freshly scaffolded. _Why:
  init.go is the entry point for first-time configuration._

## §4. Things I'm NOT doing

- Provider-level model config or multi-provider routing (S10).
- `.env` file loading or API key management beyond the existing `--api-key` flag (S10).
- TUI settings screen for model selection (S17).
- Verifier escalation models or retry logic — verifier stays single-model.
- Changing `run.DefaultEscalationModels` — that's the programmatic safety net,
  not the config default.

## §5. Reachability plan

**Artefact:** `sworn init` smoke step (manual, documented in `proof.md`):
1. Set `SWORN_CONFIG_PATH` to a temp file.
2. Run `sworn init --yes` with piped stdin providing implementer model, escalation
   list, and max attempts.
3. `cat` the written config file.
4. Assert the JSON contains `verifier.model`, `implementer.model`,
   `implementer.escalation_models`, and `implementer.max_attempts`.

Additionally, `go test ./internal/config/...` covers every resolver precedence
path, and `go build ./...` confirms no new external deps.

## §6. Open questions for the Coach

None.