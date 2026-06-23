---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal — S48-baton-vendor

## Planner routing — 2026-06-23 (replan; BLOCKED was misrouted)

**Actor**: planner (`/replan-release`)

The verifier returned BLOCKED with reason "track worktree has 19 uncommitted modifications;
implementer must clean the working tree" and routed to `/replan-release`. The verifier's own
verdict states **there is no spec defect** ("the spec contract is intact; this is an
implementation/working-tree hygiene fault"). A spec-intact, working-tree fault is **not** a
planner concern — there is no planning artefact to correct. Routing it to the planner was a
misroute; it is owned by the **implementer** (and/or the S36 Captain dirty-worktree mechanism).

**Additional finding (planner inspection of the T14 worktree):** the 19 uncommitted modifications
are the output of running `sworn baton vendor`, and that output is **corrupt** — the transform
degrades the real Baton rules to degenerate stubs. Example: `internal/prompt/baton/rules.md`
collapses from 1112 lines of actual rule content to 29 lines of `# Rule: test` / `No scripts.`
(the verifier measured ~3,596 net deletions). The committed embed (HEAD) is correct; re-running
the transform produces garbage. So this is **not** just a forgotten commit — S48's transform is
**lossy / non-deterministic**, which fails the spec's own determinism acceptance check
("running it again on the same pin produces an identical embed").

**Action for the implementer (re-enter via `/implement-slice S48-baton-vendor`):**
1. **DISCARD** the broken working-tree output — do **NOT** commit it:
   `git checkout -- internal/adopt/baton internal/prompt` (restores the good committed embed).
2. **Fix the transform** in `internal/baton/transform.go` so re-running `sworn baton vendor`
   reproduces the committed embed instead of stubbing the rules to "No scripts." — the
   substitution map / regex is over-stripping rule content, not just script references.
3. Confirm a clean tree, re-prove (incl. the idempotency/determinism test), re-stage as
   `implemented`, and re-run `/verify-slice`.

State set to `failed_verification` to route this to the implementer (the verifier's intent —
"the implementer must clean / fix"). No planner spec change was made; the spec is correct.

**Propagation note:** the T14 worktree is dirty, so this replan did **not** forward-merge
`release-wt` into it (track-mode: never merge into a dirty worktree). This routing reaches T14
when the implementer's next `/implement-slice` Step 0 runs — after the broken output is discarded.
