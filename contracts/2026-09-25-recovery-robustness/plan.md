```sworn-release-manifest-v1
{
  "approval_ref": "operator://2026-09-25-recovery-robustness/1",
  "previous_plan": null,
  "release": "2026-09-25-recovery-robustness",
  "repository": "sworn",
  "revision": 1,
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
        }
      ]
    }
  ]
}

```

# Goal

Make launch and recovery robust to the environment and engine faults that cost
release 2026-09-23-launch-legibility ten of its sixteen failed implementation
tries. This is the third release of ADR-0014 Track B (legibility). It delivers
the follow-ups that release filed: #355, #357, #358, the remainder of #359, and
#361.

# Authority and preparation

Brad is the Principal and the external approver. This revision is a proposal,
not an approval or a run receipt. Nothing here has been recorded, approved or
launched. The operator reference above is the proposed approval identity and
carries no authority by itself. Review these exact pinned manifest bytes and
the five contract files.

The inspection baseline is main at 79af516f, which contains the launch
legibility release (#362) and the engine and policy changes merged during it
(#352, #353, #354, #356, #360). Prepare the named release target from that main
before recording this plan, and record its exact base and contract-tree
identity then. No existing release, track or live run is taken over by this
plan.

# What was found

Every park in the previous release was an environment or engine gap before it
was a candidate finding, and each was a check that held when it ran but not
for the life of the work.

Host environment. A host check runs through `sh -c` with the serve process's
environment. Under a systemd user unit without an explicit PATH, `go` was not
found (exit 127); the engine treated that as a candidate failure, handed it to
the implementer three times, stored it, and replayed it after the PATH was
fixed, so only a new run could recover (#355).

Pause. `watchOwner` cancels the run context within 250 ms of a pause. The
running host check ignores that context and finishes, but the next journal
write fails on the cancelled context, is labelled DATABASE_BUSY and then
JOURNAL_WRITE_FAILED, spends the try and discards a candidate that had passed
the end-to-end suite (#357).

Credentials. The native credential preflight refuses only a token that is
already expired. A dispatch can run for two hours; a token valid at start
expired at turn 246 and the failure read PROVIDER_TRANSPORT_FAILED (#358).

Broker. Refused tool calls spent the 512-call budget; exhaustion answered
`closed` without ending the dispatch, the model pinged for fifteen minutes,
and the CLI's `error_max_turns` exit read PROVIDER_TRANSPORT_FAILED. #360
queued concurrent calls; exhaustion and the turn cap remain (#359).

Repair input. A seal refusal is captured only for a later try in the same
epoch, so a retry that starts a new epoch loses it, and an already fixed
submission repair is replayed instead. The implementer is never told that
`anchor_substitutes` exists (#361).

# What changes

One track, five slices, serial. Parallel tracks are not admissible for work in
internal/runtime and internal/driver, which cmd/sworn imports.

S1-host-check-environment-failures classifies exit 127 or an unresolved
command as a typed environment failure that parks at once, spends no try and
is never stored or replayed, and refuses a run start whose declared checks
name a command the host cannot resolve.

S2-pause-safe-host-checks stops the host check loop cleanly at the next
boundary on pause or cancel, keeps passing results and the try, and makes the
journal report a cancelled context as a cancellation, never as a busy database
or a write failure.

S3-credential-lifetime refuses a Claude credential that will expire before
the dispatch's timeout, and gives the CLI's own authentication failure a typed
credential code.

S4-broker-budget-and-turn-cap ends a dispatch at once with a typed code when
the broker's call budget is exhausted or the CLI hits its turn cap, and
reports refused broker requests in the dispatch view.

S5-repair-input-across-epochs carries the last seal refusal into the first
try of a new epoch, drops repairs a later try superseded, and offers
`anchor_substitutes` to the implementer with the route named in the refusal.

# Deliberately not in this release

A durable default journal location in the engine (#351, handled in the
Manager skill), the assembly BLOCKED routing (#349, a protocol decision), the
Slack and Teams seat (#363), and anything in ADR-0014 Track D.

# How this run is driven

By the Manager seat: /sworn-orchestrate under docs/policy/manager.md version
3, on a fresh run id, with the roles named planner, implementer, lead and
verifier. The seat holds the Principal's delegation for operational recovery
(environment fixes, pause, retry, cancel and relaunch with only the run id
changed); plan, roster and merge decisions are Type-1. Every decision is in
the ops-home decision journal. It is a candidate for the v1.0.0 gate 3 streak
(a release with three or more slices) if it runs without hand operator work.

# Proposal status

Proposed, not approved. No plan revision has been recorded, no approval
receipt exists and no run has been started. Approval is Brad's and is
separate from this document.
