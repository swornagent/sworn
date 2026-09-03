```sworn-release-manifest-v1
{
  "approval_ref": "operator://2026-09-03-foreign-repo-honesty/1",
  "previous_plan": null,
  "release": "2026-09-03-foreign-repo-honesty",
  "repository": "sworn",
  "revision": 1,
  "schema_version": "sworn.release-manifest/v1",
  "target_ref": "refs/heads/release/2026-09-03-foreign-repo-honesty",
  "tracks": [
    {
      "depends_on": [],
      "id": "T1-honesty",
      "slices": [
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-03-foreign-repo-honesty/S1-refusals-carry-cause.json",
          "depends_on": [],
          "digest": "sha256:8e2f38c754d2f2e8120915609be1cacb5723e3a87414d434fed068ad8f44706a",
          "id": "S1-refusals-carry-cause",
          "outcome": "Operator-facing refusals stop discarding the reason they refused: writeCommandFailure and writeKnownFailure render the raising error's own text (bounded, secret-free) beneath the fixed fallback sentence and Technical code, gitx.Error's Op (the git args) and Err (the stderr/timeout tail) reach the operator for GIT_EXECUTION_FAILED, baton.RecordError.Msg reaches the operator for INVALID_FIELD/INVALID_PATH/INVALID_PLAN_FENCE with INVALID_PATH naming which canonicalization rule failed and the offending value's actual length when it hits the 512 bound, and driver's NATIVE_NOT_CERTIFIED/INVALID_ADAPTER carry a Detail drawn from the 19-term closed vocabulary A4 enumerates (one term per independently-reachable validateNativeConfig/validatePinnedRuntimeFiles/NewNativeAdapter condition, covering cli identity, admission bounds, pin mode, credential target, version, version output, digest, family, runtime-file presence/shape/digest/duplicate/missing, trust anchor, toolchain root, and NewNativeAdapter's own key/id/version/resolver checks), surfaced by the doctor registry-build path and by init's admission messages - closing sworn#254 (INVALID_PLAN discarded the error that caused it), sworn#273 (GIT_EXECUTION_FAILED printed with no cause), and the sworn#267 residual (INVALID_ADAPTER/NATIVE_NOT_CERTIFIED name no condition). Today commandErrorCode (cmd/sworn/main.go:1094-1120) does errors.As against every one of these error types and returns only .Code; writeCommandFailure (main.go:1055-1092) never prints err.Error(), .Op, .Err, .Msg, or .Detail; writeKnownFailure (main.go:1038-1053) never touches an error at all - so every refusal beneath these codes reads identically regardless of which raise site actually fired: internal/gitx/repository.go:499, internal/baton/plan.go (:133, :143, :760, :1404, :1409, :1413) and plan_authoring.go (:43, :274), and internal/driver/native.go (:173-182 NewNativeAdapter's own four checks folding into INVALID_ADAPTER, :214-280 validateNativeConfig's eleven OR-bundled raise sites, and :311-358 validatePinnedRuntimeFiles's further sub-conditions reached through :261-268).",
          "touchpoints": [
            "cmd/sworn",
            "internal/baton",
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/baton, internal/driver by import only; no cockpit test pins the behaviour this slice changes"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/baton, internal/driver by import only; no observe test pins the behaviour this slice changes"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/baton, internal/driver by import only; no runtime test pins the behaviour this slice changes"
            },
            {
              "package": "internal/skill",
              "reason": "its tests reach internal/baton by import only; no skill test pins the behaviour this slice changes"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/baton, internal/driver by import only; no tui test pins the behaviour this slice changes"
            },
            {
              "package": "tools/batongolden",
              "reason": "its tests reach internal/baton by import only; no batongolden test pins the behaviour this slice changes"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-03-foreign-repo-honesty/S2-foreign-layouts.json",
          "depends_on": [],
          "digest": "sha256:69fbc74de66341fd09c7e0bb5cc7dd65037eef640d98978e1d15a630a82925cd",
          "id": "S2-foreign-layouts",
          "outcome": "Scope lint stops passing vacuously on a foreign repository's nested Go module layout, and a plan record stops discovering a symlinked-or-blob documents/records root only when git itself refuses the write with an opaque error: scope lint now locates the go.mod governing every candidate .go file (not just the repository root) by finding each file's nearest ancestor go.mod as the walk descends from the repository root (so a nested module like go/go.mod is found, and every file beneath it - including under non-cmd/internal/tools directories such as go/pkg/tools/ - is kept, because isModulePackagePath (internal/baton/scopelint.go:151-164) drops its hardcoded cmd/internal/tools top-segment allowlist), rebases each file's import-derived internal package id to repository-root-relative by joining its own go.mod's directory with the import-path-derived suffix so it matches the repo-root-relative dir key computePackageGraph already uses (scopelint.go:182 vs :206-213, previously divergent bases that silently dropped every cross-package edge for any nested module), refuses SCOPE_LINT_UNRESOLVED naming any scoped path no go.mod governs instead of ever returning PASS, and drops the hardcoded fallback modulePath := \"github.com/swornagent/sworn\" (scopelint.go:52,111) that today makes every reverse-dependency check silently inert whenever the real module path differs - computePackageGraph's importPath match (scopelint.go:207-213) never fires, LintSlice's findings stay empty (:349-351), and RunPlanScopeLint records PASS for every slice regardless of actual violations (plan_authoring.go:320-323) - closing sworn#272 including its real go/pkg/tools/... probe-path shape, not merely the go.mod-lookup half of it; and PrepareRecordTransition now refuses before any git write when an ancestor of the configured documents root or the configured records root is a symlink or a regular-file blob in the base tree, naming the ancestor and the remedy of declaring documents_root in docs/sworn/sworn.json, where today no ancestor check exists for either root at transition time (only ValidatePath and isReservedRecordPath for documents, internal/gitx/prepare.go:211-214, and a lexical recordRoot-prefix test for changes, :204-208) and the failure surfaces only after prepareRecord's git object writes as an opaque GIT_EXECUTION_FAILED (internal/gitx/repository.go:499) - closing sworn#273 for both roots, not documents-only. sworn's own repository, governed by its single root go.mod and carrying no top-level .go files outside cmd/, internal/, and tools/, lints exactly as before.",
          "touchpoints": [
            "internal/baton",
            "internal/gitx"
          ],
          "waivers": [
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/baton, internal/gitx by import only; no cockpit test pins the behaviour this slice changes"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/baton, internal/gitx by import only; no observe test pins the behaviour this slice changes"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/baton, internal/gitx by import only; no runtime test pins the behaviour this slice changes"
            },
            {
              "package": "internal/skill",
              "reason": "its tests reach internal/baton, internal/gitx by import only; no skill test pins the behaviour this slice changes"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/baton, internal/gitx by import only; no tui test pins the behaviour this slice changes"
            },
            {
              "package": "tools/batongolden",
              "reason": "its tests reach internal/baton, internal/gitx by import only; no batongolden test pins the behaviour this slice changes"
            },
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/baton, internal/gitx by import only; no sworn test pins the behaviour this slice changes"
            },
            {
              "package": "internal/driver",
              "reason": "its tests reach internal/baton, internal/gitx by import only; no driver test pins the behaviour this slice changes"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-03-foreign-repo-honesty/S3-bootstrap-replan-honesty.json",
          "depends_on": [],
          "digest": "sha256:711c1f9d75edd13afe3268807b1638cf935fd61deb9e6aaeb8d7a09191869b83",
          "id": "S3-bootstrap-replan-honesty",
          "outcome": "A run whose authority is a bootstrap-approved plan digest stops burning a turn on a planner dispatch no proposal it produces can ever clear, and stops that park from going invisible to every status reader: when NextRole is planner - from either a slice's captain-escalate or verifier-blocked (internal/baton/state.go:2959-2960, :2968-2969) or, when no slice carries it, the assembly's own verifier-blocked (state.go:3948-3958, reached once every track's slices have already passed, :3859-3877) - and BootstrapApprovedPlanDigest (internal/runtime/manifest.go:95) is the run's only authority, driveLoop parks the run under a new ParkCauseBootstrapAuthority instead of dispatching the planner, with a reason built from the triggering receipt's Summary (internal/baton/receipts.go:29-49, populated at state.go:2947 for a slice or state.go:3957 for the assembly) verbatim plus a fixed operator-path sentence, and RunStatus (internal/runtime/status.go) surfaces that cause and reason on every read - closing sworn#259 (a bootstrap-authority run admits a revision proposal it can never approve: run-start refuses it PLAN_AUTHORITY_CONFLICT at internal/runtime/scheduler.go:6998-7004 whenever the proposal's digest disagrees with the fixed bootstrap digest it can never equal, and sworn approve refuses it APPROVAL_AUTHORITY_CONFLICT at internal/runtime/approval.go:364-370) and sworn#278 in the narrow sense that no in-run proposal is attempted under bootstrap authority (the planner cannot compute the canonical contract digest the fence requires - internal/baton/plan.go:972-1083 - and no worker tool returns it, internal/driver/tools.go:45); today internal/runtime/scheduler.go:5690-5695 and :5698-5706 dispatch driver.RolePlanner unconditionally via proposeRevision (:6529-6537) -\u003e proposePlan (:6398-6406) -\u003e proposePlanAttempt (:6408-6527) -\u003e dispatchRoleWithScope (:6493-6495) whenever any slice or the assembly carries NextRole planner, and RunStatus has no boolean input that knows this condition at all - a dispatch observed in the fired r1 journal as a planner yielding a human attention it has no legitimate way to answer into approval and in r2 as a turn spent bisecting INVALID_PLAN_BYTES, a park that an incomplete fix would leave structurally invisible to sworn status, the TUI, and internal/cockpit.",
          "touchpoints": [
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/runtime by import only; no cockpit test pins the behaviour this slice changes"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/runtime by import only; no observe test pins the behaviour this slice changes"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/runtime by import only; no tui test pins the behaviour this slice changes"
            },
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/runtime by import only; no sworn test pins the behaviour this slice changes"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-03-foreign-repo-honesty/S4-sandbox-start-evidence.json",
          "depends_on": [],
          "digest": "sha256:5aba076c7eff729eff893c978708cd5f8f71b5e03d77872b24387d7c5d088bbf",
          "id": "S4-sandbox-start-evidence",
          "outcome": "A hosted-runner evidence refusal caused by a sandbox that failed to start now names the sandbox_start.* check and its bounded cause in both the check_evidence_incomplete refusal and the e2e mock's rerun-cap diagnostic, and TestKilledHostMidDriveRecoversCleanly's lease/sleep margin is widened so a loaded runner cannot race it - closing sworn#277, where the evidence rerun cap exhausts after 3 re-runs with only a hardcoded \"sandbox starts likely failing\" guess (test/e2e/production_journey_linux_test.go:251-254) because contractErrorCode (internal/driver/checkevidence.go:131-137) returns only contractErr.Code and discards contractErr.Detail, so the structured check+cause envelope failSandboxStart already builds (internal/driver/sandbox_start_detail.go:94-99) never reaches applyCheckEvidence's CHECK_EVIDENCE_INCOMPLETE refusal (checkevidence.go:313-320); and closing sworn#263, where the kill-race test's 200ms margin (SWORN_TEST_OWNER_LEASE_MILLIS=500 at cmd/sworn/main_test.go:1111, sleep 700ms at :1122) races a loaded CI runner per the same reasoning PR #276 (6375a327) already applied twice to timing pins in test/e2e/surface_parity_linux_test.go, and sworn#277's own evidence cites 6 of 8 hosted runs red on this class in one day, never locally.",
          "touchpoints": [
            "internal/driver",
            "test/e2e",
            "cmd/sworn"
          ],
          "waivers": [
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins the behaviour this slice changes"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver by import only; no observe test pins the behaviour this slice changes"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; no runtime test pins the behaviour this slice changes"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver by import only; no tui test pins the behaviour this slice changes"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-03-foreign-repo-honesty/S5-codex-tool-budget-parity.json",
          "depends_on": [
            "S1-refusals-carry-cause"
          ],
          "digest": "sha256:28b73dd291f89fed80678dec8b09117e3b9425dd42d519822cb8a8e123172a76",
          "id": "S5-codex-tool-budget-parity",
          "outcome": "The codex lane's per-tool-call MCP budget follows sworn's invocation deadline exactly as the claude lane's already does, closing the codex-lane follow-up to PR #275 (tracked as S5 in contracts/2026-09-03-foreign-repo-honesty/plan.md; no sworn issue number exists yet for it) - internal/driver/native_linux.go:2883 today hardcodes `tool_timeout_sec = 300` into every codex [mcp_servers.sworn] TOML block built by nativeConfigFiles, with no comment and zero references to tool_timeout_sec across any *_test.go file, so a codex worker running a contract check longer than five minutes hits the same wall the fired dogfood verifier hit on claude before PR #275 lifted MCP_TOOL_TIMEOUT/MCP_TOOL_IDLE_TIMEOUT to MaxTimeoutMillis for that lane (internal/driver/native_environment_linux.go:13,30-31; MaxTimeoutMillis=86_400_000 at internal/driver/contract.go:32). After this slice, codex's tool_timeout_sec derives from that same MaxTimeoutMillis constant (seconds, floor), sharing one source with the claude lane's millis value so the two numbers cannot drift apart again; startup_timeout_sec and every other codex TOML key stay exactly as they are.",
          "touchpoints": [
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins the behaviour this slice changes"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver by import only; no observe test pins the behaviour this slice changes"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; no runtime test pins the behaviour this slice changes"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver by import only; no tui test pins the behaviour this slice changes"
            },
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver by import only; no sworn test pins the behaviour this slice changes"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-09-03-foreign-repo-honesty/S6-double-park-honesty.json",
          "depends_on": [],
          "digest": "sha256:0a49e7bad1b02cd8bebc3de181cd3c90e3838c6c5064e277a395dbd5cc0d2dbd",
          "id": "S6-double-park-honesty",
          "outcome": "A second park on one claimed dispatch whose first attention is already resolved recovers cleanly instead of returning CORRUPT_JOURNAL on every start forever, closing sworn#257 - recoverHumanParkCheckpoint (internal/runtime/turn_recovery.go:2258-2265) matches a checkpoint's own attention against the active[WorkIdentity] entry from activeAttentionWork (turn_recovery.go:1923-1942, which excludes resolved attentions at 1931-1934), so a resolved first checkpoint sees the still-open second attention under the same key, its ID differs, and the loop fails closed on every replay; observed in R6-r2 (2026-08-29, docs/captures/2026-08-30-legible-refusals-run-report.md) on a revision-2 planner dispatch whose confirmation yield resolved before a bytes-unrecoverable second yield, tolerated only by an unmerged operator patch (patch 9, ~/.local/share/sworn/sworn/2026-08-28-legible-refusals/ops/patch-9-repark-checkpoint.diff).",
          "touchpoints": [
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/runtime by import only; no cockpit test pins the behaviour this slice changes"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/runtime by import only; no observe test pins the behaviour this slice changes"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/runtime by import only; no tui test pins the behaviour this slice changes"
            },
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/runtime by import only; no sworn test pins the behaviour this slice changes"
            }
          ]
        }
      ]
    }
  ]
}

```

