# Design TL;DR — S35-mutation-guard

## §1. User-visible change

When a Captain runs `/design-review` on any slice, the review function now includes a
standing check that flags any design whose tests or code mutate process-global state
(`os.Chdir`, a raw `git` invocation with a cwd argument, worktree creation/switching, or
a global env/cwd mutation in tests) without a guaranteed restore, a non-empty-dir
assertion before git ops, and a reachability artefact showing the guard. The same guard
is codified as a new Baton rule clause (`11-process-global-mutation.md`) so it persists
as a first-class rule beyond the prompt. This makes the sworn#6 class (a git op with an
empty dir flipping a worktree to `main`; recurrence caught on S28 as `os.Chdir` →
`t.Chdir`) a **systematic** catch rather than an incidental one.

## §2. Design decisions not in spec (3)

1. **Rules clause placement: new file `11-process-global-mutation.md` (not Rule 2 addition).**
   The spec's Risk section flags Rule 2 as a poor semantic fit and prefers a focused new
   clause or an addition to Rule 1. After reading `internal/adopt/baton/rules/`, a focused
   new clause is the better choice: Rule 1 is about reachability-through-integration-points,
   not about process-level mutation; a dedicated clause reads as a first-class standing
   check and can cite sworn#6 as its motivating bug directly.

2. **Captain check placement: new Step 7 in the review function.**
   The existing six steps cover drift, memory, design-fit, inference, cross-stack drift,
   missing-prereq audits, and inter-slice handoffs. None naturally owns process-global
   mutation. Adding a Step 7 "Process-global mutation guard" after Step 6 (inter-slice
   handoffs) keeps the review function's existing structure intact and makes the new
   check discoverable as a named step.

3. **Check fires on exactly the four patterns the spec enumerates:** `os.Chdir`, a raw
   `git` invocation with a cwd argument, worktree creation/switching, and a global
   env/cwd mutation in tests. No additional patterns — the spec's in-scope list is precise.

## §3. Files I'll touch grouped by purpose

- **Captain prompt** — `internal/prompt/captain.md`: add Step 7 (process-global mutation
  guard) to the review function, between Step 6 (inter-slice handoffs) and the Output
  section. Why: this is where the Captain's standing checks live; the new check must be
  part of the same walk.

- **Baton rules** — `internal/adopt/baton/rules/11-process-global-mutation.md`: new rule
  clause codifying the (a) restore, (b) non-empty-dir assertion, (c) reachability-artefact
  requirements, with sworn#6 cited as the motivating bug. Why: this is where all Baton
  rules live; numbering at 11 follows the existing 01–10 sequence.

## §4. Things I'm NOT doing

- **NOT modifying `02-no-silent-deferrals.md`.** The spec and journal both flag this as a
  poor semantic fit. `status.json`'s `planned_files` lists it only as a placeholder
  pending the implementer's rules-dir read.
- **NOT adding Go code or a `sworn lint` target.** This slice is a prompt/rule change
  only — no production code. The spec explicitly excludes mechanical scanning.
- **NOT touching S27's planned captain.md additions.** S27 is in track T10 (depends on
  all tracks including T12), so S35 lands first; S27 re-touches `captain.md` afterwards
  — sequential per the spec's touchpoint note.

## §5. Reachability plan

The "user-reachable artefact" for a prompt/rule change is the prose itself — the new
Captain check block and the new rule clause, quoted verbatim in `proof.md`. Sanity
check: `go build ./...` to confirm no incidental Go breakage from the rules-dir
addition (rules are `.md`, not compiled, but `embed` directives elsewhere may scan
the dir).

## §6. Open questions for the Coach

None.