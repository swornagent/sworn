---
title: Release intake — 2026-07-01-loop-cli-ux
description: Persisted active-release selection + a zero-flag `sworn loop` invocation, restoring the coach-loop two-command ergonomics on sworn's own CLI.
---

# Release Intake: `2026-07-01-loop-cli-ux`

## Release goal

An operator can select which release they're working on once, then drive the
autonomous loop with a bare `sworn loop` — no `--release <name> --parallel`
flags to remember, and no `--parallel` name that reads as "run things
concurrently" when what it actually gates is "read state from a release
board" (true even for a single-track release). This restores the two-step
ergonomics the retired coach-loop bash harness had (`coach use <release>` +
`coach loop`), ported to sworn's own CLI, persisted in `.sworn/sworn.db`
(sworn's existing local orchestration-state store) rather than a shell/env-var
convention. "Shipped" = `sworn use <release>` persists a selection, and a
bare `sworn loop` (no flags) resolves it and drives the release exactly as
`sworn loop --release <name> --parallel` does today.

## Source of truth

- **Human stakeholder**: Brad (project owner)
- **Tracking issue / epic**: none yet — to be filed once slices are scoped
- **Related captures**: `docs/captures/2026-07-01-board-oracle-legacy-fallback-gap.md` (same session; unrelated bug but same conversation), `docs/captures/2026-07-01-baton-docs-reconciliation.md`
- **Related memory entries**: `project_sworn_home_surface` (2026-06-27 ratified direction: bare `sworn` becomes a context-aware home screen; "release active but idle" is one of its branches and needs exactly an active-release concept — this release is a small, self-contained building block toward that, not a duplicate of it), `project_sworn_operational_loop_pivot` (2026-06-30: coach-loop retired, sworn is THE loop now — this intake's precedent citation), `project_parallel_cold_start_broken` (2026-06-28 eval found real bugs in `--parallel` cold-start, since partially addressed by the 2026-06-30-sworn-operational-readiness release — relevant risk context, not this release's scope to fix)

## Users and their gestures

- **Operator (Brad, or anyone driving a sworn-managed repo)**: runs `sworn use <release>` once per work session to select the release they're driving; runs bare `sworn loop` afterward (no flags) to kick off/resume the autonomous implement→verify loop against that release; can check `sworn use` (no args) to see which release is currently selected; can `sworn use <other-release>` to switch.

## What's currently broken or missing

- Driving an existing, already-planned release through the autonomous loop requires `sworn loop --release <name> --parallel` — `--parallel` is a required flag even for a release with exactly one track, and its name describes a side effect (concurrent track execution) rather than what it actually gates (read state from a release board vs. the older `--task` ad-hoc single-slice bootstrap path). Brad's words: "this is a bit dumb, we should change that."
- No persisted notion of "the active release" exists anywhere in sworn today (confirmed by grep — `ActiveRelease`/`CurrentRelease` returns zero real hits; `internal/db`'s `tracks` table records per-track-per-release runtime bookkeeping as a side effect of a run, not a durable "this one is selected" pointer). Every invocation must fully re-specify the release name.
- `--base` (accepted by `sworn loop`/`sworn run`) is dead code: parsed then immediately discarded (`_ = base` in `cmd/sworn/run.go:38`), never dereferenced again in that file. `internal/run/run.go` hardcodes `"main"` as the base branch for the `--task` bootstrap path specifically. Confirmed the `--parallel --release` path never touches a base/integration branch at all — it stops at `release-wt`/track branches and leaves the final merge-to-integration-branch step to the human-run `/merge-release`, which is consistent with this session's own manual `/merge-release` flow, not a gap in that path. Whether `--base`'s dead code is in scope for this release (clean it up) or a separate small fix is an open question below.

## What the human wants

- **N-01**: `sworn use <release>` selects and persists "the active release" (validated against the board oracle) for subsequent commands.
- **N-02**: `sworn loop --release <name>` works standalone — `--parallel` is no longer a required companion flag.
- **N-03**: A bare `sworn loop` (no flags at all) drives the currently-selected (via N-01) release's autonomous loop.
- **N-04**: the dead `--base` flag (parsed, never used) is removed from `sworn loop`'s flag surface.
- The coach-loop precedent (`coach use <release>` + `coach loop`) as the UX model to port, not reinvent from scratch.

## Constraints and non-negotiables

- This repo is public-safe (project CLAUDE.md): no business/pricing/competitive/strategy content in any spec, and the reference implementation stays a single Go binary with minimal, justified deps — no new dependency needed here since `.sworn/sworn.db` (SQLite, already an ADR-0003-justified dependency) is the proposed persistence layer.
- Backward compatibility: `sworn loop --release <name> --parallel` is the form used throughout this session's own work and documented in `--help`. Whether the old flags are removed, deprecated-but-working, or kept as a permanent alias is an open decision (see below) — sworn is pre-1.0 (`0.1.0`), so a breaking CLI change is plausible but should be a deliberate choice, not a silent one.

## Adjacent / out of scope

- **Item**: the full context-aware `sworn` home-screen direction (`project_sworn_home_surface` — bare `sworn` branches on loop-running / release-idle / no-release / not-a-sworn-repo). **Why deferred**: much larger scope (webUI, mobile, omni-channel notifications per the memory's 2026-06-30 sharpening); this release is a small prerequisite primitive (persisted active-release selection), not the home screen itself. **Tracking**: `project_sworn_home_surface` memory entry; no release/issue yet. **Acknowledged**: 2026-07-01 (Brad, this session).
- **Item**: fixing `sworn doctor --fix`'s destructive full-file `AGENTS.md` overwrite. **Why deferred**: unrelated to loop CLI UX; already filed as `sworn#43` earlier this session. **Tracking**: `sworn#43`. **Acknowledged**: 2026-07-01 (Brad, this session, prior conversation turn).

## Decisions made during planning

### 2026-07-01 — validate release name on `sworn use`

- **Context**: should `sworn use <release>` accept any string, or check it against the board oracle first?
- **Options considered**: accept any string (simpler); validate via the board oracle (rejects typos immediately).
- **Decision**: validate. `sworn use <release>` calls the board oracle before persisting; an unknown release name is rejected with a clear error rather than silently accepted and failing later on the first `sworn loop` call.
- **Why**: fail-closed matches this project's own conventions (Baton Rule 1/7 fail-closed posture); a typo persisted silently is a worse failure mode than an immediate, clear rejection.

### 2026-07-01 — `--release <name>` works standalone; `--parallel` requirement dropped

- **Context**: today `sworn loop --release <name>` requires `--parallel` too, even for a single-track release, and the flag name misleadingly describes a side effect (concurrent execution) rather than what it gates (board-driven mode vs. the `--task` ad-hoc bootstrap path).
- **Options considered**: keep `--parallel` mandatory (status quo); make board-driven mode the default whenever `--release` is passed, dropping the `--parallel` requirement; deprecate/remove the old flag form entirely in favour of `sworn use` + bare `sworn loop`.
- **Decision**: `sworn loop --release <name>` works standalone — passing `--release` alone selects board-driven mode; `--parallel` is no longer required (may remain accepted as a harmless no-op for any existing scripts, TBD in S03 spec). This form is NOT deprecated — it is the permanent, concurrency-safe, explicit primitive.
- **Why**: `--parallel`'s name never matched its purpose (it gates "read from a release board," not "run concurrently" — even a one-track release needed it). Making `--release` alone sufficient removes the misleading requirement without inventing a new flag name.

### 2026-07-01 — real concurrent multi-release support, reconciled with the convenience layer

- **Context**: if more than one release is actionable, does bare `sworn loop` need a "which one" disambiguation, or is a single active-release selection sufficient? Initial answer: genuine concurrent multi-release support is needed (running two releases' loops at once from the same repo checkout must not have them stomp a single shared "active release" pointer).
- **Options considered**: (a) single global active-release pointer only, no true concurrency; (b) per-terminal/per-process selection state; (c) explicit `--release <name>` stays the permanent, standalone, concurrency-safe primitive (self-contained per invocation, touches no shared "active" state), and `sworn use` + bare `sworn loop` is layered on top purely as an optional convenience default for the common single-release-at-a-time case — not itself required to support concurrency, because explicit `--release` usage already does.
- **Decision**: (c). Confirmed with Brad directly.
- **Why**: this reconciles both answers without extra complexity — concurrency is achieved by always passing `--release` explicitly in each concurrent terminal (already true after the decision above); the `.sworn/sworn.db`-backed "active release" pointer that `sworn use` sets is a single shared value by design, used only when `--release` is omitted, and is never consulted by an explicit invocation.

### 2026-07-01 — dead `--base` flag cleanup folded into S03

- **Context**: `cmd/sworn/run.go:38` parses `--base` then immediately discards it (`_ = base`); it's never dereferenced again in that file. Separate, pre-existing bug, unrelated to this release's core ask.
- **Options considered**: fold into S03 (same file being touched for old-flag handling); file as a separate follow-up issue.
- **Decision**: fold into S03.
- **Why**: cheap to bundle — S03 is already editing `cmd/sworn/run.go`'s flag handling; opening a second small release for the same file later would be pure overhead.

## Schema-vs-spec audit notes

`.sworn/sworn.db`'s `tracks` table already has a `release` column and per-track `state`/`current_slice`/`pid` fields (`internal/db/db.go`), but no table or column expresses a singular "currently selected release" — that's genuinely new schema, not an existing field being repurposed. Confirmed via direct read of `internal/db/db.go`'s `schema` DDL list (4 tables: `schema_version`, `tracks`, `events`, `decisions`, `circuit_failures` — no settings/config-style key-value table exists yet either).

## Proposed slice decomposition (final)

- `S01-active-release-store` — `sworn use <release>` validates the release against the board oracle then persists it to `.sworn/sworn.db` (new `settings` table); `sworn use` (no args) prints the current selection or a clear "none selected" message. Touches `internal/db`, a new `cmd/sworn/use.go`. No changes to `run.go`.
- `S02-loop-explicit-standalone` — `sworn loop --release <name>` works without requiring `--parallel` (board-driven mode is the default whenever `--release` is passed; `--parallel` becomes accepted-but-optional, not removed). Also removes the dead `--base` flag (parsed, never dereferenced — folded in per the decision above, same file). Touches `cmd/sworn/run.go` only. Independent of S01 (no shared files) — could run in a parallel track, but given the total release size (~3 small chore-scale slices, single-digit files each) a single serial track is the simpler default; flagged as a Type-2 implementation-scale choice, not asked as a separate question.
- `S03-loop-default-from-store` — a bare `sworn loop` (no `--release` at all) resolves the active release from S01's store and drives it through the exact same code path S02 established for `--release <name>` alone; clear, actionable error if no release is selected ("no active release — run `sworn use <release>` first"). Depends on both S01 and S02.

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Should `sworn use <release>` validate the release exists (via the board oracle) before persisting, or accept any string? | S01 AC | **Resolved 2026-07-01**: validate via the board oracle; reject unknown release names. |
| A-02 | Does `--release <name> --parallel` on `sworn loop` keep working after this release (alias) or get removed? | S02 AC | **Resolved 2026-07-01**: `--release <name>` works standalone (no `--parallel` required); this is the permanent, concurrency-safe, explicit form — not deprecated. |
| A-03 | Is the dead `--base` flag cleanup in scope for this release? | S02 scope | **Resolved 2026-07-01**: yes, folded into S02 (same file). |
| A-04 | If multiple releases are actionable, does bare `sworn loop` ever need a "which one" disambiguation, or is single-active-release sufficient? | S03 AC | **Resolved 2026-07-01**: explicit `--release <name>` (S02) is the concurrency-safe primitive for running multiple releases' loops at once; the S01/S03 active-release store is a single shared convenience default, used only when `--release` is omitted, never a concurrency mechanism itself. |

## Screenshots / references

(none)
