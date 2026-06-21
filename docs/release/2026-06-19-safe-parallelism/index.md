---
title: 'Release board — 2026-06-19-safe-parallelism'
description: 'R3 — safe parallelism: concurrent multi-track delivery, fail-closed verify gate under concurrency, sworn TUI cockpit, overclaim benchmark, sworn login credits on-ramp, webhook paging, MCP server for AI-driven planning + resolution, multi-provider model support with TUI settings, and cross-harness semantic memory search.'
release_worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism
release_worktree_branch: release-wt/2026-06-19-safe-parallelism
tracks:
  - id: T1-concurrency-core
    slices: [S01-process-ownership, S02a-run-refactor, S02b-concurrent-scheduler, S03-verify-under-concurrency]
    depends_on: null
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T1-concurrency-core
    worktree_branch: track/2026-06-19-safe-parallelism/T1-concurrency-core
    state: merged
  - id: T2-monitoring
    slices: [S04a-tui-foundation, S04b-tui-live, S04c-tui-resolution, S05-overclaim-benchmark, S34-tui-merge-actor]
    depends_on: T1-concurrency-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T2-monitoring
    worktree_branch: track/2026-06-19-safe-parallelism/T2-monitoring
    state: in_progress
  - id: T3-commercial
    slices: [S06a-sworn-login-auth, S06b-sworn-proxy-credits, S07-paging, S09-per-role-model-config, S18-consideration-catalog, S19-sworn-induction, S21-canonical-baton]
    depends_on: T1-concurrency-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T3-commercial
    worktree_branch: track/2026-06-19-safe-parallelism/T3-commercial
    state: in_progress
  - id: T4-mcp
    slices: [S08a-mcp-transport, S08b-mcp-ops-tools, S08c-mcp-plan-tools, S22-sworn-doctor]
    depends_on: T1-concurrency-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T4-mcp
    worktree_branch: track/2026-06-19-safe-parallelism/T4-mcp
    state: merged  - id: T5-providers
    slices: [S10-provider-foundation, S11-anthropic-driver, S12-google-driver, S13-bedrock-driver, S14-azure-driver, S15-oci-driver, S16-ollama-driver, S39-openai-responses-provider]
    depends_on: [T1-concurrency-core, T3-commercial]
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T5-providers
    state: planned
  - id: T6-provider-ux
    slices: [S17-tui-provider-config]
    depends_on: [T2-monitoring, T5-providers]
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T6-provider-ux
    state: planned
  - id: T7-mcp-extensions
    slices: [S20-mcp-catalog-tools]
    depends_on: [T3-commercial, T4-mcp]
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T7-mcp-extensions
    state: planned
  - id: T8-memory
    slices: [S23-memory-config, S24-memory-engine, S25-memory-search, S40-memory-test-hygiene]
    depends_on: T1-concurrency-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T8-memory
    worktree_branch: track/2026-06-19-safe-parallelism/T8-memory
    state: in_progress
  - id: T9-telemetry
    slices: [S26-telemetry]
    depends_on: T1-concurrency-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T9-telemetry
    worktree_branch: track/2026-06-19-safe-parallelism/T9-telemetry
    state: merged
  - id: T10-public-readiness
    slices: [S27-public-readiness-scrub]
    depends_on: [T1-concurrency-core, T2-monitoring, T3-commercial, T4-mcp, T5-providers, T6-provider-ux, T7-mcp-extensions, T8-memory, T9-telemetry, T11-infra-safety, T12-harness-hardening, T13-sworn-role-parity]
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T10-public-readiness
    state: planned
  - id: T11-infra-safety
    slices: [S28-git-dir-guard]
    depends_on: T1-concurrency-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T11-infra-safety
    worktree_branch: track/2026-06-19-safe-parallelism/T11-infra-safety
    state: merged
  - id: T12-harness-hardening
    slices: [S29-lint-deps, S30-lint-touchpoints, S31-lint-symbols, S32-designfit-decisions-gate, S33-spec-template-hardening, S35-mutation-guard, S36-captain-resolve-dirty-worktree, S37-telemetry-tui-exclusion, S38-verifier-blocked-violations, S41-build-bin-target, S42-implement-step-timeout, S43-agent-loop-natural-stop, S44-feedback-driven-retry]
    depends_on: T1-concurrency-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T12-harness-hardening
    worktree_branch: track/2026-06-19-safe-parallelism/T12-harness-hardening
    state: in_progress
  - id: T13-sworn-role-parity
    slices: [S45-design-tldr, S46-captain-review, S47-orchestrator-recovery]
    depends_on: T12-harness-hardening
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T13-sworn-role-parity
    state: planned
---

# Release Board: `2026-06-19-safe-parallelism`

> Frontmatter is the machine-readable registry; the tables below mirror it.
> **Prerequisite**: `2026-06-16-fidelity-layer` must be fully merged before R3 implementation begins.

## Release summary

- **Goal**: concurrent multi-track delivery with fail-closed verify gate provably intact
  under concurrency; `sworn` TUI cockpit with blocked-slice TL;DR and interactive settings;
  `sworn login` credits on-ramp; webhook paging; `sworn mcp` as a universal AI planning +
  operations interface; multi-provider model support (Anthropic, Google/Vertex, Bedrock,
  Azure, OCI, Ollama native, plus OAI-compat presets for Groq, Mistral, DeepSeek,
  OpenRouter); per-role model config in config.json.
