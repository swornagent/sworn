---
title: 'Release intake: 2026-06-19-safe-parallelism'
description: 'Discovery output for the safe-parallelism release — R3 of the SwornAgent roadmap. Concurrent multi-track delivery with the fail-closed verify gate intact under concurrency, plus the commercial on-ramp (sworn account / credits).'
---

# Release Intake: `2026-06-19-safe-parallelism`

> Durable record of the requirements conversation. The slice list is downstream of this file.

## Release goal

`sworn run` today runs slices serially — one track at a time, one implementer session blocking
the next. This release makes it genuinely parallel: multiple tracks run concurrently in
isolated worktrees, each verified by a fresh-context adversarial verifier, with the
**fail-closed gate provably intact under concurrency** — overclaim rate flat as N scales 1→N.

The parallel story is inseparable from the quality story: "N faithfully-verified-wrong things,
fast" is not the product. The product is velocity **and** quality, both at once. R2's fidelity
layer is the prerequisite (spec quality before parallelism); R3's concurrency is only safe
because R2 hardened the specs the agents implement against.

Also in this release: the commercial on-ramp (`sworn account`) — opt-in cloud registration
and managed model proxy, the SwornAgent Credits tier. Binary stays MIT; the managed service
is the revenue surface.

"Shipped" looks like: `sworn run --parallel` builds four tracks concurrently, all verified
with independent fresh-context sessions, proof bundles for each, `sworn top` shows live
concurrent status, and a formal benchmark proves overclaim rate is flat 1→4. A developer
can start a full release run, close their laptop, and come back to completed proof bundles
or a paged notification that something needs their input.

## Source of truth

- **Human stakeholder**: Brad (maintainer)
- **Tracking issue / epic**: TBD — create before first implementation session (Rule 5)
- **Integration branch**: `release/v0.1.0`
- **Prerequisite release**: `2026-06-16-fidelity-layer` (R2) — must be fully merged before
  R3 implementation begins. R3 planning can proceed in parallel with R2 implementation.
- **ROADMAP reference**: `internal-docs/ROADMAP.md` §R3 safe-parallelism
- **Related decisions**: `internal-docs/decisions/2026-06-14-swornagent-INDEX.md`
- **Market research**: planning session 2026-06-19 (conversation context); OpenCode Zen
  ($25M ARR proof of managed proxy model); Jules (Google async agent) pricing precedent;
  Devin Agent Compute Unit billing precedent.

## Users and their gestures

- **Solo developer / small-team lead (the human driving sworn)**: starts `sworn run` on a
  release with multiple planned tracks, walks away, comes back to proof bundles or a page
  saying a slice needs attention. Does not watch the terminal. Credits consumption visible
  in `sworn top`.
- **SwornAgent cloud user (opted into sworn account)**: routes model calls through the
  SwornAgent proxy instead of direct API keys. No keys to manage in CI. Sees credit balance
  in `sworn top`. Converts from BYO-key free tier when the convenience is worth $20/month.
- **OSS / BYO-key user**: uses MIT binary forever, manages own API keys, no credits. Full
  sworn functionality; no managed service. The word-of-mouth / star-growth segment.
- **Team user (future)**: shared credit pool, PR attestation badge, team release board.
  Scoped post-R3; not in this release.

## What's currently broken or missing

- `sworn run` is serial. One track. If you have four tracks planned (as R2 does), you run
  them one after the other in one terminal session. The velocity promise of track mode is
  only partially delivered.
- No process-ownership registry. The zombie issue (`sworn#2`, 2026-06-16): concurrent
  workers can corrupt each other's state without a single-owner identity + supervisor model.
  This is a hard safety precondition for parallelism — the zombie killed 2026-06-16 is
  "test case #1."
- No commercial on-ramp. Binary is MIT, free forever with BYO keys. Users who want managed
  model proxying have no path to it yet.
