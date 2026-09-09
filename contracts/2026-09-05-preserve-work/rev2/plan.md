```sworn-release-manifest-v1
{
  "approval_ref": "operator://2026-09-05-preserve-work/2",
  "previous_plan": "cf6bb6e45599c7f4e45d57d762d6feb947c52abc",
  "release": "2026-09-05-preserve-work",
  "repository": "sworn",
  "revision": 2,
  "schema_version": "sworn.release-manifest/v1",
  "target_ref": "refs/heads/release/2026-09-05-preserve-work",
  "tracks": [
    {
      "depends_on": [],
      "id": "T1-preservation",
      "slices": [
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-05-preserve-work/rev2/S1-durable-unverified-checkpoints.json",
          "depends_on": [],
          "digest": "sha256:72f865550c85c0b7a6670c39834fb86bd12ab13cb97399d28b799630ccc846a6",
          "id": "S1-durable-unverified-checkpoints",
          "outcome": "An orderly implementation stop preserves a measured unverified product checkpoint before disposable workspace cleanup, and an authorized retry with unchanged authority restores that work without publishing a candidate or a verdict.",
          "touchpoints": [
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/baton",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "cmd/sworn",
            "tools/batongolden",
            "test/e2e",
            "docs/run.md",
            "contracts/2026-09-05-preserve-work/recovery/s1-2fcaa718.patch.gz.b64"
          ]
        },
        {
          "consumes": [
            "S1-durable-unverified-checkpoints"
          ],
          "contract_path": "contracts/2026-09-05-preserve-work/S2-interrupted-work-reconciliation.json",
          "depends_on": [
            "S1-durable-unverified-checkpoints"
          ],
          "digest": "sha256:d8debfa48f4aff5c67cca4e0fe565c6a7d6dc05230b88263639c6f00c2b90c5c",
          "id": "S2-interrupted-work-reconciliation",
          "outcome": "After process interruption, Sworn reconciles owned implementation work before abandoned-workspace cleanup, restoring the last durable checkpoint or preserving attributable interrupted files as explicitly unverified recovery data.",
          "touchpoints": [
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/baton",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "cmd/sworn",
            "tools/batongolden",
            "test/e2e",
            "docs/run.md"
          ]
        },
        {
          "consumes": [
            "S2-interrupted-work-reconciliation"
          ],
          "contract_path": "contracts/2026-09-05-preserve-work/S3-targeted-handoff-repair.json",
          "depends_on": [
            "S2-interrupted-work-reconciliation"
          ],
          "digest": "sha256:ab6eca505d6a459efed87080963d8fba399db86ba592c547763ce73e261cc33e",
          "id": "S3-targeted-handoff-repair",
          "outcome": "Submission and evidence defects are repaired against retained implementation work, without forcing a code change or rebuilding a valid candidate merely to satisfy closing prose.",
          "touchpoints": [
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/baton",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "cmd/sworn",
            "tools/batongolden",
            "test/e2e",
            "docs/run.md"
          ]
        },
        {
          "consumes": [
            "S3-targeted-handoff-repair"
          ],
          "contract_path": "contracts/2026-09-05-preserve-work/S4-resumable-budget-stops.json",
          "depends_on": [
            "S3-targeted-handoff-repair"
          ],
          "digest": "sha256:de0614ce12b822b483a4259ed173ccf0276c4394a39621770fd6bd13141ef1a4",
          "id": "S4-resumable-budget-stops",
          "outcome": "An implementation reaching its configured API-turn, API-output-token or native-output-byte budget parks with preserved work and truthful recorded usage, and can continue in the same run after an explicit bounded capacity grant.",
          "touchpoints": [
            "internal/runtime",
            "internal/journal",
            "internal/gitx",
            "internal/driver",
            "internal/baton",
            "internal/cockpit",
            "internal/observe",
            "internal/tui",
            "internal/skill",
            "cmd/sworn",
            "tools/batongolden",
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

Complete the same four approved preservation outcomes while retaining the
implementation already produced by Sworn. Revision 2 adds exact unverified
recovery material to S1 and clarifies its unchanged retention obligation.
No slice is added, retired or declared passed by this revision.

# Authority and exact input

Brad is the external approver. This revision is proposed, not approved.
Previous plan blob: cf6bb6e45599c7f4e45d57d762d6feb947c52abc.
The named target remains refs/heads/release/2026-09-05-preserve-work.
Prepare it with this revision's contract and recovery-input files before native
recording, and bind the actual resulting target/contract-tree commit.

The prior release target is 3ddcfadeeffb1ddc4a071d1b638711d5b809bb98.
Run r2 is deliberately paused at control generation 1 with no active owner.
Its original plan, manifest, configuration, journal, design and Captain
receipts remain historical facts. Do not rewrite them.

The recovery artifact contracts/2026-09-05-preserve-work/recovery/s1-2fcaa718.patch.gz.b64 contains a gzip-compressed,
base64-encoded patch. Its decoded SHA-256 is 4262bd2414056a5f916b027ed60b7c56b96149329aa085a31e59feb7ddbed1ce
and decoded length is 79480 bytes. It contains only the product
delta from bdd1e7055f202284ab000e7cbaeedcea7ef2eaaf to prepared candidate
2fcaa718e2379400feb18f7973b8f0b441b485a8. That candidate never received a
Verifier PASS. The artifact changes these paths:

- docs/run.md
- internal/cockpit/model.go
- internal/cockpit/presentation.go
- internal/cockpit/presentation_test.go
- internal/cockpit/projector.go
- internal/cockpit/terminal.go
- internal/cockpit/terminal_test.go
- internal/gitx/checkpoint.go
- internal/gitx/checkpoint_test.go
- internal/gitx/repository.go
- internal/gitx/workspaces.go
- internal/journal/checkpoint.go
- internal/journal/checkpoint_test.go
- internal/runtime/production_dispatch_test.go
- internal/runtime/scheduler.go
- internal/runtime/service.go
- internal/runtime/status.go
- test/e2e/production_journey_linux_test.go

The patch is repair input, not an approved candidate. Apply only its product
delta to the engine-prepared revision-2 base so current plan/record authority
remains intact. Do not copy old control records or substitute the old full
tree for the current base. Remove the temporary input from the final product.

# What happened and what must be repaired

1. The full product host suite passed on 2fcaa718. The end-to-end suite ran
   for 1988.824 seconds and failed TestConfiguredProductionPreservationAndRestorationJourney:
   the fixture injected provider-unavailable responses and then demanded
   empty stderr from the resume command. Its restoration/completion proof
   must remain effective when the expected injected diagnostics are handled.
2. An independent operator probe confirmed an A4 defect: with valuable
   allowed.txt progress plus forbidden.txt, CaptureCheckpoint refused scope,
   then closing the lease/workspaces removed allowed.txt. The new candidate
   test TestCheckpointScopeViolationDoesNotFence actually asserts the bad
   behavior, following a Captain correction that exceeds the original
   contract's permission. Scope failure does not authorize discarding data.
3. The current engine does not restore this failed prepared implementation
   into its next writer automatically. Its approved base is immutable.
   An explicit recovery input in a newly approved base is the supported way
   to let a fresh model repair retained work without an out-of-band code
   injection, fabricated receipt or blind regeneration.

The temporary diagnostic probe is retained in the operator archive. It was
not left in the candidate checkout and is not an engine Verifier verdict.
Review all acceptance boundaries, not just these two observations.

# Slice and dependency changes

- S1 keeps its ID, outcome and A1-A6 acceptance bytes unchanged. Its scope
  gains only the temporary recovery-input path, and its constraints bind that
  input and clarify the required repairs. A new design/Captain review must
  account for the recovered implementation and original retention promises.
- S2, S3 and S4 contracts remain byte-identical to approved revision 1.
- No passed work is invalidated: S1 had no admitted implementation candidate
  or Verifier PASS, and S2-S4 had not begun. Their future dependency closure
  still consumes the eventual corrected S1 product through the same track.
- The separately approved roster/live-switch plan is unchanged and follows
  completion of preservation. Live-stream and TUI discovery requirements
  remain captured for subsequent operator-UI work.

# Checks and operating setup

Keep the original declared checks and host_checks unchanged. Native plan
pin/lint must validate this revision. Decode the recovery artifact and verify
its digest, then dry-run its application against the compatible source base
before handing it to a worker.

Every corrected candidate must pass the required host checks and independent
verification. Existing green results for 2fcaa718 do not certify corrected
code. The new host must include the actual Node installation in PATH as well
as Go and Git, so optional Node-based oracle/browser checks do not silently
skip as they did in the earlier service environment.

Use the established explicit roster: planner/recovery qwencloud/qwen3.8-max;
implementer google-native/gemini-3.8-flash with its existing 2.4M input-token
per-minute pacing cap; Captain claude/claude-opus-5; Verifier
claude/claude-sonnet-5. This keeps the native connection that passed live
certification and avoids returning to the unpaced compatibility connection.

Because the current bootstrap manifest binds revision-1 plan bytes, record
the externally approved revision through Sworn's native plan surface and
start a new explicitly bound run for revision 2. Preserve r1/r2 journals;
do not change their manifests or reset their accounting. Operate through
MCP with persistent request/process lifetime. The work must be repaired from
the retained input and checked, not asserted complete by the operator.

# Review status

This is a forward recovery proposal prompted by measured failures. It is not
a new product goal, a relaxed acceptance criterion, an approval receipt, or
a claim that the recovered code is correct.
