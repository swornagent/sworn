---
title: Track mode — safe parallelism for release work
description: The canonical model for running release slices in parallel. Tracks group slices into disjoint, sequentially-implemented units, each in its own worktree. Referenced by /plan-release, /implement-slice, /verify-slice, /merge-track, /merge-release.
---

# Track Mode

This document is the **single source of truth** for how release work is parallelised. The role prompts (`role-prompts/planner.md`, `implementer.md`, `verifier.md`) and the slash commands (`/plan-release`, `/implement-slice`, `/verify-slice`, `/merge-track`, `/merge-release`) all defer to the definitions here. When they disagree with this file, this file wins — fix the role prompt.

## The problem track mode solves

A release contains many slices. Some are independent and could be built at the same time; the planner routinely identifies "parallel batches." But the older **one-worktree-per-release** model could not actually run them in parallel:

- **Shared git index.** All slices landed on one `release-wt/<release>` branch in one worktree. Two concurrent implementer sessions shared one index — one session's `git commit` swept the other's staged files into the wrong commit.
- **Interleaved commits.** Slice A, B, and C commits interleaved on the shared branch. No single commit range isolated a slice, so neither the verifier (scoping its diff) nor a merge (selecting one slice) could cleanly address one slice.
- **Scattered recovery refs.** The `release/slice/<slice-id>` "recovery anchor" got treated as a real branch and merged from, scattering a slice's commits topologically so a later merge silently dropped it.

Track mode fixes all three at the root: it gives each parallel unit of work its **own worktree** (own index), keeps each unit's commits **contiguous on its own branch**, and makes the **track branch itself** the durable home — no separate recovery ref to misuse.

## Definitions

- **Release** — a themed body of work, folder `YYYY-MM-DD-<theme>`, targeting a version integration branch (e.g. `release/v0.5.0`).
- **Slice** — a single user-reachable outcome; one implementer session + one verifier session. Id `S<NN>-<short-kebab-name>`.
- **Track** — an **ordered sequence of slices** that (a) is implemented **sequentially in a single worktree**, and (b) whose file touchpoints are **collectively disjoint** from every other track in the release. Id `T<N>-<short-kebab-name>` (e.g. `T1-identity-account`).

A track of one slice is normal and fine. A track is the unit of parallelism; a slice is the unit of implementation and verification.

## Branch and worktree hierarchy

```
<version integration branch>            e.g. release/v0.5.0   — production-bound
  ▲   /merge-release   gate: every track merged
  │
release-wt/<release>                     the release assembly branch — ONE worktree
  ▲   /merge-track     gate: every slice in the track verified
  │
  ├── track/<release>/T1-<name>          worktree A ┐
  ├── track/<release>/T2-<name>          worktree B ├─ run in parallel
  └── track/<release>/T3-<name>          worktree C ┘
        └── slices commit directly on the track branch, in sequence
```

Three levels, three branch families:

| Level | Branch | Worktree | Created by |
|---|---|---|---|
| Version | `release/v*` (or `main`) | primary repo (`<REPO_ROOT>`) | pre-exists |
| Release | `release-wt/<release>` | `$HOME/projects/<REPO_BASENAME>-worktrees/release-<release>` | first `/implement-slice` of the release |
| Track | `track/<release>/<track-id>` | `$HOME/projects/<REPO_BASENAME>-worktrees/release-<release>-<track-id>` | first `/implement-slice` of that track |

`<REPO_ROOT>` is the primary worktree's absolute path (`git rev-parse --show-toplevel` from the project's main checkout); `<REPO_BASENAME>` is `basename "<REPO_ROOT>"`, used to namespace the worktrees folder so multiple projects on the same machine don't collide.

Both worktree levels are materialised **lazily** — the planner creates no worktrees. The release worktree is created by the first `/implement-slice` in the release; each track worktree is created by the first `/implement-slice` in that track and branches **from `release-wt/<release>`**.

## The safety invariants

Parallelism is safe **only** while all four hold. Every command enforces one or more of them.

