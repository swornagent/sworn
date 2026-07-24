```baton-plan-v2
{
  "schema_version": "baton.plan/v2",
  "release": "sworn-v0.3.0-baton-v2",
  "revision": 4,
  "previous_plan": "308c2aed213c0fef769ed5495248fa1a704de67f",
  "repository": "swornagent/sworn",
  "target_ref": "refs/heads/release/v0.3.0",
  "approval_ref": "github://swornagent/sworn/issues/157#baton-plan-approval-sworn-v0.3.0-baton-v2-v4",
  "tracks": [
    {
      "id": "T0-admission",
      "depends_on": [],
      "slices": [
        {
          "id": "W0-reset-admission",
          "outcome": "Admit the exact Baton RC5 release and the reviewed Sworn foundation into a new v2 record line without importing legacy authority.",
          "scope": {
            "include": [".gitattributes", ".github/workflows", "AGENTS.md", "README.md", "cmd/sworn", "go.mod", "go.sum", "internal", "tools/batonassets", "tools/batongolden"],
            "exclude": [".baton/releases"]
          },
          "acceptance": [
            {
              "id": "A-W0-base",
              "text": "Immediately before materialization, refs/heads/release/v0.3.0 is commit 2c9ce0493971e0e833d4dec6c562b030315e33c9, tree a10d213da750ece28a6dc066e2170c76fc959def, with parents c32d6846a98aef59a33d0a4bca89a4fde434a1d1 and 045d6e9c56c5da523f5d8a149e29fefeb7c56f17. Drift requires a plan revision."
            },
            {
              "id": "A-W0-baton",
              "text": "The embedded release is Baton v1.0.0-rc.5: annotated tag 306ed09c3152e8a7413e6b9d09d63d00ee12ff4a, peeled commit b0133b9e53755484f7aa9140fc3c1b349e2f50dd, tree c079d41d3955d9690a9be39d1711ef45fa3625f3, archive SHA-256 8fea81036dc678e9a0aa4c2d1fb0c8ed016c23b9e7d77c183f3f168467002dd5, and generated package digest sha256:cd3f1285318820ca5ee3a96785ab40915f7b2970ec14d9e3f578898de4a953c1."
            },
            {
              "id": "A-W0-lineage",
              "text": "All RC2 and RC3 plan-v1 refs, plans, approvals, receipts, statuses, proofs, and verdicts remain immutable archaeology. Reviewed RC3 product bytes may be replayed as implementation input, but no prior authority or PASS is reused."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test ./tools/batonassets/... ./tools/batongolden/... ./cmd/sworn/...", "git diff --check"],
          "constraints": ["Baton records are never product, model, build, package, or candidate input.", "Official builds use -buildvcs=false and -trimpath.", "The measured post-foundation production-line, dependency, and binary baselines must be recorded before W3 begins."],
          "depends_on": [],
          "consumes": []
        }
      ]
    },
    {
      "id": "T1-authority",
      "depends_on": ["T0-admission"],
      "slices": [
        {
          "id": "W1-authority-core",
          "outcome": "Implement Baton RC5 authority, product identity, operation derivation, and exact Git composition in pure Go.",
          "scope": {
            "include": ["internal/baton", "internal/gitx", "tools/batongolden"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W1-contract",
              "text": "Strict baton.plan/v2 and receipt-v1 parsing, bindings, operation derivation, selective invalidation, exact composition, and compare-and-set integration match the pinned RC5 goldens. The board is derived and never action authority."
            },
            {
              "id": "A-W1-git",
              "text": "Callers and model processes cannot choose raw Git commands, authority refs, parents, merge mode, or arbitrary paths. Stale, conflicting, symbolic, consumed, or ambiguous state fails without moving a ref."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/baton/... ./internal/gitx/... ./tools/batongolden/..."],
          "constraints": ["Legacy RC3 W1 code may be replayed only into a fresh candidate with fresh Captain review, evidence, and verification.", "Scheduler policy, provider transport, and operator UI remain outside this slice."],
          "depends_on": ["W0-reset-admission"],
          "consumes": ["W0-reset-admission"]
        }
      ]
    },
    {
      "id": "T2-driver",
      "depends_on": ["T0-admission"],
      "slices": [
        {
          "id": "W2-driver-core",
          "outcome": "Provide one role-neutral bounded invocation and sealed-submission contract with a deterministic fake.",
          "scope": {
            "include": ["internal/driver"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W2-contract",
              "text": "Planner, Implementer, Captain, and Verifier use one contract with explicit driver and model selection. Merge is deterministic engine work and is never dispatched to a model."
            },
            {
              "id": "A-W2-isolation",
              "text": "Each invocation receives only its bounded workspace and ordered digest-bound inputs. Fresh Verifier access is clean and read-only; one schema-checked submission is the only role-output seam; cancellation cleans the process tree."
            },
            {
              "id": "A-W2-truth",
              "text": "Transport failure creates no Baton decision. Reported usage, legitimate zero, and unavailable usage remain distinct, and Sworn does not estimate tokens or cost."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/driver/..."],
          "constraints": ["Legacy RC3 W2 code may be replayed only into a fresh candidate with fresh Captain review, evidence, and verification.", "Production provider codecs remain W5 work.", "No role-specific driver, model default, provider fallback, or retained raw model transcript."],
          "depends_on": ["W0-reset-admission"],
          "consumes": ["W0-reset-admission"]
        }
      ]
    },
    {
      "id": "T3-runtime",
      "depends_on": ["T1-authority", "T2-driver"],
      "slices": [
        {
          "id": "W3-walking-skeleton",
          "outcome": "Drive one approved track through every Baton responsibility and exact integration.",
          "scope": {
            "include": ["cmd/sworn", "go.mod", "go.sum", "internal/gitx", "internal/journal", "internal/runtime", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W3-flow",
              "text": "The real binary proposes a plan, pauses for protected external approval, installs the exact bytes, runs Implementer design, distinct Captain review, resumed implementation, fresh read-only work verification, deterministic composition, fresh assembly verification, and exact Merge."
            },
            {
              "id": "A-W3-journal",
              "text": "One SQLite journal durably records replay-stable commands, finite claims, attempts, before-effect identity, effect receipts, usage, and append-only events. Replay is idempotent and an uncertain external effect is reconciled before retry."
            },
            {
              "id": "A-W3-boundary",
              "text": "Agents never receive a canonical authority worktree or approval credentials. Runtime state makes effects recoverable but never becomes a second Baton lifecycle."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/gitx/... ./internal/journal/... ./internal/runtime/... ./cmd/sworn/... ./test/e2e/..."],
          "constraints": ["No delivery actor can mint plan approval.", "Only typed internal APIs may create worktrees, advance refs, or persist Baton receipts.", "Production provider adapters and the browser cockpit remain outside this slice."],
          "depends_on": ["W1-authority-core", "W2-driver-core"],
          "consumes": ["W1-authority-core", "W2-driver-core"]
        },
        {
          "id": "W4-topology-recovery",
          "outcome": "Add Coach-loop worktree topology, bounded parallel tracks, and honest recovery.",
          "scope": {
            "include": ["cmd/sworn", "internal/baton", "internal/gitx", "internal/journal", "internal/runtime", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W4-topology",
              "text": "A release worktree owns assembly; dependency-ready tracks may run concurrently with one serial writer per track; composition is serial and plan-ordered. When composition creates a tree not already covered by an exact fresh work PASS, a distinct fresh assembly Verifier gates Merge. A one-track direct candidate may reuse only its exact fresh work PASS; any tree change requires new verification."
            },
            {
              "id": "A-W4-recovery",
              "text": "Pause, resume, cancel, retry, and takeover are typed idempotent commands with persisted hard caps. Timeouts, crashes, lease expiry, logs, and bookkeeping gaps are operational facts, never Baton verdicts or reasons to replan. Every mutating runtime action and candidate seal binds the exact installed plan, release, target, and Baton before-state vector. Authority drift atomically prevents stale mutation; recovery reconciles every claimed effect before new work, safely rolls back an exact stale candidate when required, and terminalizes stale effects without reuse; target staleness preempts model work."
            },
            {
              "id": "A-W4-replan",
              "text": "Replanning appends an approved plan revision, preserves the release and unchanged slice identities, and invalidates only changed slices and actual consumers. It never resets or locks an otherwise valid slice set."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/baton/... ./internal/gitx/... ./internal/journal/... ./internal/runtime/... ./cmd/sworn/... ./test/e2e/..."],
          "constraints": ["Each dispatch or external effect gets one initial attempt plus at most two automatic retries in a persisted epoch; exhaustion parks the slice for explicit typed operator action.", "A parked track does not stop independent ready tracks, but its consumers and assembly remain gated.", "Provider codecs, telemetry export, and browser styling remain outside this slice."],
          "depends_on": ["W3-walking-skeleton"],
          "consumes": ["W3-walking-skeleton"]
        }
      ]
    },
    {
      "id": "T4-adapters",
      "depends_on": ["T2-driver"],
      "slices": [
        {
          "id": "W5-production-adapters",
          "outcome": "Implement the required production driver profiles behind the common role-neutral contract.",
          "scope": {
            "include": ["internal/driver"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W5-profiles",
              "text": "Codex CLI, Claude Code CLI, OpenAI-compatible HTTP, DeepSeek, Gemini, and Bedrock are profiles behind one contract, alongside the deterministic fake. Every role chooses an explicit profile and model; no silent default or fallback exists."
            },
            {
              "id": "A-W5-native",
              "text": "Codex uses unattended `exec --ephemeral --yolo`; clean Verifier invocation also ignores user config and rules and disables memories and external memory import. Claude Code uses a bounded non-interactive native mode with no ambient MCP, browser, session, or unapproved tool surface."
            },
            {
              "id": "A-W5-http",
              "text": "The HTTP and cloud profiles share one bounded allowlisted workspace-tool loop and one terminating sworn_submit seam. OpenAI-compatible and DeepSeek, Gemini generateContent, and Bedrock Converse/SigV4 codecs preserve tool calls, cancellation, configured credentials, reported usage, and provider errors without owning orchestration. Bedrock alone resolves the standard AWS region and credential chain and records only its non-secret source kind."
            },
            {
              "id": "A-W5-certification",
              "text": "Every profile passes the shared fake-server corpus and exposes secret-free inspect, doctor, and certify results through the common driver API. Credential-gated live smokes report PASS, FAIL, or NOT CERTIFIED per configured profile without substitution."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/driver/... ./cmd/sworn/..."],
          "constraints": ["No managed inference, credential custody, provider marketplace, provider SDK without measured necessity, or provider-specific workflow.", "Provider/model identifiers may be recorded locally but never become unbounded metric labels."],
          "depends_on": ["W2-driver-core"],
          "consumes": ["W2-driver-core"]
        }
      ]
    },
    {
      "id": "T5-operator",
      "depends_on": ["T3-runtime"],
      "slices": [
        {
          "id": "W6-operator-evidence",
          "outcome": "Deliver the truthful local cockpit, evaluation facts, and non-controlling observability.",
          "scope": {
            "include": ["cmd/sworn", "go.mod", "go.sum", "internal/cockpit", "internal/journal", "internal/observe"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W6-cockpit",
              "text": "Terminal and responsive browser views render the same Baton graph plus a durable runtime overlay. They reconstruct from snapshot and event offset, never invent progress, and submit only closed typed local commands."
            },
            {
              "id": "A-W6-open-operations",
              "text": "The MIT product includes the complete local operations API, responsive WebUI, generic webhook delivery, durable notification outbox, and secure self-hosting capabilities. Repository and Baton authority remain local."
            },
            {
              "id": "A-W6-eval",
              "text": "A versioned local eval stream records exact outcomes, timings, retries, recovery, reported tokens and cost, and quality denominators independently of OTLP configuration or export. Unknown remains null, content is excluded, and exported scores never control delivery."
            },
            {
              "id": "A-W6-otel",
              "text": "Direct Go OpenTelemetry emits only a versioned positive allowlist of opt-in, bounded, asynchronous traces and fixed-enum low-cardinality metrics. Local status exposes exporter health, failures, and dropped telemetry. Prompts, completions, source, diffs, paths, credentials, raw argv, arbitrary errors, and tool payloads cannot leave the process; exporter failure cannot affect delivery."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/cockpit/... ./internal/journal/... ./internal/observe/... ./cmd/sworn/..."],
          "constraints": ["The board is a projection and typed command client, never a second scheduler or Baton writer.", "Hosted control plane, managed identity, managed notifications, and privileged remote control remain outside this slice.", "No LangChain, LangGraph, DBOS, Temporal, embedded collector, OTel logs, or vendor telemetry SDK in v0.3."],
          "depends_on": ["W4-topology-recovery"],
          "consumes": ["W4-topology-recovery"]
        }
      ]
    },
    {
      "id": "T6-release",
      "depends_on": ["T3-runtime", "T4-adapters", "T5-operator"],
      "slices": [
        {
          "id": "W8-parity-release",
          "outcome": "Prove Coach-loop parity and Sworn v0.3 technical readiness through the real binary.",
          "scope": {
            "include": ["README.md", "cmd/sworn", "docs/releases/v0.3.0", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W8-conformance",
              "text": "The real binary passes every autonomous-engine case in the pinned RC5 manifest. Missing, duplicate, skipped, or NOT RUN cases fail the gate, and runtime success never substitutes for a Baton PASS."
            },
            {
              "id": "A-W8-parity",
              "text": "The Coach parity baseline at sawy3r/baton v1.0.0-rc.5, path docs/captures/2026-07-24-coach-loop-parity-baseline.md, blob ed1ec7963aa37c204f080567c208f0879f0fd6cb, SHA-256 8ad596e72fefb1b4cb43fdcce8cf4a705f65ead7618bb575dca1675cb9c7c39c, has no MISSING row. The real binary proves parallel tracks, fresh work and assembly verification, exact integration, timeout/no-verdict, crash recovery, stale target, repair after FAIL, BLOCKED routing, composition conflict, truthful restart views, multi-driver per-role models, and telemetry non-interference."
            },
            {
              "id": "A-W8-journey",
              "text": "One disposable fixture completes unattended delivery across three tracks, includes a dependency and two serial slices, uses two driver families and distinct configured role models, and finishes with a target tree exactly equal to the passed assembly."
            },
            {
              "id": "A-W8-drivers",
              "text": "The real CLI exposes inspect, doctor, and certify for Codex CLI, Claude Code CLI, OpenAI-compatible HTTP, DeepSeek, Gemini, and Bedrock. Each family passes the shared corpus and a credential-backed live smoke with an explicit configured model. NOT CERTIFIED, skipped, substituted, or fallback results fail technical readiness unless the repository owner ratifies an exact named deferral in a revised plan."
            },
            {
              "id": "A-W8-release",
              "text": "Fresh product-only copies pass full, race, vet, format, and reproducible build checks. Release evidence binds exact product, binary, package, dependency, size, scenario, usage, retry, timing, and quality facts."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test ./...", "GOFLAGS=-buildvcs=false go test -race ./...", "GOFLAGS=-buildvcs=false go vet ./...", "bash -o pipefail -c 'unformatted=\"$(git ls-files -z -- \"*.go\" \":(exclude,top).baton/releases/**\" | xargs -0 -r gofmt -l)\"; test -z \"$unformatted\"'", "CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -mod=readonly -buildvcs=false -trimpath -ldflags='-s -w' -o /tmp/sworn-v0.3.0 ./cmd/sworn", "git diff --check"],
          "constraints": ["The measured legacy baseline at bad1a6767994cacef2c354061d22db842cb6ca08 is 10464 physical lines in tracked .go files under cmd and internal, excluding _test.go, testdata or fixtures paths, and files carrying the standard Code generated ... DO NOT EDIT marker; blank and comment lines count. Exact approval of this plan supersedes all provisional numeric budgets in issue #157.", "Report the exact total and delta under that classifier plus production package count, direct dependencies, and stripped binary size. Material growth triggers proof-backed Captain architecture review of capability ownership, duplicate policy or orchestration, package boundaries, dependency necessity, and binary composition. Measurements are telemetry; no raw count is by itself a release verdict or plan-revision trigger.", "Each lifecycle, retry, broker, tool, credential, and provider mechanic has one authoritative owner. Common mechanics live behind role-neutral interfaces; adapters translate only native or provider surfaces. Duplicate policy or orchestration fails review; coincidental syntax alone does not require abstraction.", "This gate proves technical readiness only and grants no tagging, main merge, hosted deployment, or sworn.sh publication authority."],
          "depends_on": ["W4-topology-recovery", "W5-production-adapters", "W6-operator-evidence"],
          "consumes": ["W4-topology-recovery", "W5-production-adapters", "W6-operator-evidence"]
        }
      ]
    }
  ]
}
```

