---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S35-mutation-guard`

## 2026-06-21 — planned (replan)

Added during `/replan-release` to harvest §5 (theme T-F) from the trial-log analysis
(`2026-06-21-captain-trial-log-harvest.md`). Process-global mutation (cwd / git-state /
`os.Chdir` / global env) in tests and CLI code is the class behind **sworn#6** (a git op
run with an empty dir flipped a worktree to `main`) and was caught again on
`S28-git-dir-guard` (`os.Chdir` → `t.Chdir`). The harvest recommends codifying a standing
"process-global mutation guard" so the catch is systematic, not incidental — the design
gate already recognised the pattern on S28; this slice makes it durable.

**Rationale:** S28 fixed the code (git fails closed on empty `Dir`); S35 adds the *process*
guard — a standing Captain check plus a Baton-rule clause requiring (a) restore, (b) a
non-empty/expected-dir assertion before git ops, (c) a reachability artefact showing the
guard before `verified`. References sworn#6 (github swornagent/sworn#6).

Placed in new track `T12-harness-hardening` (depends_on `T1-concurrency-core`).

**Sequencing note:** S35 shares `internal/prompt/captain.md` with `S27-public-readiness-scrub`.
S27 runs last (track T10 depends on all tracks including T12), so S35 lands first and S27
re-touches `captain.md` afterwards — sequential, no parallel collision.

## Open questions

- Exact rules-clause placement: the brief said "likely Rule 2 or a new sub-rule." Rule 2
  (`02-no-silent-deferrals.md`) is specifically about deferrals and is a poor semantic fit;
  the implementer should confirm placement after reading `internal/adopt/baton/rules/` —
  a focused new clause file, or an addition to `01-reachability-gate.md`'s test-isolation
  surface, may read better. status.json currently lists `02-no-silent-deferrals.md` as the
  placeholder touchpoint pending that decision.

## Deferrals surfaced

None.

## Verifier verdicts received

None yet.

## 2026-07-03 — implemented

Entered at `design_review` with Coach-approved ack (PROCEED). Captain pin: 1
mechanical (populate `design_decisions` in status.json) + 2 minor flags.

### Design decisions (all Type-2, ratified in `design_decisions`)

1. **Rule clause placement:** new `11-process-global-mutation.md` — not an
   addition to Rule 2 (no-silent-deferrals) or Rule 1 (reachability-gate). A
   dedicated clause reads as a first-class standing check and can cite sworn#6
   directly.
2. **Captain check placement:** new Step 7 "Process-global mutation guard" in
   the review function, inserted after Step 6 (inter-slice handoffs) and before
   `## Output`. Keeps the existing six-step structure intact.
3. **Four-pattern scope:** fires on exactly `os.Chdir`, raw `git` with cwd,
   worktree creation/switching, and global env/cwd mutation in tests. No
   additional patterns — the spec's in-scope list is precise.

### Mechanical registrations (beyond spec's planned_files)

Adding a new rule file in the vendored baton directory required updating three
registration surfaces:
- `internal/adopt/adopt.go` — `files` slice for `sworn init` extraction
- `cmd/sworn/doctor.go` — `batonRuleFiles` list for `sworn doctor` checks
- `cmd/sworn/doctor_test.go` — expected rule count 10/10 → 11/11

These are registration-only; no logic change. The embed directive
(`//go:embed baton/rules/*`) already auto-covers new files.

### Captain flags addressed

- (a) S36-captain-resolve-dirty-worktree also touches `captain.md` — sequential
  in T12, no collision. Step 7 inserted between Step 6 and Output; S36 should
  land cleanly after it. Named in proof.md.
- (b) §4 stale `planned_files` reference to `02-no-silent-deferrals.md` — the
  NOT-doing item is correct; the supporting rationale just cited a prior state.
  No action required.

### Reachability

The reachability artefact is the prose itself — Captain Step 7 and Rule 11
clause quoted verbatim in proof.md. `go build ./...` passes as sanity check.

### Panel

Skeptic panel skipped — the runtime does not support subagent dispatch in this
session configuration. Noted here per implement-slice.md Step 5. The real
verifier (Rule 7) is the backstop.