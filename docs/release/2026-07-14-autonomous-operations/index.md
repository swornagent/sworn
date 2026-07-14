---
title: Autonomous operations plane
description: Truthful recoverable loops, durable controls and paging, and a mobile web board.
---

# Autonomous operations plane

## Release summary

- **Tracking:** [#109](https://github.com/swornagent/sworn/issues/109), from architecture review [#108](https://github.com/swornagent/sworn/issues/108)
- **Target / integration branch:** `release/v0.2.0`
- **State:** planned
- **Benefit:** A SwornAgent operator can trust a recoverable autonomous release loop, receive durable attention pages, and monitor or safely control it from a mobile web board.
- **Default release authority:** autonomous through verified release assembly; final integration merge remains human-gated unless a valid standing delegation exists. Production `main` promotion remains separately human-authorized initially.

## Tracks and order

| Track | Slices | Dependency |
|---|---|---|
| `T1-engine-truth` | S01 terminal outcome commit → S02 execution authority → S03 cancellation/recovery | root |
| `T2-control-core` | S04 command/event service → S05 revisioned state ownership → S06 adapter parity | T1 |
| `T3-durable-paging` | S07 notification outbox → S08 webhook/mobile delivery | T2 |
| `T4-mobile-board` | S09 operations read API → S10 responsive board → S11 authenticated controls | T3 |
| `T5-assembled-journey` | S12 complete autonomous operations journey | T4 |

```text
T1-engine-truth
       |
       v
T2-control-core
       |
       v
T3-durable-paging
       |
       v
T4-mobile-board
       |
       v
T5-assembled-journey
```

## Slice outcomes

| Slice | User-reachable outcome | Needs | Contracts |
|---|---|---|---|
| S01-terminal-outcome-commit | Terminal labels cannot outrun status/Git/event persistence | N-01 | C-01, C-03, C-04 |
| S02-execution-authority | The real task/parallel CLI selects one fail-closed engine and honors its flags | N-01 | C-01 |
| S03-cancellation-recovery | Signals and deadlines stop children and restart reconciles interrupted work | N-01 | C-01, C-03 |
| S04-command-event-service | Every operator intent and lifecycle transition has one durable command/event contract | N-02 | C-02, C-03 |
| S05-revisioned-state-ownership | Stale writers conflict; only the loop applies runtime transitions | N-01, N-02 | C-02, C-04 |
| S06-control-adapter-parity | CLI, MCP, and TUI use the same reads, commands, and typed errors | N-02 | C-02, C-03, C-04 |
| S07-notification-outbox | Attention events are durably queued, bounded, retried, and replayable | N-03 | C-03, C-05 |
| S08-webhook-mobile-delivery | Generic webhooks and mobile push deliver stable, safe event projections | N-03 | C-05, C-09–C-11 |
| S09-operations-read-api | One authenticated-capable read/event API exposes authoritative state and health | N-02, N-04 | C-03–C-06, C-08 |
| S10-responsive-web-board | A phone-sized read-only board monitors active loops and delivery health | N-04 | C-06 |
| S11-authenticated-remote-controls | Allowed remote actions are authenticated, idempotent, revision-checked, and audited | N-02, N-04 | C-02–C-04, C-06, C-08, C-12–C-15 |
| S12-autonomous-operations-journey | The real binary proves cold start, retry, pause/control, restart, paging, mobile view, and merge policy | N-01–N-04 | C-01–C-15 |

## Exclusions and sequencing obligations

- The twelve planned slices in `2026-07-11-contract-edge-gates` retain their ownership. Implementers must cross-check touchpoints and must not recreate provider capability, dry-run, max-turn, design-authority, or general guard-fidelity work here.
- `T4` begins after `T3` so the operations API and board consume the real notification-health contract rather than a second placeholder or test-only seam. `S11` cannot begin merely because the board renders; `S09` and `S10` must first prove the read-only boundary.
- `S12` is the release reachability gate and cannot substitute mocks for command construction, process signals, HTTP delivery, browser/mobile layout, or persisted restart state.
