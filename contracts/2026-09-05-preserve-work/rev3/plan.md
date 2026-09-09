```sworn-release-manifest-v1
{
  "approval_ref": "operator://2026-09-05-preserve-work/3",
  "previous_plan": "26b14f343cfc6bccacc20e107ae266e05b0ac6a4",
  "release": "2026-09-05-preserve-work",
  "repository": "sworn",
  "revision": 3,
  "schema_version": "sworn.release-manifest/v1",
  "target_ref": "refs/heads/release/2026-09-05-preserve-work",
  "tracks": [
    {
      "depends_on": [],
      "id": "T1-preservation",
      "slices": [
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-05-preserve-work/rev3/S1-durable-unverified-checkpoints.json",
          "depends_on": [],
          "digest": "sha256:c6f8cfe559038a480e5e9010a0bc82679ffac27a2d24231efbca419b54b7dfa0",
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
            "contracts/2026-09-05-preserve-work/recovery/s1-2fcaa718.patch.gz.b64",
            "contracts/2026-09-05-preserve-work/recovery/s1-repaired.patch.gz.b64"
          ]
        },
        {
          "consumes": [
            "S1-durable-unverified-checkpoints"
          ],
          "contract_path": "contracts/2026-09-05-preserve-work/rev3/S2-interrupted-work-reconciliation.json",
          "depends_on": [
            "S1-durable-unverified-checkpoints"
          ],
          "digest": "sha256:61e0b6bf9d4764be1703401edd26cb7d43583798cbde96bae60bae27274af4e1",
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
          "contract_path": "contracts/2026-09-05-preserve-work/rev3/S3-targeted-handoff-repair.json",
          "depends_on": [
            "S2-interrupted-work-reconciliation"
          ],
          "digest": "sha256:73c72eff5a42d1577810050d5b10b379152e647c44d79120b6b3c1a1450ed9da",
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
          "contract_path": "contracts/2026-09-05-preserve-work/rev3/S4-resumable-budget-stops.json",
          "depends_on": [
            "S3-targeted-handoff-repair"
          ],
          "digest": "sha256:60b47bbb38ef2f3ab0e7eefb5552a4aa73442451c97d241356ae6201f91a6b9c",
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

# Complete preservation from the repaired implementation

This is a proposed revision 3 of the same four-slice preservation release.
Brad remains the external approver. It has not been approved or recorded.
No slice is added, retired or declared passed. All acceptance criteria and
slice outcomes remain unchanged.

## Why this revision

The previous run produced useful code but retried host-check failures without
the failed check output, prior submission or latest candidate as repair input.
It then parked without a notification-source event. Operator diagnosis found
an endlessly failing retry fixture and impossible newline comparisons.

The retained repair input fixes those test defects, adds durable exact host
repair context, validates matching checkpoints across automatic/manual retry,
resolves manifest-backed checkpoint scope, and records the typed park event
at the between-tries gate. Review and improve this implementation where
required; do not rebuild it from scratch. Saving work remains distinct from
candidate admission and independent verification.

## Exact repair material

Input: contracts/2026-09-05-preserve-work/recovery/s1-repaired.patch.gz.b64.
Decoded SHA-256: eea487af37b7c873204acdbed76174681023d5e447c5b9365528f7ec78b4d880.
Decoded length: 130353 bytes; 25 product paths.
The patch is based on product contents at 68c4deefcd7a6eac8b91ea346ae623bfae4373f7
and applies cleanly against the current prepared target's compatible product
contents. It excludes all reserved records and contract/approval files.
Do not copy any older control tree. Remove both old and new recovery input
artifacts after application; their deletion is explicitly scoped.

The new prepared target/contract-tree commit must be bound and applicability
rechecked against that exact commit before native recording. The previous
target is 74241e177d96bc158ab40c49d468fd27fcf5b86f; the previous approved plan
blob is 26b14f343cfc6bccacc20e107ae266e05b0ac6a4. Historical journals, manifests,
records and approvals remain unchanged.

## Checks and evidence

Every slice keeps the same checks and host-only execution boundary, with two
test-runner deadline corrections: the complete serial E2E suite gets 45
minutes instead of 35; the product race suite gets an explicit 20 minutes
instead of Go's implicit 10-minute default. All assertions and tests stay
present, and race detection remains enabled. Align CI and AGENTS.md
in the prepared base. The 35-minute diagnostic run passed 88 completed cases,
including the repaired preservation journey and full topology-recovery group,
then timed out in consumed-base recovery. It is a failed full gate, not a PASS.
Operator validation of the final repair is recorded separately; it never
substitutes for Sworn's exact-candidate host evidence and independent verifier.

S1 keeps A1-A6 verbatim, adds only the new input path to scope, and clarifies
the repair constraints. S2-S4 change only the E2E and race deadlines in checks and
host_checks. Their outcomes, acceptance, scope and dependencies are unchanged.
The separately approved roster/live-switch plan still follows preservation.

## Host operation

Use a validated operator binary containing these repair mechanisms. Keep the
explicit roster: qwencloud/qwen3.8-max for planner/recovery;
google-native/gemini-3.8-flash for implementer with the existing 2.4M pacing;
claude/claude-opus-5 for Captain and claude/claude-sonnet-5 for Verifier.
Do not change model budgets or invent a fallback. Include installed Node in
PATH and pass the main checkout's operator telemetry config explicitly.
The local Langfuse exporter has been verified with an explicitly backfilled
historical evaluation; verify observation of new work at startup as well.

After external approval, record this revision through the native plan surface
and start an explicitly bound recovery run using persistent MCP host/request
lifetime. Run a fresh design/Captain review over the retained implementation,
then admit only checked candidates and independent verdicts. Do not resume an
old run against stale 35-minute contracts or fabricate legacy repair bindings.
