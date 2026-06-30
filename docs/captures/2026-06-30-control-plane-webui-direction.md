---
title: 'Direction — sworn serve: a machine-global web control plane for loops'
description: 'Open successor to the retired coach release-board-ui.mjs: a single-binary HTTP control plane that monitors and manages every sworn loop on a machine (and remote machines), mobile-first. Captured as a future release, not yet planned.'
date: 2026-06-30
---

# Direction — `sworn serve`: machine-global web control plane

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

## Relationships / prior captures
- Sharpens the `sworn serve` direction already noted in memory `project_sworn_home_surface`
  (bare `sworn` = context-aware home; `top` = default live view) and the proof-visibility
  theme (`project_proof_visibility_theme`).
- The fleet/remote design and any hosted-deployment ladder are tracked in the private
  strategy notes (out of scope for this public capture, which covers the open feature only).

## Scope / sequencing
Its own release, planned spec-first. **Not** tonight and **not** in the operational-readiness
release. Sequence after sworn is proven operational (the fired overnight run), since a
control plane is most valuable once there are real loops to watch. Natural slices: (S1) the
loop registry + `sworn run` self-registration; (S2) `sworn serve` read-only board API +
embedded mobile SPA; (S3) live updates (SSE); (S4) **next-step command surfacing** — each
slice card exposes a click-to-copy of the exact command for its current state (the board
already knows the state→action mapping; this is the router/oracle's next-step rendered for a
human instead of dispatched to an agent — the human-takeover seam); (S5) control actions
(authenticated mutations: pause/resume/ack/retry/kill); (S6) remote/fleet aggregation.

Note on S4: the state→next-command mapping is not new logic — it is what the router/oracle
already computes to decide the next dispatch. Surfacing it as a copyable human command (vs
dispatching it to an agent) is the same decision rendered for the other driver. That keeps
the autonomous and interactive paths reading from one source of truth (no drift between what
the loop would do and what the human is told to do).
