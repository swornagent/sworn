---
title: 'S35-mutation-guard — standing Captain check + Baton clause for process-global mutation (cwd / git-state / os.Chdir / global env)'
description: 'Process-global mutation (cwd, git-state, os.Chdir, global env) in tests or CLI code is the class behind sworn#6 (a git op with an empty dir flipped a worktree to main) and was caught again on S28 (os.Chdir → t.Chdir). Add a standing Captain design-review check that flags any design whose tests or code mutate process-global state without a guaranteed restore, plus a Baton-rule clause codifying the guard. Harvested from the trial-log analysis §5 (theme T-F); references sworn#6.'
---

# Slice: `S35-mutation-guard`

## User outcome

A Captain reviewing any slice design is now systematically prompted to flag
**process-global mutation** — `os.Chdir`, a raw `git` invocation with a cwd argument,
worktree creation/switching, or a global env/cwd mutation in tests — that lacks a
guaranteed restore. This makes the sworn#6 class (a git op with an empty dir flipping a
worktree to `main`; recurrence caught on S28 as `os.Chdir`→`t.Chdir`) a **systematic**
catch rather than an incidental one. The same guard is codified as a Baton-rule clause so
it persists beyond the prompt.

## Entry point

The Captain design-review prompt (`internal/prompt/captain.md`) — specifically its standing
review checks. Plus a clause in the Baton rules (`internal/adopt/baton/rules/`). Verifiable
by: reading `captain.md` and confirming a process-global-mutation check is present in the
review function, and reading the rules clause and confirming it states the (a) restore,
(b) non-empty/expected-dir assertion, (c) reachability-artefact requirements.

## In scope

- **Captain check** in `internal/prompt/captain.md`: add a standing check (in the six-step
  review function) that fires whenever a design touches `os.Chdir`, a raw `git` invocation
  with a cwd argument, worktree creation/switching, or a global env/cwd mutation in tests.
  The check requires the design to show: (a) state is restored (`t.Chdir`, `defer`),
  (b) the git op asserts a non-empty / expected dir before running, (c) the slice cannot be
  marked `verified` without a reachability artefact showing the guard.
- **Baton-rule clause** in `internal/adopt/baton/rules/`: codify the process-global-mutation
  guard as a durable rule clause. Read the rules dir first to place it correctly — Rule 2
  (`02-no-silent-deferrals.md`) is specifically about deferrals and is a poor fit; prefer a
  focused new clause (or an addition to the test-isolation surface of Rule 1) so the guard
  reads as a first-class standing check, not a deferral footnote.

## Out of scope

- The in-repo structural fix for sworn#6 itself — that is S28-git-dir-guard (already
  landed), which made `internal/git.Repo.run()` fail closed on an empty `Dir`. This slice
  is the *standing process* guard (Captain check + rule), not another code guard.
- Mechanically scanning code for `os.Chdir` — this is a prompt/rule change (a human/Captain
  judgement gate), not a new `sworn lint` target.

## Planned touchpoints

- `internal/prompt/captain.md` (add the process-global-mutation standing check)
- `internal/adopt/baton/rules/<clause>.md` (add the codified guard clause — exact file
  decided after reading the rules dir; candidate: a focused new clause file, or an addition
  to `01-reachability-gate.md`'s test-isolation surface)

> **Touchpoint note (sequencing):** this slice shares `internal/prompt/captain.md` with
> `S27-public-readiness-scrub`. S27 runs last (track T10 depends on all tracks including
> T12), so **S35 lands first and S27 re-touches `captain.md` afterwards** — sequential, no
> parallel collision. References sworn#6 (github swornagent/sworn#6).

## Acceptance checks

- [ ] `internal/prompt/captain.md` contains a standing check that fires on a design
  touching `os.Chdir`, a `git` invocation with a cwd argument, worktree creation/switching,
  or a global env/cwd mutation in tests
- [ ] that check requires (a) guaranteed restore (`t.Chdir`/`defer`), (b) a non-empty /
  expected-dir assertion before a git op, and (c) a reachability artefact showing the guard
  before `verified`
- [ ] a Baton-rule clause in `internal/adopt/baton/rules/` codifies the same
  process-global-mutation guard
- [ ] the clause references the sworn#6 class (git op with empty dir flipping a worktree to
  `main`) as the motivating bug
- [ ] no Go files changed; `go build ./...` still passes (sanity)

## Required tests

- Markdown/prompt-only slice; the "test" is a doc-content assertion plus a build sanity check.
- **Doc-content check**: grep `internal/prompt/captain.md` for the process-global-mutation
  check and the new rules clause file for the (a)/(b)/(c) requirements and the sworn#6
  reference.
- **Reachability artefact**: quote the new Captain check block and the rule clause in
  `proof.md` (the prose is the user-reachable artefact for a prompt/rule change); run
  `go build ./...` to confirm no incidental Go breakage. Document both in `proof.md`.

## Risks

- Placing the clause in the wrong rule file (e.g. Rule 2, which is about deferrals) would
  bury the guard where it doesn't read as a standing check. Mitigation: read
  `internal/adopt/baton/rules/` first (Rule 2 is `02-no-silent-deferrals.md`; Rule 1 is
  `01-reachability-gate.md` and already owns test-isolation concerns) and place the clause
  where the guard reads as first-class; confirm the placement in the proof bundle.

## Deferrals allowed?

None.