- **Target version / integration branch**: `release/v0.1.0`
- **Prerequisite release**: `2026-06-16-fidelity-layer` — fully merged before implementation
- **Started**: 2026-06-19
- **Target ship**: uncommitted
- **Intake**: `intake.md`
- **Stakeholder**: Brad (maintainer)
- **Tracking issue**: [#5](https://github.com/swornagent/sworn/issues/5)

## Tracks

> T1 goes first. T2, T3, T4 run in parallel after T1. T5 and T7 start after T3 merges
> (T5 also needs T1; T7 also needs T4). T6 starts after T2 + T5 merge.

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|
| `T1-concurrency-core` | S01 → S02a → S02b → S03 | — | `track/.../T1-concurrency-core` | merged |
| `T2-monitoring` | S04a → S04b → S04c → S05 → S34 | T1 | `track/.../T2-monitoring` | in_progress |
| `T3-commercial` | S06a → S06b → S07 → S09 → S18 → S19 → S21 | T1 | `track/.../T3-commercial` | in_progress |
| `T4-mcp` | S08a → S08b → S08c → S22 | T1 | `track/.../T4-mcp` | merged || `T5-providers` | S10 → S11 → S12 → S13 → S14 → S15 → S16 → S39 | T1 + T3 | `track/.../T5-providers` | planned |
| `T6-provider-ux` | S17 | T2 + T5 | `track/.../T6-provider-ux` | planned |
| `T7-mcp-extensions` | S20 | T3 + T4 | `track/.../T7-mcp-extensions` | planned |
| `T8-memory` | S23 → S24 → S25 → S40 | T1 | `track/.../T8-memory` | in_progress |
| `T9-telemetry` | S26 | T1 | `track/.../T9-telemetry` | merged |
| `T10-public-readiness` | S27 | all (T1–T9) | `track/.../T10-public-readiness` | planned |
| `T11-infra-safety` | S28 | T1 | `track/.../T11-infra-safety` | merged |
| `T12-harness-hardening` | S29 → S30 → S31 → S32 → S33 → S35 → S36 → S37 → S38 → S41 → S42 → S43 → S44 | T1 | `track/.../T12-harness-hardening` | in_progress |
| `T13-sworn-role-parity` | S45 → S46 → S47 | T12 | `track/.../T13-sworn-role-parity` | planned |

### Execution order

```
Phase 1:  T1 (sequential)
Phase 2:  T2, T3, T4, T8, T9, T11, T12 (parallel after T1 — T11/T12 are harness-hardening, dispatch early)
          T13 (after T12 — serial; product role parity, shares internal/run with T12)
Phase 3:  T5 (after T1 + T3)
          T7 (after T3 + T4; may run in parallel with T5)
Phase 4:  T6 (after T2 + T5)
Phase 5:  T10 (after ALL tracks merge — final public-readiness gate before launch)
```

### Touchpoint matrix

> No row may carry `✓` in more than one column in the same parallel phase.
> `cmd/sworn/main.go` is a **documented shared file** (additive dispatch only).
> `(dep)` notation means the track writes this file only after the named dependency merges
> — the dep-edge serialises writes so they are not truly concurrent.
> `T10-public-readiness` (S27) is omitted from the columns below: it depends on every
> other track and runs strictly last (Phase 5), so its wide touchpoints — comment scrubs
> and prompt-text edits across many files — collide with nothing in parallel.
> `T11-infra-safety` (S28) and `T12-harness-hardening` (S29–S33, S35, S36) are likewise
> omitted: T11 touches only `internal/git/`; T12's files are new (`internal/lint/`) or
> tool-specific (`internal/designfit/`, `cmd/sworn/lint.go`) plus prompt files
> (`captain.md`/`planner.md`/`verifier.md`) shared only with T10 — which depends on T12,
> so those writes are sequential, not parallel.
> **Cross-slice dependency (S08c → S21):** `internal/prompt/baton/rules.md` is created by
> `S21-canonical-baton` (T3). `S08c-mcp-plan-tools` (T4) serves it via the `sworn://baton/rules`
> MCP resource, so S08c's rules resource depends on S21's output. Resolution (Captain Pin 2,
> Coach 2026-06-21): **defer that resource as a Rule-2 deferral until S21 lands** — do not add a
> hard T4→T3 dependency that would serialise the tracks. (Exactly the consumer↔creator edge
> S30-lint-touchpoints is meant to surface at plan time.)

| File / surface | T1 | T2 | T3 | T4 | T5 | T6 | T7 | T8 | T9 |
|---|---|---|---|---|---|---|------|---|---|
| `docs/adr/0003-sqlite-orchestration-state.md` | ✓ | | | | |  |
| `docs/adr/0004-dep-policy-minimal-justified.md` (new) | | | | | ✓ |  |
| `CLAUDE.md` | | | | | ✓ |  |
| `internal/db/` (new) | ✓ | | | | |  |
| `internal/supervisor/` (new) | ✓ | | | | |  |
| `internal/run/run.go` | ✓ | | (T1 dep) | | |  |
| `internal/run/slice.go` (new) | ✓ | | | | |  |
| `internal/run/parallel.go` (new) | ✓ | | | | |  |
| `internal/run/run_test.go` | ✓ | | | | |  |
| `internal/scheduler/` (new) | ✓ | | | | |  |
| `internal/scheduler/worker.go` (new, in T1) | ✓ | | (T1 dep) | | |  |
| `internal/verify/verify.go` | ✓ | | | | |  |
| `internal/verify/concurrent_test.go` (new) | ✓ | | | | |  |
| `internal/verdict/verdict.go` | ✓ | | | | |  |
| `internal/model/oai.go` | ✓ | | | | |  |
| `internal/model/client.go` | ✓ | | (T1 dep) | | |  |
| `internal/model/env.go` (new) | | | | | ✓ |  |
| `internal/model/env_test.go` (new) | | | | | ✓ |  |
| `internal/model/provider.go` (new) | | | | | ✓ |  |
| `internal/model/provider_test.go` (new) | | | | | ✓ |  |
| `internal/model/anthropic.go` (new) | | | | | ✓ |  |
| `internal/model/anthropic_test.go` (new) | | | | | ✓ |  |
| `internal/model/google.go` (new) | | | | | ✓ |  |
| `internal/model/google_test.go` (new) | | | | | ✓ |  |
| `internal/model/bedrock.go` (new) | | | | | ✓ |  |
| `internal/model/bedrock_test.go` (new) | | | | | ✓ |  |
| `internal/model/azure.go` (new) | | | | | ✓ |  |
| `internal/model/azure_test.go` (new) | | | | | ✓ |  |
| `internal/model/oci.go` (new) | | | | | ✓ |  |
| `internal/model/oci_test.go` (new) | | | | | ✓ |  |
| `internal/model/ollama.go` (new) | | | | | ✓ |  |
| `internal/model/ollama_test.go` (new) | | | | | ✓ |  |
| `cmd/sworn/run.go` | ✓ | | (T1 dep) | | (T1+T3 dep) |  |
| `go.mod`, `go.sum` | ✓ | | | | (T1 dep) |  |
| `cmd/sworn/main.go` (DOCUMENTED SHARED — additive dispatch) | ✓ | ✓ | ✓ | ✓ | |  |
| `docs/release/<rel>/.captain-trial-log.md` (DOCUMENTED SHARED — Captain review log, append-only: every track's design reviews add one row per slice) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `cmd/sworn/top.go` | | ✓ | | | | (T2 dep)  |
| `internal/tui/` (new) | | ✓ | | | |  |
| `internal/tui/settings.go` (new) | | | | | | ✓  |
| `internal/tui/settings_test.go` (new) | | | | | | ✓  |
| `internal/tui/tui.go` (new, in T2) | | ✓ | | | | (T2 dep)  |
| `internal/bench/overclaim.go` (new) | | ✓ | | | |  |
| `internal/bench/overclaim_test.go` (new) | | ✓ | | | |  |
| `cmd/sworn/bench.go` | | ✓ | | | |  |
| `docs/benchmark/overclaim-concurrent-1to4.md` (new) | | ✓ | | | |  |
| `internal/account/account.go` (new) | | | ✓ | | |  |
| `internal/account/proxy.go` (new) | | | ✓ | | |  |
| `internal/account/notify.go` (new) | | | ✓ | | |  |
| `internal/account/account_test.go` (new) | | | ✓ | | |  |
| `internal/account/proxy_test.go` (new) | | | ✓ | | |  |
| `internal/account/notify_test.go` (new) | | | ✓ | | |  |
| `cmd/sworn/login.go` (new) | | | ✓ | | |  |
| `cmd/sworn/account.go` (new) | | | ✓ | | |  |
| `cmd/sworn/init.go` | | | ✓ | | |  |
| `internal/config/config.go` | | | ✓ | | | (T3 dep via T5)  |
| `internal/config/config_test.go` | | | ✓ | | | (T3 dep via T5)  |
| `docs/templates/considerations.md` (new) | | | ✓ | | | | |
| `docs/templates/decisions.md` (new) | | | ✓ | | | | |
| `internal/prompt/planner.md` | | | ✓ | | | | |
| `internal/prompt/implementer.md` | | | ✓ | | | | |
| `internal/prompt/verifier.md` | | | ✓ | | | | |
| `internal/prompt/prompt.go` | | | ✓ | | | | |
| `internal/prompt/baton/` (new — created by S21/T3; read by S08c/T4 via `sworn://baton/rules`, deferred) | | | ✓ | (T3 dep) | | | | |
| `cmd/sworn/induction.go` (new) | | | ✓ | | | | |
| `cmd/sworn/induction_test.go` (new) | | | ✓ | | | | |
| `internal/mcp/` (new) | | | | ✓ | | |
| `internal/mcp/catalog.go` (new) | | | | | | | ✓ |
| `internal/mcp/catalog_test.go` (new) | | | | | | | ✓ |
| `cmd/sworn/doctor.go` (new) | | | | ✓ | | | |
| `cmd/sworn/doctor_test.go` (new) | | | | ✓ | | | |
| `docs/adr/0005-canonical-baton.md` (new) | | | ✓ | | | | |
| `docs/templates/agents.md` (new) | | | ✓ | | | | |
| `cmd/sworn/mcp.go` (new) | | | | ✓ | |  |
| `docs/mcp-setup.md` (new) | | | | ✓ | |  |
| `internal/memory/` (new) | | | | | | | ✓ | |
| `cmd/sworn/memory.go` (new) | | | | | | | ✓ | |
| `internal/telemetry/` (new) | | | | | | | | ✓ |
| `cmd/sworn/telemetry.go` (new) | | | | | | | | ✓ |

**T3 `depends_on T1` notes:**
- `internal/run/run.go`: S07 adds notification calls; serialised by dep edge
- `internal/model/client.go`: S06b adds proxy routing; serialised by dep
- `internal/scheduler/worker.go`: S07 adds notify call; S02b creates it; serialised by dep
- `cmd/sworn/init.go`: S09 extends model prompts; S18 adds catalog setup prompt; serialised by T1 dep
- `docs/templates/considerations.md`: S18 adds shipped template; new touchpoint, T3 owns it
- `internal/prompt/planner.md`: S18 adds Phase 2b (DRY gate + design consultation + arch conformance); T3 owns it
- `internal/prompt/implementer.md`: S19 adds deviation check step; T3 owns it
- `internal/prompt/verifier.md`: S19 adds catalog conformance check (undocumented deviation = FAIL); T3 owns it
- `docs/templates/decisions.md`: S18 adds decision registry template; T3 owns it
- `cmd/sworn/induction.go`: S19 adds one-time induction command; T3 owns it
- `internal/mcp/catalog.go`, `catalog_test.go`: S20 in T7; new files in T4's directory but T7 depends on T4 (already merged) — no conflict

**T7 `depends_on T3+T4` notes:**
- `internal/mcp/catalog.go` calls functions from packages T3 built (internal/config/,
  docs/considerations.md + docs/decisions.md paths); T7 starts after T3 merges — safe
- `plan_release` in S20 calls T4's internal `createRelease` function from S08c;
  T7 starts after T4 merges — safe
- `internal/config/config.go`: S09 adds implementer fields; T3 owns this file

**T5 `depends_on T1+T3` notes:**
- `cmd/sworn/run.go`: S10 switches to factory; T1 creates exported RunSlice, T3 adds notify
  calls; T5 starts after both merge — no conflict
- `go.mod`, `go.sum`: T1 adds modernc.org/sqlite; T5 adds provider SDKs after T1 merges
- T5 only touches `internal/model/` (new files), go.mod, cmd/sworn/run.go, CLAUDE.md,
  docs/adr/0004-* — no conflict with T2 or T4

**T6 `depends_on T2+T5` notes:**
- `internal/tui/tui.go`: T2 creates it; T6 adds `s` key binding after T2 merges
- `cmd/sworn/top.go`: T2 owns it; T6 modifies it after T2 merges
- `internal/config/config.go`: T3 owns it; T6 adds Save() after T3 merges (via T5 dep chain)

## Slices

| ID | Track | User outcome | State | Spec |
|---|---|---|---|---|
| `S01-process-ownership` | T1 | SQLite registry + reap-on-restart; single-owner identity | verified | [spec](./S01-process-ownership/spec.md) |
| `S02a-run-refactor` | T1 | `run.RunSlice()` exported; callable from goroutine; no regression | verified | [spec](./S02a-run-refactor/spec.md) |
| `S02b-concurrent-scheduler` | T1 | `sworn run --parallel` launches all independent tracks concurrently | verified | [spec](./S02b-concurrent-scheduler/spec.md) |
| `S03-verify-under-concurrency` | T1 | Verify gate goroutine-safe and fail-closed at N>1 | verified | [spec](./S03-verify-under-concurrency/spec.md) |
| `S04a-tui-foundation` | T2 | `sworn` (no args) shows releases list + board view with navigation | planned | [spec](./S04a-tui-foundation/spec.md) |
| `S04b-tui-live` | T2 | Live concurrent track status from DB (1s poll) + credit balance in header | planned | [spec](./S04b-tui-live/spec.md) |
| `S04c-tui-resolution` | T2 | Blocked slice TL;DR panel + options + open in Claude Code / Codex | planned | [spec](./S04c-tui-resolution/spec.md) |
| `S05-overclaim-benchmark` | T2 | Overclaim rate flat at N=1/2/4; published benchmark artefact | planned | [spec](./S05-overclaim-benchmark/spec.md) |
| `S06a-sworn-login-auth` | T3 | `sworn login` device-code flow; credentials file; `sworn logout` | planned | [spec](./S06a-sworn-login-auth/spec.md) |
| `S06b-sworn-proxy-credits` | T3 | Model calls route via SwornAgent proxy; `sworn account buy`; credit display | planned | [spec](./S06b-sworn-proxy-credits/spec.md) |
| `S07-paging` | T3 | FAIL/BLOCKED fires webhook + email; developer paged without watching terminal | planned | [spec](./S07-paging/spec.md) |
| `S08a-mcp-transport` | T4 | `sworn mcp` JSON-RPC server; initialize handshake; tools scaffold | planned | [spec](./S08a-mcp-transport/spec.md) |
| `S08b-mcp-ops-tools` | T4 | 9 ops tools: get_board, get_blocked, get_slice_context, rerun, patch, merge, defer | planned | [spec](./S08b-mcp-ops-tools/spec.md) |
| `S08c-mcp-plan-tools` | T4 | 4 planning tools + resources + prompts + mcp-setup.md | planned | [spec](./S08c-mcp-plan-tools/spec.md) |
| `S09-per-role-model-config` | T3 | Config file gains implementer.model, escalation_models, max_attempts; sworn init prompts for both roles | planned | [spec](./S09-per-role-model-config/spec.md) |
| `S10-provider-foundation` | T5 | ADR 0004 + provider router + OAI-compat presets (8 providers) + .env file loading + typed `model.Error{Kind}` taxonomy (classify/UserMessage) | planned | [spec](./S10-provider-foundation/spec.md) |
| `S11-anthropic-driver` | T5 | Anthropic Claude models work as verifier and implementer via Messages API | planned | [spec](./S11-anthropic-driver/spec.md) |
| `S12-google-driver` | T5 | Google Gemini and Vertex AI models work as verifier and implementer | planned | [spec](./S12-google-driver/spec.md) |
| `S13-bedrock-driver` | T5 | AWS Bedrock models work via Converse API; IAM auth | planned | [spec](./S13-bedrock-driver/spec.md) |
| `S14-azure-driver` | T5 | Azure OpenAI deployments work via api-key auth; no new SDK dep | planned | [spec](./S14-azure-driver/spec.md) |
| `S15-oci-driver` | T5 | OCI Generative AI models work via oci-go-sdk | planned | [spec](./S15-oci-driver/spec.md) |
| `S16-ollama-driver` | T5 | Ollama native /api/chat endpoint; replaces OAI-compat shim | planned | [spec](./S16-ollama-driver/spec.md) |
| `S17-tui-provider-config` | T6 | TUI settings panel: provider API keys, model per role, escalation list, max attempts; persists to config.json + ~/.sworn/.env | planned | [spec](./S17-tui-provider-config/spec.md) |
| `S18-consideration-catalog` | T3 | Typed consideration catalog + decision registry; planner Phase 2b (DRY gate, design consultation, arch conformance, capture); sworn init scaffolds both templates | planned | [spec](./S18-consideration-catalog/spec.md) |
| `S19-sworn-induction` | T3 | `sworn induction` one-time repo onboarding (design system + architecture discovery); implementer + verifier prompts gain deviation-surfacing steps | planned | [spec](./S19-sworn-induction/spec.md) |
| `S20-mcp-catalog-tools` | T7 | 8 MCP tools: plan_release (unified), get_induction_status, get_considerations, search_decisions, record_decision, check_design_system, update_design_system, record_architecture_pattern | planned | [spec](./S20-mcp-catalog-tools/spec.md) |
| `S21-canonical-baton` | T3 | Baton protocol embedded in binary (internal/prompt/baton/); sworn init writes minimal MCP-pointer AGENTS.md instead of per-repo Baton copy; ADR-0005 | planned | [spec](./S21-canonical-baton/spec.md) |
| `S22-sworn-doctor` | T4 | Prompt integrity checks; legacy docs/baton/ + AGENTS.md splice detection with --fix; optional ~/.claude/baton/ sync with --sync-baton | verified | [spec](./S22-sworn-doctor/spec.md) || `S23-memory-config` | T8 | `sworn memory status` shows harnesses, memory paths, embedding provider; global + per-project config | planned | [spec](./S23-memory-config/spec.md) |
| `S24-memory-engine` | T8 | `sworn memory build` embeds all memory entries via voyage/oai-compat/ollama; incremental SQLite index | planned | [spec](./S24-memory-engine/spec.md) |
| `S25-memory-search` | T8 | `sworn memory search <query>` returns ranked results; captain-memory-search.py becomes a shim | planned | [spec](./S25-memory-search/spec.md) |
| `S40-memory-test-hygiene` | T8 | memory tests use `t.TempDir()`; removes stray `test-fixture/` + root `fake_ollama.go` so `go test ./internal/memory/...` leaves git clean | planned | [spec](./S40-memory-test-hygiene/spec.md) |
| `S26-telemetry` | T9 | Anonymous command telemetry to api.sworn.sh; opt-out via env var or sentinel file; first-run disclosure | verified | [spec](./S26-telemetry/spec.md) |
| `S27-public-readiness-scrub` | T10 | Make repo + binary public-safe: generalise embedded role prompts (keep Captain/Coach, strip coach-loop coupling), scrub dogfood provenance comments + fired/GetFired + coach-loop refs. Final launch gate. | planned | [spec](./S27-public-readiness-scrub/spec.md) |
| `S28-git-dir-guard` | T11 | internal/git fails closed on empty Repo.Dir so a git op can't run on the ambient worktree (fixes workers writing to main, sworn#6) + regression test | verified | [spec](./S28-git-dir-guard/spec.md) |
| `S29-lint-deps` | T12 | `sworn lint deps` — go.mod/go.sum diff vs planned_files, fail-closed; planner auto-adds dep files | planned | [spec](./S29-lint-deps/spec.md) |
| `S30-lint-touchpoints` | T12 | `sworn lint touchpoints` — design files/pkgs vs planned_files + collision matrix + migration-number collision | planned | [spec](./S30-lint-touchpoints/spec.md) |
| `S31-lint-symbols` | T12 | `sworn lint symbols` — grep back-ticked design identifiers against the live codebase | planned | [spec](./S31-lint-symbols/spec.md) |
| `S32-designfit-decisions-gate` | T12 | `sworn designfit` fails closed when Type-1 work is declared but `design_decisions` is empty | planned | [spec](./S32-designfit-decisions-gate/spec.md) |
| `S33-spec-template-hardening` | T12 | spec/prompt hardening: Risk-cites-`file:line`, pure-engine two-commit note, dynamic-CORS note, + verifier watcher-block cleanup | planned | [spec](./S33-spec-template-hardening/spec.md) |
| `S34-tui-merge-actor` | T2 | render the `merge:<track>` actor as a distinct row in the TUI live view + release board | planned | [spec](./S34-tui-merge-actor/spec.md) |
| `S35-mutation-guard` | T12 | Captain check + Baton-rule clause for process-global mutation (cwd/git-state/os.Chdir) — the sworn#6 class | planned | [spec](./S35-mutation-guard/spec.md) |
| `S36-captain-resolve-dirty-worktree` | T12 | Captain auto-resolves dirty track worktrees (commit-by-default, record the diff+resolution, never page the Coach) | planned | [spec](./S36-captain-resolve-dirty-worktree/spec.md) |
| `S37-telemetry-tui-exclusion` | T12 | no-args/TUI launch no longer fires a junk telemetry event (empty cmd + session-length); exclusion in `telemetry.Fire()`, not the shared main.go (sworn#7) | planned | [spec](./S37-telemetry-tui-exclusion/spec.md) |
| `S38-verifier-blocked-violations` | T12 | a BLOCKED verdict must populate `status.json` violations (not just journal prose) + a gate rejecting blocked-with-empty-violations — fixes blank REPLAN pages | planned | [spec](./S38-verifier-blocked-violations/spec.md) |
| `S41-build-bin-target` | T12 | canonical `make build` → `bin/sworn` + `docs/build.md` run-from-root convention; stops `cmd/sworn/.sworn` + `docs/release/run-*` worktree clutter | planned | [spec](./S41-build-bin-target/spec.md) |
| `S42-implement-step-timeout` | T12 | `sworn run` bounds each implement attempt with a context deadline; a hung implementer is cancelled and escalates to the next model instead of hanging forever | planned | [spec](./S42-implement-step-timeout/spec.md) |
| `S43-agent-loop-natural-stop` | T12 | agent loop terminates on the model's natural stop (no tool calls) instead of spinning to the turn cap; salvages work from empty-final-text models (gpt-oss-class) by letting proof-from-diff + verifier judge | planned | [spec](./S43-agent-loop-natural-stop/spec.md) |
| `S44-feedback-driven-retry` | T12 | on verify FAIL, feed the verifier's rationale + violations into the next implement attempt's prompt instead of blind re-running; + provider-error retry policy (terminal→fail-fast, transient→backoff) consuming S10's `model.Error{Kind}` (depends_on S10) | planned | [spec](./S44-feedback-driven-retry/spec.md) |
| `S45-design-tldr` | T13 | `sworn run` generates a design TL;DR (§1–6) before implementation — restores the pre-code design artefact for the captain to review | planned | [spec](./S45-design-tldr/spec.md) |
| `S46-captain-review` | T13 | captain agent reviews the TL;DR + live code, emits classified pins, writes review.md, and gates implement (proceed if no escalate pins, else halt+surface) — the in-product `/design-review` | planned | [spec](./S46-captain-review/spec.md) |
| `S47-orchestrator-recovery` | T13 | on non-PASS, intelligent triage chooses resolve-in-place / escalate / halt and assesses BLOCKED resolvability — the in-product orchestrator | planned | [spec](./S47-orchestrator-recovery/spec.md) |
| `S39-openai-responses-provider` | T5 | first-class OpenAI provider via /v1/responses (reasoning_effort + tool-calls + built-in web_search) + a cross-provider WebSearch/WebFetch agent tool — fixes gpt-5.x support + 'more than 6 tools' | planned | [spec](./S39-openai-responses-provider/spec.md) |

## Aggregate state

- Planned: 31
- In progress: 0
- Design review: 3
- Implemented: 1
- Verified: 18
- Failed verification: 0
- Deferred: 0

**Tracks:** Planned: 5 / In progress: 4 / Merged: 4
> Note: T3 now has 7 slices; T4 now has 4 slices; T8 new (3 slices); T9 new (1 slice);
> T10 new (1 slice: S27, the final public-readiness gate); T11 new (1 slice: S28, the
> sworn#6 git-dir safety fix); T12 new (7 harness-hardening slices from the trial-log harvest);
> S34 appended to T2. Release now **53 slices across 13 tracks** (S40→T8, S41–S44→T12, S45–S47→new T13 — 2026-06-21 hygiene + run-reliability + role-parity replans).

## Recent activity

### 2026-06-28 — track `T4-mcp` merged to release-wt (commit 732265d)

- **Actor**: track integrator (/merge-track)
- **Note**: 4 verified slices merged: S08a-mcp-transport, S08b-mcp-ops-tools, S08c-mcp-plan-tools, S22-sworn-doctor. Track state -> merged.

### 2026-06-21 — replan: provider-error taxonomy (re-scope S10 + S44)
- **Actor**: planner (`/replan-release`)
- **Trigger**: live coach-loop run hit an OpenRouter 402 (out of credits) that masked as a cryptic "stream error" and then retry-looped. The bash harness was hardened (error surfacing, terminal-PAGE, retry cap, captain rotation); this replan brings the same robustness to **sworn the product** so a user running dry / with a bad key gets an actionable error, not a raw provider dump or a silent spin. Coach decision: land it in S10 (foundation) + S44 (consumer), not a new slice.
- **S10-provider-foundation re-scoped** (still planned, T5): adds a typed `model.Error{Kind}` taxonomy (`internal/model/errors.go`) — `ClassifyHTTP` maps 401/403→Auth, 402→Credits, 429→RateLimit, 5xx→Upstream; `IsTerminal`/`IsTransient`; `UserMessage()`. `oai.go` returns `*model.Error` on non-2xx (still satisfies `error`); `run.go` prints `UserMessage()`. New touchpoints: `internal/model/errors.go(+_test)`, `oai.go` (modify).
- **S44-feedback-driven-retry re-scoped** (still planned, T12 tail; **now depends_on S10**): adds a provider-error retry policy consuming the taxonomy — terminal (Auth/Credits) → fail fast, no model escalation; transient (RateLimit/Upstream) → backoff on the same model. Orthogonal to the existing verifier-FAIL-feedback path. Cross-track dep recorded here (schema has no per-slice `depends_on` field); both slices are planned/not-started so sequencing is clean.
- **No new slices, no new tracks** — re-scope of two planned slices only. Release count unchanged (53 slices / 13 tracks).

### 2026-06-21 — replan: new track T13-sworn-role-parity (S45/S46/S47)

- **Actor**: planner (`/replan-release`)
- **Directive**: sworn must mirror the coach-loop's roles — forward-only, no regressions. Losing captain / TL;DR-review / orchestrator is going backwards. See the parity capture (`internal-docs/captures/2026-06-21-sworn-coach-loop-role-parity.md`).
- **New track `T13-sworn-role-parity`** (depends_on T12 — both touch `internal/run`, so serialized): **S45-design-tldr** (`sworn run` emits the §1–6 design TL;DR before code), **S46-captain-review** (captain agent reviews the TL;DR, emits classified pins, gates implement — the in-product `/design-review`), **S47-orchestrator-recovery** (intelligent triage on non-PASS: resolve-in-place / escalate / halt, + BLOCKED resolvability — the in-product orchestrator; builds on S44).
- **Gap closed**: sworn had the captain's *known catches* as deterministic gates (S29–S33, S35) and the embedded `captain.md`, but not the captain's *judgment* in the loop, nor an intelligent recovery orchestrator. T13 restores both.
- **Ripple (tracked separately)**: the MCP surface (T4) and the TUI (T2) will need parity updates to expose/render the new roles + states — to be sliced next.
- **Release now 53 slices across 13 tracks.**

### 2026-06-21 — replan: S44-feedback-driven-retry (resolve, don't blind-retry)

- **Actor**: planner (`/replan-release`)
- **S44-feedback-driven-retry → T12 tail** (after S43): on a verifier FAIL, `RunSlice` clears `status.json` verification (`slice.go:123`) and re-implements with the next model but never passes the verifier's rationale to the implementer — a blind retry. S44 preserves the rationale + violations and injects them into the next implement attempt's prompt, so retry resolves the named failure instead of re-deriving from the spec. Most direct embodiment of "don't fail what an intelligent agent could resolve." Touches T1-owned `internal/run` + `internal/implement` (merged → no collision).
- **Release now 50 slices across 12 tracks.** (First of the sworn↔coach-loop role-parity work — see the parity capture; captain/design-review + orchestrator/interpreter slices to follow.)

### 2026-06-21 — replan: S43-agent-loop-natural-stop (salvage empty-final-text work)

- **Actor**: planner (`/replan-release`)
- **S43-agent-loop-natural-stop → T12 tail** (after S42): the agent loop (`internal/agent/agent.go:111`) returns cleanly only on text+no-tool-calls; a model that finishes its work then stops with empty content + no tool calls (gpt-oss-class) spins to `MaxTurns` and errors, discarding the diff and forcing a blind model escalation. S43 treats "no tool calls" as terminal regardless of content — sworn judges ground truth (proof built from `git diff`, not prose), so the verifier decides PASS/FAIL over the actual work. In-product analogue of the coach-loop's force-summary, but simpler (nothing downstream consumes the agent's prose). Touches T1-owned `internal/agent` (T1 merged → no collision).
- **Release now 49 slices across 12 tracks.**

### 2026-06-21 — replan: S42-implement-step-timeout (run-loop reliability)

- **Actor**: planner (`/replan-release`)
- **S42-implement-step-timeout → T12 tail** (after S41): the `internal/run/slice.go` escalation loop already advances `escalationModels[attempt]` on an `implement.Run` error, but nothing bounds the implement step — `cmd/sworn/run.go` passes `context.Background()`, `internal/model/oai.go` defaults to `http.DefaultClient` (no timeout). A hung implementer (model API stall / agent infinite loop) blocks the run forever and never escalates. S42 wraps each attempt in `context.WithTimeout`; the model call already honours ctx cancellation, so a deadline-exceeded return flows into the existing escalate path. Touches T1-owned `internal/run` files (T1 merged → no in-flight collision). This is the in-product version of the gap that pinned `gpt-oss-120b` at slot-1 in the coach-loop (whose rotation only counts verifier FAILs); the coach-loop is **not** being changed — the logic belongs in sworn.
- **Release now 48 slices across 12 tracks.**

### 2026-06-21 — replan: two worktree-hygiene slices (S40, S41) from the cleanup session

- **Actor**: planner (`/replan-release`)
- **Base-sync (Step 1)**: forward-merged `release/v0.1.0` into release-wt cleanly — pulled `4c47ac5` (gpt-4.1→claude-sonnet-4-6 default).
- **S40-memory-test-hygiene → T8 tail** (after S25): the memory tests write `test-fixture/` + a root `fake_ollama.go` into the tree instead of `t.TempDir()`, tripping the Gate -1 cleanliness check on T8 (a `.gitignore test-fixture/` stopgap landed at `5d1b7c4`). Placed in **T8, not T12** — it edits `internal/memory/*_test.go`, which the touchpoint matrix assigns to T8; a T12 placement would collide.
- **S41-build-bin-target → T12 tail** (after S38): canonical `make build` → `bin/sworn` + a new `docs/build.md` run-from-repo-root convention, so sworn run-state stops cluttering `cmd/sworn/` (the recurring `cmd/sworn/.sworn` + `docs/release/run-*`). Documented in a new `docs/build.md` rather than `AGENTS.md` (owned by S21/T3, S22/T4) to stay collision-free. Defers the in-code state-dir resolution and the prompt smoke-step wording (the latter to S33).
- **Release now 47 slices across 12 tracks.** Both slices append to non-started tails; Step 6 forward-merged release-wt into the in-flight tracks (T2/T3/T4/T8/T12).

### 2026-06-21 — replan: harness-hardening batch (S29–S36) from the trial-log harvest

- **Actor**: planner (`/replan-release`)
- **New track `T12-harness-hardening`** (depends T1; dispatch early): **S29-lint-deps**, **S30-lint-touchpoints**, **S31-lint-symbols**, **S32-designfit-decisions-gate**, **S33-spec-template-hardening**, **S35-mutation-guard**, **S36-captain-resolve-dirty-worktree**. Each hardens the automation against a recurring class the Captain design-gate has been catching by hand (186-review harvest at `internal-docs/captures/2026-06-21-captain-trial-log-harvest.md`).
- **S34-tui-merge-actor** appended to T2's tail: render the `merge:<track>` actor (now emitted by the coach-loop merge-tag) in the TUI live view + board.
- **S36** added per Coach direction: dirty worktrees are only worker-caused, so the Captain auto-resolves (commit-by-default, record diff+resolution) rather than paging.
- **Also landed live this session** (outside the release tree): coach-loop merge-actor tag + post-dispatch worktree-flip guard (sworn#6); verifier `## Status block` watcher-wrapper removed (metadata kept). 10 fired latent bugs filed at `firedau/fired#968–977`.
- **Release now 45 slices across 12 tracks.** Lightweight add — T12 is a new planned track and S34 appends to T2's tail, so no cross-track forward-merge was needed.

### 2026-06-21 — track `T11-infra-safety` merged to release-wt (commit d242687)

- **Actor**: track integrator (/merge-track)
- **Note**: 1 verified slice merged: S28-git-dir-guard. Track state → merged. (Forward-merged release-wt into track worktree before integration; 18 sibling commits reconciled, tests re-run green.)

### 2026-06-21 — S28 verifier verdict: PASS (round 1)

- **Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant)
- **Slice**: S28-git-dir-guard → state: **verified**
- **All six gates passed.** `Repo.run()` guard fires before exec on empty Dir; `TestRunRejectsEmptyDir` and `TestEmptyDirDoesNotTouchCwd` both PASS; 11/11 full suite PASS; `go build ./...` + `go vet ./internal/git/...` clean; no silent deferrals; all 4 ACs delivered.
- **T11-infra-safety is complete.** S28 is its only slice; track state → ready_to_merge.
- **Next**: `/merge-track T11-infra-safety 2026-06-19-safe-parallelism` in a fresh session.

### 2026-06-21 — track `T9-telemetry` merged to release-wt (commit ee4b729)

- **Actor**: track integrator (/merge-track)
- **Note**: 1 verified slice merged: S26-telemetry. Track state → merged.

### 2026-06-21 — S26 verifier verdict: PASS (round 3)

- **Actor**: verifier (fresh context, Rule 7 compliant)
- **Slice**: S26-telemetry → state: **verified**
- **All six gates passed.** 19/19 tests pass with `-race`. `sworn telemetry on|off|status` and `main.go` dispatch wrapper fully wired. Smoke test confirmed disclosure text on stderr against clean config dir. Proof.md accurately reflects full 21-file diff with per-group provenance for forward-merge artefacts. AC1/AC2 deferrals carry all three Rule-2 fields.
- **T9-telemetry is complete.** S26 is the only slice; track state → ready_to_merge.
- **Next**: `/merge-track T9-telemetry 2026-06-19-safe-parallelism` in a fresh session.

### 2026-06-21 — S26 verifier verdict: FAIL (round 2, 2 violations)

- **Actor**: verifier (fresh context, Rule 7 compliant)
- **Slice**: S26-telemetry → state: **failed_verification**
- **Violation 1 (Gate 2)**: Commit `5139882` landed on T9 track and modified `internal/prompt/implementer.md` (T3-owned per touchpoint matrix) and `internal/adopt/baton/rules/10-customer-journey-validation.md` (not in any planned touchpoints). Neither file appears in proof.md "Files changed" or "Divergence from plan".
- **Violation 2 (Gate 2)**: proof.md "Files changed" lists 8 files; actual diff spans 21 entries. S21 replan artefacts (`d4f886b`), `approved-ack.md` deletion, S27 specs are all committed to T9 track but unexplained in proof.md. All other gates (1, 3, 4, 5, 6) PASS; 19/19 tests pass with -race.
- **Next**: `/implement-slice S26-telemetry 2026-06-19-safe-parallelism` in a fresh session.

### 2026-06-21 — replan: S21 re-scoped + S27 added (public-readiness gate)

- **Actor**: planner (`/replan-release`)
- **S21-canonical-baton re-scoped**: embed **10 rules** (not 7), built from the in-repo canonical `internal/adopt/baton/rules/` (`01`–`10`) instead of "verbatim from `~/.claude/baton/`" (stale at 7, would drop Rules 8/9/10). The role-prompt generalisation the verbatim copy would have leaked is split out to S27.
- **S27-public-readiness-scrub added** in new track **T10-public-readiness** (depends on every track; runs last — the launch gate): generalise the embedded role prompts (keep Captain/Coach, strip coach-loop/`--auto-ack`/`approved-ack`/S21-stall/project-memory; operationally intact), scrub the 8 dogfood provenance comments, the `fired`/GetFired leak, and `coach-loop` references across source + release artefacts.
- **Base sync**: release-wt forward-merged `release/v0.1.0` to pick up the no-mock→Rule-10 reconciliation (`5139882`).
- **S28-git-dir-guard added** in new track **T11-infra-safety** (depends on T1; dispatch early) — the in-repo structural fix for **sworn#6** (workers writing to `main`): `internal/git.run()` fails closed on empty `Repo.Dir` + regression test. Harness defence-in-depth (a coach-loop post-dispatch worktree-branch guard) landed separately in `~/.claude/bin/coach-loop`.
- **Release now 34 slices across 11 tracks.**
- **Staleness note**: the per-slice State column and the Aggregate-state block remain stale vs the board oracle (known release-wt/track-branch lag); `release-board-status.sh` is authoritative. Not fully reconciled in this pass.

### 2026-06-21 — track `T1-concurrency-core` merged to release-wt (commit 581b6a9)

- **Actor**: track integrator (/merge-track)
- **Note**: 4 verified slices merged: S01-process-ownership, S02a-run-refactor, S02b-concurrent-scheduler, S03-verify-under-concurrency. Track state → merged.

### 2026-06-21 — S03 verifier verdict: PASS (round 1)

- **Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant)
- **Slice**: S03-verify-under-concurrency → state: **verified** (SHA ed4919d)
- **All six gates passed.** `verify.Run()` wired in `internal/run/slice.go:183`; both concurrent tests pass under `-race`; `go test -race -count=10` zero races; no silent deferrals; all 6 ACs delivered with evidence.
- **T1-concurrency-core is complete.** All slices (S01, S02a, S02b, S03) are now verified. T1 state → ready_to_merge.
- **Next**: `/merge-track T1-concurrency-core` in a fresh session, then `/merge-release 2026-06-19-safe-parallelism` once every track is merged.

### 2026-06-21 — S02b verifier verdict: PASS (round 5)

- **Verifier**: fresh-context session, artefact-only inputs
- **Slice**: S02b-concurrent-scheduler → state: **verified** (SHA ac62587)
- **All six gates passed.** `sworn run --parallel` wired end-to-end; `TestCmdRun_Parallel` proves full CLI path; concurrency/failure-cascade/dependency-ordering proven by test suite; no silent deferrals; all delivered items verified.
- **Next**: `/implement-slice S03-verify-under-concurrency 2026-06-19-safe-parallelism` in a fresh session.

### 2026-06-21 — S02b verifier verdict: FAIL (round 4, 2 violations)

- **Actor**: verifier (fresh context, Rule 7 compliant)
- **Slice**: S02b-concurrent-scheduler → state: failed_verification
- **Violation 1 (Gate 3 + AC-2)**: Context-chain bug in `RunParallel` (`parallel.go:110`): `phaseCtx, phaseCancel = context.WithCancel(phaseCtx)` derives each phase's context from the previous (cancelled) phase context. After phase 0 completes and `phaseCancel()` is called, phase 1's context is immediately cancelled. All dependent tracks (phase 1+) are skipped with "depends_on failed (phase barrier)" even when their dependencies PASS. Verified: T1 passes → T2 (depends_on T1) is SKIPPED. Fix: `context.WithCancel(ctx)` at `parallel.go:110`.
- **Violation 2 (Gate 3)**: No test covers the AC-2 success path. All existing tests use single-phase plans (no deps exercised). The bug persisted through 4 rounds because no test placed a dependent track in phase 1 with a passing dependency.
- **Next**: `/implement-slice S02b-concurrent-scheduler 2026-06-19-safe-parallelism` in a fresh session. Fix: (1) change `context.WithCancel(phaseCtx)` → `context.WithCancel(ctx)` at `parallel.go:110`; (2) add `TestRunParallel_DependentTrackRunsAfterSuccess` in `parallel_test.go`.

### 2026-06-21 — S02b verifier verdict: FAIL (round 3, 1 violation)

- **Actor**: verifier (fresh context, Rule 7 compliant)
- **Slice**: S02b-concurrent-scheduler → state: failed_verification
- **Violation 1 (Gate 4)**: Spec prescribes "smoke step — `sworn run --parallel --release <fixture>`" as the reachability artefact. Proof substitutes unit test output from `TestRunParallel_TimingConcurrency` (which calls `RunParallel()` directly). The `cmdRun()` entry point in `cmd/sworn/run.go:63-91` (DB open, RunSliceFn closure, RunParallel dispatch) is exercised by no test and no documented binary invocation.
- **All other gates (1, 2, 3, 5, 6) passed.** Tests all pass with `-race` (fresh run verified). Implementation is functionally correct.
- **Next**: `/implement-slice S02b-concurrent-scheduler 2026-06-19-safe-parallelism` in a fresh session. Fix: either run the binary against a fixture and paste actual stderr output into proof.md, OR add `TestCmdRun_Parallel` in `cmd/sworn/run_test.go` invoking `cmdRun()` with `--parallel`.

### 2026-07-01 — S02b verifier verdict: FAIL (round 2, 1 violation)

- **Actor**: verifier (fresh context, Rule 7 compliant)
- **Slice**: S02b-concurrent-scheduler → state: failed_verification
- **Violation 1 (Gate 2)**: `start_commit` in status.json is `d9ff1b1` (re-implementation start), but planned touchpoints (scheduler.go, worker.go, parallel.go, track.go, run.go, scheduler_test.go) were committed in round-1 commit `5bb3666` which predates `d9ff1b1`. `git diff --name-only d9ff1b1` shows only docs/prompt/binary files, not the planned implementation files. proof.md "Files changed" falsely claims these files "were committed in start_commit `d9ff1b1`" — only parallel_test.go and worker_test.go were in that commit.
- **Note**: All tests pass with -race. Gate 1, 3, 4, 5, 6 all pass. The implementation is functionally correct; only the proof.md accuracy and start_commit value are at issue.
- **Next**: `/implement-slice S02b-concurrent-scheduler 2026-06-19-safe-parallelism` in a fresh session to fix start_commit (→ 821edf2) and update proof.md "Files changed."

### 2026-06-27 — S02b verifier verdict: FAIL (6 violations)

- **Actor**: verifier (fresh context, Rule 7 compliant)
- **Slice**: S02b-concurrent-scheduler → state: failed_verification
- **Violation 1 (Gate 3)**: `TestWorkerMaterialisesWorktree` absent; worktree materialisation branch untested.
- **Violation 2 (Gate 3)**: `TestWorkerCallsRunSlice` absent; single-slice count assert only.
- **Violation 3 (Gate 3)**: AC-3 failure cascade has no functional test — both fake-fail helpers return nil and are never called.
- **Violation 4 (Gate 3)**: "fake workers with controllable timing channels" not implemented; AC-1 concurrency assertion has zero coverage.
- **Violation 5 (Gate 2)**: `parallel_test.go` and `sworn` binary in committed diff but absent from proof.md and unexplained in Divergence from plan.
- **Violation 6 (Gate 4)**: Reachability artefact shows commented expected output with inconsistent fixture (2 tracks vs. real 9-track board); smoke step not demonstrably executed.
- **Next**: `/implement-slice S02b-concurrent-scheduler 2026-06-19-safe-parallelism` in a fresh session to address all 6 violations.

### 2026-06-20 — board reconciliation: T1 slice states corrected from oracle

- **Actor**: planner (Claude)
- **Note**: index.md body tables were stale vs. branch reality. Corrected:
  S01 `implemented` → `verified` (verifier PASS on T1 branch); S02a `planned` →
  `verified` (verifier PASS on T1 branch); S02b `planned` → `design_review`
  (implementer escalated: design.md committed, awaiting Captain ack).
  T1 Tracks table row corrected `planned` → `in_progress`. Aggregate state updated:
  Planned 31 → 29; Implemented 1 → 0; Verified 0 → 2; Design review: 1 added.
  No spec changes. Replan trigger: `/replan-release` invoked while S02b was in
  `design_review` state; correct next step is `/design-review S02b-concurrent-scheduler`.

### 2026-06-20 — S02a verifier verdict: PASS

- **Actor**: verifier (fresh context, Rule 7 compliant)
- **Slice**: S02a-run-refactor → state: verified
- **All six gates passed.** `RunSlice()` exported and wired; 12/12 tests pass with `-race`; state transitions verified live; no deferrals; `Run()` regression suite clean.
- **Next**: `/implement-slice S02b-concurrent-scheduler 2026-06-19-safe-parallelism` in a fresh session.

### 2026-06-20 — S02a verifier verdict: FAIL

- **Actor**: verifier (fresh context)
- **Slice**: S02a-run-refactor → state: failed_verification
- **Violation 1**: `start_commit` is null in status.json — required field not set; diff range cannot be formally bounded. Fix: set `start_commit` to `0aaa4b1`.
- **Violation 2**: Gate 6 — test names `TestRunSlice_Pass` and `TestRunSlice_Fail` do not match spec AC names `TestRunSlice` and `TestRunSliceFail`; proof.md "Divergence from plan" incorrectly records "(none)". Fix: rename tests and update proof.md.
- **Note**: 11/11 tests pass with `-race`; functional implementation is sound. Both violations are process/naming compliance issues.
- **Next**: `/implement-slice S02a-run-refactor 2026-06-19-safe-parallelism` in a fresh session.

### 2026-06-20 — S01 verifier verdict: PASS

- **Actor**: verifier (fresh context, session 3)
- **Slice**: S01-process-ownership → state: verified
- **All six gates passed.** Entry point wired (`sworn run --task` → `run.Run()` → `supervisor.Reap()/Acquire()`); touchpoint divergences documented; all required tests pass with `-race`; proof.md documents exact crash-and-reap smoke commands; no silent deferrals in code; all delivered items verified against live repo.
- **Next**: `/implement-slice S02a-run-refactor 2026-06-19-safe-parallelism` in a fresh session.

### 2026-06-20 — S01 verifier verdict: FAIL (Gate 4 + Gate 6)

- **Actor**: verifier (fresh session)
- **Slice**: S01-process-ownership → state: failed_verification
- **Gate 4**: proof.md reachability artefact lacks required exact smoke-step commands (crash-and-reap cycle); spec requires them documented in proof.md.
- **Gate 6**: proof.md "Delivered" claims `cmd/sworn/run.go` was updated but it is not in the diff; actual supervisor integration is in `internal/run/run.go`. Replan explicitly required this correction before re-verification; it was not applied.
- **Next**: `/implement-slice S01-process-ownership 2026-06-19-safe-parallelism` to address both violations.

### 2026-06-20 — replan: T9-telemetry added (S26; anonymous usage telemetry)

- **Actor**: planner (Claude)
- **Note**: Added T9-telemetry (depends T1, parallel Phase 2). Single slice S26:
  anonymous command telemetry to api.sworn.sh/v1/events. Schema: cmd, sub,
  duration_ms, exit_code, sworn_version, os/arch, anonymous install_id (UUIDv4).
  No code/paths/content collected. Opt-out via SWORN_NO_TELEMETRY env var or
  ~/.config/sworn/.no-telemetry sentinel file. First-run disclosure on stderr.
  Client fails silently if api.sworn.sh is unreachable (ships ready; backend
  goes live separately). No new external deps (stdlib net/http only). cmd/sworn/
  main.go wrap is additive. Release now 32 slices across 9 tracks.

### 2026-06-20 — replan: T8-memory added (S23/S24/S25; cross-harness semantic memory search)

- **Actor**: planner (Claude)
- **Note**: Added T8-memory (depends T1, parallel with T2/T3/T4). Three sequential
  slices: S23-memory-config (config schema + harness path discovery + `sworn memory
  status`), S24-memory-engine (embedding adapter for voyage-code-3/oai-compat/ollama
  + SQLite vector index + `sworn memory build`), S25-memory-search (`sworn memory
  search` + captain-memory-search.py shim). No new external deps: embedding API
  calls use stdlib net/http; SQLite index reuses modernc.org/sqlite from T1.
  Phase 2 now: T2, T3, T4, T8 run in parallel after T1. Release now 31 slices
  across 8 tracks.

### 2026-06-26 — S01 verifier verdict: BLOCKED (spec defect) — Resolved by replan above

- **Actor**: verifier (fresh session)
- **Slice**: S01-process-ownership → state unchanged (implemented, verification.result = blocked)
- **Reason**: Spec names `sworn run --parallel` as S01's entry point (Gate 1). S02b's spec explicitly owns this flag. S01's correct entry point is `sworn run --task`, which the implementation correctly wires.

### 2026-06-20 — replan: S01 spec corrected; BLOCKED verdict cleared

- **Actor**: planner (Claude)
- **Note**: Verifier returned BLOCKED on S01. Primary defect: spec named `sworn run
  --parallel` as entry point (that flag is S02b scope); implementation correctly uses
  `sworn run --task`. Spec amended throughout. Gate 6 (subsumed): `proof.md` falsely
  attributes supervisor integration to `cmd/sworn/run.go`; actual file is
  `internal/run/run.go`. Proof correction deferred to implementer before next
  verification attempt. `verification.result` cleared to `pending`; state stays
  `implemented`. S01 row corrected from `planned` to `implemented` in board.

### 2026-06-20 — replan: canonical Baton + sworn doctor (S21 T3, S22 T4; S08c fixed)

- **Actor**: planner (human + Claude)
- **Note**: Identified seam between binary-embedded prompts, Baton on developer machines,
  and per-repo Baton copies. Resolution: binary IS canonical Baton. S21 embeds full
  Baton protocol at internal/prompt/baton/ (go:embed); rewrites sworn init to stop
  writing docs/baton/ and stop splicing AGENTS.md — minimal MCP-pointer AGENTS.md
  written instead; ADR-0005 documents the architecture; user prompt overrides deferred
  post-launch. S22 (T4) adds sworn doctor: embedded prompt integrity checks, legacy
  artifact detection (docs/baton/, old-style AGENTS.md splice) with --fix, optional
  ~/.claude/baton/ sync with --sync-baton. S08c spec fixed: sworn://prompts/* and
  new sworn://baton/* resources now explicitly read from internal/prompt/ embed, NOT
  from $HOME/.claude/baton/. 28 slices across 7 tracks.

### 2026-06-20 — replan: induction + MCP catalog tools (S19 T3, S20 T7-new; S18 revised)

- **Actor**: planner (human + Claude)
- **Note**: S18 revised (adds decision registry docs/decisions.md, full design consultation
  + architecture conformance pattern in planner Phase 2b, DRY gate). S19 added to T3:
  `sworn induction` one-time repo onboarding; implementer + verifier prompts gain
  deviation-surfacing steps (undocumented deviation = BLOCKED/FAIL). New T7-mcp-extensions
  track (depends T3+T4) with S20: 8 MCP tools for catalog/decision management + unified
  plan_release tool (replaces create_release from S08c; S08c now implements createRelease
  as an internal function called by S20's plan_release). 26 slices across 7 tracks.

### 2026-06-20 — replan: consideration catalog added (S18, T3 append)

- **Actor**: planner (human + Claude)
- **Note**: S18-consideration-catalog appended to T3-commercial. Typed dimension catalog
  (security/api/data/observability/ui/performance/compliance) at docs/considerations.md;
  planner prompt gains Phase 2b audit step; sworn init scaffolds starter from shipped
  template. RAG-backed NFR sources and guided elicitation wizard deferred post-R3 (Rule 2
  cards in spec). Release now has 24 slices across 6 tracks.

### 2026-06-20 — replan: provider support + TUI settings added (9 new slices, 2 new tracks)

- **Actor**: planner (human + Claude)
- **Note**: Added T5-providers (S10-S16: provider router, ADR 0004, native drivers for
  Anthropic/Google/Bedrock/Azure/OCI/Ollama, OAI-compat presets for Groq/Mistral/
  DeepSeek/OpenRouter) and T6-provider-ux (S17: TUI settings panel). S09-per-role-model-
  config appended to T3-commercial. Dep policy revised from "zero runtime deps" to
  "minimal, justified deps + ADR required" (ADR 0004, documented in S10). OpenCode
  provider coverage used as baseline for provider scope.
- **Replan trigger**: user requested multi-provider model driver support, per-role config,
  .env file loading, and TUI settings for provider/model configuration.

### 2026-06-20 — bootstrapped release-wt branch

- **Actor**: planner (Claude)
- **Note**: `release-wt/2026-06-19-safe-parallelism` branch and worktree created from
  `release/v0.1.0` HEAD (bab35d3). Initial planning was committed directly to the
  integration branch before implementation started; release-wt now diverges from that
  point. `release_worktree_path` updated in frontmatter.

### 2026-06-20 — re-decomposed from 8 to 14 slices

- **Actor**: planner (human + Claude)
- **Note**: 4 over-scoped slices split on review: S02→S02a+S02b, S04→S04a+S04b+S04c,
  S06→S06a+S06b, S08→S08a+S08b+S08c. Each split slice is now a genuine
  one-implementer-session + one-verifier-session unit.

### 2026-06-19 — release planned; specs written

- **Actor**: planner (human + Claude)
- **Note**: 14 slices, 4 tracks. T1 first; T2/T3/T4 parallel after T1.

## Decisions deferred (Rule 2)

See `intake.md` "Adjacent / out of scope" for full deferral cards.

- Full SaaS billing infrastructure (post-R3)
- GitHub Action / Marketplace integration (post-R3)
- Compliance ledger (post-launch)
- Team credit pools (post-R3)
- MCP HTTP/SSE transport (post-R3)
- sworn mcp daemon mode (post-R3)
- TUI mouse support (post-R3)
- AI tool list beyond CC + Codex in TUI (post-R3)
- Windows supervisor PID liveness (post-R3)
- Email via SwornAgent API in S07 (stub if backend not ready)
- `resources/list` dynamic scanning (post-R3)
- TUI auto-fix action [1] subprocess management (may be stubbed — see S04c)
- Azure Entra ID / managed identity auth in S14 (post-R3 — api-key covers enterprise use case)
- OCI instance principal / resource principal auth in S15 (post-R3)
- Ollama model pull / list in S16 (post-R3 — inference is the scope)
- TUI "test this API key" button in S17 (post-R3)
- Verifier escalation models / cascade (deferred — verifier stays single fixed model per run)
- Additional providers beyond OpenCode baseline: Together AI, Fireworks, Cohere (post-R3)
- RAG-backed NFR sources for consideration catalog (post-R3 — see S18 spec Rule 2 card)
- Guided NFR elicitation wizard when no catalog exists (post-R3 — see S18 spec Rule 2 card)
- Semantic/vector search on decisions.md (post-R3 — see S20 spec Rule 2 card)
- Multi-language architecture pattern inference beyond Go (post-R3 — see S19 spec deferral)
- Azure Entra ID / managed identity auth in S14 (post-R3)
- CI lint for catalog conformance (post-R3 — S19 adds role prompt enforcement; automated lint deferred)
- User prompt overrides / project-level Baton customisation (post-launch — see ADR-0005 in S21)
- Slash-command harness migration to read via sworn://prompts/* MCP resources (post-launch)

## Cross-slice / cross-track notes

- **S01 is T1's keystone.** S02a, S02b, S03 all depend on the DB + supervisor API.
- **S02a before S02b.** The RunSlice() refactor must be verified before the scheduler
  is built on top of it.
- **S04a before S04b before S04c.** Each TUI slice extends the previous foundation.
- **S06a before S06b.** Proxy routing requires credentials from the auth flow.
- **S08a before S08b before S08c.** Transport must work before tools are registered.
- **S09 appended to T3 tail.** Per-role model config added after S07-paging. T3 worktree
  starts from release-wt after T1 merges, so S09 can safely touch config.go and run.go.
- **S10 before S11-S16.** The provider router and ProviderConfig struct must exist before
  any native driver can register itself. S10 is T5's keystone.
- **S10 adds stub `ErrDriverNotRegistered` for native prefixes.** S11-S16 each replace
  one stub with a real implementation.
- **S16 replaces the OAI-compat ollama preset.** After S16, `ollama/*` model IDs route
  to the native Ollama driver, not the OAI shim. Backward compatible: same prefix, better
  behaviour.
- **T5 `depends_on T1+T3`.** T5 needs T1 for the exported model interface (post-S02a
  refactor) and go.mod. T5 needs T3 for config.go (S09 adds implementer fields that S10
  reads). T5 starts after BOTH merge.
- **T6 `depends_on T2+T5`.** S17 extends T2's TUI files and T5's ProviderConfig struct.
  T6 starts after BOTH merge.
- **T3 serialised behind T1** via `depends_on`: S07 touches `run.go` and `worker.go`
  created by T1. The dep edge ensures T3's worktree starts from release-wt after T1's
  changes are merged — no touchpoint conflict.
- **`cmd/sworn/main.go`** is documented shared. Each track adds additive dispatch
  cases only. Each command implementation lives in its own `cmd/sworn/<cmd>.go` file.
  T5 and T6 do not need new top-level commands; they touch existing files only.
- **R2 S15 (`sworn top`) coordination**: S04a absorbs/extends `cmd/sworn/top.go`.
  R3 implementation gates on R2 being merged; no parallel-edit risk.
- **T8 `depends_on T1`**: S24 uses `modernc.org/sqlite` from go.mod (added by S01).
  T8 starts after T1 merges. `internal/memory/` is entirely new; no collision with
  any other track. `cmd/sworn/memory.go` is new; additive dispatch to `main.go` only.
- **T9 `depends_on T1`**: `internal/telemetry/` is entirely new. `cmd/sworn/main.go`
  touch is additive (wrap dispatch + disclosure call). No collision with any track.
  T9 can start immediately after T1 merges, parallel with T2/T3/T4/T8.
- **T9 → T3/S09 soft dependency**: T9 ships `internal/telemetry.ShowConsent()`; T3/S09
  adds the consent question to `sworn init` by importing it. T9 should merge before S09
  starts. If T9 is still in flight when S09 begins, the T3 implementer stubs the call
  and wires it in a follow-up commit once T9 lands (still within T3's worktree, no new
  touchpoint). This soft dep is not enforced via `depends_on` (that would delay T9 until
  T3 completes, which inverts the flow) — it is a best-effort merge ordering preference.
- **S26 `api.sworn.sh` backend**: endpoint may not be live at R3 ship time.
  `Fire()` silently drops on any network error — no user impact. Telemetry begins
  flowing once the SwornAgent backend endpoint is deployed.
- **S25 touches `~/.claude/bin/captain-memory-search.py`**: This is a baton install
  file, not in the repo. S25 spec documents the shim update as an out-of-tree
  deliverable; the implementer applies it to the local baton install and notes the
  path in `proof.md`.

### 2026-06-28 — S22-sworn-doctor verified

- **Actor**: verifier (`/verify-slice`)
- **Verdict**: PASS — all six verification gates passed. `sworn doctor` runs cleanly with all expected OK/WARN output, exit 0. 12/12 tests pass, `go build ./...` clean.
- **State**: S22 → verified. T4-mcp now has all 4 slices verified (S08a, S08b, S08c, S22). Track ready for `/merge-track T4-mcp`.
