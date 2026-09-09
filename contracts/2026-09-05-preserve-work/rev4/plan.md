```sworn-release-manifest-v1
{
  "approval_ref": "operator://2026-09-05-preserve-work/4",
  "previous_plan": "27379f92b9f6f1d33a196084e19f8f72bae3974d",
  "release": "2026-09-05-preserve-work",
  "repository": "sworn",
  "revision": 4,
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
          "contract_path": "contracts/2026-09-05-preserve-work/rev4/S2-interrupted-work-reconciliation.json",
          "depends_on": [
            "S1-durable-unverified-checkpoints"
          ],
          "digest": "sha256:5a92ba01d576f2c980a4256744ef4153ac8b358639bc75de6b6b4a4a217024dd",
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

# Complete preservation on a batching implementer lane

This is a proposed revision 4 of the same four-slice preservation release.
Brad remains the external approver. No slice is added, retired or declared
passed. All acceptance criteria and slice outcomes remain unchanged. S1, S3
and S4 keep their revision-3 contract bytes and paths; S1's verified pass
(receipt 05fbba49, candidate fa2aebf2) carries unchanged.

## Why this revision

Under revision 3 the S2 implementation ran on google-native/gemini-3.8-flash
as run 2026-09-05-preserve-work-recovery. The first try was lost to an
adapter continuation defect (sworn#291) and a scope fence on a stray build
artifact; three further tries each consumed the full 60-minute invocation
limit and ended INVOCATION_TIMEOUT. S1's checkpoints kept the tries
cumulative, but each try began a fresh conversation (sworn#292), spent about
half its turns re-reading, and converged at roughly 27 net lines per hour on
the crash-cut recovery seam. That run is parked and stays inert; its
checkpoints remain anchored as stale reference.

This revision changes only the S2 contract: it adds operator-context
constraints naming the stale checkpoints as reference-only, the contained-
workspace facts that wasted turns (a credential-less TUI test, the 10-minute
package deadline, building binaries into the workspace root), and the
observed recovery seam. Outcome, scope, acceptance, checks and host_checks
are unchanged. S2 restarts at design under the new contract digest.

## Host operation

Roster for the replacement run: qwencloud/qwen3.8-max for planner/recovery;
claude/claude-sonnet-5 (native, batching tool calls) for implementer;
claude/claude-opus-5 for Captain and claude/claude-opus-5 for Verifier, so
the implementer and the independent verifier remain different models. Set
the per-invocation timeout to 7200000 ms; keep max_turns_per_work 800 and
max_output_tokens_per_work 1048576. Do not invent a fallback. Use the
validated ops/sworn-repair binary, installed Node in PATH and the main
checkout's operator telemetry config, as for revision 3.

After external approval, bind the prepared target and contract-tree commit,
record this revision through the native plan surface, and start an
explicitly bound run with persistent MCP host/request lifetime. Do not
resume the parked revision-3 run. Continuation of the implementer's own
session across tries is not promised by this release (sworn#292 follows).
