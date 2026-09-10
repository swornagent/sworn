```sworn-release-manifest-v1
{
  "approval_ref": "operator://2026-09-05-preserve-work/1",
  "previous_plan": null,
  "release": "2026-09-05-preserve-work",
  "repository": "sworn",
  "revision": 1,
  "schema_version": "sworn.release-manifest/v1",
  "target_ref": "refs/heads/release/2026-09-05-preserve-work",
  "tracks": [
    {
      "depends_on": [],
      "id": "T1-preservation",
      "slices": [
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-05-preserve-work/S1-durable-unverified-checkpoints.json",
          "depends_on": [],
          "digest": "sha256:f2af22a99150fefbe246058c8ff51bfc5812f507cbb4b759ea5d4c44b6fc52dd",
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
            "docs/run.md"
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

Preserve implementation progress through failed handoffs, bounded stops and interruption, so recovery repairs or continues the work instead of repeating it. This is the first implementation release in the agreed stabilization programme. It does not make the orchestrator more autonomous yet.

# Authority and preparation

Brad is the external approver. This revision is a proposal, not an approval or run receipt. The operator reference above is the proposed approval identity and carries no authority by itself. Review these exact pinned manifest bytes and the four contract files.

The inspection baseline is main at 5a9aefeced602114f0dee25fc3d2b370651c82cf. Implementation follows integration of the in-flight 2026-09-03-foreign-repo-honesty release. Prepare the named release target from the resulting verified main before recording this plan; record its exact base and contract-tree identity then. Recheck the contracts against that base; a changed product promise requires a forward-only revision and approval, not silent widening. No existing release, track or live run is taken over by this plan.

# Current evidence

- internal/runtime/scheduler.go runImplementationCycle closes a writable workspace on a production dispatch error.
- internal/gitx/workspaces.go removes abandoned owned workspaces during new run-workspace initialization, before ordinary runtime recovery. A shutdown defer alone cannot cover abrupt interruption.
- internal/driver/tools.go rejectSubmission already permits bounded in-session correction; preserve and extend this instead of rebuilding it.
- Valid production handoffs already prepare and journal exact candidate identity before success. Checkpoints must remain a distinct, unverified recovery state.
- internal/runtime/track_base.go evidenceOnlyReseal already distinguishes an independent evidence-only FAIL from a code defect.
- Issue #288 documents productive work stopped at 200 turns; widening a run manifest is currently an operator workaround and can require replaying lost work.

# Ordered slices

1. S1 provides safe, durable unverified capture and same-authority restoration after orderly stops.
2. S2 extends that mechanism across process interruption and abandoned-workspace reconciliation.
3. S3 uses preserved work to finish handoffs and evidence repair, removing prose-length requirements without weakening substantive checks.
4. S4 makes configured budget stops recoverable in the same run after an explicit bounded grant, preserving cumulative usage.

One serial track owns the shared runtime, driver, journal and workspace boundaries. Dependencies and consumed products are real: each slice exercises preservation provided by its predecessor. Independent review and host verification can run separately; this plan does not pretend shared mutation paths are parallel tracks.

# Acceptance and evidence

Each contract names a real built-product acceptance boundary, existing fixtures to extend, and negative cases. Mock or package-only checks cannot establish the production preservation promise. Use deterministic local providers through production adapters for fault injection; separately authorized live-model delivery is the subsequent reliability campaign, not a substitute for deterministic crash-cut evidence.

Saved work is not verified work. Preserve the measured product tree and its provenance, never synthesize an approval or verdict. A crash may leave partial unverified filesystem state: report this honestly and restore only under the correct authority and exclusive writer ownership.

# Scope and exclusions

This release covers implementation filesystem progress and related submissions, evidence repair, recovery status and bounded continuation grants. It excludes planner draft persistence, automatic plan approval, orchestrator-first question routing, provider/model selection, ecosystem toolchain provisioning, context compaction, remote publication, promotional work and broad architecture changes.

Supporting package scope includes affected tests/projections so a passing contract cannot omit composition checks through import-only consumer packages. It authorizes only the promised preservation behavior and its support.

# Checks and host execution

The contract check lists include the repository-required product suite, sequential host end-to-end suite, product race detector, vet, asserted formatting, module tidiness, diff checks and Darwin build. They are explicitly host_checks. Contained workers/verifiers must not execute nested containment or fabricate empirical evidence; they consume the engine-bound host results.

The long process suites run once, in order, for an exact candidate. Do not duplicate them in a contained role, run the E2E suite under race, or rerun an unchanged green candidate for ceremony. A changed candidate must obtain fresh applicable evidence. Before a commit, obey AGENTS.md's required checks.

# Delivery programme after this release

Second implementation release: give the current recovery orchestrator useful approved context and bounded read-only investigation, then route ordinary questions through it before a human. Explicit human decisions and independent verdict authority stay intact. The present human-first rule prevented content-free automation loops; richer recovery must precede changing that rule.

Reliability campaign: repeated real serial and parallel releases including an interruption, a budget stop, evidence repair, verifier remediation and exact assembly. Measure avoidable human interventions, recovered versus discarded work, recovery time and accepted-slice cost. Full Fired-stack claims require its toolchain provisioning work first.

# Review record

A separate read-only technical review confirmed the terminal-error and startup-cleanup loss paths, the existing in-session correction path and evidence-only reseal, and the need to distinguish retained checkpoints from candidate authority. No implementation or approval has been recorded by this proposal.
