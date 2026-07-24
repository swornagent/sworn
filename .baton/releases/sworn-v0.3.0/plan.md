```baton-plan-v1
{
  "schema_version": "baton.plan/v1",
  "release": "sworn-v0.3.0",
  "repository": "swornagent/sworn",
  "target_ref": "refs/heads/release/v0.3.0",
  "release_ref": "refs/heads/release-wt/sworn-v0.3.0",
  "record_root": ".baton/releases",
  "approval_ref": "github://swornagent/sworn/issues/157#baton-plan-approval-sworn-v0.3.0",
  "tracks": [
    {
      "id": "T0-reset",
      "ref": "refs/heads/track/sworn-v0.3.0/T0-reset",
      "depends_on": [],
      "touch_surfaces": [".github/workflows", "AGENTS.md", "cmd/sworn", "go.mod", "go.sum", "internal", "tools/batonassets", "tools/batongolden"],
      "work": [
        {
          "id": "R0-reset-admission",
          "outcome": "Replace the superseded kernel with the six-package S0/S1 seam and admit the exact published Baton v1.0.0-rc.2 assets.",
          "scope": {
            "include": [".github/workflows", "AGENTS.md", "cmd/sworn", "go.mod", "go.sum", "internal", "tools/batonassets", "tools/batongolden"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-R0-pin",
              "text": "The real binary startup-validates tag object b80f3e27f0e0a71a4883bcc282e4843e085f0e04, commit 890238ef063bb53cf51fb3359f1ff527f14846c6, tree 97513f3e6f798f3ad04d5b510a49496a605a8ea4, release archive sha256:968088ede0c3bfbafb0a9372d3abbf6853556cc2a6e85ffc25615d6332977e63, support package sha256:676c630c6a4ef3f752d604efaa5e51958adec0d8580b74cec7fb1e689b1d3436, and the deterministic 14-asset, 50387-byte embed including all three templates; any mismatch fails closed."
            },
            {
              "id": "A-R0-cut",
              "text": "The old production packages are removed, AGENTS.md describes v0.3.0, the replacement builds only through cmd/sworn plus internal/baton, runtime, journal, gitx, and driver seams, an old or foreign database is rejected untouched, and every unexecuted autonomous case remains NOT RUN."
            }
          ],
          "checks": ["go test ./tools/batonassets/... ./tools/batongolden/...", "go test ./...", "go vet ./...", "git diff --check"],
          "constraints": ["Do not reuse archived source, execute Node in production, create owner refs or statuses, or store a second Baton lifecycle."],
          "depends_on": []
        }
      ]
    },
    {
      "id": "T1-baton",
      "ref": "refs/heads/track/sworn-v0.3.0/T1-baton",
      "depends_on": ["T0-reset"],
      "touch_surfaces": ["internal/baton", "tools/batongolden"],
      "work": [
        {
          "id": "R1-baton-compatibility",
          "outcome": "Implement the complete deterministic RC2 record, transition, product-identity, composition, and seven-action contract in pure Go.",
          "scope": {
            "include": ["internal/baton", "tools/batongolden"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-R1-contract",
              "text": "Strict records, limits, bindings, evidence, status semantics, candidate history, product identity, all seven actions, exact fast-forward and two-parent composition, expected-head transactions, retry reconciliation, and adversarial errors match immutable RC2 goldens byte-for-byte; the real harness passes every portable fixture without claiming an autonomous result."
            }
          ],
          "checks": ["go test -race ./internal/baton/...", "go test ./tools/batongolden/...", "go vet ./internal/baton/... ./tools/batongolden/...", "git diff --check"],
          "constraints": ["The JavaScript reference is a development oracle only; unknown inputs fail closed and callers cannot choose refs, paths, Git commands, trees, parents, messages, or merge modes."],
          "depends_on": ["R0-reset-admission"]
        }
      ]
    },
    {
      "id": "T2-journal",
      "ref": "refs/heads/track/sworn-v0.3.0/T2-journal",
      "depends_on": ["T0-reset"],
      "touch_surfaces": ["internal/journal"],
      "work": [
        {
          "id": "R2-runtime-journal",
          "outcome": "Provide the small SQLite journal for idempotent commands, finite claims, external effects, immutable receipts, events, and a lossy outbox.",
          "scope": {
            "include": ["internal/journal"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-R2-journal",
              "text": "Exactly seven tables atomically enforce replay-stable command receipts, conflicting-byte refusal, finite owner/generation claims, immutable observations, and uncertain-effect stop-before-retry; read-only open never creates or migrates, and journal facts cannot synthesize Baton truth."
            }
          ],
          "checks": ["go test -race ./internal/journal/...", "go vet ./internal/journal/...", "git diff --check"],
          "constraints": ["Use database/sql plus modernc.org/sqlite with one serialized durable connection; a new table requires fresh Captain review and outbox loss never controls delivery."],
          "depends_on": ["R0-reset-admission"]
        }
      ]
    },
    {
      "id": "T3-gitx",
      "ref": "refs/heads/track/sworn-v0.3.0/T3-gitx",
      "depends_on": ["T0-reset"],
      "touch_surfaces": ["internal/gitx"],
      "work": [
        {
          "id": "R3-git-boundary",
          "outcome": "Provide confined repository, object, worktree, quarantine, composition, and atomic-ref primitives without advancing Baton lifecycle independently.",
          "scope": {
            "include": ["internal/gitx"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-R3-git",
              "text": "Literal immutable Git facts drive canonical identity, private worktrees, quarantine, history and product-tree checks, deterministic commits, and atomic full-ref compare-and-set; tests distinguish exact old, exact new, mixed, symbolic, missing, and third values and preserve any live, dirty, foreign, or replaced workspace for recovery."
            }
          ],
          "checks": ["go test -race ./internal/gitx/...", "go vet ./internal/gitx/...", "git diff --check"],
          "constraints": ["Use a sanitized Git executable with literal OIDs and no Git library, force, rebase, squash, model conflict resolution, canonical object exposure, unsafe config, hooks, alternates, or replacement objects."],
          "depends_on": ["R0-reset-admission"]
        }
      ]
    },
    {
      "id": "T4-driver-core",
      "ref": "refs/heads/track/sworn-v0.3.0/T4-driver-core",
      "depends_on": ["T0-reset"],
      "touch_surfaces": ["internal/driver"],
      "work": [
        {
          "id": "R4-driver-proxy-fake",
          "outcome": "Implement baton.driver/v1, the strict invocation-bound submission proxy, contained processes, and the deterministic external fake.",
          "scope": {
            "include": ["internal/driver"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-R4-boundary",
              "text": "One executable implements the role-neutral contract and preserves operation, workspace mode, ordered input digests, freshness and limits; Planner, Implementer, Captain and Verifier use explicit configured models, while portable Merge-role coverage accepts model null only and Sworn delivery keeps Merge deterministic, engine-owned and undispatched to a model. One sealed submission may contain only permitted exact artifacts, decision and logical action, transport state remains separate from Baton outcomes, and the fake passes P01-P10 including cancellation, caps, seeded delivery and read-only verification."
            },
            {
              "id": "A-R4-handoffs",
              "text": "Model-facing Baton proof handoffs are size-bounded and retain only exact deterministic local check commands, tool versions, exit statuses, bounded salient acceptance facts and SHA-256 references to engine-owned deterministic local check logs held digest-addressed and out-of-band. They never retain raw agent or provider stdout/stderr, prompts, completions, source, diffs, credentials, or tool request/response payloads; those are bounded in transit and discarded or allowlist-sanitized as appropriate. Tests reject inline check logs, missing or mismatched log digests, forbidden retained content, and later role input that substitutes log bodies for bounded references."
            }
          ],
          "checks": ["go test -race ./internal/driver/...", "go vet ./internal/driver/...", "git diff --check"],
          "constraints": ["Every model-facing process is fresh, bounded and externally contained; no role-specific driver, lifecycle, default, fallback, retry, provider rotation, caller-selected ref, arbitrary effect, model-configurable Merge, or synthesized Baton decision or verdict is allowed."],
          "depends_on": ["R0-reset-admission"]
        }
      ]
    },
    {
      "id": "T5-loop",
      "ref": "refs/heads/track/sworn-v0.3.0/T5-loop",
      "depends_on": ["T1-baton", "T2-journal", "T3-gitx", "T4-driver-core"],
      "touch_surfaces": ["cmd/sworn", "internal/driver", "internal/runtime", "test/e2e"],
      "work": [
        {
          "id": "S1-walking-skeleton",
          "outcome": "Start from bounded user intent, obtain exact external plan approval, then drive one track through all Baton responsibilities, fresh verification, and exact Merge.",
          "scope": {
            "include": ["cmd/sworn", "internal/driver", "internal/runtime", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-S1-planner-gate",
              "text": "A real cmd/sworn integration starts sworn run from bounded intent plus configured repository, target, release and approval constraints, dispatches the configured role-neutral Planner driver and model, accepts exactly one strict baton.plan/v1 proposal, validates its bytes without approving them, durably records awaiting external approval, exposes the exact raw plan digest, and proves no Baton authority ref, status, installation effect or delivery effect exists."
            },
            {
              "id": "A-S1-approved-flow",
              "text": "Only a trusted protected-evidence resolver matching the proposed raw digest releases the wait; Sworn journals one canonical install effect, creates one baseline commit and records one resulting receipt. After a crash, repeated resolver, reconciliation or installApprovedPlan action calls may return that identical canonical result receipt with changed:false, but never duplicate a mutation, commit, ref, status, effect or receipt identity. Sworn then uses the external fake to complete Implementer design stop, Captain routing, resumed build, fresh read-only work Verifier, deterministic engine-owned track composition with no model dispatch, distinct fresh assembly Verifier, and atomic integration whose target tree equals the passed assembly."
            }
          ],
          "checks": ["go run ./test/e2e/cmd/asserttests -execute-and-assert -package=./cmd/sworn/... -package=./test/e2e/... -expect=TestRunPlannerApprovalGate -regex='^(TestRunPlannerApprovalGate)$'", "go test -race ./internal/driver/... ./internal/runtime/... ./cmd/sworn/... ./test/e2e/...", "go vet ./internal/driver/... ./internal/runtime/... ./cmd/sworn/... ./test/e2e/...", "git diff --check"],
          "constraints": ["The Planner proposes but cannot approve; no driver output, Sworn process, local command or conversation grants authority. Each scheduling pass re-derives Baton truth from committed records and refs, Merge is deterministic and engine-owned with model null and no model configuration or dispatch, and only one writer owns the track."],
          "depends_on": ["R1-baton-compatibility", "R2-runtime-journal", "R3-git-boundary", "R4-driver-proxy-fake"]
        },
        {
          "id": "S2-native-cli",
          "outcome": "Run Codex CLI and Claude Code CLI through the common driver with explicit models for the four model-facing roles, bounded cancellation, and clean Verifier contexts.",
          "scope": {
            "include": ["cmd/sworn", "internal/driver", "internal/runtime", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-S2-cli",
              "text": "Per-role configuration independently selects driver and an explicit model for Planner, Implementer, Captain and Verifier; Merge has no driver or model configuration, remains engine-owned, and is represented only by model null in portable contract coverage. Versioned fake-executable fixtures prove supported non-interactive ephemeral argv, bounded output/cancellation, no resume/fallback, and clean read-only Verifier execution with Codex user config, rules and memories disabled; Claude argv comes from captured installed help, both pass P01-P10, and credential-gated live smokes report independent PASS, FAIL or NOT RUN."
            },
            {
              "id": "A-S2-codex-argv",
              "text": "The Codex CLI 0.145.0 fixture binds exact non-Verifier argv `codex --yolo exec --ephemeral -C ${workspace} --json -o ${engine_control_dir}/last-message --ignore-user-config --ignore-rules --model ${model} -`. Every Verifier argv inserts both exact pairs `--disable memories` and `--disable external_agent_memory_import` between `--ignore-rules` and `--model`, with its bounded control output outside the read-only candidate; omission, reordering, resume, inherited configuration, rules, either memory capability, or an unversioned argv shape fails closed."
            }
          ],
          "checks": ["go run ./test/e2e/cmd/asserttests -execute-and-assert -package=./internal/driver/... -expect=TestCodexCLI0145ExactArgv -expect=TestCodexCLI0145VerifierDisablesMemoryFeatures -regex='^(TestCodexCLI0145ExactArgv|TestCodexCLI0145VerifierDisablesMemoryFeatures)$'", "go test -race ./internal/driver/... ./internal/runtime/... ./cmd/sworn/... ./test/e2e/...", "go vet ./internal/driver/... ./internal/runtime/... ./cmd/sworn/... ./test/e2e/...", "git diff --check"],
          "constraints": ["Native CLIs retain their own tool loops; an unavailable Claude account is NOT RUN. Credentials, prompts, completions, source, diffs, raw argv, tool request/response payloads and raw agent/provider stdout/stderr are bounded in transit and never retained or emitted."],
          "depends_on": ["S1-walking-skeleton"]
        },
        {
          "id": "S3-coach-topology-recovery",
          "outcome": "Add dependency-ready parallel tracks, one serial writer per track, serial exact composition, typed operator commands, and full crash recovery.",
          "scope": {
            "include": ["cmd/sworn", "internal/runtime", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-S3-topology",
              "text": "Independent ready tracks run concurrently in isolated worktrees, work stays serial within a track, the oracle chooses the most advanced authoritative status, composition stays serial, conflicts preserve the prior release ref, and a distinct fresh assembly Verifier gates exact Merge."
            },
            {
              "id": "A-S3-control-recovery",
              "text": "Pause, resume, cancel, retry and takeover are closed replay-stable commands, and the generated real-binary crash matrix covers every workspace, model-facing role invocation, candidate, Baton action, verifier, stop and cleanup edge with no blind uncertain retry, duplicate effect, leaked process, phantom verdict or view. Verifier transport, runner, tool, environment or invalid-handoff failure is operational NO_VERDICT over the unchanged candidate; Captain failure produces no decision or transition; Planner and Implementer failure is operational and leaves durable Baton status byte-for-byte unchanged. None is converted to PASS, FAIL, BLOCKED, PROCEED, REVISE, ESCALATE or MERGED."
            },
            {
              "id": "A-S3-verdict-recovery-matrix",
              "text": "An explicit real-binary recovery matrix proves work FAIL returns the same work identity to implement/ready/implementer for a new candidate and proof, while work BLOCKED, assembly FAIL at verify/ready/planner and assembly BLOCKED at verify/blocked/planner preserve the old materialised lineage and route to Planner. Each blocked or assembly-failed continuation requires newly approved plan bytes bound by a new protected exact-digest approval plus new work and release identities; no in-place assembly retry, rebound or gate clearing is accepted."
            },
            {
              "id": "A-S3-approval-recovery",
              "text": "Restart while awaiting approval restores the same proposal bytes and digest without dispatching Planner again; missing or explicitly rejected approval stays paused with no Baton namespace, and changed proposal or mismatched approval is refused and requires a new digest approval. Repeated approval resolution, reconciliation, resume or crash replay returns the identical canonical install result with changed:false, one baseline commit and one receipt identity, with no duplicate mutations, refs, statuses or effects."
            }
          ],
          "checks": ["go run ./test/e2e/cmd/asserttests -execute-and-assert -package=./cmd/sworn/... -package=./test/e2e/... -expect=TestRunPlannerApprovalRecovery -expect=TestRunPlannerApprovalRejection -expect=TestRunRoleTransportFailureMatrix -expect=TestRunBatonVerdictRecoveryMatrix -regex='^(TestRunPlannerApprovalRecovery|TestRunPlannerApprovalRejection|TestRunRoleTransportFailureMatrix|TestRunBatonVerdictRecoveryMatrix)$'", "go test -race ./internal/runtime/... ./cmd/sworn/... ./test/e2e/...", "go vet ./internal/runtime/... ./cmd/sworn/... ./test/e2e/...", "git diff --check"],
          "constraints": ["Recovery uses normal command/effect paths, external approval comes only from the protected resolver, concurrency is bounded, pause/cancel never rewrites Baton, and release-worktree composition has one writer."],
          "depends_on": ["S2-native-cli"]
        }
      ]
    },
    {
      "id": "T6-cloud-drivers",
      "ref": "refs/heads/track/sworn-v0.3.0/T6-cloud-drivers",
      "depends_on": ["T5-loop"],
      "touch_surfaces": ["internal/driver"],
      "work": [
        {
          "id": "S4-http-cloud-drivers",
          "outcome": "Add OpenAI-compatible, DeepSeek, Gemini, and Bedrock adapters behind one bounded workspace-tool loop and the role-neutral contract.",
          "scope": {
            "include": ["internal/driver"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-S4-drivers",
              "text": "Each named adapter independently passes P01-P10 through its real translation with fake servers/signing, while one stdlib tool loop enforces path, read-only, command, call, byte, time and cancellation bounds; configured live smokes report only their own PASS, FAIL or NOT RUN with explicit configured/observed driver/model and nullable usage/cost."
            }
          ],
          "checks": ["go test -race ./internal/driver/...", "go vet ./internal/driver/...", "git diff --check"],
          "constraints": ["Use standard-library HTTP/crypto: no provider SDK, managed inference, hosted credential, marketplace, bundled default, scheduling, retry or fallback; DeepSeek is an OpenAI-compatible profile."],
          "depends_on": ["S3-coach-topology-recovery"]
        }
      ]
    },
    {
      "id": "T7-operator",
      "ref": "refs/heads/track/sworn-v0.3.0/T7-operator",
      "depends_on": ["T5-loop"],
      "touch_surfaces": ["cmd/sworn", "internal/cockpit", "internal/observe"],
      "work": [
        {
          "id": "S5-observability-eval",
          "outcome": "Make local evaluation authoritative and project privacy-safe opt-in OTLP traces and metrics without affecting control truth.",
          "scope": {
            "include": ["cmd/sworn", "internal/observe"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-S5-measure",
              "text": "Local records bind scenario, versions, outcomes, quality, exact integration, timing, overhead, retries, recovery, driver/model and nullable usage/cost; direct OTLP is disabled by default, bounded, asynchronous and lossy with stable low-cardinality metrics and restart-aware spans."
            },
            {
              "id": "A-S5-safety",
              "text": "Allowlist tests reject retention or export of raw agent/provider stdout/stderr, prompts, completions, source, diffs, credentials, argv and tool request/response payloads; only sanitized operational metadata and digest references to engine-owned deterministic local check logs are eligible. Deterministic delivery has identical candidates, verdicts, receipts, integration and exit status when export is disabled, failing, overflowing or backpressured."
            }
          ],
          "checks": ["go test -race ./internal/observe/... ./cmd/sworn/...", "go vet ./internal/observe/... ./cmd/sworn/...", "git diff --check"],
          "constraints": ["OTel and optional Langfuse export are projections only; a bounded non-production DBOS comparison may be measured, but no LangChain, LangGraph, Temporal, DBOS or other orchestrator enters production under this plan."],
          "depends_on": ["S3-coach-topology-recovery"]
        },
        {
          "id": "S6-release-cockpit",
          "outcome": "Deliver truthful terminal and embedded responsive WebUI projections with typed local controls, sanitized operational events, digest-addressed deterministic check-evidence drill-down, reconnection, and multi-run discovery.",
          "scope": {
            "include": ["cmd/sworn", "internal/cockpit"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-S6-cockpit",
              "text": "sworn board and sworn serve share one read-only Baton oracle plus a sanitized durable runtime overlay, loopback-authenticated controls submit only typed commands, snapshot-plus-offset reconnection rejects stale events, terminal/browser views agree through restart/failure, drill-down exposes only allowlisted operational facts and digest references or sanitized summaries for deterministic local check logs, and the embedded no-runtime-dependency UI passes responsive, keyboard and accessibility checks."
            }
          ],
          "checks": ["go test -race ./internal/cockpit/... ./cmd/sworn/...", "go vet ./internal/cockpit/... ./cmd/sworn/...", "git diff --check"],
          "constraints": ["The cockpit is a projection and typed command client, never a scheduler or Baton writer; it never exposes raw agent/provider stdout/stderr, prompts, completions, source, diffs, credentials or tool request/response payloads."],
          "depends_on": ["S5-observability-eval"]
        }
      ]
    },
    {
      "id": "T8-release",
      "ref": "refs/heads/track/sworn-v0.3.0/T8-release",
      "depends_on": ["T6-cloud-drivers", "T7-operator"],
      "touch_surfaces": ["README.md", "cmd/sworn", "docs/releases/v0.3.0", "test/e2e"],
      "work": [
        {
          "id": "S7-parity-release-proof",
          "outcome": "Prove useful Coach-loop parity and technical v0.3 readiness through the real binary's unattended multi-track, multi-driver delivery and failure corpus.",
          "scope": {
            "include": ["README.md", "cmd/sworn", "docs/releases/v0.3.0", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-S7-delivery",
              "text": "The built release binary returns PASS for all 12 case IDs under profiles.autonomous_engine.cases in the immutable RC2 conformance/manifest.json through the real baton.engine-conformance/v1 adapter and its normal persistence, scheduler, driver, workspace and process boundaries; no autonomous-engine case may be NOT RUN. A fail-closed preflight maps every exact manifest case ID to one exact anchored top-level Go test, requires each case ID and test name exactly once, and runs only after the complete mapping passes enumeration. The binary also completes a disposable multi-track repository from bounded intent through configured Planner and protected exact-digest approval, with a dependency edge, two driver families, different models among the four model-facing roles, fresh work/assembly verification, exact identities and a final target tree equal to the passed assembly integration."
            },
            {
              "id": "A-S7-failures",
              "text": "Real-binary scenarios prove Verifier timeout as operational NO_VERDICT, Captain timeout with no decision, Planner and Implementer timeout with unchanged durable Baton status, process-death recovery, stale-target refusal, work Verifier FAIL/repair, work and assembly BLOCKED routing, assembly FAIL replanning, composition conflict, truthful restart views and telemetry non-interference. Each driver has independent P01-P10 totals and truthful credential-gated live-smoke status without borrowed evidence; NOT RUN is permitted only for those separate credential-gated live provider smokes."
            },
            {
              "id": "A-S7-parity",
              "text": "The Coach parity matrix has no unratified gap and reports per-scenario and aggregate time, protocol tokens, artifacts, invocations, retries, recovery, quality and exact-integration denominators; all documented CLI commands are reached through cmd/sworn and claims distinguish proof from NOT RUN."
            },
            {
              "id": "A-S7-budgets",
              "text": "The result has at most eight production packages, 15000 non-generated production Go lines, ten direct dependencies, no unexplained production file over 700 lines, and a stripped Linux binary no larger than 25 MiB."
            }
          ],
          "checks": ["go run ./test/e2e/cmd/asserttests -execute-and-assert -package=./test/e2e/... -manifest=internal/baton/snapshot/assets/conformance/manifest.json -case=protected-external-approval=TestAE01 -case=role-instruction-credential-workspace-process-isolation=TestAE02 -case=clean-read-only-fresh-verifier-dispatch=TestAE03 -case=one-writer-per-track-with-independent-track-concurrency=TestAE04 -case=durable-invocation-attempt-and-effect-identity=TestAE05 -case=crash-recovery-at-every-effect-boundary=TestAE06 -case=timeout-cancellation-cleanup-and-bounded-retry=TestAE07 -case=dependency-scheduling-and-one-serial-worker-per-track=TestAE08 -case=exact-track-composition-and-ownership-transfer=TestAE09 -case=fresh-assembly-verification=TestAE10 -case=moved-target-compare-and-set-refusal=TestAE11 -case=exact-release-integration=TestAE12 -regex='^(TestAE01|TestAE02|TestAE03|TestAE04|TestAE05|TestAE06|TestAE07|TestAE08|TestAE09|TestAE10|TestAE11|TestAE12)$'", "go test ./...", "go test -race ./...", "go vet ./...", "bash -o pipefail -c 'unformatted=\"$(git ls-files -z \"*.go\" | xargs -0 -r gofmt -l)\" || exit $?; test -z \"$unformatted\"'", "go build -trimpath -ldflags='-s -w' -o /tmp/sworn-v0.3.0 ./cmd/sworn", "git diff --check"],
          "constraints": ["Every manifest-named autonomous-engine case must PASS; only separate credential-gated live provider smokes may be NOT RUN. Failures return according to the work-versus-assembly recovery matrix; S7 does not authorize a tag, main merge, hosted change or sworn-web, which is a separate post-gate repository."],
          "depends_on": ["S4-http-cloud-drivers", "S6-release-cockpit"]
        }
      ]
    }
  ]
}
```

