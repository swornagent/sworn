---
title: S07-run-loop
description: `sworn run` — the end-to-end orchestration: implement → verify → retry/escalate → gated merge.
---

# Slice: `S07-run-loop`

## User outcome

A developer runs `sworn run --task "<what to build>"` and the binary executes the
full loop: spec → implement → verify → (on FAIL: retry / escalate the model up to
N, then surface to the human) → **gated merge on PASS only**. This is the turnkey
E2E payoff.

## Entry point

CLI: `sworn run` (`cmd/sworn/run.go`).

## In scope

- Orchestration (`internal/run/`): sequence implement (S06) → verify (S01/S02);
  on FAIL retry/escalate the implementer model up to N then escalate to the human;
  on PASS perform a **gated merge** (merges only if state == `verified`).
- Fail-closed: unverified work never merges.

## Out of scope

- TUI (`sworn top`); multi-slice planning (v0.1 takes a single task/spec).

## Planned touchpoints

- `internal/run/`, `cmd/sworn/run.go`, `cmd/sworn/main.go` (dispatch — shared)

## Acceptance checks

- [ ] A passing task ends with the change **merged**.
- [ ] A failing task **never merges**; after N retries it escalates to the human.
- [ ] The verdict (PASS/FAIL) drives control flow (verified-gated merge).
- [ ] Retry escalates the model per config (cheap → frontier) on repeated FAIL.

## Required tests

- **Integration**: fake implementer + verifier models scripted for the PASS path
  (→ merged) and the FAIL path (→ not merged, escalated). Assert merge happens
  **only** when state == `verified`.
- **playwright-screenshot** — not used; no visual acceptance checks in this slice.
- **CLI reachability**: `cmd/sworn/run_test.go` exercises `cmdRun` flag parsing
  and error paths through the integration point (`sworn run`).
## Risks

- Infinite retry — hard cap + escalate.
- Merging unverified work — hard gate on `state == verified` (the core invariant).

## Deferrals allowed?

No.
