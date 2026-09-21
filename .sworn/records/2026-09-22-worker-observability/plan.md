```sworn-release-manifest-v1
{
  "approval_ref": "operator://2026-09-22-worker-observability/1",
  "previous_plan": null,
  "release": "2026-09-22-worker-observability",
  "repository": "sworn",
  "revision": 1,
  "schema_version": "sworn.release-manifest/v1",
  "target_ref": "refs/heads/release/2026-09-22-worker-observability",
  "tracks": [
    {
      "depends_on": [],
      "id": "T1-worker-observability",
      "slices": [
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-22-worker-observability/S1-native-turn-journal.json",
          "depends_on": [],
          "digest": "sha256:73a7d7d12896b9679b882ee87a2c561ad07ac768e95ab99a1287792823019477",
          "id": "S1-native-turn-journal",
          "outcome": "A dispatch on a native CLI lane journals one bounded, redacted record per worker turn as it happens, carrying what the worker said and which tools it called, and its tool results are keyed to the turn they belong to, so a native-lane dispatch can be read back and profiled turn by turn exactly as an HTTP-lane dispatch can today.",
          "touchpoints": [
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/protocol",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "cmd/sworn",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md"
          ]
        },
        {
          "consumes": [
            "S1-native-turn-journal"
          ],
          "contract_path": "contracts/2026-09-22-worker-observability/S2-live-worker-stream.json",
          "depends_on": [
            "S1-native-turn-journal"
          ],
          "digest": "sha256:39d031e1a571a23d4f1a49ff38ba3cd587fb03888b6d042bb33dd1cbd3ee0911",
          "id": "S2-live-worker-stream",
          "outcome": "An operator watching a run sees what each worker is doing turn by turn while it happens, on every lane, in an activity pane on the browser board and an activity screen in the TUI, fed from the journaled worker turns and tool results and pushed live over a content-bearing server-sent-event route when the serve host is driving the run.",
          "touchpoints": [
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/protocol",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "cmd/sworn",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md"
          ]
        },
        {
          "consumes": [
            "S1-native-turn-journal"
          ],
          "contract_path": "contracts/2026-09-22-worker-observability/S3-failure-turn-context.json",
          "depends_on": [
            "S1-native-turn-journal"
          ],
          "digest": "sha256:04c915816e227c0d1e850d99568aaacd3ee7eb41cda4f250c7f8357b4a026def",
          "id": "S3-failure-turn-context",
          "outcome": "When a dispatch fails operationally, its durable failure record carries a bounded tail of the worker's last turns, and the status projection shows that tail for the failed work, so an operator or a Manager seat can see what the worker was doing when it failed without opening the journal.",
          "touchpoints": [
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/protocol",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "cmd/sworn",
            "tools/protocolgolden",
            "test/e2e",
            "docs/run.md"
          ]
        }
      ]
    }
  ]
}

```

# Goal

Make every worker legible while it works and after it fails, on every lane.
This is the first release of ADR-0014 Track B (legibility) and delivers
sworn#294 in full: per-turn journaling on native CLI lanes, a live worker
stream for the browser board and the TUI, and the worker's last turns on
every operational failure record.

# Authority and preparation

Brad is the Principal and the external approver. This revision is a
proposal, not an approval or a run receipt. Nothing here has been recorded,
approved or launched. The operator reference above is the proposed approval
identity and carries no authority by itself. Review these exact pinned
manifest bytes and the three contract files.

The inspection baseline is main at a4a3e6f9, which contains the seal-time
gates release (#323) and the legacy record read fix (#321). Prepare the named
release target from that main before recording this plan, and record its
exact base and contract-tree identity then. No existing release, track or
live run is taken over by this plan.

# What was found

The native adapter already asks the Claude CLI for stream-json and reads it
line by line, but the reader handles only the init event and the final
result event. Assistant and user events fall through and are discarded, and
each line is cleared after it is read. The Codex family is read through the
same switch.

Tool results on native lanes are already observed through the MCP broker,
but the broker takes its turn number from a counter that the Claude family
advances only on the final result event. Every tool result of a dispatch
therefore lands on turn 0, which is why a Claude-lane dispatch journals one
or two rows where an HTTP-lane dispatch journals one per turn (302 rows for
one implementation try in run 2026-09-05-preserve-work-recovery).

The serve host already has a server-sent-event route, but it carries only
invalidation ticks, and the event projection the board and the TUI share is
metadata-only by construction. The TUI reads the journal directly and never
calls its own event page. The mechanism for bounded, redacted, non-blocking
observation exists and is well tested; it has one consumer.

# What changes

One track, three slices, serial.

S1-native-turn-journal teaches the native reader the worker-turn events of
both CLI families, advances the observation turn at every assistant turn so
tool results are keyed correctly, and journals one bounded, redacted
worker-turn event per turn through the existing observation mechanism. It
adds no presentation.

S2-live-worker-stream adds one activity projection over those events, a
content-bearing stream route beside the unchanged invalidation route, an
in-memory ring that shortens latency when the serve host drives the run, an
activity pane on the browser board and an activity screen in the TUI.

S3-failure-turn-context attaches a bounded tail of the last turns to every
operational failure record in the same write as the failure, and shows it in
the shared status projection, so a failure arrives with what the worker was
doing. This is the seat-facing half: it is one more typed fact the Manager
does not have to open a journal for.

# Deliberately not in this release

Token-level deltas. sworn#294 lists them as optional for the pane. They need
a new CLI flag on a pinned argv and they multiply stream bytes against the
manifest-governed output budget, which is an accounting decision of its own.
Granularity in this release is the turn. A follow-up issue is filed when
this plan is approved.

A seat-facing MCP read of worker turns. It belongs with the typed explain
read that follows Track B; S2 and S3 shape their projections so that read
can reuse them.

Telemetry export of the new events, the launch legibility fixes (#324 to
#327, the next release on this track) and the typed park and refusal audit.

# How this run is driven

By the Manager seat: /sworn-orchestrate under docs/policy/manager.md version
1, on a fresh run id, with the roles named planner, implementer, lead and
verifier. Every decision the seat takes is in the ops-home decision journal.
Anything the policy does not name is escalated to the Principal. A merged
outcome, or a typed park decided within the policy, is the result this plan
expects. This release has three slices and real design work in each, so
parks are likely; how the seat handles them is part of what the run shows.

# Proposal status

Proposed, not approved. No plan revision has been recorded, no approval
receipt exists and no run has been started. Approval is Brad's and is
separate from this document.
