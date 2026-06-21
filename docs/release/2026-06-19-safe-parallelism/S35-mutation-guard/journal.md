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
