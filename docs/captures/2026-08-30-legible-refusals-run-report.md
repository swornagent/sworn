# 2026-08-28-legible-refusals - run report

**Outcome: MERGED.** Six slices, all verified PASS, assembly composed at
`b45806f6` on `release/v1.0.0`. One plan revision (operator-carried), four
run journals (r1-r3 abandoned to engine walls, r4 delivered), one operator
engine patch minted mid-run (patch 9), and an evidence-gates convergence
round of six test-side fixes with zero product changes (`05497403`).

Theme delivered: every refusal names its mechanism. The release proved its
own thesis twice over - first by needing it (three of the four run walls
printed bare codes that took debug builds to translate), then by using it
(S1's named sandbox refusal diagnosed the #251 flake on sight; the e2e
harness now recovers by re-running exactly the check the S5 refusal names).

## Slice results (r4, revision 2 authority)

| Slice | Fixes | Outcome | Attempts |
|---|---|---|---|
| S1 sandbox refusal detail | #251 | pass | 1 |
| S2 submission refusals name the field | #245 | pass | 3 (amended contract) |
| S3 re-seal without a code change | #246 | pass | 2 |
| S4 automation refusal legibility | #250 | pass | 1 |
| S5 verification integrity | #249 | pass | 4 |
| S6 attested operator answers | #247 | pass | 1 |

Assembly verification parked once (3x `adapter_failed` capture discards,
each after a real ~49-minute verifier session); `sworn retry` - first live
use of the verb - passed on try 4.

## Revision 2 (the S2 escalate)

S2's captain escalated legitimately at design: A3's content floor in
`toolSession.submit` necessarily refuses below-floor fixtures in
`cmd/sworn/tui_pty_linux_test.go` and the `test/e2e` production-lane
providers, unlandable under `scope.include ["internal/driver"]`. The
operator ruled: widen scope by exactly those three test files; A3 lands
unreshaped with a fixture-elevation obligation; a new constraint bars
widening admission via `plainDetailCode`; the false `cmd/sworn` waiver
reason corrected. The r3 planner authored the bytes (sealed
`sha256:53696f85...`, 17507B), the operator verified the full contract
diff and carried the record per the R3/R5 precedent: committed `90874172`,
`sworn plan record` -> revision 2 APPROVED (blob `50ab8d07`, release-wt
`ee50d06d`), r4 manifest bound to the new digest.

## The four walls (each cost a journal or a patch)

1. **r1 - INVALID_PLAN, detail-less.** S2's escalate put the run in a
   revision-2 planner cycle; the planner (misled by an operator
   confirmation that had not checked the escalate) sealed a revision-1
   retention into the revision-2 slot. Every restart replays the seal
   through `validatePlanBinding`, which fails `PLAN_REVISION_MISMATCH` -
   and `scheduler.go:6481` discards that error. Permanent wall; no verb
   voids a sealed effect.
2. **r2 - seal-slot deadlock.** The first proposal sealed without the
   `contracts` member; `validateProposalContracts` fails it on every
   replay while the correction loop re-drives the worker, whose corrected
   submissions can never replace the occupied seal slot. Three ~120k-token
   planner turns burned before abandonment.
3. **r2 late / r3 - false CORRUPT_JOURNAL.** `recoverHumanParkCheckpoint`
   assumes one human park per claimed dispatch; the planner dispatch
   parked twice (confirmation yield, then a bytes-unrecoverable yield
   after its drafted plan died with its turn environment). Operator patch
   9 tolerates resolved historical checkpoints.
4. **r3 - unapprovable proposal.** A bootstrap-manifest run refuses any
   in-run revision approval: run start dies `PLAN_AUTHORITY_CONFLICT`,
   `sworn approve` dies `APPROVAL_AUTHORITY_CONFLICT`
   (`approval.go:368`). The engine admits a proposal it can never accept;
   record-and-relaunch is the only path.

## Findings for the issue batch (11)

F1 `INVALID_PLAN` swallows the binding error (6481/6996) · F2 seal/replay
validation asymmetry walls journals · F3 no draft persistence across a
yield boundary (digest pins undeliverable; planner deadlock shape) · F4
#250 third live repro (`ask_captain` with answer ->
`AUTOMATION_BINDING_MISMATCH`) · F5 double-park false `CORRUPT_JOURNAL`
(patch 9 carries the fix shape) · F6 stale seal squats the slot while the
correction loop burns full turns forever · F7 the revision dispatch hides
its own cause - two independent planners proposed rev-1 retention, never
reading the escalate receipt · F8 bootstrap-manifest runs admit
unapprovable proposals · F9 a delegated captain decision that fails
binding revalidation of a sealed submission loops silently (found only by
instrumentation; surfaced by S2's raise sweep adding a trailing newline
that `validCaptainDecisionText` refuses) · F10 contract checks exclude
`./test/e2e`, so slice verification cannot see e2e breakage · F11 the S1
naming tests skip under nested containment, so the slice verifier never
ran them (the #248 skip-prone-fixture class).

Operator lessons recorded alongside: read the dispatch context before
answering any yield; never digest-pin across ephemeral turn environments;
never host a drive in a foreground Bash with a timeout (the R5 SIGTERM
lesson, re-learned once).

## Evidence gates

Round 1 red: 17 failures, 4 mechanisms, all harness-side (the release
changed what the engine demands; out-of-scope fixtures predated it).
Convergence commit `05497403`: six test files, zero product changes. Final
round fully green - product suite, `internal/driver -race`, and
`./test/e2e`: 14/14 ok.

Ops home: `~/.local/share/sworn/sworn/2026-08-28-legible-refusals/ops`
(binary lineage, patches 8+9 archive, monitors, r1-r4 manifests, and
`r2-operator-findings.md` with the full mechanism detail).

## CI close

Green at `1ace3944` (run 33287977723), every step verified individually:
product tests, end-to-end, race, vet, darwin build, official binary,
formatting. Five rounds to get there; the flake ledger (in the ops
findings file) attributes each: two real harness gaps fixed
(`ab2a52cb` recoveryE2E resume tolerance, `1ace3944` bounded evidence
reruns), plus three distinct pre-existing load flakes - the #251
sandbox-start class (which S1's detail envelope named in CI for the
first time: `sandbox_start.process_group_handshake_read`), one
first-sighting post-answer park, and one drive-survival kill-timing
race. F12 added for the last: unbounded-retry-against-flaky-dependency
is the recurring shape (engine F6, the harness evidence loop, and the
correction loop all share it).
