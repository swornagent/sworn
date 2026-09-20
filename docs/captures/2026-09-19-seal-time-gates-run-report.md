# 2026-09-18-seal-time-gates: run report

First release driven end to end by the Manager seat (ADR-0014 Track A
acceptance, Track E). Run `2026-09-18-seal-time-gates-r1`, one track, one
slice, on main 6bd059ff.

## Outcome

Merged. S1-seal-time-gates: design accepted by the Lead on the first
review, implementation sealed on the first try, all eight declared host
checks passed, Verifier PASS on the first attempt, assembly verdict and
merge. Zero operator interventions: the decision journal holds the launch
entry and the end-of-release entry and nothing between them.

## Timeline (AEST, from the status watch)

| Offset | Time | Event |
|---|---|---|
| 1 | 22:20 | run registered, authority approved |
| 4 | 22:21 | first dispatch claimed (design) |
| 16 | 22:32 | Lead PROCEED; implementation seal cycle claimed |
| 18 | 23:21 | candidate sealed; quick host checks running |
| 19-20 | 23:27 to 00:13 | long suites (product, serial e2e, race) |
| 33 | 00:24 | seal cycle closed with eight PASS; Verifier dispatched |
| 36 | 00:34 | Verifier PASS |
| 38 | 00:35 | assembly verdict, merged, run complete |

Wall clock 2 h 15 min from start to merge: about 11 min design and review,
49 min implementation, 63 min host checks, 10 min verification.

## What was delivered

The seal-time gates verified on 2026-09-11-phased-evidence (candidate
fc8edaa0) ported onto the renamed engine and reconciled with #311 to #314:
quick declared checks run before any long suite; an acceptance anchor the
candidate touches no file of refuses the seal with a typed code that binds
as repair context; a degenerate submission body refuses at the author-side
boundary with both measured ratios; substitute anchors are declared on the
submission surface; docs/run.md documents the gates. Product diff against
the prep commit: 20 files, 1855 insertions, 35 deletions.

## Verification of the merged head

Target head 0ecb05d6 checked out in a fresh worktree; all twelve non-e2e
packages green, one package at a time. The full end-to-end suite is CI's
witness on the promotion pull request.

## Seat observations

- The projection was enough. Every tick was decided from `sworn status
  --json` and the watch line; no journal was opened.
- Launch, not the run, was where the cost went: the manifest must be the
  Go encoder's canonical bytes plus a newline, `plan pin` prints rather
  than writes, `plan record` needs 40-character ids, the operator config
  must be mode 0600, and `sworn serve` reports every launch failure as one
  generic line. Each is a legibility gap for the seat and is filed.