1. **One worktree per track; one implementer at a time; slices sequential.** A track worktree has its own working tree and its own git index, so concurrent implementers in *different* tracks cannot race. Within a track, slices are done one after another — never two implementers in one track worktree.
2. **Tracks are touchpoint-disjoint.** No file is written by two tracks — with one narrow, documented exception. The planner proves disjointness with the **touchpoint matrix** (below). **Documented region-split exception:** a large, append-mostly module (a shared types file, a registry, a barrel export) MAY appear in two tracks IF the touchpoint matrix row records the specific, well-separated region/symbol each track edits and they do not overlap. Such a row is a *documented shared file*. This exception is for genuinely additive, region-separable modules only — never a component, a hook, or a logic file.
3. **A track branch is linear and contiguous.** Because a track's slices are sequential, the track branch carries only that track's commits, in order. A slice's diff is therefore exactly `start_commit..HEAD` — no commit-range archaeology.
4. **A conflict at `/merge-track` is a planner error — except on a documented shared file.** Invariant 2 means track branches never touch the same file *except documented shared files*, so `track → release-wt` merges are conflict-free on every other file. A conflict on `index.md` or a **documented shared file** is expected reconciliation, and `/merge-track` resolves it. A conflict on **any other file** means the touchpoint matrix was wrong: `/merge-track` BLOCKs and the release returns to `/plan-release` (or `/replan-release`) to re-group.

## The touchpoint matrix — the planner's load-bearing artefact

After decomposing a release into slices, the planner groups slices into tracks and **proves disjointness** with a matrix in `index.md`. Every file (or narrow surface) any slice will touch gets a row; every track gets a column. A `✓` marks intent to write.

```
| File / surface                                  | T1 | T2 | T3 |
|--------------------------------------------------|----|----|----|
| src/components/.../AccountProfileSection.tsx     | ✓  |    |    |
| src/components/.../DataTableRow.tsx              |    | ✓  |    |
| src/components/.../NotificationSync.tsx          |    |    | ✓  |
```

**No row may have a `✓` in more than one column** — except a *documented shared file* (invariant 2). If decomposition cannot achieve single-column rows, the colliding slices belong in the **same track** (serialised), or one track is declared **dependent** on another (see below). The matrix is a contract: the implementer surfaces any touch outside its track's rows as a collision (not a silent addition), and `/merge-track`'s conflict check is the mechanical backstop.

**Documented shared files.** A large, append-mostly module may carry a `✓` in two columns *if the row names the distinct region/symbol each track edits* and they are well-separated and non-overlapping. Write the row as, e.g. `| lib/types.ts (DOCUMENTED SHARED) | ✓ InvoiceFormState | ✓ CheckoutPlan |`. `/merge-track` reconciles a conflict on such a row instead of BLOCKing. Use it only for genuinely additive, region-separable modules (a types file, a registry) — never a component, hook, or logic file. If the regions turn out to overlap at merge time, that is still a planner error and `/merge-track` BLOCKs.

## Cross-track dependencies

If slice B needs slice A's code, A and B share a surface — they are not touchpoint-disjoint. Two legal resolutions, decided by the planner:

- **Same track.** Put A before B in one track. Default choice; simplest.
- **Dependent track.** Track T2 branches from T1's tip *after T1 merges to `release-wt`*. Record this in `index.md` as `T2 depends on T1`. T2's worktree is created only once T1 has merged. Use this only when the two bodies of work are large enough to deserve separate tracks despite the dependency.

Independent tracks are the common case; dependencies are the exception and must be explicit on the board.

## Lifecycle

1. **`/plan-release`** — planner decomposes into slices, groups slices into tracks, builds the touchpoint matrix, records tracks + slices in `index.md`. No worktrees, no code.
2. **`/implement-slice <slice>`** — discovers the slice's track from `index.md`; materialises the release worktree (if first slice in the release) and the track worktree (if first slice in the track); implements one slice; terminal state `implemented`.
3. **`/verify-slice <slice>`** — fresh context; discovers and operates inside the track worktree; adversarial verification; terminal state `verified` or `failed_verification`.
4. Repeat 2-3 for each slice of the track, **in order**, in the **same track worktree**.
5. **`/merge-track <track-id>`** — gate: every slice in the track is `verified`. Merges `track/<release>/<track-id>` → `release-wt/<release>` with `--no-ff`. Conflict ⇒ BLOCK (invariant 4).
6. **`/merge-release`** — gate: every track is merged into `release-wt/<release>` (which implies every slice verified). Merges `release-wt/<release>` → version branch with `--no-ff`.

