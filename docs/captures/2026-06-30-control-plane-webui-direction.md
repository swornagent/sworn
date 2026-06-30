---
title: 'Direction — omni-channel control plane for autonomous loop orchestration'
description: 'Open successor to the retired coach release-board-ui.mjs: one event+command core behind a machine-global web control plane (mobile-first, remote-capable) AND push/chat channels (ntfy.sh / Slack / Telegram, all webhooks). Captured as a future release, not yet planned.'
date: 2026-06-30
---

# Direction — omni-channel control plane for loop orchestration

The web control plane (`sworn serve`) and the push/chat channels (ntfy.sh / Slack /
Telegram) are NOT separate features — they are channel adapters over **one event+command
core**. The core emits loop lifecycle events and accepts authenticated control commands; each
channel is a thin projection of the same two streams. Build the core once; the channels are
small. This is the "notify" surface of the proof-visibility theme made concrete.

## Part A — `sworn serve`: machine-global web control plane

> Captured for a FUTURE release (spec-first when planned). NOT part of
> `2026-06-30-sworn-operational-readiness` (that release is D6 + the board renderer).
> This is a real feature direction with open architecture decisions, recorded so it
> survives conversation context.

## The need
The operator monitors running loops from mobile while away from the computer. The old
coach-loop board UI (`release-board-ui.mjs`, a private Node app) is being retired with
coach-loop, so its monitoring value needs an **open replacement built into sworn**. The
replacement should be:

1. **Machine-global** — one view of *every* loop running on a machine, not a single
   release. Today `sworn top <release>` is per-release and terminal-only.
2. **Mobile-first** — usable from a phone to watch progress, see pages, and (later) act.
3. **Remote-capable** — able to monitor/manage sworn loops on *other* machines through
   the same interface, so one control plane covers a fleet.
4. **A fluid autonomous↔interactive bridge** — one-click copy of the exact next-step
   command for any slice, so the operator can drop into an interactive session and drive it
   by hand *at will* — not only on failure. Recovering a stuck loop is one trigger, but the
   value is general: being able to switch into interactive mode at any point is its own
   capability. This was invaluable in the old board UI: each card exposed the command string
   (`/implement-slice <slice> <release>`, `/verify-slice ...`, or `sworn ...` for the slice's
   current state) as a click-to-copy icon. It makes leaving a loop running unattended *safe*,
   because stepping in is always one paste away.

   Companion goal: ideally loops don't get stuck at all — that is the **resilience
   hardening** deferred from the operational-readiness release (retry-reset, escalation
   default, turn-cap; intake S02/S03 there). The two are complementary: hardening reduces how
   often you *need* to step in; the interactive bridge makes stepping in trivial when you
   *want* to. Neither replaces the other.

## Where it sits among the surfaces
sworn already has three surfaces over one core (the oracle + slice records): the **CLI**,
the **TUI** (`sworn top`, per-release, terminal), and **MCP** (the agent-facing surface).
This adds a fourth: a **web control plane** that is machine-global and mobile-reachable.
Same oracle data, new presentation + reach. It supersedes `release-board-ui.mjs`.

## Proposed shape (to be confirmed at planning)
- **`sworn serve`** — an HTTP server in the single Go binary. An embedded SPA (`go:embed`)
  + a JSON API over: oracle board state, proof bundles, paging/breaker events, dispatch
  telemetry. No new runtime dependency on a separate web stack.
- **Machine-global discovery via a per-machine loop registry.** sworn already uses
  `~/.sworn/` (`.env`, `memory.db`) and per-project `.sworn/`. A registry there (e.g.
  `~/.sworn/loops/<id>.json` written by each `sworn run`: repo, release, worktree, pid,
  started_at, status path) lets `sworn serve` enumerate and watch every active loop without
  a daemon. `serve` aggregates their oracle state.
- **Read-first, then act.** v1 is monitoring (board, slice states, proof, pages). Control
  actions (pause/resume, ack a design gate, retry, kill) layer on after the read surface is
  proven — each action is a fail-closed, authenticated mutation.
- **Remote = the same JSON API across machines.** `serve` can also point at remote sworn
  nodes and aggregate them, so the local control plane is also a client of remote nodes —
  one surface whether the loops are local or on a fleet.

## Open architecture decisions (Type-1 — resolve at planning, record in status.json)
1. **State model**: `serve` polls the shared oracle/git-ref state (simple, matches the
   oracle-driven design) vs each loop pushes events to `serve` (lower latency, more moving
   parts). Likely poll + SSE fan-out to clients.
2. **Discovery**: registry file (above) vs a long-lived daemon vs process scan. Registry
   file is the lowest-friction and matches the existing `~/.sworn/` convention.
