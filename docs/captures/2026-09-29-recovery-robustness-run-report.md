# 2026-09-25-recovery-robustness: run report

Release for ADR-0014 Track B release 3 (recovery robustness): the follow-ups
#355, #357, #358, #359 (the half #360 left open) and #361 from release 2.
One track, driven by the Manager seat across three runs,
`2026-09-25-recovery-robustness-r1` to `-r3`. Plan revision 1 (five slices,
plan sha256 a5006745, approved by the Principal 2026-09-26) ran in r1 and r2;
revision 2 (the same five slices plus five repair slices S6 to S10, plan
sha256 6b3ae2f6, approved by the Principal 2026-09-28) ran in r3.

## Outcome

Merged. All ten slices verified PASS by the in-run Verifier. r2 assembled and
merged S1 to S5 (assembly PASS d3264043, merge c8236ac4); r3 carried S6 to
S10 and protocol.merge moved the release branch to 051532ba on 2026-09-29.
The release closes #355, #357, #358, #359 and #361.

| Slice | Lead design reviews | Sealed candidates / failed tries | Verifier |
|---|---|---|---|
| S1-host-check-environment-failures | revise, revise, proceed | 1 / 2 | PASS 0eeda0fb |
| S2-pause-safe-host-checks | revise, revise, proceed | 1 / 0 | PASS f1dbad3d |
| S3-credential-lifetime | revise, revise, proceed | 4 / 6 | FAIL, FAIL, FAIL, PASS c4d0be04 |
| S4-broker-budget-and-turn-cap | revise, proceed | 1 / 3 | PASS 3f4f1d0c |
| S5-repair-input-across-epochs | proceed | 1 / 1 | PASS 1fa5d724 |
| Assembly (r2) | | 1 | PASS d3264043, merge c8236ac4 |
| S6-host-environment-park-projection | revise, proceed | 1 / 0 | PASS 9b1d34f7 |
| S7-pause-safe-host-checks-repair | proceed | 1 / 0 | PASS f69a7fb4 |
| S8-credential-lifetime-repair | revise, proceed | 2 / 0 | FAIL, PASS 05ec087c |
| S9-broker-budget-and-turn-cap-repair | revise, revise, proceed | 1 / 0 | PASS 83b47b7f |
| S10-repair-input-across-epochs-repair | proceed | 1 / 0 | PASS b15f439b |
| Assembly (r3) | | 1 | PASS 22d99f82, merge 38ddba4f |

