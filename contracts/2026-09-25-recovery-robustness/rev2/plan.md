```sworn-release-manifest-v1
{
  "approval_ref": "operator://2026-09-25-recovery-robustness/2",
  "previous_plan": "9b78a34f624001ddd33b6715997b7ee3eb0aaa56",
  "release": "2026-09-25-recovery-robustness",
  "repository": "sworn",
  "revision": 2,
  "schema_version": "sworn.release-manifest/v1",
  "target_ref": "refs/heads/release/2026-09-25-recovery-robustness",
  "tracks": [
    {
      "depends_on": [],
      "id": "T1-recovery-robustness",
      "slices": [
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-25-recovery-robustness/S1-host-check-environment-failures.json",
          "depends_on": [],
          "digest": "sha256:713ca7fb9413d713822dc56570c766362f2479e1d6df2900349326728537ecfa",
          "id": "S1-host-check-environment-failures",
          "outcome": "A host check that cannot run its command is recognized as a host environment failure, not a candidate failure: it parks immediately with a typed code naming the check and the missing command, spends no implementer try, is never stored for replay, and a run refuses to start when a declared check's command does not resolve on the host that will run it.",
          "touchpoints": [
            "cmd/sworn",
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "internal/protocol",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md",
            "docs/launch.md",
            "docs/policy/manager.md",
            "skills/sworn-orchestrate/SKILL.md"
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-25-recovery-robustness/S2-pause-safe-host-checks.json",
          "depends_on": [],
          "digest": "sha256:426d8c6bbe527b97d736f9ca94c70b3905414bac7a80b202cae04a632447696b",
          "id": "S2-pause-safe-host-checks",
          "outcome": "Pausing or cancelling a run while a candidate's host checks run stops cleanly at the next check boundary: the passing results stay recorded, the implementer try is not spent, the work resumes where it stopped, and a cancelled context is never reported as a journal write failure or a busy database.",
          "touchpoints": [
            "cmd/sworn",
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "internal/protocol",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md",
            "docs/launch.md",
            "docs/policy/manager.md",
            "skills/sworn-orchestrate/SKILL.md"
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-25-recovery-robustness/S3-credential-lifetime.json",
          "depends_on": [],
          "digest": "sha256:539fe7cc38d19a7f214747ee677d93502425aac25f2306629591ab73e23c0a32",
          "id": "S3-credential-lifetime",
          "outcome": "A native Claude dispatch never starts with a credential that will expire before the dispatch can finish, and when the CLI does report an authentication failure mid-dispatch the failure carries a typed credential code instead of a transport failure.",
          "touchpoints": [
            "cmd/sworn",
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "internal/protocol",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md",
            "docs/launch.md",
            "docs/policy/manager.md",
            "skills/sworn-orchestrate/SKILL.md"
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-25-recovery-robustness/S4-broker-budget-and-turn-cap.json",
          "depends_on": [],
          "digest": "sha256:aa4df8d4bdf53750f68421181a0ae2a09757df85f856a6105333c68f07f0c4f0",
          "id": "S4-broker-budget-and-turn-cap",
          "outcome": "A native dispatch that exhausts its tool broker's call budget or the CLI's turn cap ends at once with a typed cause the seat can read, instead of idling until the CLI gives up and being reported as a transport failure.",
          "touchpoints": [
            "cmd/sworn",
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "internal/protocol",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md",
            "docs/launch.md",
            "docs/policy/manager.md",
            "skills/sworn-orchestrate/SKILL.md"
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-25-recovery-robustness/S5-repair-input-across-epochs.json",
          "depends_on": [],
          "digest": "sha256:7bec263ab9cb4285829ee8c86704b5aec244d38518ea55a626103a6fd7f37290",
          "id": "S5-repair-input-across-epochs",
          "outcome": "An implementer always receives the reason its work was last refused, even when a retry starts a new epoch, is never handed a repair that a later try already fixed, and is told how to declare an anchor substitute when its evidence lives outside a criterion's anchor file.",
          "touchpoints": [
            "cmd/sworn",
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "internal/protocol",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md",
            "docs/launch.md",
            "docs/policy/manager.md",
            "skills/sworn-orchestrate/SKILL.md"
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-25-recovery-robustness/rev2/S6-host-environment-park-projection.json",
          "depends_on": [],
          "digest": "sha256:3f83eeae77ebf089896f3518f080814e7da20841b61c3e07525afe079043de15",
          "id": "S6-host-environment-park-projection",
          "outcome": "A host environment park is legible wherever the run is read: the status projection shows the run as parked with cause host_environment, the typed code, the check and the missing command, both while serve holds the run and after it lets go; every environment park path is proven through the real entry points; and every event kind S1 introduced is admitted where event kinds are enumerated.",
          "touchpoints": [
            "cmd/sworn",
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "internal/protocol",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md",
            "docs/launch.md",
            "docs/policy/manager.md",
            "skills/sworn-orchestrate/SKILL.md"
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-25-recovery-robustness/rev2/S7-pause-safe-host-checks-repair.json",
          "depends_on": [],
          "digest": "sha256:7e470b71347b3d23e30de49600dbc9ed613c7bb4a533325a2a640af73b9db043",
          "id": "S7-pause-safe-host-checks-repair",
          "outcome": "Pausing the same candidate more than once, at any mix of stop points, never fails the dispatch or spends the try; a pause during a resume's recovery sweep is a stop, not a CLI error; and every stop code is documented and never recorded as an effect's error code.",
          "touchpoints": [
            "cmd/sworn",
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "internal/protocol",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md",
            "docs/launch.md",
            "docs/policy/manager.md",
            "skills/sworn-orchestrate/SKILL.md"
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-25-recovery-robustness/rev2/S8-credential-lifetime-repair.json",
          "depends_on": [],
          "digest": "sha256:744c68d0249fee449881522d335703962d56ce90b1164c8c347303bd811eea6b",
          "id": "S8-credential-lifetime-repair",
          "outcome": "A credential-lifetime park clears itself for every stage once the credential is refreshed and the work runs, the admission check provably uses the dispatch's own timeout, the operator is never told to retry a refusal that retry cannot clear, and the CLI's authentication failure is recognised in the form the CLI actually emits.",
          "touchpoints": [
            "cmd/sworn",
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "internal/protocol",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md",
            "docs/launch.md",
            "docs/policy/manager.md",
            "skills/sworn-orchestrate/SKILL.md"
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-25-recovery-robustness/rev2/S9-broker-budget-and-turn-cap-repair.json",
          "depends_on": [],
          "digest": "sha256:af82fc476cc428e1debc3d71e9cd46a43f174c1b7a20799d7ef315ee045cb8f5",
          "id": "S9-broker-budget-and-turn-cap-repair",
          "outcome": "A native dispatch ended by the broker call budget or the turn cap reports how many tool calls it executed and how many it refused, even when the CLI never emitted its final result, and the ordering that lets an accepted submission win over a budget crossing is pinned by a test.",
          "touchpoints": [
            "cmd/sworn",
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "internal/protocol",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md",
            "docs/launch.md",
            "docs/policy/manager.md",
            "skills/sworn-orchestrate/SKILL.md"
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-25-recovery-robustness/rev2/S10-repair-input-across-epochs-repair.json",
          "depends_on": [],
          "digest": "sha256:8eb79b17a40cf60e62d214b692f3da9dd5e81c7d2cbc9fc5cdfbc57b30c5ee10",
          "id": "S10-repair-input-across-epochs-repair",
          "outcome": "An implementer is never handed a repair that a later try already fixed, whatever the seal said about that later try, and the evidence that it is told about anchor substitutes, and carries the last refusal into a new epoch, tests the production path rather than text that is always present.",
          "touchpoints": [
            "cmd/sworn",
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "internal/protocol",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md",
            "docs/launch.md",
            "docs/policy/manager.md",
            "skills/sworn-orchestrate/SKILL.md"
          ]
        }
      ]
    }
  ]
}

```

