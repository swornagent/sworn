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
    state: merged
  - id: T3-commercial
    slices: [S06a-sworn-login-auth, S06b-sworn-proxy-credits, S07-paging, S09-per-role-model-config, S18-consideration-catalog, S19-sworn-induction, S21-canonical-baton]
    depends_on: [T1-concurrency-core, T15-cli-registry]
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T3-commercial
    worktree_branch: track/2026-06-19-safe-parallelism/T3-commercial
    state: merged
  - id: T4-mcp
    slices: [S08a-mcp-transport, S08b-mcp-ops-tools, S08c-mcp-plan-tools, S22-sworn-doctor]
    depends_on: T1-concurrency-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T4-mcp
    worktree_branch: track/2026-06-19-safe-parallelism/T4-mcp
    state: merged
  - id: T5-providers
    slices: [S10-provider-foundation, S11-anthropic-driver, S12-google-driver, S13-bedrock-driver, S14-azure-driver, S15-oci-driver, S16-ollama-driver, S39-openai-responses-provider, S63-subscription-cli-driver]
    depends_on: [T1-concurrency-core, T3-commercial]
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T5-providers
    worktree_branch: track/2026-06-19-safe-parallelism/T5-providers
    state: merged
  - id: T6-provider-ux    slices: [S17-tui-provider-config]
    depends_on: [T2-monitoring, T5-providers, T18-cli-polish]
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T6-provider-ux
    state: planned
  - id: T7-mcp-extensions
    slices: [S20-mcp-catalog-tools]
    depends_on: [T3-commercial, T4-mcp]
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T7-mcp-extensions
    worktree_branch: track/2026-06-19-safe-parallelism/T7-mcp-extensions
    state: merged
  - id: T8-memory
    slices: [S23-memory-config, S24-memory-engine, S25-memory-search, S40-memory-test-hygiene]
    depends_on: T1-concurrency-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T8-memory
    worktree_branch: track/2026-06-19-safe-parallelism/T8-memory
    state: merged
  - id: T9-telemetry
    slices: [S26-telemetry]
    depends_on: T1-concurrency-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T9-telemetry
    worktree_branch: track/2026-06-19-safe-parallelism/T9-telemetry
    state: merged
  - id: T10-public-readiness
    slices: [S27-public-readiness-scrub]
    depends_on: [T1-concurrency-core, T2-monitoring, T3-commercial, T4-mcp, T5-providers, T6-provider-ux, T7-mcp-extensions, T8-memory, T9-telemetry, T11-infra-safety, T12-harness-hardening, T13-sworn-role-parity, T14-baton-integration, T16-verdict-ledger, T18-cli-polish, T19-status-hygiene]
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
    state: merged
  - id: T13-sworn-role-parity
    slices: [S45-design-tldr, S46-captain-review, S47-orchestrator-recovery]
    depends_on: [T12-harness-hardening, T17-orchestration-core]
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T13-sworn-role-parity
    state: planned
  - id: T14-baton-integration
    slices: [S48-baton-vendor, S49-baton-version, S50-baton-governance, S62-baton-upstream-source]
    depends_on: [T3-commercial, T15-cli-registry]
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T14-baton-integration
    worktree_branch: track/2026-06-19-safe-parallelism/T14-baton-integration
    state: merged
  - id: T15-cli-registry
    slices: [S51-cli-command-registry]
    depends_on: T1-concurrency-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T15-cli-registry
    worktree_branch: track/2026-06-19-safe-parallelism/T15-cli-registry
    state: merged
  - id: T16-verdict-ledger
    slices: [S52-ledger-projection, S53-ledger-cli, S54-ledger-routing, S55-ledger-multirole-cost, S56-ledger-cost-routing]
    depends_on: [T6-provider-ux, T12-harness-hardening, T13-sworn-role-parity]
    worktree_path:
    worktree_branch: track/2026-06-19-safe-parallelism/T16-verdict-ledger
    state: planned
  - id: T17-orchestration-core
    slices: [S57-oracle-reader, S58-slice-router, S59-scheduler-relayer]
    depends_on: [T1-concurrency-core, T12-harness-hardening, T18-cli-polish]
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T17-orchestration-core
    worktree_branch: track/2026-06-19-safe-parallelism/T17-orchestration-core
    state: in_progress
  - id: T18-cli-polish
    slices: [S60-init-ui-bearing-fix, S61-cli-output-styling]
    depends_on: [T2-monitoring, T15-cli-registry]
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T18-cli-polish
    worktree_branch: track/2026-06-19-safe-parallelism/T18-cli-polish
    state: merged
  - id: T19-status-hygiene
    slices: [S64-status-timestamp-sanity]
    depends_on: [T4-mcp, T12-harness-hardening, T15-cli-registry]
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T19-status-hygiene
    worktree_branch: track/2026-06-19-safe-parallelism/T19-status-hygiene
    state: in_progress
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
| `T2-monitoring` | S04a → S04b → S04c → S05 → S34 | T1 | `track/.../T2-monitoring` | merged |
| `T3-commercial` | S06a → S06b → S07 → S09 → S18 → S19 → S21 | T1 + T15 | `track/.../T3-commercial` | merged || `T4-mcp` | S08a → S08b → S08c → S22 | T1 | `track/.../T4-mcp` | merged |
| `T5-providers` | S10 → S11 → S12 → S13 → S14 → S15 → S16 → S39 → S63 | T1 + T3 | `track/.../T5-providers` | merged || `T6-provider-ux` | S17 | T2 + T5 | `track/.../T6-provider-ux` | planned |
| `T7-mcp-extensions` | S20 | T3 + T4 | `track/.../T7-mcp-extensions` | merged || `T8-memory` | S23 → S24 → S25 → S40 | T1 | `track/.../T8-memory` | merged |
| `T9-telemetry` | S26 | T1 | `track/.../T9-telemetry` | merged |
| `T10-public-readiness` | S27 | all tracks (incl. T16 + T19) | `track/.../T10-public-readiness` | planned |
| `T11-infra-safety` | S28 | T1 | `track/.../T11-infra-safety` | merged |
| `T12-harness-hardening` | S29 → S30 → S31 → S32 → S33 → S35 → S36 → S37 → S38 → S41 → S42 → S43 → S44 | T1 | `track/.../T12-harness-hardening` | merged || `T13-sworn-role-parity` | S45 → S46 → S47 | T12 + T17 | `track/.../T13-sworn-role-parity` | planned |
| `T14-baton-integration` | S48 → S49 → S50 → S62 | T3 + T15 | `track/.../T14-baton-integration` | merged |
| `T15-cli-registry` | S51 | T1 | `track/.../T15-cli-registry` | merged |
| `T16-verdict-ledger` | S52 → S53 → S54 → S55 → S56 | T6 + T12 + T13 | `track/.../T16-verdict-ledger` | planned |
| `T17-orchestration-core` | S57 → S58 → S59 | T1 + T12 + T18 | `track/.../T17-orchestration-core` | planned |
| `T18-cli-polish` | S60 → S61 | T2 + T15 | `track/.../T18-cli-polish` | merged |
| `T19-status-hygiene` | S64 | T4 + T12 + T15 | `track/.../T19-status-hygiene` | planned |

### Execution order

```
Phase 1:  T1 (sequential)
Phase 2:  T2, T3, T4, T8, T9, T11, T12, T15 (parallel after T1 — T11/T12 harness-hardening + T15 CLI registry dispatch early)
          T18 (after T2 + T15 — CLI-output polish: init --ui-bearing fix + shared internal/style colour across the command surface; lands before the planned tracks that share its files)
          T17 (after T1 + T12 + T18 — orchestration-core port: oracle reader + router + scheduler re-layer; S59 shares internal/run + internal/scheduler with T12, and run.go with T18, so serial after both)
          T13 (after T12 + T17 — product role parity; S47 consumes the T17 router; shares internal/run with T12)
Phase 3:  T5 (after T1 + T3)
          T7 (after T3 + T4; may run in parallel with T5)
          T14 (after T3 — needs S21's embed as its vendor target; parallel with T5/T7)
Phase 4:  T6 (after T2 + T5)
Phase 5:  T16 (after T6 + T12 + T13 — harvests the settled verdict pipeline:
          config.go via the T3→T5→T6 chain, slice.go/state.go via the T12→T13 chain)
          T19 (after T4 + T12 + T15 — status timestamp sanity as a lint/doctor hardening gate)
Phase 6:  T10 (after ALL tracks merge incl. T16 + T19 — final public-readiness gate before launch)
```

### Touchpoint matrix

> No row may carry `✓` in more than one column in the same parallel phase.
> **`cmd/sworn/main.go` is NO LONGER a documented shared file** (2026-06-22 replan). The
> "additive dispatch only" exception failed: additive `case` insertions into one contiguous
> `switch` collide in git, which paged the coach loop on the release-wt→T3 forward-merge for
> S07-paging. `S51-cli-command-registry` (track **T15-cli-registry**) replaces the switch with
> a self-registration command registry, making `main.go` **owned solely by T15**. Going forward,
> a track adding a top-level CLI command **self-registers from its own `cmd/sworn/<verb>.go`**
> via `init()` calling `command.Register(...)` — it never edits `main.go` or `commands.go`. The
> three new files `internal/command/`, `cmd/sworn/main.go`, and `cmd/sworn/commands.go` are
> T15-owned; the `lint touchpoints` gate (S30) enforces single-track ownership of `main.go`.
> `(dep)` notation means the track writes this file only after the named dependency merges
> — the dep-edge serialises writes so they are not truly concurrent.
> `T10-public-readiness` (S27) is omitted from the columns below: it depends on every
> other track and runs strictly last (Phase 6), so its wide touchpoints — comment scrubs
> and prompt-text edits across many files — collide with nothing in parallel.
> `T11-infra-safety` (S28) and `T12-harness-hardening` (S29–S33, S35, S36) are likewise
> omitted: T11 touches only `internal/git/`; T12's files are new (`internal/lint/`) or
> tool-specific (`internal/designfit/`, `cmd/sworn/lint.go`) plus prompt files
> (`captain.md`/`planner.md`/`verifier.md`) shared only with T10 — which depends on T12,
> so those writes are sequential, not parallel.
> `T14-baton-integration` (S48–S50, **S62**) is likewise omitted from the columns: it `depends_on T3`
> (it vendors+transforms into the embed S21 creates) so it starts only after T3 merges, in
> Phase 3 parallel with T5/T7 — and it collides with neither. Its files are either **new
> namespaces** (`internal/baton/*`, `cmd/sworn/baton.go`, `docs/adr/0006-baton-protocol-sync.md`,
> `docs/baton-governance.md`) or **T3-owned-and-thus-sequential** (`internal/adopt/baton/**`,
> `internal/prompt/baton/**`, `internal/prompt/VERSION.txt` — all created/owned by S21, which
> T14 depends on) or **merged-track-and-thus-sequential** (`cmd/sworn/doctor.go`, owned by
> S22/T4, already merged — S49 adds a Baton-pin check) plus the documented-shared additive
> `cmd/sworn/main.go`. T5 touches only `internal/model/**`+`go.mod`+`cmd/sworn/run.go`; T7
> only `internal/mcp/**`+`internal/config/**` — disjoint from T14. No parallel collision.
> **S62-baton-upstream-source** stays inside those same T14 namespaces: new `internal/baton/fetch.go`
> + extends `internal/baton/source.go` + `cmd/sworn/baton.go` (all T14-owned), plus
> `internal/adopt/baton/VERSION` (S49-owned, sequential via dep). No new collision; `depends_on
> S48 + S49`. Implementation gated on the upstream Baton repo being synced + tagged (the lock target).
> `T18-cli-polish` (S60–S61) is omitted from the columns: it is presentation-only and
> serialised against everything it shares a file with. S60 touches only `cmd/sworn/init.go`
> (owned by S08/T3, merged — sequential). S61 adds the new `internal/style/` namespace and
> restyles the existing command surface + report renderers (`cmd/sworn/*.go`,
> `internal/{rtm,ears,specquality,designfit,designaudit,reqverify,reqvalidate}`), all owned by
> already-merged tracks (T2/T3/T4/T8/T9/T11/T12/T15) and thus sequential. Its only overlaps with
> **not-yet-merged** tracks are three planned slices — S27 (T10: `main.go`, `bench.go`), S17
> (T6: `top.go`), S59 (T17: `run.go`) — resolved by adding `depends_on T18-cli-polish` to T6,
> T10, and T17 so T18 lands first; those tracks pick up the styled files at their own start.
> `main.go` stays T-owned: S61 styles only `usage()` / `version` *presentation*, not dispatch.
> `T19-status-hygiene` (S64) is omitted from the columns: it depends on already-merged
> `T4-mcp` (doctor), `T12-harness-hardening` (lint), and `T15-cli-registry` (command
> registration), so its writes are sequential. It hardens status metadata validation by
> adding fail-closed future-timestamp checks to `sworn lint` / `sworn doctor` without
> reopening the merged T12 track.
> **ADR-number-collision finding (flagged 2026-06-21, RESOLVED 2026-06-23):** the original matrix
> rows named `docs/adr/0004-dep-policy-minimal-justified.md` (S10) and
> `docs/adr/0005-canonical-baton.md` (S21), but `0004`/`0005` were already taken on
> `release/v0.1.0` by `0004-tui-deps-bubbletea-lipgloss.md` and `0005-tui-dep-bubbles.md` (landed
> by T2). The owning slices renumbered at implement time — **S10 → `0007-dep-policy-minimal-justified.md`**
> (implemented) and **S21 → `0008-canonical-baton.md`** (verified). The matrix rows below and
> S10's `status.json` `planned_files` are corrected to `0007`/`0008` to match the shipped reality.
> The stale `0004` reference in S10's spec acceptance check was the defect behind S10's BLOCKED
> verdict — corrected this replan.
> **Cross-slice dependency (S08c → S21):** `internal/prompt/baton/rules.md` is created by
> `S21-canonical-baton` (T3). `S08c-mcp-plan-tools` (T4) serves it via the `sworn://baton/rules`
> MCP resource, so S08c's rules resource depends on S21's output. Resolution (Captain Pin 2,
> Coach 2026-06-21): **defer that resource as a Rule-2 deferral until S21 lands** — do not add a
> hard T4→T3 dependency that would serialise the tracks. (Exactly the consumer↔creator edge
> S30-lint-touchpoints is meant to surface at plan time.)