3. **Wire API**: REST + Server-Sent Events vs websocket vs gRPC. The API shape is
   load-bearing — it is the same surface remote/fleet use, so design it as a stable contract.
4. **Auth tiers**: localhost = open by default; remote/fleet = token-authenticated. Spell
   out the boundary before any non-localhost bind.
5. **UI stack**: embedded SPA framework choice; keep the single-binary property (no separate
   deploy). Mobile-responsive is a first-class requirement, not a retrofit.
6. **Proof-bundle surfacing**: ties directly to the proof-visibility theme — the control
   plane is a natural home for surfacing proof bundles per slice.

## Part B — push/chat channels (ntfy.sh / Slack / Telegram)

Brad: the bot/notification approach is needed too, and they are "all just webhooks." The
discipline: do NOT write three bespoke integrations. Write one event+command core and three
thin adapters.

- **The event+command core (shared with Part A).**
  - *Outbound events*: the loop already pages (max-turns, circuit-breaker halt) and changes
    slice state (design-gate halt, verified, failed_verification, run-complete). Formalise
    these as a typed event stream. Each event carries the same next-step command Part A
    surfaces (the router/oracle's next action), so a notification can *embed the command to
    paste* — the autonomous↔interactive bridge over a phone.
  - *Inbound commands*: the authenticated control actions (ack a gate, pause, resume, retry,
    kill) are one API. The web UI calls it; so do the chat bots.
- **Channel adapters (thin):**
  - **ntfy.sh** — outbound push, near-zero setup (HTTP POST to a topic), great for "page me
    when X." Supports action buttons / a copyable body for the next-step command. Mostly
    one-way (monitor + nudge).
  - **Slack / Telegram bots** — outbound push AND *inbound* control: a reply or slash command
    (`/sworn pause T1`, an "Ack gate" button) drives the loop through the same control API.
    This is the chat-native form of the interactive bridge.
- **Config**: per-machine channel config (which channels, which webhook URLs/tokens/topics,
  which event severities route where) under the existing `~/.sworn/` convention. Secrets
  (bot tokens, webhook URLs) are local, never committed — same rule as `~/.sworn/.env`.
- **Open architecture decisions (resolve at planning)**: (a) outbound-only vs bidirectional
  per channel (ntfy mostly outbound; Slack/Telegram bidirectional); (b) where the inbound
  webhook receiver lives — `sworn serve` already runs an HTTP server, so chat callbacks are
  routes on it (one server, all channels); (c) event taxonomy + severity → channel routing;
  (d) auth on inbound commands (a chat message must not be able to kill a run without a
  verified sender / signed webhook).

## Relationships / prior captures
- Sharpens the `sworn serve` direction already noted in memory `project_sworn_home_surface`
  (bare `sworn` = context-aware home; `top` = default live view) and the proof-visibility
  theme (`project_proof_visibility_theme`).
- The fleet/remote design and any hosted-deployment ladder are tracked in the private
  strategy notes (out of scope for this public capture, which covers the open feature only).

## Scope / sequencing
Its own release, planned spec-first. **Not** tonight and **not** in the operational-readiness
release. Sequence after sworn is proven operational (the fired overnight run), since a
control plane is most valuable once there are real loops to watch. Natural slices, ordered so
the shared core lands before the channels that ride it:

- **S1 — loop registry + `sworn run` self-registration** (machine-global discovery under `~/.sworn/`).
- **S2 — event+command core**: typed loop event stream (formalise the existing pages + state
  changes) + the authenticated control-action API (ack/pause/resume/retry/kill). The
  foundation BOTH the web UI and the chat bots project. Each event carries the router/oracle
  next-step command.
- **S3 — `sworn serve` read-only board API + embedded mobile SPA** (web projection of the core).
- **S4 — live updates (SSE)** to the web clients.
- **S5 — next-step command surfacing** in the web UI: each slice card click-to-copies the
  exact command for its state (the same next-step the core already computes — rendered for a
  human instead of dispatched to an agent).
- **S6 — ntfy.sh adapter** (outbound push; near-zero setup; embed the next-step command).
- **S7 — Slack/Telegram bot adapter** (outbound push + inbound control via the S2 command API).
- **S8 — web control actions** (authenticated mutations wired to the S2 API).
- **S9 — remote/fleet aggregation** (the core's API consumed cross-machine).

The through-line: S2 is the keystone. Every channel (web, ntfy, Slack, Telegram) is a thin
adapter over the one event+command core, so the autonomous loop, the web UI, and the chat
bots all read the same next-step from one source of truth — no drift between what the loop
would do and what any channel tells (or lets) you do.