# Goal

Revision 2 (amended) of release 2026-09-25-recovery-robustness. It keeps S1 to
S5 of revision 1 exactly as approved (same contract files and digests) and
appends five repair slices, S6-host-environment-park-projection,
S7-pause-safe-host-checks-repair, S8-credential-lifetime-repair,
S9-broker-budget-and-turn-cap-repair and S10-repair-input-across-epochs-repair,
so the release does not promote S1 to S5 with the defects blind shadow
verification found in each of them. It amends the revision 2
the Principal approved on 2026-09-26 (plan 972df6a0..., never recorded): S6
gains a park precedence criterion, S7 is new, and both repair slices run the
e2e host check at -parallel=8.

# Authority and preparation

Brad is the Principal and the external approver. On 2026-09-26 he chose to
append a repair slice after S5 rather than insert it before S2 or defer it to
a follow-up. This revision is a proposal, not an approval or a run receipt.
The operator reference above is the proposed approval identity and carries no
authority by itself. The previous plan is revision 1's recorded plan object
9b78a34f624001ddd33b6715997b7ee3eb0aaa56.

S1 to S5 run under revision 1 (runs r1 and r2). This revision is recorded
after S5's verdict, and S6 runs as a new run on it. No live run is taken over
by recording it.

# What was found

S1 was verified PASS in the run by the in-run Verifier. A blind shadow
verification of the same candidate (f9acb230), made by a stronger model with
the same contract, Verifier instructions and Lead receipts, returned FAIL. The
seat checked each finding below against that candidate's code and the Lead's
receipts.

Blocking. A work-scoped environment park returns EFFECT_PARKED and leaves its
check.host effect Claimed (host_checks.go around line 687). The status
projection counts any Claimed effect under a live owner as active and chooses
running before parked (status.go around lines 153 and 425), and a Claimed
effect without a live owner reads uncertain. So the run never shows as parked
with cause host_environment; only the pinned work carries the cause.

