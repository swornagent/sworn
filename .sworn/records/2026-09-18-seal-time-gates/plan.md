```sworn-release-manifest-v1
{
  "approval_ref": "operator://2026-09-18-seal-time-gates/1",
  "previous_plan": null,
  "release": "2026-09-18-seal-time-gates",
  "repository": "sworn",
  "revision": 1,
  "schema_version": "sworn.release-manifest/v1",
  "target_ref": "refs/heads/release/2026-09-18-seal-time-gates",
  "tracks": [
    {
      "depends_on": [],
      "id": "T1-seal-time-gates",
      "slices": [
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-18-seal-time-gates/S1-seal-time-gates.json",
          "depends_on": [],
          "digest": "sha256:0f689f9d73941f8df69c9a7d5c45ae673d39434658f6b15804305eabc80453a7",
          "id": "S1-seal-time-gates",
          "outcome": "Deterministic engine-owned gates refuse a candidate at seal time, before any long process suite runs, for a failing quick declared check, an acceptance anchor the candidate touches no file of, or a degenerate submission body, each with a typed code that reaches the worker through the existing repair path.",
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

Land the seal-time gates the 2026-09-11-phased-evidence release already
verified, on the renamed engine, in one slice, driven end to end by the
Manager seat. This is Track E of ADR-0014 and the acceptance run for Track A.

# Authority and preparation

Brad is the Principal and the external approver. This revision is a
proposal, not an approval or a run receipt. Nothing here has been recorded,
approved or launched. The operator reference above is the proposed approval
identity and carries no authority by itself. Review these exact pinned
manifest bytes and the one contract file.

The inspection baseline is main at 6bd059ff, which contains ADR-0014 (#315),
the vocabulary rename (#316), the four cheap engine fixes (#311, #312, #313,
#314) and the Manager skill with its policy (#318). Prepare the named release
target from that main before recording this plan, and record its exact base
and contract-tree identity then. No existing release, track or live run is
taken over by this plan.

# Why one slice, and why a port

Release 2026-09-11-phased-evidence verified S1 (candidate fc8edaa0, receipt
7cf43906, stage merge) and then looped on S2 for engine reasons that are now
filed and, for the cheap ones, fixed. The vocabulary rename that followed is
a hard cut: that release's journal and records are unreadable by this
engine, and its candidate no longer compiles against main, because it names
the retired package and the old role. So its evidence cannot carry, but its
design and its diff can. This slice asks the implementer to port the verified
diff onto this base and reconcile it with the fixes that landed after it,
rather than to design the gates a second time. The Lead reviews the port as a
design; the Verifier judges it on this base with full evidence, as always.

S2, S3 and S4 of phased-evidence are not in this release and are not
retired. They are re-examined under ADR-0014 before they are re-cut: on
2026-09-17 no model reliably predicted the Verifier's verdict from the diff
plus the product suite, so S2's mechanism is an open question, not a slice.

# What changes

One slice on one track. S1-seal-time-gates keeps the revision-6 outcome,
acceptance criteria, checks and host checks; its scope and wording move to
the ADR-0014 vocabulary; the serial end-to-end deadline is 60 minutes, as
revision 6 set for it; and a first constraint names the retained candidate,
its diff, the vocabulary mapping and the landed fixes it must reconcile with.

# How this run is driven

By the Manager seat: /sworn-orchestrate under docs/policy/manager.md version
1, on a fresh run id, with the roles named planner, implementer, lead and
verifier, and no hand operator work. Every decision the seat takes is in the
ops-home decision journal. Anything the policy does not name is escalated to
the Principal. A merged outcome, or a typed park decided within the policy,
is the result this plan expects.

# Proposal status

Proposed, not approved. No plan revision has been recorded, no approval
receipt exists and no run has been started. Approval is Brad's and is
separate from this document.