# Goal

Deliver Sworn v0.3 as the lean Go engine that runs Baton autonomously: protected
approval, the four model-facing roles, deterministic Merge, parallel track
worktrees, common multi-vendor drivers, crash recovery, a truthful graph
cockpit, local evaluation, opt-in telemetry, and exact integration.

# Authority

This is a proposal. Only the repository owner may approve these exact bytes
through the protected marker
`baton-plan-approval-sworn-v0.3.0-baton-v2-v4` on
`swornagent/sworn#157`. The legacy RC3 approval does not approve these bytes.

# Migration boundary

The prior RC3 line predates Baton 1.0 and uses `baton.plan/v1`. RC5 strictly
reads `baton.plan/v2`, so that legacy line cannot be a current plan revision or
revision predecessor. It remains immutable migration input outside the Baton
1.0 plan lineage. This one-time format bridge starts the first RC5-conforming
internal record identity, `sworn-v0.3.0-baton-v2`, at revision 1. It retains all
seven track and eight slice identities, leaves every legacy ref untouched, and
imports no legacy authority.

This is not ordinary replanning. After this bridge, future changes stay on this
release identity, append revisions, preserve unchanged slices, and invalidate
only changed contracts and their actual consumers. A later release identity is
permitted only if the goal, target, or authority is replaced.