- `sworn top` (R2's S15) is a read-only journey board. It does not show concurrent track
  execution status in real time.
- No formal benchmark proving the verify gate holds under concurrency. The claim is
  theoretical until a benchmark runs it.

## What the human wants

- **Parallel track execution**: `sworn run --parallel` (or auto-parallel when >1 track
  is planned) runs all independent tracks concurrently, each in its own worktree, without
  corrupting each other.
- **Process-ownership safety**: a registry + supervisor that guarantees single-owner
  identity per slice, reaps zombies on restart, scopes worker processes correctly. This is
  non-negotiable; parallelism without it is dangerous.
- **Verify gate under concurrency**: the adversarial fresh-context verify gate must be
  provably correct when N slices verify simultaneously. Overclaim rate flat 1→N.
- **sworn top as concurrency monitor**: extends R2's read-only board to show live
  concurrent track status — which tracks are running, which slice each is on, credits
  consumed, ETA.
- **Overclaim benchmark**: a formal, repeatable benchmark that publishes overclaim rate
  at N=1, N=2, N=4 concurrent tracks. This is a launch-gate prerequisite per the ROADMAP.
- **sworn account + credits** (scope TBD — see open questions): opt-in cloud registration,
  managed model proxy, credits-based billing. The on-ramp from MIT free to paying user.
  Narrow slice; may be R3 or immediately post-R3.
- **Walk away and get paged**: when a slice fails verification or hits a blocker that
  needs human input, the human is notified (webhook/email/Slack) rather than the terminal
  sitting blocked. Scope TBD.

## Market context (captured from planning session)

The async coding agent market is real and crowded in 2026: Jules (Google), Cursor Background
Agents, Codex Cloud, Copilot Coding Agent, Devin all do "assign a task, walk away, PR
arrives." None of them do verified delivery. Their quality bar is "passes CI tests."

SwornAgent's differentiation is the fresh-context adversarial verify gate: a DIFFERENT model,
in a DIFFERENT context, tries to fail the implementation against the spec before anything
merges. No current competitor does this. The whitespace is verified async delivery, not just
async delivery.

Pricing precedents from the market:
- OpenCode Zen: $25M ARR from managed model proxy on top of a free MIT binary — direct
  proof of concept for the credits/proxy model.
- Jules: free (15 tasks/day) / $20/mo / $125/mo — validates Pro tier price point.
- Devin: ACU-based metered billing at $20-$500/mo — validates outcome-unit billing.

Cursor acquired by SpaceX at $60B (June 2026) — validates the AI coding tool market at
scale; also signals Cursor becoming a corporate tool, widening the solo-dev / async gap.

## Commercialisation decisions (captured during planning session 2026-06-19)

**Model**: Free (MIT binary, BYO API keys, unlimited) + SwornAgent Credits (managed model
proxy, no keys needed, pay per E2E slice attempt).

```
Free (BYO key):    unlimited, MIT binary, manage own model API costs
Credits:           $10 = 10 credits; 1 credit = 1 E2E slice attempt
                   SwornAgent manages model calls, rate limiting, provider fallback
Pro ($20-25/mo):   100 credits/month + async notifications (Slack/email/webhook)
                   + release dashboard
Team (~$12/dev/mo): shared credit pool, PR attestation badge, team board
Enterprise:        volume + compliance ledger + SSO + SLAs + audit log
```

**Billing unit**: per E2E slice attempt (not per verified slice, to protect margin on
complex slices with retries). Optionally offer "verified or refunded" credit guarantee
as a marketing claim.

**OSS line**: `sworn` binary = MIT (trust + distribution). SwornAgent cloud service =
proprietary. Compliance ledger = proprietary (value IS the trust chain, not the code).
Baton protocol = MIT (neutral standard, stays separate from SwornAgent).

**Why not BSL**: fork risk (OpenTofu precedent), enterprise procurement blocks non-OSI
licenses, enforcement requires infrastructure we don't have.

**Positioning vs. competitors**: Jules/Devin/Cursor Background deliver async. SwornAgent
delivers async AND verified. Same PR, different quality bar. This gap is unoccupied.

## Constraints and non-negotiables

- **Zero runtime dependencies** (from R1 ADR-0001): stdlib + net/http + encoding/json
  only. Any concurrency supervisor must be Go stdlib (sync, context, os/exec). No new
  external deps without an ADR.
- **Fail-closed under concurrency**: if the supervisor or scheduler fails, in-flight
  slices must not auto-merge. The gate closes, not opens.
- **Process isolation**: concurrent implementer workers must not share a worktree,
  git index, or state file. Each track has its own worktree (from track mode).
- **Public-safe**: no business-confidential content in the repo. Commercialisation
  strategy lives in `internal-docs/` (private). R3 specs describe capabilities only.
- **R2 prerequisite**: R3 implementation does not begin until R2 is fully merged.
  Planning is fine; worktree materialisation waits for R2.

## Adjacent / out of scope (Rule 2 deferrals)

- **Full SaaS billing infrastructure**: the `sworn account` slice creates the
  registration on-ramp and credit proxy; the full billing engine (Stripe integration,
  subscription management, dunning) is post-R3. **Why**: too large for this release;
  R3 credits can start as manually-granted beta credits. **Tracking**: launch-gate
  workstream. **Acknowledged**: 2026-06-19 planning session.
- **GitHub Action / Marketplace integration**: the managed Action wrapping `sworn` with
  billing is the next monetisation surface after `sworn account`, but is post-R3.
  **Why**: Action scope is its own release. **Tracking**: TBD issue. **Acknowledged**:
  2026-06-19.
- **Compliance ledger**: signed attestation records, CA infrastructure. Post-launch moat.
  **Why**: requires legal + infrastructure investment. **Tracking**: post-launch roadmap.
  **Acknowledged**: strategy docs 2026-06-14.
- **Team collaboration features** (shared boards, PR badges): post-R3. The Team tier
  is defined but not built yet. **Tracking**: TBD. **Acknowledged**: 2026-06-19.
- **Multi-git-provider support**: GitLab, Bitbucket etc. GitHub only for launch.
  **Tracking**: post-launch platform adapters. **Acknowledged**: prior roadmap.
- **Async paging / notifications** (webhook/Slack/email on slice fail): may fit in R3
  as part of `sworn account` or as its own slice; TBD during decomposition.

## Decisions made during planning

### 2026-06-19 — Release name confirmed: `2026-06-19-safe-parallelism`

- **Context**: command was invoked as `/plan-release R3 for sworn`; parser took "for"
  as the release name. Corrected to date-prefixed form per convention.
- **Decision**: `2026-06-19-safe-parallelism`
- **Why**: matches ROADMAP R3 theme; date = planning-start (2026-06-19).

### 2026-06-19 — R2/R3 sequencing: plan now, implement after R2

- **Context**: R2 (fidelity-layer) has 13/16 slices verified on track branches (oracle
  state); T1/T2/T4 ready to merge; T3 has S03 implemented + S08/S09 planned.
- **Decision**: plan R3 now; R3 implementation gate = R2 fully merged.
- **Why**: planning can proceed in parallel with R2 finishing; implementation requires
  the fidelity layer as prerequisite per ROADMAP rationale.

### 2026-06-19 — Process improvement: oracle-check mandatory at session start

- **Context**: planner incorrectly reported R2 as "0 slices implemented" based on stale
  `index.md` on the integration branch; oracle (track branch status.json) shows 13/16
  verified. The planner.md requires this check for replanning but not for initial
  planning sessions.
- **Decision**: update planner.md session-start handshake to require oracle-check for
  all prerequisite releases before assessing their state.
- **Why**: `index.md` on the integration branch is always stale during in-flight work;
  the oracle is status.json on track branches.
- **Tracking**: planner.md update; meta-task outside R3 scope.

### 2026-06-19 — Commercialisation model: credits + managed proxy (not BSL, not SaaS-first)

- **Context**: MIT binary + no payment surface at launch. User questioned how to create
  a payment obligation. BSL rejected (fork risk, enterprise procurement, enforcement
  cost). Full SaaS tier rejected (privacy objection from target customers, infrastructure
  scope, timing). Professional services rejected (doesn't scale).
- **Decision**: OpenCode Zen model — free MIT binary forever (BYO key), revenue from
  SwornAgent Credits (managed model proxy, $10/10 credits). Pro/Team/Enterprise tiers
  above that. Compliance ledger = future ACV.
- **Why**: OpenCode Zen at $25M ARR is direct proof. Jules/Devin validate price points.
  Binary being MIT is the distribution mechanism; proxy/ledger is the revenue surface.

### 2026-06-19 — Market positioning: verified async delivery vs. async delivery

- **Context**: Jules, Cursor Background Agents, Devin, Codex Cloud all do async delivery
  (assign task, walk away, PR arrives). Quality bar = CI tests pass.
- **Decision**: SwornAgent's positioning is the layer above: async AND verified. Fresh-
  context adversarial review against a written spec, fail-closed. This gap is unoccupied
  in the current market.
- **Why**: research confirmed no competitor does spec-first adversarial verification.
  "Did the code pass tests?" vs. "Did a different model try to prove the code wrong
  against the spec and fail?" are fundamentally different quality bars.

## Open questions (must resolve before dependent slices move to in_progress)

1. **Does `sworn account` go in R3 or as an immediate post-R3 launch-gate item?**
   Thin slice; one command; credits on-ramp. R3 scope adds commercial hook at launch.
   Post-R3 keeps R3 pure-engineering. TBD during decomposition.

2. **Async paging / notifications**: when a slice fails or blocks, how does the human
   find out without watching the terminal? Webhook? Slack? Email? Is this in R3 or
   deferred? Depends on whether `sworn account` is in R3 (account = endpoint for
   notifications).

3. **sworn top extension**: R2's S15 delivers a read-only journey board. R3 needs
   concurrent track execution status. Is this a new `sworn top` mode (flag) or a
   separate surface? Coordinate with S15 touchpoints.

4. **Process-ownership registry spec**: what exactly does `sworn#2` require? Reap-on-
   restart (kill stale workers on `sworn run` startup), single-owner identity (a slice
   can only be owned by one implementer PID at a time), scoping (workers can't read
   each other's worktree state). Needs an architecture decision before spec is written.

5. **Benchmark format**: is the overclaim benchmark a committed Go test, a standalone
   script, or an integration test that requires model API keys? Affects which track it
   belongs to and what "verified" means for that slice.

## Proposed slice decomposition (draft — not yet confirmed)

See Phase 3 conversation for the Scope-Ceiling Bar and Dependency Graph.

- `S01-process-ownership` — process registry + supervisor; reap-on-restart; single-owner
  identity per slice; safe concurrent implementer isolation
- `S02-concurrent-scheduler` — `sworn run` launches multiple tracks in parallel; each in
  its own worktree; scheduler coordinates without sharing state
- `S03-verify-under-concurrency` — verify gate provably correct at N>1; adversarial fresh-
  context sessions don't corrupt each other; fail-closed under concurrency
- `S04-sworn-top-concurrency` — `sworn top` extended to show live concurrent track status,
  credits consumed, ETA; extends R2's S15 read-only board
- `S05-overclaim-benchmark` — formal repeatable benchmark: overclaim rate at N=1/2/4;
  published as a release artefact; launch-gate requirement met
- `S06-sworn-account` *(TBD — in R3 or post-R3)*: `sworn account register`; opt-in cloud
  registration; model calls routed through SwornAgent proxy; credit balance in `sworn top`

## Screenshots / references

*(None yet — add here when screenshots are shared during planning.)*
