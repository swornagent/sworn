---
title: 'Release intake: autonomous operations plane'
description: 'Planning record for truthful autonomous loops, durable controls and notifications, and a mobile web board.'
---

# Release Intake: `2026-07-14-autonomous-operations`

## Release goal

Give a SwornAgent operator a trustworthy autonomous release loop that can be
started, monitored, interrupted, recovered, and controlled through one durable
operations plane. Shipped means the real binary proves cold-start-to-release
assembly, every terminal label is backed by persisted effects, webhook/mobile
notifications survive transient delivery failure, and a responsive web board
shows authoritative live state and accepts authenticated conflict-safe controls.

## What the human wants

- N-01: **Truthful autonomous execution.** A maintainer can start a current-format release and trust that PASS, FAIL, BLOCKED, paused, cancelled, and ready-to-merge labels reflect durable state and Git effects, including after interruption and restart.
- N-02: **One operations authority.** CLI, TUI, MCP, and web clients observe the same source ref/revision and submit typed idempotent commands instead of independently rewriting runtime state.
- N-03: **Durable attention delivery.** Terminal and action-required events reach generic webhooks and mobile push through a bounded, replayable outbox whose delivery history is visible.
- N-04: **Mobile operations.** An operator can monitor active loops on a phone and, after authentication, perform explicitly allowed revision-checked controls with a complete audit trail.

## Source of truth