- W0 binds the published RC5 release and exact untouched target.
- W1 and W2 rebuild current authority from reviewed legacy product bytes under
  fresh design, Captain, evidence, and Verifier decisions.
- W3, W4, W5, W6, and W8 retain their stable outcome identities.
- No outcome is added or retired, and no prior PASS is reused.

Revision 4 changes only W4: it admits `internal/gitx` and binds recovery to
exact authority. W0 through W3 retain their current PASS because their contracts
and consumed inputs are unchanged. W5, W6, and W8 retain their identities and
remain downstream gates. No slice is added, retired, renamed, or reset.

# Scope

Included are the complete local engine, six production driver profiles, safe
parallelism and recovery, the terminal/browser cockpit, public local operations
surfaces, evaluation, opt-in OTel, and executable Coach parity.

Excluded are managed inference, hosted credentials, the hosted control plane,
publication, and speculative website claims.

# Acceptance

The acceptance IDs above are observable product gates. Compact receipts bind
the exact plan, design, candidate, checks, evidence, Verifier decision, and Git
objects. Raw logs and long evidence stay in the engine evidence store.

# Ordered tracks and slices

W0 admits the exact base and Baton package. W1 and W2 then rebuild authority and
the common driver boundary in parallel. W3 and W4 deliver the runtime and
topology. W5 production adapters can advance from W2 while W6 cockpit and
observability advance from W4. W8 is the only technical-readiness gate.

# Dependencies and inputs

`depends_on` controls eligibility. `consumes` names the exact passed product
inputs whose change invalidates a consumer. Tracks may advance concurrently only
when these edges and their scopes permit it; one writer remains serial within a
track.

# Checks

Each slice runs its listed focused checks. W8 repeats the full, race, vet,
format, reproducibility, and real-binary parity suites. A worker exit or command
success is never evidence for a Baton verdict.

# Constraints

Sworn derives Baton operations; it does not duplicate Baton's lifecycle.
Drivers are common and role-neutral. Telemetry is optional and non-controlling.
The visual board is a graph projection and typed command client, not an
editable workflow authority. Git history carries archaeology; normal delivery
keeps only the approved plan and compact receipts.