# Goal

Deliver Sworn v0.3 as the lean autonomous Go engine for Baton RC2: five
responsibilities, safe parallel tracks, honest recovery, one common
multi-vendor driver layer with explicit models for the four model-facing roles,
truthful operator views, and deterministic engine-owned Merge of only the exact
fresh-verified candidate. This recovers the useful Coach loop, not its Bash
machinery or the later artifact-heavy process.

# Authority

The external decision-maker is the repository owner acting on
[swornagent/sworn#157](https://github.com/swornagent/sworn/issues/157).
`github://swornagent/sworn/issues/157#baton-plan-approval-sworn-v0.3.0`
identifies the unique issue comment containing that marker and this file's raw
SHA-256 digest.

This proposal is not yet approved. Issue scope or conversation does not
substitute for the exact digest comment. No Baton plan action, status, release
ref, or track ref may exist before trusted approval evidence is resolved.

# Scope

Included are S0 immutable pin/reset/conformance; S1 bounded intent through
Planner proposal, external digest approval, plan installation and one-track
delivery; S2 native CLIs; S3 Coach topology and approval/effect recovery; S4
HTTP/cloud drivers; S5 local evaluation and direct opt-in OTel; S6
terminal/WebUI cockpit; and S7 real-binary parity and technical release proof.

Excluded are archived-source reuse; managed inference; provider SDKs; hosted
credentials; bundled defaults; role-specific drivers; silent fallback or
rotation; production Node, LangChain, LangGraph, Temporal, DBOS or another
workflow vocabulary; commercial/hosted work; tags and `main` integration; and
`sworn-web`, which starts under a separate post-S7 gate.

# Acceptance

Metadata acceptance IDs are the gate. Each model-facing Baton proof handoff is
bounded and retains exact deterministic local check commands, tool versions,
exit codes, salient structured acceptance facts, digests, commits/trees,
invocation/effect receipts, verdicts, nullable usage/cost, and SHA-256
references only to engine-owned deterministic local check logs. Those check
logs stay digest-addressed and out-of-band and are not repeatedly loaded into
later model contexts.

A design, mock, leaf-only test, UI animation, unexecuted case, or another
adapter's result is insufficient. All 12 immutable RC2 manifest-named
autonomous-engine cases must `PASS` through the real adapter; `NOT RUN` is
reserved only for separate credential-gated live provider smokes.

S7 alone declares technical release readiness after the complete real-binary
corpus passes. It does not authorize publication.

# Ordered tracks and work

```text
T0 reset
  ├─ T1 Baton ────┐
  ├─ T2 journal ──┤
  ├─ T3 Git ──────┼─ T5 loop: S1 -> S2 -> S3 ─┬─ T6 S4 cloud ─┐
  └─ T4 driver ───┘                             └─ T7 S5 -> S6 ┤
                                                               └─ T8 S7
```

T1-T4 are disjoint and parallel after T0. T5 orders the minimum integrated
loop. T6 and T7 are disjoint and parallel after that boundary. T8 owns only
real-binary integration, parity evidence and technical release docs; defects
return to the package-owning work.

# Dependencies and touch surfaces

| Track | Depends on | Product touch surfaces |
| --- | --- | --- |
| T0 | none | `.github/workflows`, `AGENTS.md`, `cmd/sworn`, module files, `internal`, asset/golden tools |
| T1 | T0 | `internal/baton`, `tools/batongolden` |
| T2 | T0 | `internal/journal` |
| T3 | T0 | `internal/gitx` |
| T4 | T0 | `internal/driver` |
| T5 | T1-T4 | `cmd/sworn`, `internal/driver`, `internal/runtime`, `test/e2e` |
| T6 | T5 | `internal/driver` |
| T7 | T5 | `cmd/sworn`, `internal/observe`, `internal/cockpit` |
| T8 | T6-T7 | `README.md`, `cmd/sworn`, `docs/releases/v0.3.0`, `test/e2e` |

Overlaps exist only across declared dependency edges. One worker is active
within a track; independent tracks do not edit shared paths.

# Checks

Each work runs focused tests, race tests where state/process concurrency is
involved, vet and diff checks. S1 adds the dependency-free test-only
`go run ./test/e2e/cmd/asserttests` enumeration gate. Repeated `-package` and
`-expect` inputs plus one literal anchored `-regex` cause it to run
`go test -count=1 -list`, reject empty, missing, duplicate or unexpected exact
top-level names, and require every expected name exactly once. With `-manifest`
and repeated `-case=case-id=TestName`, it also requires the ordered unique case
IDs to equal the complete immutable RC2 autonomous-engine manifest and maps
each to exactly one test. Every focused check invokes `-execute-and-assert`,
which then runs uncached `go test -count=1 -json -run '^(...)$'` over the exact
packages, requires exactly one terminal `pass` event for every expected
top-level test, and rejects `skip`, no event, duplicate or unexpected test
events and any failing test process. Manifest mode binds each of the 12 case
IDs to its mapped terminal PASS. The helper is test-only and adds no production
framework or dependency.

S7 reruns all tests, race, vet, formatting over every tracked Go file including
`test/e2e`, stripped build and measured package/line/dependency/binary budgets,
plus Baton portable cases, all 12 RC2 autonomous-engine cases through the real
adapter, P01-P10 per driver, crash cuts, browser/a11y checks and the executable
Coach parity scenarios. Proof handoffs retain bounded salient facts and digest
references only to engine-owned deterministic local check logs held
out-of-band; generated fixtures must regenerate cleanly.

# Constraints

- Baton records and Git refs own lifecycle truth; SQLite owns runtime recovery.
- Every Verifier is fresh, read-only and isolated from implementation context.
- External effects are bounded, journaled before execution and reconciled
  before retry. Verifier operational failure may produce `NO_VERDICT`; Captain
  failure produces no decision; Planner or Implementer failure leaves durable
  Baton status unchanged. None synthesizes a Baton decision or verdict.
- Merge is deterministic and engine-owned with model `null`, no model or driver
  configuration or dispatch, and exact compare-and-set refs.
- Production is `cmd/sworn` plus at most seven internal packages; target
  15,000 Go lines and a 25 MiB stripped binary. Stop for review before 18,000
  lines, a ninth package, a file over 700 lines, or a runtime framework.
- `modernc.org/sqlite` is the sole S0/S1 non-stdlib dependency. S5 may add
  narrowly justified OTel protocol/export dependencies under an ADR and the
  ten-dependency budget; any other production dependency needs a newly
  authorized plan.
- Raw agent/provider stdout/stderr, prompts, completions, source, diffs,
  credentials, raw argv, and tool request/response payloads are bounded in
  transit and discarded or allowlist-sanitized as appropriate; none is retained
  in Baton handoffs, runtime records, cockpit views, logs or telemetry. Only
  sanitized operational metadata and engine-owned deterministic local check
  logs may be retained, with those logs local, digest-addressed and out-of-band.
- `.baton/releases` stays behaviorally inert and outside product scope. Only
  the admitted Baton action surface moves authority refs or canonical status.
- Existing captures remain authority/evidence and are not rewritten.
- Changed scope, ownership, acceptance, checks, constraints, topology or Baton
  semantics requires a newly approved plan.