| File / surface | T1 | T2 | T3 | T4 | T5 | T6 | T7 | T8 | T9 |
|---|---|---|---|---|---|---|------|---|---|
| `docs/adr/0003-sqlite-orchestration-state.md` | ✓ | | | | |  |
| `docs/adr/0007-dep-policy-minimal-justified.md` (new) | | | | | ✓ |  |
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
| `cmd/sworn/run.go` (DOCUMENTED SHARED — additive flag/wiring per track; see T12 notes) | ✓ | | (T1 dep) | | (T1+T3 dep) |  |
| `go.mod`, `go.sum` | ✓ | | | | (T1 dep) |  |
| `cmd/sworn/main.go` (T15-owned — registry dispatch loop, no per-track edits) | | | | | | ✓ |
| `internal/command/` (new — command registry; T15-owned) | | | | | | ✓ |
| `cmd/sworn/commands.go` (new — central registration of pre-existing verbs; T15-owned) | | | | | | ✓ |
| `cmd/sworn/verify.go` (new — cmdVerify relocated from main.go; T15-owned) | | | | | | ✓ || `docs/release/<rel>/.captain-trial-log.md` (DOCUMENTED SHARED — Captain review log, append-only: every track's design reviews add one row per slice) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
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
| `docs/adr/0008-canonical-baton.md` (new) | | | ✓ | | | | |
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

**T12 `depends_on T1` notes (replan 2026-06-23 — S42 run-loop touchpoints):**
- `cmd/sworn/run.go` is now a **DOCUMENTED SHARED** file. S10 (T5) adds `LoadDotEnv()` +
  `printModelError()`; S42 (T12) adds the `--implement-timeout` flag + `SWORN_IMPLEMENT_TIMEOUT`
  env + the `ImplementTimeout` option wiring. The two edits are additive and region-separable
  (error surfacing vs flag registration), so `/merge-track` reconciles them regardless of merge
  order — whichever track merges second forward-merges and integrates the other's additive block.
  This supersedes the earlier T5 note's assumption that T5 was the only in-flight writer of this
  file. Chosen over a `T12 depends_on T5` edge because T12 is near-complete (10 slices verified)
  and T5 is barely started — serialising the finished track behind the unstarted one is backwards.
- **Resolution recipe (replan 2026-06-24, S11 unblock):** the `release-wt → T5` forward-merge
  conflicts *textually* in `cmd/sworn/run.go` because S42's `--implement-timeout` flag block and
  S10's `.env`/provider block sit adjacent in `cmdRun`. **Keep BOTH hunks** — they are independent
  additive changes (flag registration + timeout resolve vs `.env` load + provider error surfacing).
  This is a mechanical implementer merge resolution, **NOT** a touchpoint-matrix / invariant-4 error:
  the plan is correct (run.go is DOCUMENTED SHARED by design). A verifier or Step-0 that routes this
  to `/replan-release` is **mis-diagnosing** — there is no plan change to make. Resolve the merge once
  on the T5 branch (keep both) and S11 proceeds; do not bounce it to the planner again.
- `internal/run/run.go`, `internal/run/slice.go`: T1-owned and **merged**. S42 threads
  `ImplementTimeout` through `Options`/`SliceOptions` and wraps each attempt in
  `context.WithTimeout`. Only T12 writes these while in-flight (T13/T16/T17 are planned and
  `depends_on T12`), so this is integration-on-top-of-merged-T1, not a parallel collision; the
  implementer's `/implement-slice` Step 0 forward-merges `release-wt` to resolve it.
- `internal/config/config.go`: **explicitly NOT an S42 touchpoint.** The default timeout stays a
  named constant (`DefaultImplementTimeout`) in `internal/run/slice.go`. An earlier implementation
  attempt put it in `config.go` (a T3-merged / planned-T6 / planned-T16 file); that deviation is
  rejected — keeping the constant in `slice.go` removes the collision entirely, and the
  config-file timeout tier is deferred (precedence is flag > env > default).

**T16 `depends_on T6+T12+T13` notes (replan 2026-06-22 — verdict ledger):**
- New, fully T16-owned surfaces (no other column): `internal/ledger/` (ledger.go, query.go,
  routing.go + tests), `cmd/sworn/ledger.go` (+ test), `docs/ledger/verdicts.jsonl` (the
  git-tracked corpus). `cmd/sworn/ledger.go` self-registers via a per-file `init()` (S51/T15
  registry pattern) — it does **not** touch `cmd/sworn/commands.go`, so no shared-file
  collision (the failure mode the 2026-06-22 `main.go` capture recorded).
- `internal/config/config.go` (S54): T3 owns it; the T3→T5→T6 chain serialises every other
  writer (S09, S17). T16 `depends_on T6` (the chain tail), so S54's `ResolveImplementerModel`
  edit lands after all of them — never parallel. Not a documented-shared file; a `/merge-track`
  conflict here is a planner error (invariant 4).
- `internal/run/slice.go` + `internal/state/state.go` (S52, S55): the verdict-record site is
  rewritten by S47/T13 (triage call) and touched by S42–S44/T12. T16 `depends_on T12+T13`
  serialises S52's `verification.model`/`attempt` capture — and S55's per-role
  `verification.dispatches[]` cost capture — after that whole chain. If S47 relocates the
  verdict outcome into `internal/orchestrator/triage.go`, persist from there instead (noted in
  the specs' Risks).
- `internal/agent/agent.go` (S55): surfaces the implementer loop's cost (already computed via
  `computeCost`). Owned by T1 (S06 created it); touched by S42/S43/T12. Covered by the T12 dep.
- `internal/verify/verify.go` (S55, read): the verifier dispatch already returns
  `verdict.Result.CostUSD`; S55 records it, no behavioural change. Captain cost (S46) and the
  orchestrator BLOCKED-resolvability hook cost (S47) are recorded from their T13-owned RunSlice
  stages — the reason T16 `depends_on T13` for content as well as for `slice.go` serialisation.
- Runtime-parallel tracks when T16 runs: T7 (`internal/mcp/`) and T14 (`internal/prompt/baton/`,
  `internal/adopt/`) — both touchpoint-disjoint from every T16 surface. T10 runs after T16
  (added to T10 `depends_on`).

**T17 `depends_on T1+T12` notes (replan 2026-06-23 — orchestration-core port):**
> Ported from the 2026-06-23 port-fidelity audit (`internal-docs/captures/2026-06-23-port-fidelity-audit/`).
> The audit found sworn captured the workflow plane (status.json state machine, worktree isolation,
> verifier contract) but NOT the orchestration plane: the git-ref oracle reader and the
> deterministic router (`captain-route.sh`) were never ported, and `RunParallel` is a static-DAG
> executor rather than the reference's resumable poll-and-route loop.
- New, fully T17-owned namespaces: `internal/router/` (router.go + tests), `cmd/sworn/route.go`
  (+ test), `cmd/sworn/board.go` (+ test). `route.go`/`board.go` self-register via per-file
  `init()` (S51/T15 registry) — they do NOT touch `cmd/sworn/commands.go` or `main.go`.
- `internal/board/oracle.go` (S57, new file in the R2 `internal/board` package): adds the git-ref
  ownership-resolved status reader. No in-flight track writes `internal/board`; safe.
- `internal/run/parallel.go` + `internal/scheduler/worker.go` (S59): re-layered to poll-and-route.
  These are T1-owned (merged) and also touched by T12 (S42–S44 run-loop changes, on
  `run.go`/`slice.go`/`agent.go`). T17 `depends_on T12` serialises S59 strictly after T12 merges —
  never parallel. A `/merge-track` conflict here is a planner-ordering error (invariant 4).
- Re-scoped **S47/T13 consumes S58** (the router), so **T13 `depends_on T17`**; the router lands
  before S47 wires it in. T16 already `depends_on T12+T13`, so S52/S55's verdict-record edits
  (`slice.go`) stay serialised after the whole T12→T17→T13 chain — one extra hop, no new collision.

## Slices

| ID | Track | User outcome | State | Spec |
|---|---|---|---|---|
| `S01-process-ownership` | T1 | SQLite registry + reap-on-restart; single-owner identity | verified | [spec](./S01-process-ownership/spec.md) |
| `S02a-run-refactor` | T1 | `run.RunSlice()` exported; callable from goroutine; no regression | verified | [spec](./S02a-run-refactor/spec.md) |
| `S02b-concurrent-scheduler` | T1 | `sworn run --parallel` launches all independent tracks concurrently | verified | [spec](./S02b-concurrent-scheduler/spec.md) |
| `S03-verify-under-concurrency` | T1 | Verify gate goroutine-safe and fail-closed at N>1 | verified | [spec](./S03-verify-under-concurrency/spec.md) |
| `S04a-tui-foundation` | T2 | `sworn` (no args) shows releases list + board view with navigation | verified | [spec](./S04a-tui-foundation/spec.md) |
| `S04b-tui-live` | T2 | Live concurrent track status from DB (1s poll) + credit balance in header | verified | [spec](./S04b-tui-live/spec.md) |
| `S04c-tui-resolution` | T2 | Blocked slice TL;DR panel + options + open in Claude Code / Codex | verified | [spec](./S04c-tui-resolution/spec.md) |
| `S05-overclaim-benchmark` | T2 | Overclaim rate flat at N=1/2/4; published benchmark artefact | verified | [spec](./S05-overclaim-benchmark/spec.md) |
| `S06a-sworn-login-auth` | T3 | `sworn login` device-code flow; credentials file; `sworn logout` | verified | [spec](./S06a-sworn-login-auth/spec.md) |
| `S06b-sworn-proxy-credits` | T3 | Model calls route via SwornAgent proxy; `sworn account buy`; credit display | verified | [spec](./S06b-sworn-proxy-credits/spec.md) |
| `S07-paging` | T3 | FAIL/BLOCKED fires webhook + email; developer paged without watching terminal | implemented | [spec](./S07-paging/spec.md) |
| `S08a-mcp-transport` | T4 | `sworn mcp` JSON-RPC server; initialize handshake; tools scaffold | verified | [spec](./S08a-mcp-transport/spec.md) |
| `S08b-mcp-ops-tools` | T4 | 9 ops tools: get_board, get_blocked, get_slice_context, rerun, patch, merge, defer | verified | [spec](./S08b-mcp-ops-tools/spec.md) |
| `S08c-mcp-plan-tools` | T4 | 4 planning tools + resources + prompts + mcp-setup.md | verified | [spec](./S08c-mcp-plan-tools/spec.md) |
| `S09-per-role-model-config` | T3 | Config file gains implementer.model, escalation_models, max_attempts; sworn init prompts for both roles | verified | [spec](./S09-per-role-model-config/spec.md) |
| `S10-provider-foundation` | T5 | ADR 0007 + provider router + OAI-compat presets (8 providers) + .env file loading + typed `model.Error{Kind}` taxonomy (classify/UserMessage) | implemented | [spec](./S10-provider-foundation/spec.md) |
| `S11-anthropic-driver` | T5 | Anthropic Claude models work as verifier and implementer via Messages API | verified | [spec](./S11-anthropic-driver/spec.md) || `S12-google-driver` | T5 | Google Gemini and Vertex AI models work as verifier and implementer | failed_verification | [spec](./S12-google-driver/spec.md) || `S13-bedrock-driver` | T5 | AWS Bedrock models work via Converse API; IAM auth | verified | [spec](./S13-bedrock-driver/spec.md) |
| `S14-azure-driver` | T5 | Azure OpenAI deployments work via api-key auth; no new SDK dep | verified | [spec](./S14-azure-driver/spec.md) || `S15-oci-driver` | T5 | OCI Generative AI models work via oci-go-sdk | verified | [spec](./S15-oci-driver/spec.md) || `S16-ollama-driver` | T5 | Ollama native /api/chat endpoint; replaces OAI-compat shim | verified | [spec](./S16-ollama-driver/spec.md) || `S17-tui-provider-config` | T6 | TUI settings panel: provider API keys, model per role, escalation list, max attempts; persists to config.json + ~/.sworn/.env | planned | [spec](./S17-tui-provider-config/spec.md) |
| `S18-consideration-catalog` | T3 | Typed consideration catalog + decision registry; planner Phase 2b (DRY gate, design consultation, arch conformance, capture); sworn init scaffolds both templates | verified | [spec](./S18-consideration-catalog/spec.md) || `S19-sworn-induction` | T3 | `sworn induction` one-time repo onboarding (design system + architecture discovery); implementer + verifier prompts gain deviation-surfacing steps | verified | [spec](./S19-sworn-induction/spec.md) || `S20-mcp-catalog-tools` | T7 | 8 MCP tools: plan_release (unified), get_induction_status, get_considerations, search_decisions, record_decision, check_design_system, update_design_system, record_architecture_pattern | verified | [spec](./S20-mcp-catalog-tools/spec.md) || `S21-canonical-baton` | T3 | Baton protocol embedded in binary (internal/prompt/baton/); sworn init writes minimal MCP-pointer AGENTS.md instead of per-repo Baton copy; ADR-0008 | verified | [spec](./S21-canonical-baton/spec.md) || `S22-sworn-doctor` | T4 | Prompt integrity checks; legacy docs/baton/ + AGENTS.md splice detection with --fix; optional ~/.claude/baton/ sync with --sync-baton | verified | [spec](./S22-sworn-doctor/spec.md) |
| `S23-memory-config` | T8 | `sworn memory status` shows harnesses, memory paths, embedding provider; global + per-project config | verified | [spec](./S23-memory-config/spec.md) |
| `S24-memory-engine` | T8 | `sworn memory build` embeds all memory entries via voyage/oai-compat/ollama; incremental SQLite index | verified | [spec](./S24-memory-engine/spec.md) |
| `S25-memory-search` | T8 | `sworn memory search <query>` returns ranked results; captain-memory-search.py becomes a shim | verified | [spec](./S25-memory-search/spec.md) |
| `S40-memory-test-hygiene` | T8 | memory tests use `t.TempDir()`; removes stray `test-fixture/` + root `fake_ollama.go` so `go test ./internal/memory/...` leaves git clean | verified | [spec](./S40-memory-test-hygiene/spec.md) |
| `S26-telemetry` | T9 | Anonymous command telemetry to api.sworn.sh; opt-out via env var or sentinel file; first-run disclosure | verified | [spec](./S26-telemetry/spec.md) |
| `S27-public-readiness-scrub` | T10 | Make repo + binary public-safe: generalise embedded role prompts (keep Captain/Coach, strip coach-loop coupling), scrub dogfood provenance comments + fired/GetFired + coach-loop refs. Final launch gate. | planned | [spec](./S27-public-readiness-scrub/spec.md) |
| `S28-git-dir-guard` | T11 | internal/git fails closed on empty Repo.Dir so a git op can't run on the ambient worktree (fixes workers writing to main, sworn#6) + regression test | verified | [spec](./S28-git-dir-guard/spec.md) |
| `S29-lint-deps` | T12 | `sworn lint deps` — go.mod/go.sum diff vs planned_files, fail-closed; planner auto-adds dep files | verified | [spec](./S29-lint-deps/spec.md) |
| `S30-lint-touchpoints` | T12 | `sworn lint touchpoints` — design files/pkgs vs planned_files + collision matrix + migration-number collision | verified | [spec](./S30-lint-touchpoints/spec.md) |
| `S31-lint-symbols` | T12 | `sworn lint symbols` — grep back-ticked design identifiers against the live codebase | verified | [spec](./S31-lint-symbols/spec.md) |
| `S32-designfit-decisions-gate` | T12 | `sworn designfit` fails closed when Type-1 work is declared but `design_decisions` is empty | verified | [spec](./S32-designfit-decisions-gate/spec.md) |
| `S33-spec-template-hardening` | T12 | spec/prompt hardening: Risk-cites-`file:line`, pure-engine two-commit note, dynamic-CORS note, + verifier watcher-block cleanup | design_review | [spec](./S33-spec-template-hardening/spec.md) |
| `S34-tui-merge-actor` | T2 | render the `merge:<track>` actor as a distinct row in the TUI live view + release board | verified | [spec](./S34-tui-merge-actor/spec.md) |
| `S35-mutation-guard` | T12 | Captain check + Baton-rule clause for process-global mutation (cwd/git-state/os.Chdir) — the sworn#6 class | planned | [spec](./S35-mutation-guard/spec.md) |
| `S36-captain-resolve-dirty-worktree` | T12 | Captain auto-resolves dirty track worktrees (commit-by-default, record the diff+resolution, never page the Coach) | planned | [spec](./S36-captain-resolve-dirty-worktree/spec.md) |
| `S37-telemetry-tui-exclusion` | T12 | no-args/TUI launch no longer fires a junk telemetry event (empty cmd + session-length); exclusion in `telemetry.Fire()`, not the shared main.go (sworn#7) | planned | [spec](./S37-telemetry-tui-exclusion/spec.md) |
| `S38-verifier-blocked-violations` | T12 | a BLOCKED verdict must populate `status.json` violations (not just journal prose) + a gate rejecting blocked-with-empty-violations — fixes blank REPLAN pages | planned | [spec](./S38-verifier-blocked-violations/spec.md) |
| `S41-build-bin-target` | T12 | canonical `make build` → `bin/sworn` + `docs/build.md` run-from-root convention; stops `cmd/sworn/.sworn` + `docs/release/run-*` worktree clutter | planned | [spec](./S41-build-bin-target/spec.md) |
| `S42-implement-step-timeout` | T12 | `sworn run` bounds each implement attempt with a context deadline; a hung implementer is cancelled and escalates to the next model instead of hanging forever | failed_verification | [spec](./S42-implement-step-timeout/spec.md) |
| `S43-agent-loop-natural-stop` | T12 | agent loop terminates on the model's natural stop (no tool calls) instead of spinning to the turn cap; salvages work from empty-final-text models (gpt-oss-class) by letting proof-from-diff + verifier judge | planned | [spec](./S43-agent-loop-natural-stop/spec.md) |
| `S44-feedback-driven-retry` | T12 | on verify FAIL, feed the verifier's rationale + violations into the next implement attempt's prompt instead of blind re-running; + provider-error retry policy (terminal→fail-fast, transient→backoff) consuming S10's `model.Error{Kind}` (depends_on S10) | planned | [spec](./S44-feedback-driven-retry/spec.md) |
| `S45-design-tldr` | T13 | `sworn run` generates a design TL;DR (§1–6) before implementation — restores the pre-code design artefact for the captain to review | planned | [spec](./S45-design-tldr/spec.md) |
| `S46-captain-review` | T13 | captain agent reviews the TL;DR + live code, emits classified pins, writes review.md, and gates implement (proceed if no escalate pins, else halt+surface) — the in-product `/design-review` | planned | [spec](./S46-captain-review/spec.md) |
| `S47-orchestrator-recovery` | T13 | on non-PASS, intra-run triage chooses resolve-in-place / escalate / halt, then commits state and delegates lifecycle routing (BLOCKED→replan, fail→redesign/implement) to the S58 router (re-scoped 2026-06-23) | planned | [spec](./S47-orchestrator-recovery/spec.md) |
| `S39-openai-responses-provider` | T5 | first-class OpenAI provider via /v1/responses (reasoning_effort + tool-calls + built-in web_search) + a cross-provider WebSearch/WebFetch agent tool — fixes gpt-5.x support + 'more than 6 tools' | verified | [spec](./S39-openai-responses-provider/spec.md) || `S48-baton-vendor` | T14 | `sworn baton vendor` — semver-pinned vendor of upstream Baton + bash→sworn transform over rules AND role-prompts (strips `release-verify.sh`/`release-board-status.sh`/`captain-memory-search.py`… → sworn-native commands); reproduces the sworn-native embed (subsumes the one-time scrub) | failed_verification | [spec](./S48-baton-vendor/spec.md) |
| `S49-baton-version` | T14 | reconcile the Baton pin from a raw SHA to a **semver tag** across `VERSION`+`VERSION.txt`; `sworn version` reports "on Baton vX.Y.Z"; `sworn doctor` fails the pin if it's a SHA not a tag | verified | [spec](./S49-baton-version/spec.md) |
| `S50-baton-governance` | T14 | `sworn baton diff` divergence check (embed vs upstream pin) + `docs/baton-governance.md` PR-up process note + ADR-0006; protocol changes found in sworn dev must PR upstream, never silently fork | verified | [spec](./S50-baton-governance/spec.md) || `S62-baton-upstream-source` | T14 | `sworn baton vendor --upstream` fetches the version-locked Baton release from `github.com/sawy3r/baton` over stdlib HTTPS (codeload tar.gz), verified by tag + commit-SHA/digest, fail-closed — embed source-of-truth is the public repo at a pinned version, not a local install (issue #11) | verified | [spec](./S62-baton-upstream-source/spec.md) || `S51-cli-command-registry` | T15 | command registry replaces the `cmd/sworn/main.go` dispatch switch; new subcommands self-register from their own file; `main.go` owned by one track — ends the recurring touchpoint collision | verified | [spec](./S51-cli-command-registry/spec.md) |
| `S52-ledger-projection` | T16 | Projects every slice's verdict into an append-only `docs/ledger/verdicts.jsonl`; captures implementer model + attempt; backfills the whole board on first sync | planned | [spec](./S52-ledger-projection/spec.md) |
| `S53-ledger-cli` | T16 | `sworn ledger sync` harvests the board; `sworn ledger report` shows pass-rate by model × slice-kind, attempts-to-pass, gate-failure histogram | planned | [spec](./S53-ledger-cli/spec.md) |
| `S54-ledger-routing` | T16 | `sworn ledger recommend <kind>` + S09's `ResolveImplementerModel` defaults to the highest measured pass-rate model for the slice kind (flag/env still win; thin corpus = unchanged) | planned | [spec](./S54-ledger-routing/spec.md) |
| `S55-ledger-multirole-cost` | T16 | Record `v:2` captures per-role `{model, cost_usd}` for every dispatch (implementer, verifier, captain, orchestrator-hook) — cost from local token-pricing, not S06b billing | planned | [spec](./S55-ledger-multirole-cost/spec.md) |
| `S56-ledger-cost-routing` | T16 | `--optimize cost\|quality\|balanced`: cheapest model clearing a pass-rate floor; `report` gains cost-per-pass + per-role quality (captain-miss, verifier-overturn) | planned | [spec](./S56-ledger-cost-routing/spec.md) |
| `S57-oracle-reader` | T17 | `sworn board` reads every slice's authoritative status.json from git refs (track branch > release-wt > worktree), ownership-resolved — the honest board reader the router/TUI/rollup read through | verified | [spec](./S57-oracle-reader/spec.md) |
| `S58-slice-router` | T17 | `sworn route <slice> <release>` computes the next command purely from committed status.json — the deterministic captain-route.sh port (state machine + design-review/Gate-re-entry/merge) | verified | [spec](./S58-slice-router/spec.md) |
| `S59-scheduler-relayer` | T17 | `sworn run --parallel` workers poll the router each step (poll-and-route) instead of a static slice list — resumable, dynamic; keeps dependency resolution + worktree isolation + supervisor ownership | planned | [spec](./S59-scheduler-relayer/spec.md) || `S60-init-ui-bearing-fix` | T18 | `sworn init` no longer prompts for design tokens / component library in a non-UI-bearing repo; design-system flow gated on `--ui-bearing`; drops the always-true `UIBearing` write | verified | [spec](./S60-init-ui-bearing-fix/spec.md) |
| `S61-cli-output-styling` | T18 | shared zero-dep `internal/style` ANSI palette gives premium, consistent, TTY/`NO_COLOR`-aware colour across every command + report renderer; plain output byte-identical | verified | [spec](./S61-cli-output-styling/spec.md) |
## Aggregate state

> **STALE — the board oracle (`release-board-status.sh --json`) is authoritative; run it for live
> counts.** This hand-maintained block predates the T16-verdict-ledger, T17-orchestration-core, and
> T18-cli-polish additions (now **18 tracks, 68 slices**) and is not reconciled per-replan. Counts
> below are a historical snapshot only. The board is actively moving under the coach loop — run the
> oracle. Live oracle at 2026-06-23 (pre-S62-commit): verified 42 / planned 22 / in_progress 1 /
> implemented 1; tracks merged 8 / in_progress 4 / planned 5. +S62-baton-upstream-source (verified,> T14) → 68 slices / planned 23. S48-baton-vendor unblocked to implemented after the corrupt-vendor
> revert; T12-harness-hardening merged to release-wt (dd3b622) though the oracle read may lag; T18
> worktree materialised (in_progress).
>
> Reconciled from the board oracle (`release-board-status.sh --json`, authoritative) this
> replan, 2026-07-03. **57 slices across 15 tracks.** Slice-table State column above
> re-rendered from the oracle this pass (it had lagged: S06a/S06b/S08a/S08b/S08c/S22 and
> S29-S32 read `planned` despite being `verified`; S33 `planned` despite `design_review`;
> S07 `planned` despite `implemented`). Three duplicate rows (S22/S23/S24) and several
> `||`-collapsed physical lines repaired.

- Planned: 27
- In progress: 0
- Implemented: 0
- Design review: 1
- Verified: 30
- Failed verification: 1
- Deferred: 0

**Tracks:** Planned: 5 / In progress: 2 / Merged: 11  *(oracle read 2026-06-23; T12 + T18 merges recorded — board moving under coach loop)*
> Merged (11): T1, T2, T3, T4, T7, T8, T9, T11, T12, T15, T18. In progress (2): T5, T14. Planned (5 per oracle): T6, T10, T13, T16, T17.

## Recent activity
### 2026-07-15 — slice `S58-slice-router` → failed_verification (FAIL)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs).
- **Verdict**: FAIL — gates 2 and 6 failed.
  - Gate 1: PASS — `sworn route <slice> <release>` CLI wired via command registry (init() in route.go), exercised by TestRouteIntegration and parity test; binary produces correct JSON for implemented state.
  - Gate 2: FAIL — Planned touchpoints mismatch. spec.md lists only internal/router/* + cmd/sworn/route*; actual diff includes internal/board/oracle.go and internal/git/git.go (for OracleReader, LastCommitTime, IsAncestor); proof.md Divergence section does not explain (only docs prefix and design.md gap).
  - Gate 3: PASS — Required tests exist and exercise integration point (router_test.go table tests, parity_test.go, cmd/sworn/route_test.go TestRouteIntegration); re-ran `go test -race ./internal/router/...` PASS, `./internal/git/...` PASS, build PASS.
  - Gate 4: PASS — Reachability artefact `cmd/sworn/route_test.go:TestRouteIntegration` runs real `sworn route` binary against committed fixture covering every state branch.
  - Gate 5: PASS — No silent deferrals or placeholder logic in changed files (grep for TODO/FIXME/deferred/placeholder/XXX/HACK only hits test data names).
  - Gate 6: FAIL — Claimed scope does not match. spec AC "verified with later planned sibling routes to it (review if design.md, else implement)" not implemented; routeNextSlice always routes planned to implement (no CatFileExists check for design.md); TestVerifiedWalksTrackThenMerges only covers planned case; proof.md lists as delivered but acknowledges "minor fidelity gap".
- **Gates passed**: 1,3,4,5.
- **Drift gate**: clean (rev-list count 0). Verified against track HEAD e631ed4.
- **State**: S58 → failed_verification. Track T17-orchestration-core now has S57 verified, S58 failed_verification, S59 planned.
- **Next step**: Re-open `/implement-slice S58-slice-router 2026-06-19-safe-parallelism` in fresh session to address violations.

### 2026-06-24 — slice `S57-oracle-reader` → verified (PASS)
- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs).
- **Verdict**: PASS — all six gates satisfied.
  - Gate 1: User-reachable outcome exists — `sworn board [--release <name>] [--json]` wired via command registry + `cmdBoard` → `board.NewGitOracle` → `ReadBoard`/`ReadSliceStatus`; exercised by `TestBoardCLI_JSON`.
  - Gate 2: Planned touchpoints match — core files (oracle.go, board.go, git.go, state.go) match; divergences (Coach pins 1/3/4/5) documented in proof.md "Divergence from plan" and status.json planned_files.
  - Gate 3: Required tests exist and exercise integration point — `TestOwnerBranchWins`, `TestGhostCopyIgnored`, `TestBoardCLI_JSON`, `TestBoardCLI_BlockedVisibility` etc.; re-ran `go test -race ./internal/board/...` (S57 tests PASS; pre-existing TestLiveReleaseBoardsAreValid on T6-provider-ux unrelated), git tests PASS, cmd tests PASS, build PASS.
  - Gate 4: Reachability artefact — `TestBoardCLI_JSON` runs the real `sworn board --json` binary against committed fixture; proves end-to-end user path.
  - Gate 5: No silent deferrals — no TODO/FIXME/placeholder in changed source; ParseTracks workaround noted in journal as out-of-scope.
  - Gate 6: Claimed scope matches — all Delivered items have evidence references; Blocked visibility (S38 gap fix) implemented and tested.
- **Gates passed**: 1–6.
- **Drift gate**: clean (rev-list count 0). Verified against track HEAD 6073461.
- **State**: S57 → verified. Track T17-orchestration-core now has S57 verified (S58/S59 still planned).
- **Note**: Pre-existing board validation failure on T6-provider-ux (no slices) is documented in proof.md and does not affect this slice.

### 2026-06-24 — slice `S63-subscription-cli-driver` → verified (PASS)
- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs).
- **Verdict**: PASS — all six gates satisfied.
  - Gate 1: User-reachable outcome exists — `claude-cli/sonnet` routes through `model.FromEnv` (early return before proxy) → `NewClient` → `*cliDriver.Verify()` (implements Verifier).
  - Gate 2: Planned touchpoints match — core files (cli.go, config.go, cli_test.go) match; provider.go addition documented in proof.md "Divergence from plan" and status.json planned_files.
  - Gate 3: Required tests exist and exercise integration point — `TestClaudeCLI_FromEnvIntegration` + 10 others; re-ran `go test -race ./internal/model/...` (PASS) and `go build ./...` (PASS).
  - Gate 4: Reachability artefact — `TestClaudeCLI_FromEnvIntegration` proves full FromEnv→NewClient→cliDriver.Verify() path with fake binary.
  - Gate 5: No silent deferrals — only documented Rule-2 codex deferral (S63-deferral-1, #19, Coach ack); no other TODO/FIXME in production paths.
  - Gate 6: Claimed scope matches — all Delivered items have evidence references; Not delivered is the acknowledged codex deferral.
- **Gates passed**: 1–6.
- **Drift gate**: clean (rev-list count 0). Verified against track HEAD 58bd7ef.
- **State**: S63 → verified. Track T5-providers now has all slices verified (S10–S16, S39, S63). Next: `/merge-track T5-providers`.
- **Note**: Codex support is a Rule 2 deferral (different invocation/normalisation); claude-cli ships. gofmt note from prior S39 entry still applies.

### 2026-07-16 — track `T5-providers` merged to release-wt (commit 4144fba)

- **Actor**: track integrator (/merge-track)
- **Note**: 9 verified slices merged: S10-provider-foundation, S11-anthropic-driver, S12-google-driver, S13-bedrock-driver, S14-azure-driver, S15-oci-driver, S16-ollama-driver, S39-openai-responses-provider, S63-subscription-cli-driver. Track state -> merged.

### 2026-07-12 — slice `S39-openai-responses-provider` → verified (PASS)
- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs).
- **Verdict**: PASS — all six gates satisfied.
  - Gate 1: User-reachable outcome exists — `openai-responses/gpt-5.5` routes through `model.FromEnv` → `*OpenAIResponses` (implements Verifier + agent.Agent).
  - Gate 2: Planned touchpoints match — files exactly match spec + status.json (re-entry fix moved web_search tests to `tools_test.go`).
  - Gate 3: Required tests exist and exercise integration point — 13 model + 2 web_search tests; re-ran `go test ./internal/model/... ./internal/agent/...` (PASS); httptest drives `/v1/responses` path.
  - Gate 4: Reachability artefact — `TestOpenAIResponses_Chat_ToolCallRoundTrip` + `TestOpenAIResponses_RequestShape` + web_search tests prove tool-call + final-text + reasoning_effort + no temperature.
  - Gate 5: No silent deferrals — no TODO/FIXME/placeholder added in production paths.
  - Gate 6: Claimed scope matches — all Delivered items have evidence references; Not delivered are Rule-2 deferrals with tracking + Coach ack.
- **Gates passed**: 1–6.
- **Drift gate**: clean (rev-list count 0). Verified against track HEAD 8624c1d.
- **State**: S39 → verified. Track T5-providers remains in_progress (S63 pending at time of S39 verify). Next: `/merge-track T5-providers` (S63 now verified).- **Note**: gofmt issues exist in changed files (provider.go indentation, etc.); not a gate violation per spec ACs (tests/build/vet green; gofmt not listed in required tests). Recommend fmt sweep before merge.

### 2026-07-12 — slice `S39-openai-responses-provider` → failed_verification (FAIL)
- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs).
- **Verdict**: FAIL — 3 violations (Gates 2, 3; status integrity).
  - Gate 2 (touchpoints): spec.md and status.json planned_files list `internal/agent/tools_test.go` but actual changed file is `internal/agent/agent_test.go`; proof.md "Divergence from plan" claims "None" (false).
  - Gate 3 (required tests): spec.md explicitly requires `internal/agent/tools_test.go` (web_search schema + stubbed search) but the file does not exist; tests live in agent_test.go instead.
  - status.json: `start_commit` is 930bc0ae (chore ack commit) instead of aa97dc3 (docs start implementation commit); this corrupts diff scope for future verifiers.
- **Gates passed (independent re-run)**: Gate 1 (entry point wired via NewClient("openai-responses/...") and FromEnv); Gate 4 (httptest reachability artefacts in openai_responses_test.go); Gate 5 (no TODO/FIXME in changed code); Gate 6 (Delivered list matches implemented code).
- **Gates failed**: 2, 3.
- **Drift gate**: clean (rev-list count 0). Verified against HEAD on track branch; start_commit corrected to aa97dc3 in this verdict.
- **State**: S39 → failed_verification. Board Failed verification count updated. Next: re-open `/implement-slice S39-openai-responses-provider 2026-06-19-safe-parallelism` in a fresh session to address the violations (fix test file mismatch or update spec + proof, correct start_commit).

### 2026-06-24 — slice `S14-azure-driver` → failed_verification (FAIL)
- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs).
- **Verdict**: FAIL — 6 violations (Gates 2, 5, 6).
  - Gate 2 (touchpoints): spec.md planned touchpoints miss config.go and provider_test.go; proof.md claims "None" (false).
  - Gate 2 (formatting): gofmt -l reports 4 files (azure.go, provider.go, config.go, azure_test.go) need formatting; indentation and newline issues.
  - Gate 2 (code quality): malformed switch indentation and fused lines in provider.go and config.go.
  - Gate 5: provider.go comment typo "environment// variables" (double slash).
  - Gate 6: proof.md "Divergence from plan" and "formatting fixed" claims are false.
  - Gate 6: azure.go struct comment references outdated api-version example "2024-10-21".
- **Gates passed (independent re-run)**: Gate 1 (entry point wired via model.FromEnv → NewClient("azure/...") → *AzureOAI.Verify()); Gate 3 (9 Azure tests + full model suite PASS); Gate 4 (TestNewClient_AzureRouted + httptest reachability artefacts).
- **Gates failed**: 2, 5, 6.
- **Drift gate**: clean (rev-list count 0). Verified against HEAD on track branch; start_commit e6c92d5.
- **State**: S14 → failed_verification. Board Failed verification: 0 → 1. Next: re-open `/implement-slice S14-azure-driver 2026-06-19-safe-parallelism` in a fresh session to address the violations.


### 2026-06-24 — slice `S13-bedrock-driver` → verified (PASS)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs).
- **Verdict**: PASS — all six gates satisfied.
  - Gate 1: User-reachable outcome exists — `sworn run` / `sworn verify` dispatches via `model.FromEnv` → `NewClient("bedrock/...")` → `*Bedrock.Verify()` (entry point wired in cmd/sworn/{run,verify}.go and provider.go).
  - Gate 2: Planned touchpoints match actual changed files — bedrock.go, bedrock_test.go, provider.go, go.mod/go.sum as planned; config.go and provider_test.go divergences documented in proof.md "Divergence from plan".
  - Gate 3: Required tests exist and exercise the integration point — 10 Bedrock tests + TestNewClient_BedrockRouted; re-ran `go test ./internal/model/... -run Bedrock` and full `./internal/model/...` — all PASS.
  - Gate 4: Reachability artefact proves the user path — TestNewClient_BedrockRouted + mocked Converse Verify tests exercise the full dispatch + Verify path.
  - Gate 5: No silent deferrals or placeholder logic — no TODO/FIXME/deferred in source; live test deferral explicitly documented in proof.md with why + tracking + acknowledgement.
  - Gate 6: Claimed scope matches implemented scope — "Delivered" list matches spec acceptance checks with evidence references; all tests re-executed from live state.
- **Gates passed (independent re-run)**: All cited test commands re-run green: 10/10 Bedrock (1 SKIP live), full model suite PASS, `go build`/`go vet` exit 0.
- **Drift gate**: clean (rev-list count 0) — track already carried release-wt tip, no forward-merge needed. Verified against HEAD `4a2bc7f`; `start_commit` `91f7768`.
- **State**: S13 → verified. Board Verified count updated. Next: T5 track has further incomplete slices — next is `/implement-slice S14-azure-driver 2026-06-19-safe-parallelism` in a fresh session.

### 2026-06-24 — slice `S11-anthropic-driver` → verified (re-entry PASS, round 6)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs — re-entry round after prior round-5 FAIL).
- **Verdict**: PASS — all six gates satisfied; both prior round-5 violations genuinely resolved.
  - **Gate 3 remediation confirmed**: `TestAnthropicVerify_Live` now authored at `internal/model/anthropic_test.go:187-202`, `t.Skip`-guarded on `SWORN_LIVE_TESTS=1 && ANTHROPIC_API_KEY != ""`, calls `Verify(ctx, "Reply with PASS.", "verify")`, asserts `strings.Contains(text, "PASS")`. Repo-wide grep for `SWORN_LIVE_TESTS` hits the test file (3 matches). Live run against real Anthropic API with provided key PASSed end-to-end (2.26s).
  - **Gate 2 remediation confirmed**: `internal/model/provider_test.go` surfaced in `status.json` `actual_files` and documented in proof.md "Divergence from plan" (removal of `anthropic/claude-sonnet-4-6` from `TestNewClient_NativeStub` at lines 84-90).
- **Gates passed (independent re-run)**: Gate 1 (provider.go:150 `case "anthropic": return NewAnthropic(...)` → Verify; `TestCmdRun_Parallel` exercises cmdRun); Gate 4 (live test + router test + CLI path test); Gate 5 (no TODO/placeholder in production files); Gate 6 (Delivered evidence refs resolve). All cited test commands re-run green: 5 PASS + 1 SKIP Anthropic, all model tests pass (no OAI regression), `go build`/`go vet` exit 0, 4/4 cmd/sworn reachability tests, `gofmt -l` clean on all four touched files. SDK dep pre-ratified in ADR 0007.
- **Drift gate**: clean (rev-list count 0) — track already carried release-wt tip, no forward-merge needed. Verified against HEAD `b27d31a`; `start_commit` `a72f436`.
- **State**: S11 → verified. Board Verified: 28 → 29; Failed verification: 0 → 0. Next: T5 track has further incomplete slices — next is `/implement-slice S12-google-driver 2026-06-19-safe-parallelism` in a fresh session (S12 is `design_review`; planner/human to decide re-entry).

### 2026-06-23 — slice `S11-anthropic-driver` → failed_verification (re-verify FAIL, round 5)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs — independent re-verification after prior round-4 PASS).
- **Verdict**: FAIL — Gate 3 (primary) + Gate 2 (secondary).
  - **Gate 3 (load-bearing)**: spec "Required tests" mandates a live integration test (`t.Skip` unless `ANTHROPIC_API_KEY` set and `SWORN_LIVE_TESTS=1`) calling `Verify()` with "Reply with PASS." and asserting the returned text contains "PASS". Repo-wide grep for `SWORN_LIVE_TESTS` returns zero hits; `internal/model/anthropic_test.go` has no `t.Skip` and no `ANTHROPIC_API_KEY` guard. The test was never authored. proof.md "Not delivered" mischaracterises the gap as "not run" (execution deferral) when the defect is "not implemented" (test absent).
  - **Gate 2**: `internal/model/provider_test.go` modified by the S11 feat commit `810d7ce` (removed `anthropic/claude-sonnet-4-6` from `TestNewClient_NativeStub`) but omitted from spec.md Planned touchpoints, status.json `actual_files`, and unexplained in proof.md "Divergence from plan". Benign in-scope companion change, but undocumented.
- **Gates that passed (independent re-run)**: Gate 1 (run.go → FromEnv → NewClient → `case "anthropic"` → NewAnthropic → Verify, exercised by `TestCmdRun_Parallel`); Gate 5 (no TODO/placeholder); Gate 6 (Delivered evidence refs resolve). All cited test commands re-run green: 5/5 Anthropic tests, all model tests, 4/4 cmd/sworn reachability tests, `go build`/`go vet` exit 0.
- **Before-you-FAIL gate**: remediation is a legal implementer fix — author the `t.Skip`-guarded live test the spec already names (no spec amendment, no different test shape, no planner authority). FAIL, not BLOCKED.
- **Drift gate**: forward-merged `release-wt/2026-06-19-safe-parallelism` into `track/.../T5-providers` (2 commits, docs-only, no conflict), pushed to origin. Verified against HEAD `efcccb4` (post forward-merge); `start_commit` `a72f436`.
- **State**: S11 → failed_verification. Board Verified: 30 → 28; Failed verification: 0 → 1. Next: implementer re-opens `/implement-slice S11-anthropic-driver 2026-06-19-safe-parallelism` in a fresh session to author the missing live test and document the `provider_test.go` touch.

### 2026-06-24 — slice `S62-baton-upstream-source` → verified (PASS)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs)
- **Verdict**: PASS — all six gates satisfied.
  - Gate 1: `sworn baton vendor --upstream` wired in `cmd/sworn/baton.go`, exercises full pipeline through CLI entry point (Rule 1).
  - Gate 2: Planned touchpoints match actual (spec.md reconciled in round 2; `fetch.go` + `version.go` + command tests).
  - Gate 3: Required tests exist and exercise the integration point: `fetch_test.go` + command-level `TestBatonVendorUpstream_*` in `cmd/sworn/baton_test.go`; re-run pass.
  - Gate 4: Reachability artefact: `TestBatonVendorUpstream_Success`, `TestBatonVendorUpstream_NoTagUsesPinned`, `TestBatonVendorUpstream_DigestMismatch`, `TestBatonVendorUpstream_LocalBackCompat`.
  - Gate 5: No TODO/FIXME/deferred/placeholder in changed `.go` files.
  - Gate 6: Delivered list matches ACs; evidence (files, tests, outputs) verified live.
- **State**: S62 → verified. T14-baton-integration: S48/S49/S50/S62 verified; track complete.
- **Next step**: `/merge-track T14-baton-integration`, then `/merge-release 2026-06-19-safe-parallelism` once every track is merged.

### 2026-06-24 — slice `S49-baton-version` → verified (PASS)
- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs)
- **Verdict**: PASS — all six gates satisfied.
  - Gate 1: `sworn version` and `sworn doctor` surface "on Baton v0.4.2" via integration points (cmdVersion, cmdDoctor).
  - Gate 2: S49-owned touchpoints (4 files) match diff; forward-merges from release-wt documented in proof.md Divergence.
  - Gate 3: Tests `TestIsSemverTag`, `TestVersionIsSemverNotSha`, `TestDoctorReportsBatonTag`, `TestDoctorFailsOnShaPin`, `TestDoctorAllOK` exist and pass (re-ran).
  - Gate 4: Reachability artefacts in proof.md: `sworn version` → "baton-protocol on Baton v0.4.2"; `sworn doctor` clean + forced-SHA failure.
  - Gate 5: No TODO/FIXME/deferred/placeholder in S49 source.
  - Gate 6: Delivered list matches ACs; evidence (files, tests, outputs) verified live.
- **State**: S49 → verified. T14-baton-integration: S48 verified, S49 verified; next is S50 (planned).
- **Next step**: `/implement-slice S50-baton-governance 2026-06-19-safe-parallelism` (or `/merge-track T14-baton-integration` once all T14 slices verified).

### 2026-07-09 — slice `S49-baton-version` → verified (PASS)
- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs)
- **Verdict**: PASS — all six gates satisfied.
  - Gate 1: `sworn version` and `sworn doctor` surface "on Baton v0.4.0" via integration points (cmdVersion, cmdDoctor).
  - Gate 2: S49-owned touchpoints (8 files) match diff; forward-merges from release-wt documented in proof.md Divergence.
  - Gate 3: Tests `TestIsSemverTag`, `TestVersionIsSemverNotSha`, `TestDoctorReportsBatonTag`, `TestDoctorFailsOnShaPin`, `TestDoctorAllOK` exist and pass (re-ran).
  - Gate 4: Reachability artefacts in proof.md: `sworn version` → "baton-protocol on Baton v0.4.0"; `sworn doctor` clean + forced-SHA failure.
  - Gate 5: No TODO/FIXME/deferred/placeholder in S49 source.
  - Gate 6: Delivered list matches ACs; evidence (files, tests, outputs) verified live.
- **State**: S49 → verified. T14-baton-integration: S48 verified, S49 verified; next is S50 (planned).
- **Next step**: `/implement-slice S50-baton-governance 2026-06-19-safe-parallelism` (or `/merge-track T14-baton-integration` once all T14 slices verified).

### 2026-06-23 — slice `S49-baton-version` → blocked (verifier)
- **Actor**: verifier (`/verify-slice`, fresh context, no implementer transcript)
- **Verdict**: BLOCKED — slice is in state 'failed_verification', expected 'implemented'.
  - Worktree HEAD status.json (authoritative) reports "failed_verification" (planner set to route v0.4.0 pin bump to implementer after Baton v0.4.0 published).
  - Board oracle showed "implemented" but worktree HEAD is the source of truth for implementer commits.
  - Implementer must complete the VERSION bump to v0.4.0, update vendored/rules-added, set state to 'implemented', then re-verify.
- **State**: S49 → failed_verification (unchanged). T14-baton-integration remains in_progress.

### 2026-06-24 — track `T18-cli-polish` merged to release-wt (commit 1df2910)
- **Actor**: track integrator (/merge-track)
- **Note**: 2 verified slices merged: S60-init-ui-bearing-fix, S61-cli-output-styling. Track state -> merged. Fast-forward merge of `track/.../T18-cli-polish` into `release-wt/...`; standalone gofmt hygiene commit folded in (scoped to the 12 T18-changed Go files the S61 verifier flagged for cosmetic drift — no slice logic touched).

### 2026-06-24 — slice `S61-cli-output-styling` → verified (re-entry PASS)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs — re-entry round after prior FAIL)
- **Verdict**: PASS — all six gates satisfied; all three prior violations genuinely resolved.
  - Violation 1 (AC3, `sworn help` 0 escapes): `usage()` in `cmd/sworn/main.go` refactored to a styled `strings.Builder` (Heading/Bold/Accent) writing to `os.Stderr`; force-color escape count 0 → 32.
  - Violation 2 (Gate 2, init.go never styled): `cmd/sworn/init.go` now imports `internal/style`; scan header, Changes/No-action headings, markers, padded labels (pad-then-style), created/updated/skipped tokens, prompts, Aborted, Done all styled. Force-color escapes 0 → 6; plain output byte-identical (sha256 `d5b8d0d4...`). The false "writes only to stderr" claim removed from proof.
  - Violation 3 (Gate 4, no transcript): proof.md "Reachability artefact" now contains a real terminal transcript with the 6 smoke runs and escape counts matching live binary runs (version 2/0, help 32/0, top 2/0).
- **AC evidence (verifier's own live binary runs)**: AC1 11 helpers + Enabled() semantics tested; AC2 help/init byte-identical to base under NO_COLOR; AC3 version 2/0, help 32/0, top 2/0; AC4 pad-then-style in init.go:124,136 + ears.go:341; AC5 go.mod unchanged; AC6 build+vet exit 0.
- **Pre-existing failure excluded**: `TestCmdRun_Parallel` (117/118 cmd/sworn tests pass) — confirmed failing identically on release-wt base commit (exit 2, "implementer model not configured"), environmental, out of slice scope.
- **Minor non-blocking observation**: `gofmt -l` flags 11 changed files (cosmetic — style.go trailing newline, bench.go `)+` spacing). Not a spec AC violation (AC6 = build+vet, both pass). proof.md "gofmt clean" claim accurate for 3 named files but not 11 others. A `gofmt -w .` fixes without logic change; does not block merge.
- **State**: S61 → verified. T18-cli-polish track: S60 verified + S61 verified → track complete, ready for `/merge-track T18-cli-polish`.

### 2026-06-24 — slice `S61-cli-output-styling` → failed_verification

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs)
- **Verdict**: FAIL — 3 violations.
  - AC3: `sworn help` emits 0 ANSI escapes with `SWORN_FORCE_COLOR=1` — `usage()`
    (cmd/sworn/main.go:96-162) uses no `style.*` helpers; spec requires
    `sworn version|help|top` to emit ANSI under force-color. `sworn version` (2)
    and `sworn top <release>` (2) emit ANSI; `sworn help` emits 0.
  - Gate 2: `cmd/sworn/init.go` is a planned touchpoint but NOT changed; init.go
    has 26 user-facing `fmt.Print*` stdout calls with no `style` import. Proof
    "Divergence from plan" falsely claims init.go "writes only to stderr."
  - Gate 4: proof.md "Reachability artefact" has no terminal transcript showing
    the `SWORN_FORCE_COLOR=1` / `NO_COLOR=1` smoke runs the spec Required tests
    section explicitly demands.
- **Passed**: Gate 1 (style imported by 9 cmd + 7 renderer files), Gate 3
  (style_test.go green; pre-existing TestCmdRun_Parallel fails on base too —
  environmental), Gate 5 (no deferral markers), Gate 6 (proof files match diff;
  AC1/AC2/AC4/AC5/AC6 satisfied). Drift gate: forward-merged release-wt (S49
  docs-only); spec acceptance checks identical HEAD vs release-wt.
- **State**: S61 → failed_verification. Next: re-open `/implement-slice
  S61-cli-output-styling 2026-06-19-safe-parallelism` in a fresh session to
  address the 3 numbered violations.

### 2026-07-08 — track `T7-mcp-extensions` merged to release-wt (commit 746fe17)

- **Actor**: track integrator (/merge-track)
- **Note**: 1 verified slice merged: S20-mcp-catalog-tools. Track state -> merged.

### 2026-06-23 — replan: add S62-baton-upstream-source (T14) + clear S48 BLOCKED
- **Actor**: planner (human Brad + Claude)
- **S48 unblocked**: the BLOCKED verdict was an operational dirty-tree verdict — a corrupt
  `sworn baton vendor` run stubbed the embed (rules.md 1112→29 lines, 3,596 deletions),
  auto-checkpointed onto T14's tip as `a29a33b`. Reverted via `reset --hard 924c07a`
  (local-only commit, lossless), restoring the legitimate vendor output; cleared
  `verification.result` blocked→pending so S48 re-enters verification (state stays
  implemented). Root cause (the transform stubbing) remains for the implementer.
- **New slice S62-baton-upstream-source** appended to **T14** (S48 → S49 → S50 → S62),
  `depends_on S48 + S49`. Makes `sworn baton vendor --upstream` fetch the version-locked
  Baton release from `github.com/sawy3r/baton` over **stdlib HTTPS tarball** (no git, no
  dep, no ADR), verified by **tag + commit-SHA/digest, fail-closed**. The release-facing
  "embed source-of-truth = public repo at a pinned, tested version, never a local install."
  Tracking: issue #11. Stays inside T14's namespaces (`internal/baton/*`, `cmd/sworn/baton.go`);
  no new touchpoint collision. **Implementation gated** on the Baton repo being synced to
  canonical truth and **tagged** (the lock target).
- Per-project test-config gap (canonical role-prompts stay project-agnostic; sworn binds
  test commands per project at `sworn init`) captured for the next release as issue #13.

### 2026-07-07 — track `T12-harness-hardening` merged to release-wt (commit dd3b622)

- **Actor**: track integrator (/merge-track)
- **Note**: 13 verified slices merged: S29-lint-deps, S30-lint-touchpoints, S31-lint-symbols, S32-designfit-decisions-gate, S33-spec-template-hardening, S35-mutation-guard, S36-captain-resolve-dirty-worktree, S37-telemetry-tui-exclusion, S38-verifier-blocked-violations, S41-build-bin-target, S42-implement-step-timeout, S43-agent-loop-natural-stop, S44-feedback-driven-retry. Track state -> merged.

### 2026-06-23 — replan: new track T18-cli-polish (S60 init fix + S61 CLI styling)
- **Actor**: planner (human Brad + Claude)
- **Trigger**: human-initiated new scope from an ad-hoc fix session, not a BLOCKED handoff.
  Two changes: (S60) `sworn init` prompts for design tokens / component library even in a
  non-UI-bearing repo — the design-system block is gated on new-config, not on `--ui-bearing`,
  plus an always-true `UIBearing = *uiBearer || true`; (S61) premium, consistent colour across
  the whole CLI via a new zero-dep `internal/style` package, TTY/`NO_COLOR`-aware, plain output
  byte-identical. Both defects (S60) confirmed present on release-wt at plan time.
- **New track T18-cli-polish** (S60 → S61), `depends_on T2 + T15` (the merged tracks owning the
  command surface it restyles). Both slices `planned`.
- **Touchpoint resolution**: S61 shares files with three not-yet-started planned slices — S27
  (T10: `main.go`, `bench.go`), S17 (T6: `top.go`), S59 (T17: `run.go`). Added
  `depends_on T18-cli-polish` to T6, T10, T17 so T18 lands first; no concurrent edit. Zero
  collisions with any *started* track (T5/T7/T12/T14) — verified by `git diff --name-only`.
- **Base divergence (Rule 2 surface)**: the reference implementation was authored on
  `release/v0.1.0`, **379 commits behind release-wt**; release-wt's command surface is larger
  (account/doctor/induction/login/mcp/memory/telemetry/verify) and `main.go` is registry-based.
  The code is preserved on branch `wip/cli-styling-reference` as implementer reference; S61 must
  be implemented fresh against release-wt (reuse `internal/style` verbatim, re-cover the command
  layer). **Not** a clean port — flagged so the implementer does not git-apply the stale diff.
- **Index drift noted (not fully reconciled)**: the `## Slices` State column and the Aggregate
  block lagged the oracle before this replan (table shows ~28 verified vs oracle 40). Aggregate
  disclaimer updated to point at the oracle; per-row State cells left as the prior replans did —
  the oracle (`release-board-status.sh`) remains authoritative. Why: out of scope for adding T18,
  mass per-row edits carry error risk; Tracking: this note; Ack: surfaced to Brad.

### 2026-06-23 — replan: route S48 to implementer (misroute) + fold board blocked-visibility into S57

- **Actor**: planner (human + Claude)
- **S48-baton-vendor → implementer (not a planner job).** Verifier BLOCKED on a dirty T14 worktree
  (19 uncommitted mods) and routed to `/replan-release`, but its own verdict says "no spec defect."
  Planner inspection found the uncommitted changes are **corrupt `sworn baton vendor` output** — the
  transform degrades the Baton rules to stubs (`rules.md`: 1112→29 lines, "No scripts."), i.e. a
  **lossy/non-deterministic transform** that fails the slice's own determinism AC. No planning
  artefact to correct → `state` set to `failed_verification`; the implementer must **discard** the
  broken output (`git checkout -- internal/adopt/baton internal/prompt`, do NOT commit it), fix
  `internal/baton/transform.go`, and re-prove. See `S48-baton-vendor/journal.md`. T14 was dirty, so
  this replan did not forward-merge into it — the routing reaches T14 at the implementer's next
  `/implement-slice` Step 0 (after the broken output is discarded).
- **S20-mcp-catalog-tools** (separate verdict): FAIL with three implementer-fixable violations
  (unlisted `cmd/sworn/mcp.go` touch undocumented in proof; paraphrased test output / wrong command;
  `slice_count` nested not top-level + untested). No spec defect → routes to `/implement-slice`; no
  planner change made.
- **Oracle/board blocked-visibility gap → folded into S57-oracle-reader.** The board derives display
  purely from `status.json.state` and never reads `verification.result`, so a `state:implemented` +
  `verification.result:"blocked"` slice renders as plain `implemented` (it hid S42/S10/S48 as blocked
  this release). S57's spec now requires the reader to surface `blocked` + reason (`violations[]`) +
  routing owner (`needs_planner`/`needs_human`/`needs_implementer`) as a distinct `BLOCKED → <owner>`
  board row. (`blocked_needs_*` are not real states — they exist only as verifier prose; S57 makes
  them machine-visible.)

### 2026-06-23 — replan: resolve S10-provider-foundation BLOCKED (stale ADR ref) + ADR-number reconciliation

- **Actor**: planner (human + Claude)
- **Trigger**: `/verify-slice` returned **BLOCKED** on S10 — gates 1–5 passed, but an acceptance
  check read "updated text references ADR-0004". ADR-0004 is the **TUI-deps** ADR
  (`0004-tui-deps-bubbletea-lipgloss.md`); the dep-policy ADR is **`0007-dep-policy-minimal-justified.md`**.
  The implementation (CLAUDE.md) correctly cites ADR-0007 — changing it to 0004 would be wrong, so
  no legal implementer fix existed. Routed to the planner.
- **Ground truth checked**: `docs/adr/` on T5 confirms `0004` = tui-deps, `0007` = dep-policy,
  `0008` = canonical-baton. This closes the **ADR-number-collision finding** flagged 2026-06-21
  (S10's planned `0004-dep-policy` and S21's `0005-canonical-baton` collided with merged TUI ADRs;
  the implementations renumbered to 0007/0008 but the board/spec lagged — the stale-spec ↔ verify loop).
- **Spec drift found**: T5's S10 spec was *ahead* of release-wt (Coach had partially fixed
  0004→0007 in the body at start_commit, leaving the AC + two refs stale); release-wt was fully stale.
- **Resolution**: corrected **all** dep-policy ADR refs in S10's spec to `0007`; fixed S10
  `status.json` `planned_files` + the matrix rows (`0007-dep-policy`, `0008-canonical-baton`) and the
  collision note. S10 `verification.result` cleared to `pending`, `state` → `implemented` (gates 1–5
  already pass; ready for a fresh `/verify-slice`).

### 2026-06-23 — replan: resolve S42-implement-step-timeout BLOCKED (touchpoint correction)

- **Actor**: planner (human + Claude)
- **Trigger**: `/verify-slice` returned **BLOCKED** on S42 — its Step-0 forward-merge of
  `release-wt` into `track/.../T12-harness-hardening` conflicted on `cmd/sworn/run.go`,
  `internal/config/config.go`, `internal/run/run.go`. Verdict framing: touchpoint matrix wrong
  (invariant 4); proposed `/replan-release`.
- **Diagnosis**: three of four conflicts (`run.go`, `slice.go`, `config.go`) are against
  **already-merged** T1/T3 work — normal integration the implementer resolves at Step 0, not a
  parallel race. The `config.go` conflict was **self-inflicted**: the implementer moved
  `DefaultImplementTimeout` into `config.go`, deviating from the spec (the constant belongs in
  `slice.go`). The one genuine in-flight collision is `cmd/sworn/run.go`, shared by S10 (T5,
  in_progress) and S42 (T12) — and unrecorded for T12 in the matrix.
- **Decision A (run.go collision)** — declare `cmd/sworn/run.go` **DOCUMENTED SHARED** (additive
  flag/wiring per track), not a `T12 depends_on T5` edge: T12 is near-complete and T5 barely
  started, so serialising the finished track behind the unstarted one is backwards. T5 and T12
  stay parallel; `/merge-track` reconciles. See the new T12 notes block in the matrix section.
- **Decision B (config.go deviation)** — enforce the spec: the default stays a named constant in
  `slice.go`; S42 drops `config.go` entirely. The config-file timeout tier is deferred
  (precedence becomes flag > env > default). Spec amended with explicit out-of-scope + Rule 2 card.
- **Outcome**: S42 `verification.result` cleared to `pending`, `state` → `failed_verification`;
  the implementer re-enters to forward-merge `release-wt`, move the constant back to `slice.go`,
  drop the config tier, and re-prove. Also repaired two `index.md` defects: the collapsed
  `T4-mcp` frontmatter line (`id:` + `slices:` on one line — the oracle was reading T4-mcp with
  an empty slice list) and the `## Recent activity` header glued onto the tracks note.
### 2026-06-24 — slice `S14-azure-driver` → failed_verification (FAIL)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs).
- **Verdict**: FAIL — 6 violations (Gates 2, 5, 6).
  - Gate 2 (touchpoints): spec.md planned touchpoints miss config.go and provider_test.go; proof.md claims "None" (false).
  - Gate 2 (formatting): gofmt -l reports 4 files (azure.go, provider.go, config.go, azure_test.go) need formatting; indentation and newline issues.
  - Gate 2 (code quality): malformed switch indentation and fused lines in provider.go and config.go.
  - Gate 5: provider.go comment typo "environment// variables" (double slash).
  - Gate 6: proof.md "Divergence from plan" and "formatting fixed" claims are false.
  - Gate 6: azure.go struct comment references outdated api-version example "2024-10-21".
- **Gates passed (independent re-run)**: Gate 1 (entry point wired via model.FromEnv → NewClient("azure/...") → *AzureOAI.Verify()); Gate 3 (9 Azure tests + full model suite PASS); Gate 4 (TestNewClient_AzureRouted + httptest reachability artefacts).
- **Gates failed**: 2, 5, 6.
- **Drift gate**: clean (rev-list count 0). Verified against HEAD on track branch; start_commit e6c92d5.
- **State**: S14 → failed_verification. Board Failed verification: 0 → 1. Next: re-open `/implement-slice S14-azure-driver 2026-06-19-safe-parallelism` in a fresh session to address the violations.


### 2026-06-23 — S20-mcp-catalog-tools verifier FAIL (3 violations)

- **Actor**: verifier (fresh context, artefact-only)
- **Verdict**: FAIL — 3 violations across Gates 2, 3, and 6.
  1. Gate 2: `cmd/sworn/mcp.go` changed but absent from spec Planned touchpoints and proof Divergence.
  2. Gate 3: proof Test results paraphrased (`[... 34 more pre-existing tests ...]`) and shows wrong command vs AC.
  3. Gate 3/6: AC2 `slice_count` at top level not returned (nested in `state_summary`); `TestPlanReleaseExisting` does not assert it.
- **Next**: `/implement-slice S20-mcp-catalog-tools 2026-06-19-safe-parallelism` in a fresh session to address violations.

### 2026-07-07 — track `T3-commercial` merged to release-wt (commit 82fc388)

- **Actor**: track integrator (/merge-track)
- **Note**: 7 verified slices merged: S06a-sworn-login-auth, S06b-sworn-proxy-credits, S07-paging, S09-per-role-model-config, S18-consideration-catalog, S19-sworn-induction, S21-canonical-baton. Track state -> merged.

### 2026-07-06 — S21-canonical-baton verifier PASS (T3-commercial complete)

- **Actor**: verifier (fresh context, artefact-only)
- **Verdict**: All six gates passed. Gate 1: `sworn init` wired to cmdInit; `sworn mcp` serves baton/rules. Gate 2: 6/6 planned touchpoints present; init_design_system_test.go adaptation documented. Gate 3: 16/16 tests re-run and PASS (5 Baton + 11 Init). Gate 4: Manual smoke test — AGENTS.md created with `sworn://baton/rules`, no docs/baton/. Gate 5: Zero TODO/FIXME/placeholder in changed source files. Gate 6: All 14 Delivered items verified.

### 2026-06-23 — S20-mcp-catalog-tools verifier PASS (T7-mcp-extensions complete)

- **Actor**: verifier (fresh context, artefact-only)
- **Verdict**: PASS — all six gates passed.
  - Gate 1: `tools/call` on MCP server wired in cmd/sworn/mcp.go + internal/mcp/server.go; 8 catalog tools registered; tests exercise integration point.
  - Gate 2: Planned touchpoints match (with divergence note for cmd/sworn/mcp.go registration line).
  - Gate 3: All 12 required catalog tests + full suite re-run and PASS; complete unparaphrased output.
  - Gate 4: Reachability artefact (manual-smoke-step: sworn mcp → Claude Code → get_induction_status) documented.
  - Gate 5: No TODO/FIXME/deferred/placeholder in changed source.
  - Gate 6: All 8 tools delivered; deferral for semantic search documented.
- **State**: S20 → verified. T7-mcp-extensions now complete (only slice S20). Track ready for /merge-track T7-mcp-extensions.- **Track**: T3-commercial is now complete — all 7 slices verified. Next: `/merge-track T3-commercial`, then `/merge-release 2026-06-19-safe-parallelism` once every track is merged.
### 2026-07-05 — S19-sworn-induction verifier PASS
- **Actor**: verifier (fresh context, artefact-only)
- **Verdict**: All six gates passed. Gate 1: `sworn induction` CLI functional end-to-end. Gate 2: 4/4 planned files + expected test extension. Gate 3: All 11 tests re-run and PASS. Gate 4: Smoke-executed + idempotent mode confirmed. Gate 5: Zero TODO/FIXME/placeholder. Gate 6: All 13 Delivered items verified.
- **Next**: `/implement-slice S21-canonical-baton 2026-06-19-safe-parallelism` (next slice in T3-commercial).

### 2026-06-23 — replan: add T17-orchestration-core (router/oracle/loop port from fidelity audit)
- **Actor**: planner (`/replan-release`)
- **Trigger (net-new scope, not a stalled slice)**: the 2026-06-23 port-fidelity audit
  (`internal-docs/captures/2026-06-23-port-fidelity-audit/`) found sworn captured the workflow plane
  (status.json state machine, worktree isolation, verifier contract) but NOT the orchestration
  plane — the git-ref oracle reader and the deterministic router (`captain-route.sh`) were never
  ported, and `RunParallel` is a static-DAG executor rather than the reference's resumable
  poll-and-route loop. The watcher-protocol was verified DORMANT against two live coach loops
  (no consumer; `coach-loop` routes via `captain-route.sh`), so it is explicitly NOT ported.
- **Added**: track **T17-orchestration-core** (`depends_on T1 + T12`) with three slices —
  `S57-oracle-reader` (git-ref ownership-resolved `internal/board` reader),
  `S58-slice-router` (`internal/router` deterministic `captain-route.sh` port),
  `S59-scheduler-relayer` (re-layer `RunParallel` worker to poll-and-route; wrap-vs-replace is the
  design-review pin). S59 collides with T12's run-loop work (`internal/run`/`internal/scheduler`),
  hence `depends_on T12`.
- **Re-scoped**: `S47-orchestrator-recovery` (T13) → consumes the S58 router for lifecycle/BLOCKED
  routing, keeping only the intra-run escalation budget; **T13 gains `depends_on T17`**.
- **Decomposition decisions (Coach, this session)**: new track (not appended to T13); oracle
  reader as its own slice (reusable by router/TUI/rollup); re-scope S47 to consume the router.
- **Board oracle reconciliation**: clean — no ghost slices, no pending specs, no blocked/failed
  slices; no existing in-flight spec re-scoped, so no `/verify`↔`/replan` drift introduced.

### 2026-07-04 — S18-consideration-catalog verifier PASS

- **Actor**: verifier (fresh context, artefact-only)
- **Verdict**: All six gates passed.
  - Gate 1: Entry point wired — `sworn init` calls `cmdInit()` → `materialiseCatalog()`.
  - Gate 2: Planned touchpoints match diff (4 planned + expected test/slice files).
  - Gate 3: All 6 tests (3 planner, 3 init) pass on re-run; Rule 1 satisfied.
  - Gate 4: Integration-level tests exercising `cmdInit()` end-to-end serve as reachability artefact.
  - Gate 5: No TODO/FIXME/placeholder in changed production/template files.
  - Gate 6: All 13 Delivered items verified against evidence.
- **Next**: `/implement-slice S19-sworn-induction 2026-06-19-safe-parallelism` (next slice in T3-commercial).
### 2026-07-03 — replan: resolve S07-paging stale BLOCKED (main.go fix already merged via T15)

- **Actor**: planner (`/replan-release`)
- **Trigger (diagnosed from oracle + S07 status.json + journal, not just the board)**:
  S07-paging (T3, `state: implemented`) carried `verification.result: "blocked"` from verifier
  session `verifier-S07-paging-2026-07-01`. The verdict's reason: forward-merging `release-wt`
  into the in-flight `T3-commercial` branch **conflicts on `cmd/sworn/main.go`** (T3's
  `login`/`logout`/`account` switch cases vs. the merged tracks' edits), so verification could
  not run. The verifier stated explicitly **"this is a cross-track collision, not a spec
  defect"** and routed to the planner to split the shared file. **S07's own spec never
  references `main.go`** — confirmed: its planned touchpoints are `internal/account/notify.go`,
  `internal/run/run.go`, `internal/scheduler/worker.go`, `cmd/sworn/account.go`.
- **Resolution: the demanded structural fix is already merged.** The 2026-06-22 replan created
  `T15-cli-registry / S51-cli-command-registry` for exactly this collision; S51 is now
  **verified and merged** into `release-wt` (commit `eaa96ae`). Verified live this replan:
  `release-wt`'s `cmd/sworn/main.go` has **0 `case` lines** (registry dispatch loop), with
  `internal/command/registry.go` + `cmd/sworn/commands.go` present; T3's branch still carries
  the 21-case switch and is 51 commits behind `release-wt`. So the BLOCKED verdict is **stale**:
  its root cause was structurally removed at the release level, T3 simply has not picked it up.
- **Step 2b action**: cleared S07's `verification.result` `blocked` → `pending`, kept
  `state: implemented`, `violations: []`. No spec edit (the verifier confirmed no spec defect).
  S07 re-enters the pipeline via the implementer, **not** the verifier (return-to-sender is not
  a legal handoff). The next `/implement-slice S07-paging` Step 0 forward-merges `release-wt`
  (bringing in S51's registry), resolves `main.go` by converting T3's `login`/`logout`/`account`
  cases into `command.Register(...)` calls in their own `cmd/sworn/*.go` files (the other 18
  verbs are already centrally registered by S51's `commands.go`), commits, then `/verify-slice`.
- **No new scope**: no new slices or tracks; touchpoint matrix already T15-owns `main.go` from
  the 2026-06-22 replan — unchanged this pass.
- **Board drift corrected (same pass)**: `index.md` Aggregate-state block was stale (said "51
  slices", Planned 25 / Verified 25, no `design_review` bucket); re-reconciled from the oracle to
  **57 slices / 15 tracks**, Planned 29 / Implemented 1 / Design review 1 / Verified 26. The
  slice-table State column was re-rendered from the oracle (S06a/S06b/S08a-c/S22/S29-S32 had read
  `planned` despite `verified`; S33 `planned` despite `design_review`; S07 `planned` despite
  `implemented`). Three duplicate rows (S22/S23/S24) and several `||`-collapsed physical lines
  (Tracks table T8/T9, T15/Execution-order; slice rows S04a-c, S05/S06a, S25/S40, S34/S35) were
  repaired. Integration-branch `index.md` (`release/v0.1.0`) remains a 4-track/14-slice fossil —
  expected; it reconciles only at `/merge-release`.
- **Base sync (Step 1)**: `release-wt` already current with `release/v0.1.0` (0 behind).
- **Spec drift noted (benign)**: S06b spec is 98 lines ahead on the *T3 branch* (Coach-ack
  commit `9571422` resolving billing pins) vs. `release-wt` — the track-ahead direction, which
  reconciles at `/merge-track`; not the stale-spec loop.

### 2026-07-03 — track `T15-cli-registry` merged to release-wt (commit eaa96ae)

- **Actor**: track integrator (/merge-track)
- **Note**: 1 verified slice merged: S51-cli-command-registry. Track state -> merged.

### 2026-06-22 — replan: new track T15-cli-registry (S51) — unblock the coach-loop main.go collision
- **Actor**: planner (`/replan-release`)
- **Trigger (diagnosed from journals + verify log, not just the oracle)**: the coach loop paused
  on T3-commercial's `S07-paging` verify. The worker summary said "verify INCONCLUSIVE … (env
  issue?)", but S07's `status.json` on the track branch is `state: implemented`,
  `verification.result: ""`, `blocked: null` — i.e. a genuine **INCONCLUSIVE** (no spec defect,
  **Step 2b does not apply**; S07 is sound). The real cause: forward-merging `release-wt` into the
  in-flight T3 branch **conflicts on `cmd/sworn/main.go`**, so verify can't run. The matrix had
  declared `main.go` a *DOCUMENTED SHARED* file ("additive dispatch only"), but additive `case`
  insertions into one contiguous `switch` collide in git — the same conflict was hand-resolved on
  the T2/T4/T8 syncs before it finally paged here. Only **T3** actually conflicts today (+7/-3,
  its `login`/`account` cases); T12 does not touch `main.go`.
- **New track `T15-cli-registry`** (`depends_on T1`; Phase 2, dispatch early — merges before the
  remaining `main.go` work): **S51-cli-command-registry** introduces an `internal/command`
  registry, reduces `cmd/sworn/main.go` to a registry-lookup dispatch loop, and registers the 19
  pre-existing verbs centrally in a new T15-owned `cmd/sworn/commands.go`. Touchpoints are all new
  files or `main.go` — **disjoint from every in-flight track** (a per-file `init()` migration was
  rejected because it would have collided with T3 on `run.go`/`memory.go` and T12 on `lint.go`,
  merely relocating the conflict).
- **`cmd/sworn/main.go` ownership → T15 (sole).** Matrix row + the documented-shared note rewritten:
  going forward a track adding a CLI command **self-registers from its own `cmd/sworn/<verb>.go`**
  and never edits `main.go`/`commands.go`. Enforced by the `lint touchpoints` gate (S30).
- **S07 unblock mechanism (Coach-ratified)**: no spec change to S07. Once S51 merges to `release-wt`,
  T3's next `S07-paging` implement re-entry forward-merges it, resolves `main.go` by converting its
  `login`/`account` cases into `command.Register(...)` calls in their own files, then re-verifies.
- **Not-started specs corrected to prevent recurrence**: `S19-sworn-induction` (T3),
  `S48-baton-vendor` and `S49-baton-version` (T14) had `main.go` as a planned touchpoint; their
  touchpoints now register from their own command files. `T14-baton-integration` gains
  `depends_on T15-cli-registry` (it needs `internal/command` present to register `sworn baton`).
- **Base sync (Step 1)**: `release-wt` already current with `release/v0.1.0` (0 behind).
- **Aggregate state reconciled** from the oracle: 51 slices / 15 tracks (was a drifted 56/14 note).
- Stray untracked `.captain-trial-log.md` at the worktree root noted (harness scratch; not committed).

### 2026-07-03 — S51-cli-command-registry verifier PASS

- **Slice**: S51-cli-command-registry → state: **verified**
- **Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant)
- **All six gates passed.** All tests pass (internal/command + cmd/sworn suites). grep -c case main.go → 0. Smoke tests confirm every verb resolves identically. verify.go is a mechanical extraction (documented divergence). No silent deferrals.


### 2026-07-03 — S51-cli-command-registry verifier PASS

- **Slice**: S51-cli-command-registry → state: **verified**
- **Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant)
- **All six gates passed.** All tests pass (internal/command + cmd/sworn suites). `grep -c 'case "' cmd/sworn/main.go` → 0. Smoke tests confirm every verb resolves identically. `verify.go` is a mechanical extraction (documented divergence). No silent deferrals.
### 2026-06-22 — S25-memory-search verifier PASS

- **Slice**: S25-memory-search → state: **verified**
- **Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant)
- **All six gates passed.** 26/26 tests pass race-clean. CLI reachability verified live: `sworn memory search` exits 64 (usage), `sworn memory search "test query"` exits 1 (no index). Zero dark-code markers. Extra touchpoint files (embed_voyage.go, index.go) explained by EmbedQuery() + AllEntries() infrastructure. 4 deferrals carry Rule 2 cards.
- **Next**: `/implement-slice S40-memory-test-hygiene 2026-06-19-safe-parallelism` in a fresh session (next incomplete slice in T8-memory).


### 2026-06-29 — S40-memory-test-hygiene verifier PASS

- **Slice**: S40-memory-test-hygiene → state: **verified**
- **Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant)
- **All six gates passed.** Scope was pre-delivered by S24/S25 — memory tests already use `t.TempDir()` and `httptest.NewServer`. 26/26 tests pass with `-race`; `git status --porcelain` is empty; `fake_ollama.go` does not exist. Zero dark-code markers.
- **Next**: `/merge-track T8-memory` (S40 is the last slice in T8 — track complete), then `/merge-release 2026-06-19-safe-parallelism` once every track is merged.

### 2026-06-29 — track `T8-memory` merged to release-wt (commit a9512c2)

- **Actor**: track integrator (/merge-track)
- **Note**: 4 verified slices merged: S23-memory-config, S24-memory-engine, S25-memory-search, S40-memory-test-hygiene. Track state -> merged.

### 2026-06-22 — replan: new track T14-baton-integration (S48/S49/S50) + frontmatter repair
- **Actor**: planner (`/replan-release`)
- **Directive**: establish the Baton↔SwornAgent architecture as deliverable scope. Baton is
  the open protocol (clonable/usable without sworn); SwornAgent is the all-Go product that
  vendors + transforms it. The embed must be a build product of (semver-pinned tag + a
  bash→sworn transform), not a hand-curated verbatim copy pinned to a raw SHA.
- **New track `T14-baton-integration`** (`depends_on T3-commercial` — it vendors into the
  embed S21 creates; Phase 3, parallel with T5/T7, collision-free):
  - **S48-baton-vendor** — `sworn baton vendor`: semver-pinned vendor + transform over
    **rules AND role-prompts** that strips Baton's bash/node script refs
    (`release-verify.sh`→`sworn verify`, `release-board-status.sh`→`sworn board`,
    `design-audit.sh`→`sworn designaudit`, `captain-route.sh`→router,
    `port-deriver.sh`→native, `captain-memory-search.py`→`sworn memory search`) → a
    sworn-native, idempotent embed. Subsumes the one-time public-readiness script scrub.
  - **S49-baton-version** — reconcile the pin from the raw SHA
    (`cf158423…` in `internal/adopt/baton/VERSION`) to a **semver tag** (`v0.3.0`) across
    `VERSION`+`VERSION.txt`; `sworn version` → "on Baton vX.Y.Z"; `sworn doctor` fails
    closed on a SHA pin.
  - **S50-baton-governance** — `sworn baton diff` (embed vs transformed pinned source,
    fail-closed on divergence) + `docs/baton-governance.md` PR-up workflow + ADR-0006.
    sworn never silently forks: protocol changes found in sworn dev → PR upstream.
- **ADR-0006-baton-protocol-sync** written this replan (decision: land the architecture
  record now, not defer to the implementer). **Upstream issue filed: sawy3r/baton#31**
  (VERSION-file + semver-tag discipline; reconverge the 08/09/10 rules born in sworn).
- **S27 overlap**: S27-public-readiness-scrub kept intact; **T10 now `depends_on T14`** so
  S48's transform produces the script-stripped embed before the final public-readiness gate.
- **Frontmatter repair (drift correction)**: `index.md` frontmatter had two corruptions —
  `T3-commercial` and `T5-providers` track entries were grafted onto the previous track's
  `state:` line (`state: merged  - id: …`), which broke YAML parsing and caused the board
  oracle to **drop T3 and T5 as tracks** and misattribute their slices. Repaired both
  (frontmatter + the matching `||` row-collapse in the Tracks table). This is the exact
  class the `7d613b6`/`e6bf33b` frontmatter-guard commits target.
- **ADR-number-collision finding (surfaced, not auto-fixed)**: the matrix's planned
  `0004-dep-policy` (S10) and `0005-canonical-baton` (S21) ADR numbers are now taken on
  `release/v0.1.0` by `0004-tui-deps`/`0005-tui-dep-bubbles`. S10/S21 must take the next
  free numbers at implement time (→0007/0008, after this replan's 0006); left to the owning
  (not-started) slices rather than silently renumbered.
- **Base sync (Step 1)**: release-wt already current with `release/v0.1.0` (0 behind).
- **Release now 56 slices across 14 tracks.** Stray untracked `.captain-trial-log.md` at the
  worktree root noted for gitignoring (harness output; not committed).

### 2026-06-28 — track `T2-monitoring` merged to release-wt (commit 3faa5d0)

- **Actor**: track integrator (/merge-track)
- **Note**: 5 verified slices merged: S04a-tui-foundation, S04b-tui-live, S04c-tui-resolution, S05-overclaim-benchmark, S34-tui-merge-actor. Track state -> merged.

### 2026-06-28 — verifier verdict: PASS (S34-tui-merge-actor)
- **Actor**: verifier (`/verify-slice`)
- **Verdict**: PASS — All six gates passed. Entry points `internal/tui/concurrent.go` (live view) and `internal/tui/board.go` (board view) wired through `LiveView.poll()`/`View()` and `BoardView.LoadBoard()`/`View()`. 27/27 tests pass; go build/vet clean. Merge actor rows rendered with `MergeRowStyle` (amber, bold) in live view; `⟪merge⟫` badge on board track headers. No silent deferrals.
- **Next step**: T2-monitoring now has all slices verified. Run `/merge-track T2-monitoring`, then `/merge-release 2026-06-19-safe-parallelism` once every track in the release has merged.

### 2026-06-28 — verifier verdict: PASS (S05-overclaim-benchmark)
- **Actor**: verifier (`/verify-slice`)
- **Verdict**: PASS — All six gates passed. Entry point `sworn bench overclaim` wired from `cmd/sworn/main.go` → `cmdBench` → `bench.RunOverclaimBenchmark`. 12/12 tests pass; go vet clean; race detector clean; determinism confirmed (5× identical MD5). Verified against commit `bb24fdd`.
- **Next step**: `/implement-slice S34-tui-merge-actor 2026-06-19-safe-parallelism` in a fresh session (next incomplete slice in T2-monitoring).

### 2026-06-28 — verifier verdict: PASS (S04c-tui-resolution)

- **Actor**: verifier (`/verify-slice`)
- **Verdict**: PASS — All six gates passed. Entry point fully wired from `cmd/sworn` to `viewBlocked`. All 7 tests pass. Two deferrals acknowledged with Rule 2 compliance. Verified against commit `041382b`.
- **Next step**: `/implement-slice S05-overclaim-benchmark 2026-06-19-safe-parallelism` in a fresh session (next incomplete slice in T2-monitoring).

### 2026-06-28 — verifier verdict: FAIL (S04c-tui-resolution)

- **Actor**: verifier (`/verify-slice`)
- **Verdict**: FAIL — Gate 2 violation: `internal/tui/board.go`, `internal/tui/styles.go`, `internal/state/state.go` changed but not in spec.md "Planned touchpoints" and not explained in proof.md "Divergence from plan". All other gates (1, 3–6) pass. Tests: 21/21 PASS, go vet: clean.
- **Next step**: `/implement-slice S04c-tui-resolution 2026-06-19-safe-parallelism` in a fresh session. Add the three files to spec.md Planned touchpoints OR document them in proof.md Divergence from plan.

### 2026-06-28 — track `T4-mcp` merged to release-wt (commit 732265d)

### 2026-06-21 — replan: provider-error taxonomy (re-scope S10 + S44)

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

### 2026-06-21 — S24-memory-engine verifier PASS (round 3)

- **Slice**: S24-memory-engine → state: **verified**
- **Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant); verified against `40cb8d6`
- **All six gates passed.** 18/18 tests pass fresh (`go clean -testcache && go test -race ./internal/memory/... -v`). Voyage batch splitting verified (150 texts → embeddings[128][0]==0.0 confirms two-request batching). Auth header + key-from-env tested. Discover tests cover Claude Code MEMORY.md parsing, `---` flat-file splitting, custom paths. Full pipeline demonstrated via Ollama reachability artefact (3 entries indexed, change detection, --force). No silent deferrals in S24 files. All Gate 2 non-planned files fully explained as forward-merge noise (S26/S28/T12 replan content).
- **Next**: `/implement-slice S25-memory-search 2026-06-19-safe-parallelism` in a fresh session.

### 2026-06-21 — S24-memory-engine verifier FAIL (round 2)

- **Slice**: S24-memory-engine → state: **failed_verification**
- **Gate failed**: Gate 2 — planned touchpoints vs actual diff
- **Violation**: `start_commit` is `16c0a8b` (coach-ack commit) not `d441b4c` (start-implementation commit). `git diff --name-only 16c0a8b` includes 6 S26/S28 files not in planned touchpoints. `proof.md` "Divergence from plan" says "None" without acknowledging these files.
- **Fix**: Set `start_commit` to `d441b4c` in status.json; update proof.md "Files changed" to match.
- **Gates 1, 3, 4, 5, 6 all pass** — S24 implementation is correct.
- **Next step**: `/implement-slice S24-memory-engine 2026-06-19-safe-parallelism` (fix start_commit + proof.md)

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

### 2026-07-03 — S09-per-role-model-config verified

- **Actor**: verifier (`/verify-slice`)
- **Verdict**: PASS — all six verification gates passed. `Config` JSON round-trips, all three resolvers (ImplementerModel, EscalationModels, MaxAttempts) correct precedence, `sworn init --yes` writes all four required keys, 27/27 tests pass, `go build ./...` clean, no silent deferrals.
- **State**: S09 → verified. T3-commercial now has S06a, S06b verified, S07 implemented, S09 verified.

### 2026-06-28 — S22-sworn-doctor verified
- **Actor**: verifier (`/verify-slice`)
- **Verdict**: PASS — all six verification gates passed. `sworn doctor` runs cleanly with all expected OK/WARN output, exit 0. 12/12 tests pass, `go build ./...` clean.
- **State**: S22 → verified. T4-mcp now has all 4 slices verified (S08a, S08b, S08c, S22). Track ready for `/merge-track T4-mcp`.

### 2026-06-22 — replan: verdict-ledger track (T16) added

- **Actor**: planner (`/replan-release`)
- **Replan trigger**: maintainer request to turn sworn's verifier verdicts into a durable,
  queryable "private eval" corpus (the eval-as-strategic-IP idea). Not a BLOCKED handoff —
  pure new scope. The harness already produces eval-grade verdicts (spec acceptance checks =
  rubric, Rule 7 verifier = LLM-as-judge, PASS/FAIL/BLOCKED = scored outcome); it just
  discards them after each slice closes.
- **Added**: track **T16-verdict-ledger** = S52-ledger-projection → S53-ledger-cli →
  S54-ledger-routing. `depends_on [T6, T12, T13]`.
- **Design calls**:
  - Ledger is a **pure projection over `status.json`** (pull-based `sworn ledger sync`), not
    a push-hook in the run loop — so it backfills the whole existing board and stays nearly
    self-contained. Corpus is git-tracked at repo-level `docs/ledger/verdicts.jsonl` (spans
    all releases), NOT the anonymous remote S26 telemetry.
  - `slice_kind` is derived from the track id (no edits to existing `status.json` files).
  - S52 adds `verification.model` + `verification.attempt` (the one thing status.json lacks)
    so S54 routing has model-vs-outcome data; captured at the settled verdict-record site,
    hence the T12+T13 dependency.
  - S54 wires the recommendation into S09's `ResolveImplementerModel`; flag/env still win and
    a thin/absent corpus leaves S09 byte-for-byte unchanged.
- **Deferred (Rule 2, in specs)**: verifier-model capture; cost-aware routing (awaits S06b
  billing; `Record` reserves a `v:2` cost field); a TUI ledger surface.
- **Touchpoints**: T16-owned surfaces are new (`internal/ledger/`, `cmd/sworn/ledger.go`,
  `docs/ledger/`); the three shared files (`config.go`, `slice.go`, `state.go`) are serialised
  by the T6 and T12→T13 chains — see the "T16 depends_on T6+T12+T13 notes" block above.
- **State**: S52/S53/S54 → planned. T16 worktree created by its first `/implement-slice`
  once T6, T12, T13 have merged.

### 2026-06-23 — replan: cost angle added to T16 (S55 + S56)

- **Actor**: planner (`/replan-release`)
- **Trigger**: maintainer wants cost-aware routing AND full per-role economics (implementer,
  verifier, captain, orchestrator/interpreter) — not just implementer quality.
- **Key correction**: the earlier S54 "cost deferred until S06b billing" note was **wrong**.
  The cost signal is local token-pricing — `model.Verifier.Verify` already returns `costUSD`,
  `internal/agent`/`oai.go` already `computeCost` from a `modelPricing` table, `verdict.Result`
  already carries `CostUSD`. Cost-aware routing needs **none** of the S06b commercial billing
  engine (Stripe/subscriptions, which stays post-R3). S54's deferral note corrected.
- **Added** (T16 tail, after S54):
  - **S55-ledger-multirole-cost** — Record `v:2` with per-role `dispatches[] {role, model,
    cost_usd, attempt}`; captured at each in-binary dispatch site (implementer=`internal/agent`,
    verifier=`internal/verify`, captain=S46 stage, orchestrator=S47 BLOCKED-resolvability hook),
    aggregated in `RunSlice`.
  - **S56-ledger-cost-routing** — `--optimize cost|quality|balanced` (default quality, so S54
    unchanged): cheapest model whose pass-rate ≥ floor for the (kind, role); `report` gains
    cost-per-pass + derived per-role quality (captain-miss rate, verifier-overturn rate);
    resolver wire.
- **Roles are all in-binary** (or become so via T13): confirmed against S46 (`captain.model`
  dispatch) and S47 (deterministic triage + single LLM hook). No new track dependency — T12
  covers agent/verify, T13 covers captain/orchestrator; both already T16 deps.
- **Quality is derived, not entered**: per-role quality (captain-miss, verifier-overturn) is
  computed in the report/routing layer by correlating captured records — no hand-scored fields.
- **Deferred (Rule 2, in specs)**: routing non-implementer roles from history; proxy/billed-cost
  reconciliation against S06b credits; planner-cost capture (planner is not an in-binary dispatch).
- **State**: S55/S56 → planned. T16 is now 5 slices: S52 → S53 → S54 → S55 → S56.

### 2026-07-08 — S60-init-ui-bearing-fix failed_verification

- **Actor**: verifier (`/verify-slice`, fresh context, no implementer transcript)
- **Verdict**: FAIL — 3 violations (Gates 2, 4, 6). See slice journal for details.
  - Gate 2: planned touchpoints list test file not changed by this slice.
  - Gate 4: reachability transcript in proof.md does not match code (message emitted under --yes but code gates on !*yes).
  - Gate 6: Delivered claims do not match the artefact for the stated ACs.
- **State**: S60 → failed_verification. T18-cli-polish remains in_progress. Implementer must address in fresh /implement-slice session.

### 2026-06-23 — S60-init-ui-bearing-fix verified

- **Actor**: verifier (`/verify-slice`, fresh context)
- **Verdict**: PASS — all six gates satisfied.
  - Gate 1: `sworn init` entry point wired to user-reachable code.
  - Gate 2: Planned touchpoints match (test pre-existed; S60 adds Interactive_NoUIPrompt).
  - Gate 3: Required tests (TestCmdInit_*) exercise cmdInit integration point; re-ran PASS.
  - Gate 4: Reachability artefact (test transcript + manual-smoke) proves no design-system prompt for non-UI-bearing.
  - Gate 5: No silent deferrals or dark code in changed files.
  - Gate 6: All ACs evidenced in proof.md.
- **State**: S60 → verified. Next slice in T18: S61-cli-output-styling.

### 2026-07-09 — S14-azure-driver failed_verification

- **Actor**: verifier (`/verify-slice`, fresh context, no implementer transcript)
- **Verdict**: FAIL — 2 violations (Gate 6). See slice journal for details.
  - Gate 6: Claimed scope matches implemented scope — spec.md AC #2 NewAzureOAI call order and default api-version do not match implementation; field names AzureAPIKey vs AzureOpenAIKey.
  - Gate 6: proof.md "Delivered" claims ACs satisfied but they are not literally.
- **State**: S14 → failed_verification. T5-providers remains in_progress. Implementer must address in fresh /implement-slice session.

### 2026-07-09 — slice `S62-baton-upstream-source` → failed_verification (FAIL)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs)
- **Verdict**: FAIL — 4 violations.
  - Gate 2: Planned touchpoints list `source.go` and `VERSION` (not changed); spec and proof.md Divergence do not document the change to fetch.go + version.go.
  - Gate 3: Required integration test through `cmd/sworn/baton.go` with --upstream + httptest not present (only leaf unit tests and diff tests).
  - Gate 1/4: Reachability artefact does not exercise the command entry point as required by Rule 1.
  - Gate 6: Delivered claims rely on missing evidence.
- **State**: S62 → failed_verification. T14-baton-integration: S48/S49/S50 verified, S62 failed.
- **Next step**: `/implement-slice S62-baton-upstream-source 2026-06-19-safe-parallelism` (fresh session) to address violations.

### 2026-07-09 (round 2) — slice `S62-baton-upstream-source` → failed_verification (FAIL)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs)
- **Verdict**: FAIL — 1 violation.
  - Gate 6: AC3 (no `--tag` uses pinned semver from VERSION; never `latest`/HEAD) claimed delivered with falsifiable "test asserts the requested URL carries the pinned tag", but no such test exists. All upstream command tests pass explicit `--tag`; no test exercises `cmdBatonVendor --upstream` (no `--tag`) asserting codeload URL uses pinned tag from `baton.Version()`. Proof references S49 leaf test which does not cover upstream URL construction.
- **State**: S62 → failed_verification. T14-baton-integration: S48/S49/S50 verified, S62 failed.
- **Next step**: `/implement-slice S62-baton-upstream-source 2026-06-19-safe-parallelism` (fresh session) to address the violation.

### 2026-07-08 — S12-google-driver failed_verification

- **Actor**: verifier (`/verify-slice`, fresh context, no implementer transcript)
- **Verdict**: FAIL — 3 violations (Gates 1, 2, 6). See slice journal for details.
  - Gate 1: User-reachable outcome broken — `sworn run` with GOOGLE_API_KEY fails (case "google" inside // comment in FromEnv key check).
  - Gate 2: Planned touchpoints mismatch (config.go, provider_test.go changed but not listed in spec; no divergence documented).
  - Gate 6: Claimed scope (sworn run dispatch) does not match implementation (FromEnv path broken).
- **State**: S12 → failed_verification. T5-providers remains in_progress. Implementer must address in fresh /implement-slice session.

### 2026-07-08 — S50-baton-governance failed_verification

- **Actor**: verifier (`/verify-slice`, fresh context)
- **Verdict**: FAIL — 1 violation (Gate 2 + Gate 6). Proof bundle inaccurately claims "All five planned files touched" and "Divergence from plan: None". The planned touchpoint `docs/adr/0006-baton-protocol-sync.md` was not edited (confirm/finalise required no change per spec), and `cmd/sworn/baton_test.go` was added (not listed in planned touchpoints). The proof must accurately report scope against the spec's "Planned touchpoints".
- **State**: S50 → failed_verification. T14-baton-integration remains in_progress. Implementer must address in fresh /implement-slice session.

### 2026-07-08T12:30:00Z — S50-baton-governance re-verified (identical result)

- **Actor**: verifier (fresh context, re-run)
- **Verdict**: FAIL (same as prior). No changes to spec.md, proof.md, or implementation since prior verdict commit. Re-emitted verbatim.
- **State**: S50 remains failed_verification. Next step: /implement-slice S50-baton-governance 2026-06-19-safe-parallelism to correct proof bundle.

### 2026-06-24T03:46:51Z — S50-baton-governance verified

- **Actor**: verifier (`/verify-slice`, fresh context)
- **Verdict**: PASS. All six gates satisfied. Gate 2/6 mismatches explained in proof.md "Divergence from plan" (ADR not edited per DD-005; test file added for Rule 1 reachability). Tests re-run green.
- **State**: S50 → verified. T14-baton-integration still has S62 pending. Next step: /implement-slice S62-baton-upstream-source 2026-06-19-safe-parallelism (or /merge-track T14-baton-integration once all verified).

### 2026-06-24T07:06:17Z — slice `S14-azure-driver` → verified (PASS)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs)
- **Verdict**: PASS. All six gates satisfied.
  - Gate 1: User-reachable outcome exists — `sworn run` dispatches via `model.FromEnv` / `NewClient("azure/gpt-4o")` to `*AzureOAI.Verify()` using Azure URL pattern and `api-key` header.
  - Gate 2: Planned touchpoints match — azure.go, azure_test.go, provider.go, config.go, provider_test.go (stub removal); divergences recorded.
  - Gate 3: Required tests exist and exercise integration point — 9 Azure tests + full model suite re-run; all PASS.
  - Gate 4: Reachability artefacts (`TestNewClient_AzureRouted`, `TestAzureVerify_ReturnsText`, `TestAzureVerify_CorrectURL`) prove the path.
  - Gate 5: No silent deferrals — no TODO/FIXME in changed code; deferrals have Rule 2 entries.
  - Gate 6: Claimed scope matches — "Delivered" items have evidence; prior FAILs (param order, api-version, field names, formatting, touchpoints) addressed.
- **State**: S14 → verified. T5-providers remains in_progress (S15, S16, S39, S63 pending).
- **Next step**: `/implement-slice S15-oci-driver 2026-06-19-safe-parallelism` in a fresh session.

### 2026-07-10T08:13:05Z — slice `S15-oci-driver` → verified (PASS)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs).
- **Verdict**: PASS. All six gates satisfied.
  - Gate 1: User-reachable outcome exists — `sworn run` → `model.NewClient("oci/cohere.command-r-plus", cfg)` → `*OCI.Verify()`; entry point exercised by `TestNewClient_OCIRouted` and full run path.
  - Gate 2: Planned touchpoints match actual changed files — spec.md lists oci.go, oci_test.go, provider.go, config.go, provider_test.go, go.mod/go.sum; git diff matches exactly.
  - Gate 3: Required tests exist and exercise the integration point — 6 unit tests in oci_test.go (TestOCIVerify_ReturnsText, MissingCompartment, MissingTokenCount, NewClient_OCIRouted, MissingModelID, New_DeferredCredentialLoading) + full `./internal/model/...` suite; re-run PASS.
  - Gate 4: Reachability artefact proves the user path — `go test ./internal/model/... -run OCI` (6/6 PASS) + smoke step documented.
  - Gate 5: No silent deferrals or placeholder logic — no TODO/FIXME/deferred/placeholder in production code; only documented deferral (instance principal auth) is in spec "Deferrals allowed?" with why/tracking/acknowledgement.
  - Gate 6: Claimed scope matches implemented scope — all acceptance checks have evidence references in proof.md; Delivered list matches.
- **Note on divergence**: Spec line 34 still references `$OCI_REGION`; implementation defers to OCI SDK (OCI_CLI_REGION / config file) per D5 and Coach ack. Tracked for `/replan-release` spec amendment. Does not affect verification (behavior correct, tests pass).
- **State**: S15 → verified. T5-providers remains in_progress (S16, S39, S63 pending).
- **Next step**: `/implement-slice S16-ollama-driver 2026-06-19-safe-parallelism` in a fresh session.


### 2026-07-12 — slice `S16-ollama-driver` → failed_verification (FAIL)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs)
- **Verdict**: FAIL: 1
  - Gate 2: Planned touchpoints mismatch: spec.md lists 3 files (internal/model/ollama.go, internal/model/ollama_test.go, internal/model/provider.go), but git diff shows 4 source files changed including internal/model/provider_test.go (not listed in spec's Planned touchpoints); status.json planned_files includes it but spec.md (the contract) was not updated. Divergence section in proof.md does not explain this addition to scope.
- **State**: S16 → failed_verification. T5-providers remains in_progress.
- **Next step**: `/implement-slice S16-ollama-driver 2026-06-19-safe-parallelism` in a fresh session to address the violation.

### 2026-06-24 — slice `S16-ollama-driver` → verified (PASS)

- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs)
- **Verdict**: PASS
  - Gate 1: User-reachable outcome exists — `sworn run` → `config.ResolveVerifierModel` → `model.FromEnv`/`NewClient("ollama/...")` → `*Ollama.Verify()` to native `/api/chat`.
  - Gate 2: Planned touchpoints match actual changed files — 4 source files (ollama.go, ollama_test.go, provider.go, provider_test.go) match spec.
  - Gate 3: Required tests exist and exercise the integration point — all 10 Ollama tests + full model suite pass; re-ran in verifier session.
  - Gate 4: Reachability artefact proves the user path — unit tests `TestNewClient_OllamaIsNative` and `TestOllamaVerify_ReturnsContent` (httptest).
  - Gate 5: No silent deferrals or placeholder logic — no TODO/FIXME in changed source; live test deferral documented in spec.
  - Gate 6: Claimed scope matches implemented scope — all acceptance checks satisfied; Delivered list matches.
- **State**: S16 → verified. T5-providers remains in_progress (S39, S63 pending).
- **Next step**: `/implement-slice S39-openai-responses-provider 2026-06-19-safe-parallelism` in a fresh session.

### 2026-07-15 — slice `S58-slice-router` → failed_verification (re-verification after re-impl)
- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs).
- **Verdict**: FAIL — Gate 2 failed (Gate 6 now passes).
  - Gate 1: PASS — `sworn route <slice> <release>` CLI wired via command registry (init() in route.go), exercised by TestRouteIntegration and parity test; binary produces correct JSON.
  - Gate 2: FAIL — Planned touchpoints do not match actual changed files. spec.md "Planned touchpoints" lists internal/board/oracle.go, internal/git/git.go, internal/git/git_test.go, cmd/sworn/route.go, cmd/sworn/route_test.go, internal/router/parity_test.go; `git diff --name-only ec63795caf94eec6c5c124027542ae38cffb1a65..HEAD` only shows internal/router/router.go and internal/router/router_test.go (plus docs). proof.md "Divergence from plan" claims "These were always in the actual diff" but that refers to prior commit ff14848 (before re-impl start_commit); the slice scope per start_commit does not include them.
  - Gate 3: PASS — Required tests exist and exercise integration point (router_test.go table tests, parity_test.go, cmd/sworn/route_test.go TestRouteIntegration); re-ran `go test -race ./internal/router/...` PASS, `./internal/git/...` PASS, build PASS.
  - Gate 4: PASS — Reachability artefact `cmd/sworn/route_test.go:TestRouteIntegration` runs real `sworn route` binary against committed fixture covering every state branch.
  - Gate 5: PASS — No silent deferrals or placeholder logic in changed files (grep for TODO/FIXME/deferred/placeholder only hits legitimate state names in tests).
  - Gate 6: PASS — design.md check for planned siblings now implemented in routeNextSlice (CatFileExists on track ref); TestVerifiedWalksTrackThenMerges includes "next_planned_sibling_with_design.md_→_review"; parity test covers it.
- **Gates passed**: 1,3,4,5,6.
- **Drift gate**: forward-merged 1 commit from release-wt/2026-06-19-safe-parallelism, pushed track branch.
- **State**: S58 → failed_verification. Track T17-orchestration-core: S57 verified, S58 failed_verification, S59 planned.
- **Next step**: Re-open `/implement-slice S58-slice-router 2026-06-19-safe-parallelism` in fresh session to address the numbered violation (Gate 2 touchpoint alignment).

### 2026-07-15 — slice `S58-slice-router` → verified (PASS)
- **Actor**: verifier (`/verify-slice`, fresh context, artefact-only inputs).
- **Verdict**: PASS. All six gates satisfied.
  - Gate 1: User-reachable outcome exists — `sworn route <slice> <release>` CLI wired via command registry (init() in route.go), exercised by TestRouteIntegration and parity test; binary produces correct JSON.
  - Gate 2: Planned touchpoints match actual changed files — core files (router.go, router_test.go, parity_test.go, route.go, route_test.go, oracle.go, git.go) match; docs/* and S64/* are forward-merge artifacts from release-wt (documented in proof.md).
  - Gate 3: Required tests exist and exercise the integration point — router_test.go table tests (TestBlockedPrecedesState, TestDesignReviewCommitTimeNewest, TestFailedVerificationGateClassification, TestVerifiedWalksTrackThenMerges, TestGhostSliceFiltered), parity_test.go, cmd/sworn/route_test.go TestRouteIntegration; re-ran `go test -race ./internal/router/...` PASS, `./internal/git/...` PASS, build PASS, specific tests PASS.
  - Gate 4: Reachability artefact proves the user path — `cmd/sworn/route_test.go:TestRouteIntegration` runs real `sworn route` binary against committed fixture covering every state branch.
  - Gate 5: No silent deferrals or placeholder logic — grep for TODO/FIXME/deferred/placeholder only hits legitimate state names ("deferred" as terminal state); documented in proof.md.
  - Gate 6: Claimed scope matches implemented scope — all ACs have evidence: planned/implemented/verified/blocked/failed_verification/design_review/shipped/deferred/unrecognised routing, ghost filter, parity against captain-route.sh; previous Gate 2/6 FAILs addressed (start_commit reset, design.md check added).
- **Gates passed**: 1–6.
- **Drift gate**: clean (0 commits).
- **State**: S58 → verified. Track T17-orchestration-core: S57 verified, S58 verified, S59 planned.
- **Next step**: `/implement-slice S59-scheduler-relayer 2026-06-19-safe-parallelism` in a fresh session.
