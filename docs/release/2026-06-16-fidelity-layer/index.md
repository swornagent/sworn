---
title: 'Release board template'
description: 'The release board — the single source of truth for slice states and track grouping across a release. Updated by the planner during decomposition and by implementer / verifier as each slice progresses.'
# The frontmatter below is the machine-readable registry the slash commands read.
# - Planner fills `tracks:` (id, slices, depends_on, worktree_branch) during decomposition.
# - First /implement-slice in the release fills `release_worktree_path` / `release_worktree_branch`.
# - First /implement-slice in a track fills that track's `worktree_path`.
# See `docs/baton/track-mode.md` for the model these fields encode.
release_worktree_path: # <set by first /implement-slice in the release — absolute path, e.g. <HOME>/projects/<repo-basename>-worktrees/release-2026-05-20-billing-redesign>
release_worktree_branch: # release-wt/<release-name>
tracks: []
#  - id: T1-<short-name>
#    slices: [S01-<name>, S02-<name>]   # ordered; slices run sequentially within the track
#    depends_on: null                    # or another track id (see track-mode.md "Cross-track dependencies")
#    worktree_path:                      # <set by first /implement-slice in this track>
#    worktree_branch: track/<release-name>/T1-<short-name>
#    state: planned                      # planned | in_progress | merged
---

# Release Board: `<release-name>`

> Copy this file to `docs/release/<release-name>/index.md`. The frontmatter is the machine-readable registry; the tables below are the human-readable mirror. Keep them in sync.
>
> **Parallelism model:** this release runs under **track mode** — see `docs/baton/track-mode.md`. Slices are grouped into tracks; tracks run in parallel, each in its own worktree; slices within a track run sequentially. The touchpoint matrix below is what licenses the parallelism.
>
> **Naming convention:** `<release-name>` follows `YYYY-MM-DD-<theme>` where the date is planning-start. The *target version* of this release goes in the Release summary block below, not in the folder name.

## Release summary

- **Goal**: \<one sentence; cite `intake.md` for the long form\>
- **Target version / integration branch**: \<e.g. `release/v0.5.0`, `release/v0.6.0`\>
- **Started**: `<YYYY-MM-DD>` (should match the date prefix in the folder name)
- **Target ship**: `<YYYY-MM-DD or "uncommitted">`
- **Intake**: `intake.md`
- **Stakeholder**: `<name>`
- **Tracking issue**: `<link>`

## Tracks

> Each track runs in its own worktree (`/implement-slice` materialises it lazily). Slices within a track run **in the listed order**. Tracks with no `depends_on` may run fully in parallel.

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|
| `T1-<name>` | S01 → S02 | — | `track/<release-name>/T1-<name>` | planned |
| `T2-<name>` | S03 | — | `track/<release-name>/T2-<name>` | planned |

Track state: `planned` (no slice started) → `in_progress` (≥1 slice started, not all merged) → `merged` (`/merge-track` landed it in `release-wt/<release-name>`).

### Touchpoint matrix

> Proves the tracks are disjoint — **no row may carry a `✓` in more than one track column.** If you cannot achieve that, the colliding slices belong in the same track, or one track must `depends_on` another. See `track-mode.md`.

| File / surface | T1 | T2 |
|---|---|---|
| `src/.../SomeComponent.tsx` | ✓ | |
| `src/.../OtherComponent.tsx` | | ✓ |

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|
| `S01-<name>` | T1 | `<one sentence>` | planned | human | [spec](./S01-`<name>`/spec.md) | — |
| `S02-<name>` | T1 | `<one sentence>` | planned | human | [spec](./S02-`<name>`/spec.md) | — |
| `S03-<name>` | T2 | `<one sentence>` | planned | human | [spec](./S03-`<name>`/spec.md) | — |

### State legend

| State | Meaning | Who can move out of it |
|---|---|---|
| `planned` | Spec written, awaiting implementation | Implementer |
| `in_progress` | Implementer session active | Implementer |
| `implemented` | Implementer claims done; awaiting fresh-context verification | Verifier |
| `verified` | Fresh-context verifier returned PASS | Human (`/merge-track`) |
| `failed_verification` | Verifier returned FAIL; fix and re-submit | Implementer |
| `deferred` | Slice carved out per Rule 2; not in this release | Human |
| `shipped` | Slice is live in production | — (terminal) |

A slice stays `verified` through `/merge-track` and `/merge-release`; it flips to `shipped` only when the version branch deploys.

## Aggregate state

`<Rolling count, updated whenever any slice transitions.>`

- Planned: N
- In progress: N
- Implemented (awaiting verification): N
- Verified (awaiting merge): N
- Failed verification: N
- Deferred: N
- Shipped: N

**Tracks:** Planned: N / In progress: N / Merged: N

## Recent activity

`<Chronological log of the most recent state transitions, including track merges.>`

### `<YYYY-MM-DD HH:MM>` — `<slice-id>`: `<old-state>` → `<new-state>`

- **Actor**: `<implementer / verifier / human>`
- **Note**: `<one line>`

## Decisions deferred (Rule 2)

`<Items carved out of this release with explicit acknowledgement.>`

- ...

## Cross-slice / cross-track notes

`<Anything affecting more than one slice or track that needs human-level coordination — data-model migrations, env-var changes, shared infra, dependent-track ordering.>`

- ...