21 Lead reviews and 16 Verifier reviews in all. Twelve implementation tries
failed before a candidate sealed, none of them in r3: four anchor-gate
refusals (all S3, the carry-over gap S5 then fixed), three host check
failures (S1 e2e, S3 and S5 unit), two credential expiries
mid-dispatch (#358, S1 and S3) and three usage-limit failures read as
transport failures (#369, S4). The Verifier failed four sealed candidates:
three in S3 (an undisclosed change to the identical-failure park, a removed
guard, and a deleted regression test whose receipt claimed no test file
changed) and one in S8 (a 9 MB compiled test binary committed outside the
contract's scope and missing from the receipt).

Roster: Planner and Lead claude-opus-5-5 throughout; Implementer
claude-sonnet-5. The Verifier was claude-opus-5-5 in r1 (S1) and
claude-fable-5-1 from r2 on; recovery moved from claude-sonnet-5 to
claude-haiku-4-5 at the same point. The roster change was a Principal
decision, made between runs at a verdict boundary.

## Runs

| Run | Window (UTC) | Why it ended | Dispatches | Host checks (failed) |
|---|---|---|---|---|
| r1 | 09-25 20:18 to 09-26 00:29 | S1 verified; cancelled at the first dispatch boundary after S1 for the roster change | 11 | 15 (1) |
| r2 | 09-26 00:30 to 09-27 02:56 | S2 to S5 verified, assembly PASS, merged; two parks on the way (S3 try exhaustion, S4 usage limit); cancelled 09-28 11:49 after the target move described below | 43 | 68 (2) |
| r3 | 09-28 11:49 to 09-29 00:53 | S6 to S10 verified with no park, assembly PASS, merged | 31 | 58 (1) |

Dispatches: Lead 21, Implementer 48 (22 design, 26 implementation), Verifier
16. Recovery is an engine-run classifier, not a dispatched role; it ran 16
times across the three runs. r2's S4 usage-limit park sat for 5 h 56 min
before the limit reset and the seat retried.

r2's assembly reused the host checks already recorded for its tree and ran
none itself. r3's assembly ran ten host checks on the assembly candidate,
2 h 25 min in all: the e2e suite ran once with revision 1's `-parallel=1`
spelling (it failed on a 120-second wall-clock deadline in
`TestRealBinaryDelegatedLeadProceedInstallsRC14AndContinuesSerially` and
passed on the engine's rerun) and again with revision 2's `-parallel=8`
spelling. That duplication and the flake are recorded on #368.

## What was delivered

Revision 1:

- S1 (#355): a host check that cannot run its command is a host environment
  failure, not a candidate failure. It parks at once with a typed code naming
  the check and the missing command, spends no try, is never stored for
  replay, and a run refuses to start when a declared check's command does not
  resolve on the host that will run it.
- S2 (#357): pausing or cancelling while a candidate's host checks run stops
  at the next check boundary; passing results stay recorded, the try is not
  spent, and the work resumes where it stopped.
- S3 (#358): a native Claude dispatch never starts with a credential that
  will expire before the dispatch can finish, and a mid-dispatch
  authentication failure carries a typed credential code instead of a
  transport failure.
- S4 (#359): a native dispatch that exhausts the tool broker's call budget or
  the CLI's turn cap ends at once with a typed cause instead of idling until
  the CLI gives up.
- S5 (#361): an implementer always receives the reason its work was last
  refused, across epochs, is never handed a repair a later try already fixed,
  and is told how to declare an anchor substitute.

Revision 2 (repairs of defects found in S1 to S5 after they had passed; see
"Blind shadow verification" below):

- S6 repairs S1: a host environment park projects as parked with cause
  `host_environment` wherever the run is read, while serve holds the run and
  after; every park path is proven through the real entry points.
- S7 repairs S2: pausing the same candidate more than once, at any mix of stop
  points, never fails the dispatch or spends the try; a pause during a
  resume's recovery sweep is a stop, not a CLI error.
- S8 repairs S3: a credential-lifetime park clears itself for every stage once
  the credential is refreshed; the admission check uses the dispatch's own
  timeout; the CLI's authentication failure is recognised in the form the CLI
  actually emits.
- S9 repairs S4: a dispatch ended by the call budget or the turn cap reports
  how many tool calls it executed and refused, even when the CLI never emitted
  its final result.
- S10 repairs S5: a fixed repair is never replayed whatever the seal said
  about the later try, and the evidence for the anchor-substitute guidance and
  the cross-epoch refusal tests the production path, not text that is always
  present.

## Engine changes landed on main during the release

- #366 (fixes #365): a native adapter may opt in to snapshotting the host's
  CLI once per run instead of pointing at a hand-kept pinned copy; readiness
  commands resolve the snapshot from the host. Merged 2026-09-26 as 081905ca
  and merged into the release branch before promotion.

## Verification of the promotion head

Verified in a separate worktree on release 051532ba merged with main
081905ca (ab6d7dcf), 2026-09-29 10:56 to 11:41 AEST, with CI's commands
except e2e at `-parallel=8` (#368): `go mod tidy -diff`, `go vet`, gofmt,
`git diff --check`, the darwin/arm64 build, unit (12 packages), e2e (1418 s)
and race (12 packages). All passed. The build ran with
`GOFLAGS=-buildvcs=false`, as the contracts' host checks do. This PR's head
differs from the verified tree only by this report.

## Blind shadow verification

The seat ran a second, blind verification of candidates the in-run Verifier
had passed: a headless Claude Code session with read, search and shell tools
only, no MCP servers, no project settings, the slice contract and the Lead
receipts, and no access to anything after the candidate. S1's shadow was
claude-fable-5-1; every other shadow was claude-opus-5-5 at effort xhigh.
Every finding that changed a decision was reproduced by the seat before it
acted.

| Slice | In-run Verifier | Shadow | Shadow verdict | Seat check | Consequence |
|---|---|---|---|---|---|
| S1 | claude-opus-5-5 PASS 0eeda0fb | claude-fable-5-1 | FAIL | reproduced | repair slice S6 |
| S2 | claude-fable-5-1 PASS f1dbad3d | claude-opus-5-5 xhigh | FAIL | reproduced with a probe | repair slice S7 |
| S3 | claude-fable-5-1 FAIL x3, PASS c4d0be04 | claude-opus-5-5 xhigh, all four candidates | FAIL x4 (agreed on the first three) | reproduced | repair slice S8 |
| S4 | claude-fable-5-1 PASS 3f4f1d0c | claude-opus-5-5 xhigh | FAIL | reproduced on the real adapter | repair slice S9 |
| S5 | claude-fable-5-1 PASS 1fa5d724 | claude-opus-5-5 xhigh | FAIL | confirmed from the code | repair slice S10 |
| S6 | claude-fable-5-1 PASS 9b1d34f7 | claude-opus-5-5 xhigh | FAIL | reproduced | #372 |
| S7 | claude-fable-5-1 PASS f69a7fb4 | claude-opus-5-5 xhigh | FAIL | reproduced | #373 |
| S8 | claude-fable-5-1 FAIL, PASS 05ec087c | claude-opus-5-5 xhigh | FAIL | reproduced | #374 |
| S9 | claude-fable-5-1 PASS 83b47b7f | claude-opus-5-5 xhigh | FAIL, evidence only | production correct per the shadow's probes | recorded here |
| S10 | claude-fable-5-1 PASS b15f439b | claude-opus-5-5 xhigh | FAIL, evidence only | not re-run | recorded here |

Every slice that passed in the run had a gap a blind reviewer found: eight
production defects (S1 to S8) and two evidence gaps (S9, S10). The shadow won
in both directions of the model pairing (a Fable shadow against an Opus
Verifier on S1, Opus shadows against a Fable Verifier from S2), so the
variable is the Verifier's brief, affordances and effort rather than the
model. Shadows took 43 to 77 turns and 5 to 18 minutes each; the in-run Opus
Verifier passed S1 in about 70 seconds, and the native lane records no effort
(#367).

In every case the criterion's tests existed, were touched and passed, and
could not have failed:

- S1: the Lead-required Start and Resume tests were missing; the park left
  the check claimed and the run projected as running.
- S2: the anchor test declared no host checks; a second pause of the same
  candidate across stop kinds failed the dispatch and spent the try.
- S3: a guard test was deleted in a failed try and an undisclosed change
  landed; the passed candidate keyed the credential fact on one stage, so an
  implementation-stage park never cleared.
- S4: the receipt reported an executed tool-call count the code dropped on a
  budget crossing.
- S5: forcing the anchor-substitute flag to false still passed every test.
- S6: the "no second try" test ran before Resume, so it could not see the try
  being spent (#372).
- S10: the journey assertion "no repair on epoch 2, try 1" cannot fail,
  because epoch 1 records no field-level refusal. Suggested fix: send one
  malformed `sworn_submit` before the valid one in epoch 1's last try
  (`epochCarryProvider` in `turn_recovery_linux_test.go`).
- S9: the A3 test does not cover each refusal kind, and the A1 runtime anchor
  does not use the exact receipt shape the Lead required.

This is the evidence behind #341's extension (fail on base as an assertion
failure, mutation of the changed lines, one named test per criterion) and
#370 (seal-time gates: no silent test removal, changes inside the design,
receipt matches the diff).

## Seat observations

- **Repairs as appended slices.** Defects found after a slice passed were
  fixed by appending repair slices to one amended revision 2, approved once,
  rather than reopening verified slices mid-run. After the repairs were
  approved, the Principal set a stopping rule: findings on repair slices
  become follow-up issues unless severe (a wrong verdict, lost work, a
  security issue or an unrecoverable run). #372 to #374 are those follow-ups;
  #373 is flagged for the Principal at promotion: S7's new
  `normalizeRecoveryStop` maps any recovery-sweep error that carries a
  cancellation, deadline or stop code to `RUN_STOPPED` before it considers
  `RECOVERY_UNCERTAIN`, so an ambiguous ref transaction that coincides with a
  stop reads as a clean stop. Main has no such mapping, so this is a
  regression the release introduces, confined to the recovery sweep.
- **Host check time.** S1's e2e suite took 49.6 of its 60-minute limit with
  CI's `-parallel=1`. Measured on this host, `-parallel=8` ran the same suite
  in 21 to 23 minutes against 43 serially, with no failures, and the repair
  slices ran with it (#368).
- **A usage limit read as a transport failure.** A subscription usage limit
  was reported as PROVIDER_TRANSPORT_FAILED; three tries were spent in 18
  seconds and the run parked on identical failures for about six hours until
  the limit reset (#369).
- **Credential expiry mid-dispatch** (#358) hit S1's first try (the engine
  checkpointed and resumed) and S3's first epoch before S3 and S8 landed.
- **Moving a completed run's target.** Fast-forwarding the release branch to
  record revision 2 re-projected the completed r2 as running with
  `invalid_authority`. The seat cancelled r2 and stopped its serve before
  starting r3. Recording a revision onto a completed run's target should not
  revive it.
- **Seat churn.** Periodic polling monitors were replaced by one event
  waiter that wakes the seat once per candidate, verdict, park or completion.
  The seat missed two live verdicts while blocked on a question with no
  waiter armed. #371 moves the waiter into the engine as `sworn watch
  --until`.
- **Effort is invisible.** The native lane passes no `--effort` and records
  none, so a two-minute pass and an eighteen-minute blind review of the same
  candidate are indistinguishable in telemetry (#367).

## Issues filed from this release

#365 (fixed by #366), #367, #368 (plus the assembly data points), #369,
#370, #371, #372, #373, #374, #377 (the unscoped fifth ask of #359), and an
extension of #341.
