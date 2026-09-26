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
          "digest": "sha256:04202d39b11bc55616fe498af40b43d49e26a7a1a25e68f3bf909cfcafabc819",
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
        }
      ]
    }
  ]
}

```

# Goal

Revision 2 of release 2026-09-25-recovery-robustness. It keeps S1 to S5 of
revision 1 exactly as approved (same contract files and digests) and appends
one repair slice, S6-host-environment-park-projection, so the release does not
promote S1 with the illegible park it exists to remove.

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

# What changes

S6-host-environment-park-projection makes the environment park project as
parked with its typed cause, proves every park path through Start and Resume
with the tests the Lead required, fixes the rerun identity, admits the new
event kinds, resolves grouping syntax as the shell does, types a missing host
shell, and corrects the documentation. S1's behaviour is otherwise unchanged.

# How this run is driven

By the Manager seat under docs/policy/manager.md version 3, on a new run id,
with the roster the Principal approved on 2026-09-26: planner and lead
claude-opus-5-5, verifier claude-fable-5-1, implementer claude-sonnet-5,
recovery claude-haiku-4-5.

# Proposal status

Proposed, not approved. No revision 2 has been recorded, no approval receipt
exists and no run has been started on it. Approval is Brad's and is separate
from this document.