# 2026-09-03-foreign-repo-honesty

The first sworn release on a foreign repository (fired, 2026-09-02) merged
end-to-end, and on the way it found the places where sworn silently
assumed it was operating on itself. Every defect in this release was met
in the wild on that run or in the sworn-side work that made it possible;
each slice names the run or CI evidence that proved it.

## The theme

Sworn is honest at the worker boundary - R6 made every refusal there name
its cause. The operator and engine boundaries did not inherit that
honesty, and the fired run paid for it in diagnosis rounds:

- Every operator-surface refusal prints a fixed sentence and a bare code
  while the raise site discards a precise reason it already built. Three
  of yesterday's walls (an empty plan committed after a silent pin
  failure, a 512-character bound found by bisection, a symlinked docs root
  found only with a logging git wrapper) were this defect. S1.
- Scope lint hardcodes sworn's own module path as its fallback and
  reports PASS on an empty finding set; plan record writes beneath a
  symlink with no check before git refuses. S2.
- A bootstrap-authority run dispatches a planner for a revision it can
  never approve and the planner cannot even compute the digest the fence
  requires. Two of the fired run's four journals ended exactly there. S3.
- Hosted CI red-flagged six of eight runs in a day on sandbox-start and
  drive-completion timing, with a diagnostic that guesses "likely
  failing" because the evidence refusal carries no sandbox cause. S4.
- The codex lane still hardcodes a five-minute tool-call cap while the
  claude lane now follows the invocation budget (PR #275). S5.
- A dispatch that parks twice is ruled CORRUPT_JOURNAL on every start;
  R6 ran on an operator patch for it. S6 makes that patch product.

## Shape

One track, six independent slices, no ordering dependencies; S1 is listed
first because every later slice's refusals benefit from it. Deliberately
out of this release: non-Go toolchain provisioning in the sandbox
(sworn#270 - needs a design decision first), the seal-slot deadlock
(sworn#255), and whether the verifier should accept partitioned evidence
for a long check (a policy question, not a defect).

## Project facts the workers need

Module at the repository root. The sandbox PATH is `/usr/bin:/bin`; the
Go toolchain is at `/usr/local/go/bin` and every check carries the PATH
prefix. Checks that exercise the driver package start real bwrap
sandboxes; a `PROCESS_START_FAILED` under load is the class S4 is about,
and a rerun is legitimate evidence for it.
