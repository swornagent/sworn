---
title: 'S36-captain-resolve-dirty-worktree — Captain auto-resolves dirty track worktrees instead of paging the Coach'
description: 'A dirty track worktree is only ever caused by a worker (implementer) leaving uncommitted changes; the Coach has no context to resolve it. The Captain assesses the diff, commits the work by default (or discards only if clearly wrong), records what differed + which files + the resolution, and never pages the Coach for a dirty worktree.'
---

# Slice: `S36-captain-resolve-dirty-worktree`

## User outcome

When the loop reaches a gate that requires a clean track worktree (pre-merge,
pre-forward-sync, pre-replan) and finds the worktree **dirty**, it no longer pages
the Coach. Instead the **Captain** auto-resolves it: assesses the uncommitted diff,
**commits the work by default** (preserving the worker's progress), or discards it
**only if the change is clearly wrong/garbage**, and records — in the slice journal —
what differed, which files were impacted, and what the resolution was. The Coach is
informed via the durable note, not a blocking page.

## Why

A track worktree is dirtied **exclusively by workers** (implementers leaving
uncommitted changes when a session ends or is killed). The Coach has no independent
context to resolve it, so paging the Coach on a dirty worktree is a dead-end gate —
the human can only do what the Captain could do with the same artefacts. This is a
fresh-context tactical decision, which is precisely the Captain's role. Observed
repeatedly: T3/S06a left `account_test.go` + a mock server uncommitted; T8/S24 left 21
dirty files — each blocked merge/replan and (today) would page.

## Entry point

The loop's clean-worktree gates (the points that today `git status --porcelain` and
page/abort on non-empty). On a dirty tree, dispatch the Captain's
`resolve-dirty-worktree` function instead of paging.

## In scope

### Captain function: `resolve-dirty-worktree` (`internal/prompt/captain.md`)

A new Captain function. Given a dirty track worktree it:
1. Captures the diff: `git -C <wt> status --porcelain` + `git -C <wt> diff` (staged +
   unstaged) — the set of impacted files and the nature of the change.
2. **Decides** (default = commit): commit the uncommitted work to the track branch
   with a descriptive message when the change is plausibly the worker's in-progress
   slice work. Discard (`git checkout -- ` / `git clean`) **only** when the change is
   clearly wrong — e.g. a stray build artefact, an accidental mass-deletion, or edits
   to files outside the slice's declared touchpoints with no coherent intent.
3. **Records** the resolution in the slice `journal.md`: the impacted files, a
   one-line characterisation of the diff, the decision (committed / discarded), and
   the rationale. No Coach page.
4. Escalates to the Coach **only** in the genuinely ambiguous case (e.g. the diff mixes
   plausible work with destructive changes and the right split is unclear).

### Orchestration wiring

The detection/dispatch lives in the loop's clean-worktree gates. The sworn-native
successor (`sworn run`) owns this going forward; the current bash harness
(`coach-loop`) is out of this repo's scope but the same contract applies (note in the
journal that the bash harness wiring is tracked separately).

## Out of scope

- Resolving conflicts in a *merge* (that is the merge-track agent's job).
- Recovering a worktree flipped to the wrong branch (sworn#6 / S28 + the harness guard
  already cover that distinct failure mode).

## Planned touchpoints

- `internal/prompt/captain.md` (new `resolve-dirty-worktree` function)
- the clean-worktree gate in the sworn-native loop (verify the current entry point;
  if `sworn run` has no captain-orchestration surface yet, scope to the captain.md
  contract + a deterministic detector and surface the wiring as a tracked follow-up)

## Acceptance checks

- [ ] `captain.md` defines a `resolve-dirty-worktree` function with the decision rule
  (commit-by-default, discard-only-if-clearly-wrong) and the mandatory journal record
- [ ] the contract states the Coach is NOT paged for a dirty worktree except in the
  genuinely-ambiguous escalation case
- [ ] a deterministic detector exists (or is specified) for "track worktree dirty at a
  clean-required gate"
- [ ] the journal-record format (files impacted + diff characterisation + decision +
  rationale) is specified

## Required tests

- **Unit/contract**: a test (or golden check) that the captain prompt contains the
  resolve-dirty-worktree function and its commit-by-default rule.
- **Reachability artefact**: a documented walkthrough — dirty a scratch worktree, run
  the resolution path, confirm the work is committed and the journal records it; no
  page emitted. Document in proof.md.

## Risks

- **Committing genuinely-wrong work.** The "discard only if clearly wrong" bar must be
  conservative in the other direction too — auto-committing broken code to a track
  branch is recoverable (the Verifier gate still runs and will FAIL it), whereas
  discarding good work is not. Bias to commit; let Rule 7 catch bad work downstream.
- The detector must not fire on benign untracked build artefacts (e.g. a stray `sworn`
  binary) — scope the dirty-check to tracked changes + intended touchpoints.

## Deferrals allowed?

The bash `coach-loop` wiring may be tracked separately (the canonical home is
`sworn run`); note it explicitly if deferred.
