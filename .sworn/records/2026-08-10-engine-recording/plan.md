```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-10-engine-recording",
  "revision": 2,
  "previous_plan": "279a674ed37ebe908de3a3ba060c1125743db636",
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-10-engine-recording/2",
  "tracks": [
    {
      "id": "T1-engine",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-streaming-dialects",
          "outcome": "Responses-flavour invocations stream live to the operator while validation still consumes the terminal event's complete response, and the DeepSeek and Qwen reasoning dialects are first-class: plaintext and summary-shaped reasoning are accepted, thinking can be requested, and dialect quirks never fail an honest turn.",
          "contract_path": "contracts/2026-08-10-engine-recording/S1-streaming-dialects.json",
          "digest": "sha256:a9413488677a15c0913e76e31e1827f293a3ec11d9141554193764b9fd38b487",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver"
          ]
        },
        {
          "id": "S2-worker-harness",
          "outcome": "The worker and verifier harness stops taxing honest work: scratch state and build caches persist across one invocation's commands and die with it, a failing command returns its output and exit code instead of vanishing, exact bytes travel to sworn_submit by file reference, verification work-context spells out the candidate's ancestry, and read-only workspaces expose read-only git.",
          "contract_path": "contracts/2026-08-10-engine-recording/S2-worker-harness.json",
          "digest": "sha256:df0a676908e6bad1ab031cca1c3a82e0db9ca3f9d8ee2e70480f60a64ec4b491",
          "depends_on": [
            "S1-streaming-dialects"
          ],
          "consumes": [
            "S1-streaming-dialects"
          ],
          "touchpoints": [
            "internal/driver",
            "internal/runtime",
            "test/e2e"
          ]
        },
        {
          "id": "S3-seal-journal-retry",
          "outcome": "Recovery semantics are ratified and coherent: operational failures retry within budget without a human ritual, verdicts are never healed, one paid model execution per work per epoch is an enforced invariant, workspace scratch never enters a candidate, and the conformance suite pins exactly these behaviors.",
          "contract_path": "contracts/2026-08-10-engine-recording/S3-seal-journal-retry.json",
          "digest": "sha256:0966e9193018a6d2e3999e3b4c3a225205e8a7aa0cf94779f4d679bf5b12a5eb",
          "depends_on": [
            "S2-worker-harness"
          ],
          "consumes": [
            "S2-worker-harness"
          ],
          "touchpoints": [
            "internal/journal",
            "internal/gitx",
            "test/e2e"
          ]
        }
      ]
    }
  ]
}
```

# Goal

The 2026-08-09-provider-capability release was delivered by an engine that
does not exist in history: runs r8 onward executed a working tree carrying
streaming, dialect, sandbox, journal and worker-harness changes hand-patched
mid-flight to keep the release alive. The release's own receipts are sound —
candidates never contained the uncommitted diff — but the engine that
midwifed them is unauditable. This release retires that debt: the entire
remaining uncommitted diff lands as three recorded, verified slices before
any promotion of release/v1.0.0. (The sandbox toolchain and diagnostics work
originally listed alongside it was already committed pre-release as
d72e0747 and 87420a48 and needs no recording.)

Each slice's exact commitment is a reference patch committed at
`contracts/2026-08-10-engine-recording/reference/<slice>.patch`, generated
from the operator tree after rebasing the diff onto the merged target head
fcd7222f (the streaming rules composed into the capability-axes effort
vocabulary; the Qwen x_details tolerance composed into the canonical
responsesUsage helper). Implementers land exactly those changes; the
workspace contains the patch natively because it is committed at the track
base.

S3 also carries new work: the operator ratified the new recovery semantics
(bounded operational retry may heal; verdicts never heal) after live
bisection proved the journal fix changes four behaviors the conformance
suite pins, and analysis of the preserved journal exposed a single-payment
bypass: the succeeded-try guard inspects only the immediately previous try,
so a refused try's missing effect re-opens payment one try later. S3 closes
that with an any-succeeded-try guard and revises the four pins to assert
the ratified semantics with boundedness explicit.

Empirical grounding in .baton/2026-08-10-provider-capability.handoff.md
addenda 2–5: F8 (per-command sandbox wipe destroyed every build cache), F9
(failing commands discarded their own output), F10 (a live PASS verdict
bounced because a model cannot transcribe 7.5KB of base64 verbatim), F11
(retry-semantics drift, proven by bisection at 078cebbb + control.go).

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-10-engine-recording/2`. Planning did not approve itself.

Revision 2 exists because the reference patches moved. Revision 1's patches
were generated while three files were still in unmerged index state after
the three-way rebase, so Git produced combined diffs that silently omitted
changes: S1 lost the `newResponsesConversation` signature change that its
own test calls, and would not have compiled. The corrected patches were
verified cumulatively at the track base - each slice builds and tests on
top of its predecessors, and the three together reconstruct the operator
tree byte for byte - then committed, which moved the target head. Bootstrap
authority binds an approval to an exact target head, so the engine refused
the stale approval and the operator must grant a new one. The slice
contracts, their digests, and every acceptance criterion are unchanged from
revision 1; only the patch bytes the implementers must land, and the target
they land on, have moved.

The three slices form one serial track because S1 and S2 share
internal/driver and the exact product base must stay linear; nothing here is
eligible for concurrent execution. Scopes are package directories per the
revision-5 lesson: acceptance changes behavior that existing tests pin, so
the pinning tests are part of each deliverable. Each slice keeps the
existing five checks with the end-to-end gate at 60 minutes and -parallel=1;
relaxing the parallel mandate is explicitly out of scope until S3's ratified
semantics have soaked.

Development targets `refs/heads/release/v1.0.0`, not `refs/heads/main`.
Nothing commits direct to main; promotion (the 1.0.0-dev version-suffix
ritual) is a separate human-gated step after this release completes.

This release does not alter trust rules, receipt schemas, role independence
or the containment boundary between roles; it does not add providers, port
platform-gated files, or introduce context compaction. Those remain
separately approved work.