`/replan-release` revises a release that is already in flight — adding unplanned scope, re-scoping or dropping slices, re-grouping tracks. It reconciles true board state from both the integration branch and the track worktrees before proposing changes.

## Naming, locked

| Thing | Format | Example |
|---|---|---|
| Release folder | `YYYY-MM-DD-<theme>` | `2026-05-19-uat-bug-fix` |
| Slice id | `S<NN>-<short-kebab-name>` | `S04-scenario-card-always-visible` |
| Track id | `T<N>-<short-kebab-name>` | `T1-identity-account` |
| Release branch | `release-wt/<release>` | `release-wt/2026-05-19-uat-bug-fix` |
| Release worktree | `$HOME/projects/<REPO_BASENAME>-worktrees/release-<release>` | `.../release-2026-05-19-uat-bug-fix` |
| Track branch | `track/<release>/<track-id>` | `track/2026-05-19-uat-bug-fix/T1-identity-account` |
| Track worktree | `$HOME/projects/<REPO_BASENAME>-worktrees/release-<release>-<track-id>` | `.../release-2026-05-19-uat-bug-fix-T1-identity-account` |

## Recovery — the track branch is its own anchor

After every commit on a slice, the implementer pushes the track branch:

```
git push origin HEAD:refs/heads/track/<release>/<track-id>
```

`origin/track/<release>/<track-id>` is the durable home of the track's work. It survives a force-rebase of any branch above it; recovery is `git fetch && git reset --hard origin/track/<release>/<track-id>`. Unlike the retired `release/slice/<slice-id>` ref, this **is** the track branch — there is no separate "anchor" that can be mistaken for a branch and merged from. `/merge-track` merges the track branch and only the track branch.

This supersedes the older `release-mode-slice-ref.md` convention entirely.

## Session handoff — handing off blocked work

The track branch is the recovery anchor against an upstream rebase. It is also the **handoff anchor** when an implementer must abandon a slice mid-flight — an environmental fault, a discovered blocker that needs human input, a `dependent-on-bug` halt. The pattern:

1. Commit the half-authored work with an honest message naming the blocker.
2. Push the track branch (`git push origin HEAD:refs/heads/track/<release>/<track-id>`) **before the session ends** — not after.
3. Transition the slice's `status.json` to its blocked state, with `verification.notes` (or the journal) naming the blocker and the recovery path.
4. End the session. Do **not** revert or stash the work — the commit on the pushed track branch is the durable artefact.

When the blocker clears, the next `/implement-slice <slice-id>` session resumes inside the track worktree from the track branch, reading the blocker context. No re-authoring, no merge-conflict drama against work that landed elsewhere meanwhile.

**Why the branch beats `git stash` for handoff:** a stash is *machine-local* — invisible to any other session, on any other machine. The track branch is on `origin`, and the next session may run on a different machine, or with a different operator. The pushed branch is the only artefact that crosses both the session boundary and the machine boundary.

## Where the discovery data lives

`index.md` frontmatter is the machine-readable registry the commands read:

```yaml
release_worktree_path: <absolute path, set by first /implement-slice in the release>
release_worktree_branch: release-wt/<release>
tracks:
  - id: T1-identity-account
    slices: [S03-..., S07-...]      # ordered
    depends_on: null                 # or another track id
    worktree_path: <set by first /implement-slice in this track>
    worktree_branch: track/<release>/T1-identity-account
    state: planned                   # planned | in_progress | merged
```

The **Tracks table** and **touchpoint matrix** in the body of `index.md` are the human-readable mirror, kept in sync the same way the slice table mirrors each `status.json`. Each slice's `status.json` carries its `track` id and, once implementation starts, its `start_commit` (the SHA of the `docs(...): start implementation` commit) — the verifier diffs `start_commit..HEAD`.