- **Human stakeholder:** repository owner / Coach
- **Tracking epic:** [sworn#109](https://github.com/swornagent/sworn/issues/109)
- **Architecture review:** [sworn#108](https://github.com/swornagent/sworn/issues/108)
- **Review captures:**
  - `docs/captures/2026-07-14-architecture-review-brief.md`
  - `docs/captures/2026-07-14-architecture-review-root-cause.md`
  - `docs/captures/2026-07-14-architecture-review-findings.md`
- **Historical evidence:** predecessor operational behavior was inspected read-only during #108; no predecessor code or private paths are part of this public release contract.

## Users and gestures

- **Loop operator:** starts `sworn loop --parallel --release <name>`, stops it through normal process signals, restarts it, and sees recovery rather than duplicated work or optimistic PASS.
- **Mobile observer:** opens the board from a phone and sees release, track, slice, liveness, blocker, event, and notification-delivery state without editing repository files.
- **Authenticated remote operator:** pauses, resumes, retries, defers, or acknowledges an allowed operation using an expected revision and receives either one durable acceptance or a visible conflict.
- **Automation integrator:** receives stable generic webhook events, uses mobile push links, and can replay failed deliveries without repeating the underlying loop mutation.
- **Release authority:** keeps final production merge human-gated by default or records a scoped standing delegation before automatic integration merge is permitted.

## Current gaps

- The task CLI selects a different execution path from the directly tested task engine and accepts a `--base` value it does not apply (#27).
- The parallel path can lose the committed-state router and historically selected a retired static path instead of failing closed.
- Several non-PASS and track-finalization paths can report a terminal result without proving state/Git persistence.
- The CLI has no process-lifetime signal context and model-requested tools or proof commands can outlive the advertised attempt deadline.
- Runtime MCP/TUI controls perform their own state transitions and can race the loop; MCP rerun uses an obsolete CLI shape.
- Notifications are synchronous, use an unbounded default HTTP client, have no durable attempt/result, and omit several action-required events.
- There is no native web/mobile board or authenticated remote-control boundary.

## Constraints and non-negotiables

- Fail closed: absence of durable outcome evidence is non-PASS.
- Keep one Go binary. Prefer existing dependencies and standard-library HTTP/embed facilities; any new runtime dependency requires an ADR.
- Never expose API keys, bearer tokens, model payloads, request bodies, local worktree paths, or credential values in events, logs, web responses, or notifications.
- The loop owns runtime state transitions. Other processes submit durable commands with stable IDs, expected status hash/revision, and idempotency key.
- State writes are validated and atomically replaced. A stale writer receives a typed conflict; it cannot overwrite a newer verdict.
- Localhost is the default web bind. Non-loopback bind fails closed without explicit authentication configuration; state-changing HTTP requests require authorization, origin/CSRF defense, and audit.
- Mobile UI targets a 360 CSS-pixel viewport, keyboard navigation, visible focus, semantic status text, and WCAG 2.2 AA contrast/touch-target expectations.
- Notification requests use bounded total/per-attempt deadlines, no secret response bodies in errors, durable replay, and idempotent event IDs.
- Final production merge remains human-gated unless an explicit, durable, scoped standing delegation authorizes it. Readiness is not merge success.

## Adjacent and out of scope

- Provider endpoints, local/cloud model dispatch, and live dialect conformance belong to `2026-07-14-local-cloud-providers` / #15.
- Contract lint, provider capability selection, resume-worktree reset, parallel dry-run, loop max-turns, autonomous design authority, general guard fidelity, and mock-code construction remain in `2026-07-11-contract-edge-gates`; this release consumes those contracts and does not duplicate their slices.
- Replacing SQLite, introducing a hosted Sworn control service, multi-tenant SaaS administration, or internet exposure by default is out of scope.
- The read-only TUI may remain; this release adapts its runtime controls and authoritative reads rather than redesigning its visual system.
- Automatic production merge without standing delegation is prohibited, not deferred.

## Decisions

### 2026-07-14 — Build a command/event core before the web board

- **Context:** Existing CLI, MCP, and TUI surfaces answer state and mutation questions independently; a mobile UI would add another writer.
- **Decision:** First deliver a loop-owned durable command/event service and revisioned state repository. All interactive surfaces become adapters.
- **Why:** One authority is required for truthful remote operation, replay, conflict detection, and audit.

### 2026-07-14 — Use content-hash revisions at the Baton record boundary

- **Context:** Adding a Sworn-only revision field to Baton `slice-status-v1` would couple a runtime coordination concern to the protocol schema.
- **Decision:** State snapshots expose a SHA-256 content revision over the validated canonical record. Compare-and-write requires the expected hash and atomically replaces the file.
- **Why:** This gives cross-process optimistic concurrency without forking the protocol schema.

### 2026-07-14 — Keep durable operations in the existing repository-local database

- **Context:** Sworn already uses SQLite for supervisor ownership and events.
- **Decision:** Extend the repository-local database with versioned command, operation-event, and notification-outbox records; status/proof remain canonical Git artefacts.
- **Why:** It supplies crash-safe cross-process coordination inside the existing single-binary/runtime envelope.

### 2026-07-14 — Ship read-only mobile monitoring before remote mutation

- **Context:** Monitoring is useful immediately and has a smaller security boundary than remote control.
- **Decision:** Deliver the authenticated-capable read API and responsive read-only board first; enable state-changing controls only after authorization, revision preconditions, CSRF/origin protections, idempotency, and audit pass adversarial tests.
- **Why:** It prioritizes the requested mobile outcome without exposing unsafe direct writes.

### 2026-07-14 — Preserve a constitutional merge gate by default

- **Context:** Historical automation assembled a verified release but still required a person to perform the final merge. “Autonomous” must not silently erase that authority boundary.
- **Decision:** The default terminal outcome is `ready_to_merge`. Automatic integration merge requires a recorded standing delegation naming release scope, target branch, expiry, and revocation state.
- **Why:** It distinguishes operational autonomy from delegated release authority and fails closed when delegation is absent or stale.

### 2026-07-14 — Notifications project events; they do not own operations

- **Context:** Webhook/mobile notifications are valuable, but direct action payloads can become another mutation protocol.
- **Decision:** A durable outbox projects stable operation events. Mobile actions link to the authenticated board; any submitted action uses the same command API and idempotency/revision rules as every other surface.
- **Why:** Delivery retry cannot duplicate a loop mutation, and notification providers remain replaceable adapters.

## Ratification

The repository owner explicitly requested the architecture review, true automated
loop orchestration, a mobile web board, and webhook notifications on 2026-07-14.
The safe default for final merge and detailed slice decomposition are planner
recommendations to be reviewed at each Type-1 design gate before implementation.
