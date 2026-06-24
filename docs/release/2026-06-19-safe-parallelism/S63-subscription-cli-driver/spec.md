---
title: 'S63-subscription-cli-driver — run roles via a Claude Code / Codex subscription (no API key)'
description: 'A subprocess model driver that dispatches a role by spawning `claude -p` (Claude Code) or `codex exec` (Codex), which authenticate via the user''s logged-in subscription — so sworn runs with NO SWORN_*_API_KEY. Mirrors the reference coach-loop `claude-cli` driver. The open-core on-ramp: use the subscription you already pay for. depends_on S10-provider-foundation (the driver interface + typed model.Error). See ADR-0007 (dep policy) and internal-docs/decisions/2026-06-24-sworn-orchestration-surfaces-and-subscription-drivers.md.'
---

# Slice: `S63-subscription-cli-driver`

## User outcome

A developer who already pays for **Claude Code** (Claude Pro/Max) or **ChatGPT** (Codex) runs sworn with **no API key set**. sworn dispatches each role (implement / verify / …) by spawning the user's CLI (`claude -p` or `codex exec`), which authenticates through the CLI's own logged-in session. No `SWORN_*_API_KEY`, no provider account — just the subscription they already have. This is the lowest-friction adoption path (open-core on-ramp); the direct-API drivers (S10–S16, S39) remain for users who want provider routing.

## Entry point

- Driver selection in `internal/model` config: a role's driver can be set to `claude-cli[:<model>]` or `codex[:<model>]` (alongside the existing direct-API drivers). The dispatch path (`RunSlice` / the model client factory) selects the subprocess driver — **no edit to `cmd/sworn/main.go`** (selection is config-driven, per S09 per-role config).

## Background

`internal/model/` today is API-key-only (`oai.go`, HTTP + `Bearer SWORN_<PROVIDER>_API_KEY`). The reference `~/.claude/bin/coach-loop` has always run on subscriptions via a `claude-cli` driver that shells out to `claude -p`. Porting that subprocess driver is what lets sworn run with no API key. API-key vs not is a property of the **driver**, not the orchestration surface (CLI/TUI/MCP) — see the design doc.

## In scope

- A **subprocess driver** in `internal/model` implementing the same driver/client interface S10 defines: takes the role prompt, spawns `claude -p <prompt> [--output-format json] [--no-session-persistence]` (or `codex exec`), captures stdout, normalises to the standard result shape.
- **Driver config**: selectable per role as `claude-cli[:<model>]` / `codex[:<model>]`; requires **no** API key for that role.
- **Fail-closed availability/auth detection**: if the CLI binary is absent or not logged in, return a typed `model.Error{Kind: ...}` (e.g. `unavailable` / `auth_error`) — never a silent hang. Bound the subprocess with a timeout.

## Out of scope

- The direct-API drivers (S10–S16, S39) — unchanged.
- The orchestration loop / scheduler (T17) — this slice provides a driver the loop *uses*; it does not change the loop.
- Hosted/proxy execution and MCP run-control tools (separate slices).

## Planned touchpoints

- `internal/model/cli.go` (new — the subprocess driver: `claude -p` / `codex exec`)
- `internal/model/config.go` (driver selection: add the `claude-cli` / `codex` cases; no API key required for them)
- `internal/model/cli_test.go` (new — fake CLI binary on `PATH`)

## Acceptance checks

- [ ] With **no `SWORN_*_API_KEY` set** and a (fake) `claude` binary on `PATH`, a role dispatch completes via the subprocess driver — asserted with a fake `claude`/`codex` executable that records its invocation (args include the prompt) and returns a canned result the driver normalises correctly.
- [ ] The driver is selectable via config as `claude-cli[:<model>]` and `codex[:<model>]`, per role (composes with S09 per-role config).
- [ ] A missing or unauthenticated CLI yields a typed `model.Error{Kind}` (e.g. `unavailable`/`auth_error`), surfaced to the caller — not a hang; the call is timeout-bounded.
- [ ] `go test -race ./internal/model/...` passes.

## Design decisions (for the Captain review to ratify)

- Exact `claude -p` invocation (output format, `--no-session-persistence`, model flag) and the `codex exec` equivalent.
- How login state is detected cheaply (probe vs first-call error classification).
- Whether the subprocess driver reuses the runtime-driver contract shape used by the reference `bin/drivers/`.

## Deferrals allowed?

Codex support MAY land as a Rule 2 deferral after `claude-cli` if the two CLIs' invocation/normalisation diverge enough to warrant splitting — declared in `journal.md`, tracked, acknowledged.