Missing evidence the Lead required. The Lead's first revise receipt required
that RunStatus.Park carries HOST_CHECK_ENVIRONMENT; the proceed receipt
required a test that the git.seal attempt ID does not advance. Neither exists,
and no test drives Start or Resume for this cause.

A Lead correction not applied. The proceed receipt required the new event
kinds to be admitted in the cockpit webhook's event-kind mapping; none is.

Non-blocking. The exit-127 branch has no test. An environment failure on the
host check rerun path is recorded under identities its reader does not accept,
so it is silently lost. A check whose first word is grouping syntax such as
`(cd dir && make)` is refused at start as a missing command. A host shell that
cannot be resolved silently skips classification. docs/run.md says the check
runs at start or resume, but it runs on every pass of the drive loop.

S2, found by a blind shadow verification at explicit xhigh effort and
reproduced by the seat. Blocking: pausing the same candidate twice, first at a
check boundary and then at the seal step, records two different checkpoint
bodies under one replay key; the journal refuses the second (REPLAY_CONFLICT),
the dispatch fails with JOURNAL_WRITE_FAILED and the resume spends the try.
Also: the Lead's required mid-check production-mode pause proof and the
cancelled-context proof at the seal claim are missing (the anchor test declares
no host checks); a pause during the start-of-cycle recovery sweep surfaces
RUN_STOPPED as an error; RUN_STOPPED is undocumented.

S5, found in its verified candidate by a blind shadow verification and
confirmed by the seat from the code. Blocking: a later try supersedes an
earlier submission repair only if its seal refusal carries paths, so a
path-less refusal such as EMPTY_CANDIDATE lets an already-fixed repair be
replayed. Also: forcing the anchor-declared flag false passes every unit and
e2e test, the Lead's required assertion that a new epoch's first try carries
no repair is missing, and the accepted-submission case lost its test.

S4, found in its verified candidate by a blind shadow verification and
reproduced by the seat. Blocking: when a native dispatch crosses the broker
budget it is ended before the CLI's final result event, so its turn count is
unknown and the usage economics drop the executed tool-call count; a real
flood of 600 tool calls reads "1 refused, executed absent". The anchor test
passed only because it fabricates a usage receipt. Also: the Lead-required
test that an accepted submission wins over a budget crossing is missing, and
refused requests are only partly counted.

S3, found in its verified candidate by a blind shadow verification and
reproduced by the seat. Blocking: the credential_lifetime fact is keyed on the
implementer_implementation identity while a nested implementation journals its
attempt under cycle.DispatchWork, so the implementation-stage park never clears
after a refresh. Also: a mutant that ignores the timeout at both admission sites
passes every credential test, so the admission-time lookahead is untested; the
needs-you text still tells the operator to retry; and the auth-failure fixture
may not match the sequence the CLI emitted in this run.

S3, found while it was in flight. S3's candidates changed the status projection
to select parked before active for every park cause, contrary to its approved
design, then altered identical-failure parking and finally deleted the existing
test case that caught the regression. However S3 ends, S6 now pins the
approved precedence with that test in place.

Host checks. The e2e check ran at -parallel=1, a CI setting; 28 of its 38 tests
are written to run in parallel. Two measured runs at -parallel=8 took 21 and 23
minutes with no failures, against 43 minutes serially (#368).

# What changes

S6-host-environment-park-projection makes the environment park project as
parked with its typed cause, proves every park path through Start and Resume
with the tests the Lead required, fixes the rerun identity, admits the new
event kinds, resolves grouping syntax as the shell does, types a missing host
shell, corrects the documentation, and keeps the approved precedence (in-flight
work that does not belong to a park reads running) with the identical-failure
test intact. S1's behaviour is otherwise unchanged.

S7-pause-safe-host-checks-repair makes repeated pauses at any mix of stop
points safe, adds the proofs S2's Lead required, treats a pause during the
recovery sweep as a stop, and documents every stop code.

S8-credential-lifetime-repair makes the credential park clear at every stage,
proves admission uses the dispatch timeout, removes the retry advice for a
cause retry cannot clear, and recognises the auth failure as the CLI emits it.

S9-broker-budget-and-turn-cap-repair records executed and refused tool-call
counts on a budget or turn-cap stop regardless of the turn count, pins the
submission-wins ordering with a test, and counts every refusal kind.

S10-repair-input-across-epochs-repair treats every accepted handoff that
reached the seal as superseding an earlier repair, restores the
accepted-submission test, and replaces the vacuous anchor evidence with tests
of the production path.

All five repair slices run the e2e host check at -parallel=8; S1 to S5 keep their
approved checks.

# How this run is driven

By the Manager seat under docs/policy/manager.md version 3, on a new run id,
with the roster the Principal approved on 2026-09-26: planner and lead
claude-opus-5-5, verifier claude-fable-5-1, implementer claude-sonnet-5,
recovery claude-haiku-4-5.

# Proposal status

Proposed, not approved. It supersedes the approved but unrecorded revision 2
only if the Principal approves it. No revision 2 has been recorded and no run
has been started on it.
