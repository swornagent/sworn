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
    state: in_progress
  - id: T2-monitoring
    slices: [S04a-tui-foundation, S04b-tui-live, S04c-tui-resolution, S05-overclaim-benchmark]
    depends_on: T1-concurrency-core
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T2-monitoring
    state: planned
  - id: T3-commercial
    slices: [S06a-sworn-login-auth, S06b-sworn-proxy-credits, S07-paging, S09-per-role-model-config, S18-consideration-catalog, S19-sworn-induction, S21-canonical-baton]
    depends_on: T1-concurrency-core
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T3-commercial
    state: planned
  - id: T4-mcp
    slices: [S08a-mcp-transport, S08b-mcp-ops-tools, S08c-mcp-plan-tools, S22-sworn-doctor]
    depends_on: T1-concurrency-core
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T4-mcp
    state: planned
  - id: T5-providers
    slices: [S10-provider-foundation, S11-anthropic-driver, S12-google-driver, S13-bedrock-driver, S14-azure-driver, S15-oci-driver, S16-ollama-driver]
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
    slices: [S23-memory-config, S24-memory-engine, S25-memory-search]
    depends_on: T1-concurrency-core
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T8-memory
    state: planned
  - id: T9-telemetry
    slices: [S26-telemetry]
    depends_on: T1-concurrency-core
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T9-telemetry
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
| `T1-concurrency-core` | S01 → S02a → S02b → S03 | — | `track/.../T1-concurrency-core` | in_progress |
| `T2-monitoring` | S04a → S04b → S04c → S05 | T1 | `track/.../T2-monitoring` | planned |
| `T3-commercial` | S06a → S06b → S07 → S09 → S18 → S19 → S21 | T1 | `track/.../T3-commercial` | planned |
| `T4-mcp` | S08a → S08b → S08c → S22 | T1 | `track/.../T4-mcp` | planned |
| `T5-providers` | S10 → S11 → S12 → S13 → S14 → S15 → S16 | T1 + T3 | `track/.../T5-providers` | planned |
| `T6-provider-ux` | S17 | T2 + T5 | `track/.../T6-provider-ux` | planned |
| `T7-mcp-extensions` | S20 | T3 + T4 | `track/.../T7-mcp-extensions` | planned |
| `T8-memory` | S23 → S24 → S25 | T1 | `track/.../T8-memory` | planned |
| `T9-telemetry` | S26 | T1 | `track/.../T9-telemetry` | planned |

### Execution order

```
Phase 1:  T1 (sequential)
Phase 2:  T2, T3, T4, T8, T9 (parallel after T1)
Phase 3:  T5 (after T1 + T3)
          T7 (after T3 + T4; may run in parallel with T5)
Phase 4:  T6 (after T2 + T5)
```

### Touchpoint matrix

> No row may carry `✓` in more than one column in the same parallel phase.
> `cmd/sworn/main.go` is a **documented shared file** (additive dispatch only).
> `(dep)` notation means the track writes this file only after the named dependency merges
> — the dep-edge serialises writes so they are not truly concurrent.

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
| `internal/prompt/baton/` (new) | | | ✓ | | | | |
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
| `S02b-concurrent-scheduler` | T1 | `sworn run --parallel` launches all independent tracks concurrently | design_review | [spec](./S02b-concurrent-scheduler/spec.md) |
| `S03-verify-under-concurrency` | T1 | Verify gate goroutine-safe and fail-closed at N>1 | planned | [spec](./S03-verify-under-concurrency/spec.md) |
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
| `S10-provider-foundation` | T5 | ADR 0004 + provider router + OAI-compat presets (8 providers) + .env file loading | planned | [spec](./S10-provider-foundation/spec.md) |
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
| `S22-sworn-doctor` | T4 | Prompt integrity checks; legacy docs/baton/ + AGENTS.md splice detection with --fix; optional ~/.claude/baton/ sync with --sync-baton | planned | [spec](./S22-sworn-doctor/spec.md) |
| `S23-memory-config` | T8 | `sworn memory status` shows harnesses, memory paths, embedding provider; global + per-project config | planned | [spec](./S23-memory-config/spec.md) |
| `S24-memory-engine` | T8 | `sworn memory build` embeds all memory entries via voyage/oai-compat/ollama; incremental SQLite index | planned | [spec](./S24-memory-engine/spec.md) |
| `S25-memory-search` | T8 | `sworn memory search <query>` returns ranked results; captain-memory-search.py becomes a shim | planned | [spec](./S25-memory-search/spec.md) |
| `S26-telemetry` | T9 | Anonymous command telemetry to api.sworn.sh; opt-out via env var or sentinel file; first-run disclosure | planned | [spec](./S26-telemetry/spec.md) |

## Aggregate state

- Planned: 29
- In progress: 0
- Design review: 1
- Implemented: 0
- Verified: 2
- Failed verification: 0
- Deferred: 0

**Tracks:** Planned: 8 / In progress: 1 / Merged: 0

> Note: T3 now has 7 slices; T4 now has 4 slices; T8 new (3 slices); T9 new (1 slice).

## Recent activity

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
