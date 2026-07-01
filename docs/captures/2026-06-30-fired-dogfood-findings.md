---
title: 'Fired dogfood — first sworn run on a real coach-loop release'
description: 'sworn run --parallel on the live fired repo (release 2026-06-28-yearSnapshot-schema-cleanup, deepseek-v4-pro). The engine cold-started a real release end-to-end; three findings, two fixed live, the third is the D6 type-drift migration.'
date: 2026-06-30
---

# Fired dogfood findings

## Scope
First run of the open `sworn` engine against a real, coach-loop-produced release
in the live fired repo (`~/projects/fired`, release `2026-06-28-yearSnapshot-
schema-cleanup`, 3 slices / 1 track, model `deepseek/deepseek-v4-pro`). Goal: prove
the engine runs the loop on real-world content, not synthetic smoke.

## Verdict: success as a diagnostic
The engine cold-started a real release with zero manual scaffolding — loaded the
monorepo board, adopted the existing release/track worktrees, ran the router,
dispatched S01 — and surfaced three concrete gaps that were invisible to synthetic
tests. The 2026-06-28 eval's "DOA parallel loop" is dead.

## Findings

### 1. Cold-start self-bootstrap — FIXED (committed, proven)
The engine could not cold-start a freshly-planned release (assumed the private
Driver-1 scaffold). Fixed: release-worktree-path default, release-wt branch
auto-create, YAML inline-comment strip, start_commit planned→in_progress bootstrap,
repo-local track worktrees. Proven by the smoke run
(`2026-06-30-cold-start-smoke-proof.md`). Commits `9338b57`, `cda544e`, `04abe06`.

### 2. Monorepo docs-prefix — FIXED (committed)
fired's `docs/` is a git symlink → `apps/docs/content/docs` (Fumadocs monorepo).
git-ref readers can't traverse symlinks in trees, so the router + planned-files
reader couldn't find the board on a ref (the oracle already auto-detected it).
Fixed: `--docs-prefix` flag (default `docs`; `apps/docs/content/docs` for fired)
threaded to the router + planned-files reader + board read. Commit `b7ac4d0`.
The run confirmed it: the board loaded and S01 dispatched.

### 3. D6 type drift — THE BLOCKER (planned step-1b slice, NOT yet done)
sworn's Go types lag the slice-status-v1 schema. The run failed at:
`json: cannot unmarshal object into Go struct field Status.open_deferrals of
type string`. fired's `open_deferrals` are schema-conformant objects
(`{id, description, why, tracking, acknowledged_by}`); `state.Status.OpenDeferrals`
is `[]string`. `Verification.Violations` (`[]string`) almost certainly drifts the
same way. This is the keystone brief's **step 1b / D6**: "migrate Go types UP to
the schemas (`open_deferrals`/`violations` `[]string`→object; `need_ids`→
`covers_needs`)".

**Why no inline hack:** sworn rewrites `status.json` on every slice transition. A
flatten-objects-to-strings read tolerance would rewrite fired's rich deferral
objects back as flat strings — silently degrading real data on a live repo. The
only safe fix preserves the object form (round-trips it) = the proper struct
migration. It is a bounded but careful slice (the field types + ~6 consumers:
`notDeliveredItems`, `notDeliveredFromDeferrals`, `CheckBoundaryMocks`, the mcp
ops append, slice.go pass-through, cmd/verify), explicitly scoped in the keystone
brief as deliberate ("merged last, expect breakage").

## Reachability artefact
Live run output (`scratchpad/fired-run.log`):
```
sworn run --parallel: loaded 1 tracks in 1 phases
[T1-schema-cleanup] running slice S01-networth-hierarchy-remap
[T1-schema-cleanup] slice S01 failed: RunSlice: read status: ... cannot unmarshal
  object into Go struct field Status.open_deferrals of type string
```
The failure is a clean, instant status-read rejection — no commits, no mutation of
fired. Findings 1 + 2 are *proven working* by the fact the run got this far.

## Recommendation
Do **D6 / step-1b** as the next focused slice (fresh context): migrate
`OpenDeferrals` and `Violations` to structured types that round-trip the schema's
object form, update the ~6 consumers, reconcile `need_ids`→`covers_needs`. Then the
fired run completes. This is also a keystone prerequisite (the `baton.Validate`→
`ValidateSchema` rewire surfaces the same drift). Not an inline edit — a slice.

## Not delivered (tracked, Rule 2)
- The fired release itself (3 slices) — blocked on D6, which is the next slice.
- T16 capture remainder (durable store, token enrichment) — #26 / driver-contract S07.
