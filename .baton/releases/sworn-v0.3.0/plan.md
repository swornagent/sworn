```baton-plan-v1
{
  "schema_version": "baton.plan/v1",
  "release": "sworn-v0.3.0",
  "repository": "swornagent/sworn",
  "target_ref": "refs/heads/release/v0.3.0",
  "release_ref": "refs/heads/release-wt/sworn-v0.3.0",
  "record_root": ".baton/releases",
  "approval_ref": "github://swornagent/sworn/issues/157#baton-plan-approval-sworn-v0.3.0-post-maintenance-bridge-v2",
  "tracks": [
    {
      "id": "T0-admission",
      "ref": "refs/heads/track/sworn-v0.3.0/T0-admission",
      "depends_on": [],
      "touch_surfaces": [".gitattributes", ".github/workflows", "AGENTS.md", "README.md", "cmd/sworn", "go.mod", "go.sum", "internal", "tools/batonassets", "tools/batongolden"],
      "work": [
        {
          "id": "W0-reset-admission",
          "outcome": "Start the replacement engine from the exact maintenance bridge and admit immutable Baton RC2 assets.",
          "scope": {
            "include": [".gitattributes", ".github/workflows", "AGENTS.md", "README.md", "cmd/sworn", "go.mod", "go.sum", "internal", "tools/batonassets", "tools/batongolden"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W0-base",
              "text": "Delivery starts only when refs/heads/release/v0.3.0 is exact commit 2c9ce0493971e0e833d4dec6c562b030315e33c9 with ordinary tree a10d213da750ece28a6dc066e2170c76fc959def and descends from c32d6846a98aef59a33d0a4bca89a4fde434a1d1. The rehearsal commit, prior plan digest and prior approval marker are never admitted; any other head requires newly rendered and approved plan bytes."
            },
            {
              "id": "A-W0-assets-product",
              "text": "The reset removes the remaining v0.2 production graph and introduces only cmd/sworn plus internal/baton, runtime, journal, gitx and driver seams. The admission lock validates Baton tag object b80f3e27f0e0a71a4883bcc282e4843e085f0e04, commit 890238ef063bb53cf51fb3359f1ff527f14846c6, tree 97513f3e6f798f3ad04d5b510a49496a605a8ea4, release archive sha256:968088ede0c3bfbafb0a9372d3abbf6853556cc2a6e85ffc25615d6332977e63 and support package sha256:676c630c6a4ef3f752d604efaa5e51958adec0d8580b74cec7fb1e689b1d3436. VCS-free twin builds from separate product copies and fresh caches are byte-identical."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test ./tools/batonassets/... ./tools/batongolden/... ./cmd/sworn/..."],
          "constraints": [".baton/releases is control authority only and never a product, model, check, workspace, candidate, build or package input; official builds use -buildvcs=false and -trimpath, and product identity remains distinct from Git provenance."],
          "depends_on": []
        }
      ]
    },
    {
      "id": "T1-authority",
      "ref": "refs/heads/track/sworn-v0.3.0/T1-authority",
      "depends_on": ["T0-admission"],
      "touch_surfaces": ["internal/baton", "internal/gitx", "tools/batongolden"],
      "work": [
        {
          "id": "W1-authority-core",
          "outcome": "Implement Baton RC2 records, actions, product identity and exact Git composition in pure Go.",
          "scope": {
            "include": ["internal/baton", "internal/gitx", "tools/batongolden"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W1-authority",
              "text": "Strict records, bindings, lifecycle, all seven Baton actions, retry reconciliation, track composition, assembly preparation and compare-and-set integration match RC2 goldens. Product identity hashes the ordered product paths, modes, types and objects outside .baton/releases; a record-only change may preserve it, but exact candidate OID, tree, ancestry, expected target and record history remain mandatory. Callers cannot choose authority refs, paths, Git commands, parents or merge mode; stale, conflicting, consumed or unknown state fails without moving a ref."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/baton/... ./internal/gitx/... ./tools/batongolden/..."],
          "constraints": ["The JavaScript reference is a development oracle only; Git runs through one sanitized literal boundary, and models never resolve conflicts or advance authority."],
          "depends_on": ["W0-reset-admission"]
        }
      ]
    },
    {
      "id": "T2-driver",
      "ref": "refs/heads/track/sworn-v0.3.0/T2-driver",
      "depends_on": ["T0-admission"],
      "touch_surfaces": ["internal/driver"],
      "work": [
        {
          "id": "W2-driver-core",
          "outcome": "Provide one role-neutral invocation and sealed-submission contract with a deterministic fake.",
          "scope": {
            "include": ["internal/driver"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W2-contract",
              "text": "The role-neutral driver implements all five baton.driver/v1 role values. Sworn dispatches only Planner, Implementer, Captain and Verifier with an explicit driver and model for each invocation; portable Merge coverage preserves deliberate model:null, while production Merge is deterministic, engine-owned and undispatched. Fresh bounded processes receive only their workspace plus ordered digest-bound inputs through a read-only .sworn-inputs/v1 overlay; no Baton record root, journal, resolver, canonical worktree or engine log is mounted. One sealed submission binds the invocation and permits only the role's exact artifacts and decision."
            },
            {
              "id": "A-W2-isolation-usage",
              "text": "Cancellation, clean read-only verification, output limits and the shared adapter conformance suite pass with the fake. Transport failure creates no Baton decision or verdict. A normalized usage receipt distinguishes reported from unavailable: reported token fields are nullable non-negative values, reported cost is integer micro-units with currency and source, legitimate zero remains zero, and Sworn never estimates either."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/driver/..."],
          "constraints": ["No role-specific driver, model default, fallback, provider rotation, driver-owned lifecycle or retained raw model transcript is permitted."],
          "depends_on": ["W0-reset-admission"]
        }
      ]
    },
    {
      "id": "T3-runtime",
      "ref": "refs/heads/track/sworn-v0.3.0/T3-runtime",
      "depends_on": ["T1-authority", "T2-driver"],
      "touch_surfaces": ["cmd/sworn", "internal/journal", "internal/runtime", "test/e2e"],
      "work": [
        {
          "id": "W3-walking-skeleton",
          "outcome": "Drive one approved track through all five Baton responsibilities and exact integration.",
          "scope": {
            "include": ["cmd/sworn", "internal/journal", "internal/runtime", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W3-flow",
              "text": "Bounded intent produces one strict Planner proposal and pauses on its raw digest without creating authority. Only matching protected external approval installs it. Implementer design, distinct Captain decision, resumed implementation and fresh adversarial work Verifier invocations complete through the fake driver. After work PASS, engine-owned Merge composes the track and prepares assembly; a fresh assembly Verifier runs through the fake, and assembly PASS permits engine-owned exact target integration."
            },
            {
              "id": "A-W3-journal",
              "text": "SQLite durably and transactionally records replay-stable commands, finite claims, invocation attempts, before-effect identities, immutable receipts, normalized usage, and a versioned append-only local event/outbox stream with increasing run offsets. These facts own recovery, local evaluation and cockpit replay but never synthesize Baton lifecycle. Identical replay returns the same identity; conflicting bytes and uncertain effects stop before retry."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/journal/... ./internal/runtime/... ./cmd/sworn/... ./test/e2e/..."],
          "constraints": ["No delivery actor approves its own plan; durable local events are not an OTLP queue and cannot be deleted or acknowledged by an exporter."],
          "depends_on": ["W1-authority-core", "W2-driver-core"]
        },
        {
          "id": "W4-topology-recovery",
          "outcome": "Add Coach-loop track topology, bounded parallelism and honest crash recovery.",
          "scope": {
            "include": ["cmd/sworn", "internal/journal", "internal/runtime", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W4-topology",
              "text": "release-wt owns assembly while product-only isolated track worktrees run dependency-ready tracks concurrently with one serial writer per track. The oracle reads all authoritative refs and selects the furthest valid status without timestamp guessing; composition is serial, exact and engine-owned, and a distinct fresh assembly Verifier alone gates release Merge."
            },
            {
              "id": "A-W4-recovery",
              "text": "Pause, resume, cancel, retry and takeover are typed replay-stable commands. Uncertain effects reconcile before retry. Verifier transport, runner, tool or environment failure is operational NO_VERDICT over the unchanged candidate and status; Planner, Implementer or Captain operational failure emits no Baton outcome and leaves durable state unchanged. Captain REVISE and work FAIL return to Implementer. Captain ESCALATE, work BLOCKED and assembly FAIL/BLOCKED route to Planner; after materialisation they preserve the old lineage and require newly approved plan, work and release identities, while REBOUND remains pristine-unmaterialised only. Moved targets and conflicts stop without changing the prior release ref."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/runtime/... ./cmd/sworn/... ./test/e2e/..."],
          "constraints": ["Concurrency is bounded; cancellation cleans process trees and never rewrites Baton records."],
          "depends_on": ["W3-walking-skeleton"]
        }
      ]
    },
    {
      "id": "T4-adapters",
      "ref": "refs/heads/track/sworn-v0.3.0/T4-adapters",
      "depends_on": ["T2-driver"],
      "touch_surfaces": ["internal/driver"],
      "work": [
        {
          "id": "W5-production-adapters",
          "outcome": "Implement every required provider behind the common driver contract.",
          "scope": {
            "include": ["internal/driver"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W5-adapters",
              "text": "Codex CLI, Claude Code CLI, OpenAI-compatible, DeepSeek, Gemini and Bedrock independently pass the same ABI conformance plus version-pinned native or wire fixtures for request construction, response, stream and tool-loop parsing, usage, limits, cancellation and error mapping. HTTP adapters share one bounded workspace-tool loop; DeepSeek is an OpenAI-compatible profile. Technical readiness requires credential-backed live PASS from at least one native CLI and one HTTP adapter; unavailable providers may report NOT RUN only in separate live-smoke evidence."
            },
            {
              "id": "A-W5-native-cli",
              "text": "Version-pinned fixtures prove Codex --yolo exec --ephemeral operation with user configuration and rules ignored; Verifier fixtures additionally disable memories and external-agent memory import. Claude Code uses equivalent fresh non-interactive isolation. Native CLIs retain their own agentic tool loops, and volatile argument ordering stays inside adapter tests rather than plan authority."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/driver/..."],
          "constraints": ["Use lean direct drivers without provider SDKs, Sworn-hosted inference, proxying, gateways or hosted credential custody."],
          "depends_on": ["W2-driver-core"]
        }
      ]
    },
    {
      "id": "T5-operator",
      "ref": "refs/heads/track/sworn-v0.3.0/T5-operator",
      "depends_on": ["T3-runtime"],
      "touch_surfaces": ["cmd/sworn", "internal/cockpit", "internal/observe"],
      "work": [
        {
          "id": "W6-operator-evidence",
          "outcome": "Deliver one truthful operator plane: thin terminal/WebUI cockpit, authoritative local evaluation and privacy-safe opt-in OTel.",
          "scope": {
            "include": ["cmd/sworn", "internal/cockpit", "internal/observe"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W6-cockpit",
              "text": "sworn board --json and sworn serve share the ref-aware Baton oracle plus sanitized durable events across releases, release-wt, tracks and worktrees. Both expose active responsibility and worker, current and next gate, driver/model, duration, retries, usage/cost availability, recovery state, typed-command receipts, deterministic-check references and evidence drill-down. Frozen scenarios for moved refs, stale events, missing/corrupt records, reconnect gaps, restart and concurrent updates produce identical authoritative state or explicit unknown/error in terminal JSON and a real browser, never inferred progress. The embedded no-runtime-dependency UI passes desktop/mobile, keyboard and accessibility checks and never queries OTLP."
            },
            {
              "id": "A-W6-local-eval",
              "text": "A documented CLI command emits versioned canonical JSON from one pinned journal snapshot. Scenario rows bind outcome, exact integration, role, driver/model, gates, categorized findings, rework, retry, recovery, protocol artefact count, orchestration-only and model time, artifact digests and usage/cost availability. Every aggregate exposes numerator, denominator, unlabeled and unavailable counts; quality and false-green/false-red use only immutable expected-outcome labels, and unavailable baseline values remain unknown rather than zero. Replaying the same snapshot yields the same semantic report, and evaluation never controls a run."
            },
            {
              "id": "A-W6-otel-fixture",
              "text": "Direct Go OTel emits versioned sworn.* restart-honest run segments with track, work, role, effect, gate, recovery and Merge spans/events plus fixed-enum low-cardinality metrics to an explicitly configured loopback Collector; provider, model and other IDs occur only on traces/events, never metric labels, and inline exporter credentials are forbidden. Export is disabled by default, bounded, asynchronous and non-controlling; local status exposes enabled state, queue depth, drops, failures and last success. A no-drop replay of the admitted 151014-token bridge fixture agrees with local authority on categorized Captain findings and causally linked repair, while unavailable, slow, rejecting or restarted Collectors produce explained gaps without changing delivery. Historical symbols, paths and finding prose are fixture inputs only; retained/exported output contains allowlisted categories, counts, usage and causal digests, and hostile canaries prove forbidden content absent."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/cockpit/... ./internal/observe/... ./cmd/sworn/..."],
          "constraints": ["The cockpit is a projection and typed command client, never a scheduler or Baton writer. A fixed allowlist permits opaque IDs, enums, counts, durations, usage/cost metadata and digests only: no prompts, completions, raw output, source, diffs, repository names, paths, credentials, argv, arbitrary errors, finding prose or tool payloads are retained or exported. No OTel logs, embedded Collector, vendor telemetry SDK, remote scoring, pricing catalogue, Langfuse or DBOS is added."],
          "depends_on": ["W4-topology-recovery"]
        }
      ]
    },
    {
      "id": "T6-release",
      "ref": "refs/heads/track/sworn-v0.3.0/T6-release",
      "depends_on": ["T3-runtime", "T4-adapters", "T5-operator"],
      "touch_surfaces": ["README.md", "cmd/sworn", "docs/releases/v0.3.0", "test/e2e"],
      "work": [
        {
          "id": "W8-parity-release",
          "outcome": "Prove Coach-loop parity and technical Sworn v0.3 release readiness through the real binary.",
          "scope": {
            "include": ["README.md", "cmd/sworn", "docs/releases/v0.3.0", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W8-conformance",
              "text": "The built binary passes all 12 immutable RC2 autonomous-engine cases through the real adapter; the manifest runner rejects missing, duplicate, skipped, NOT RUN or non-PASS cases. Every production adapter passes the shared conformance suite, with NOT RUN allowed only for its separate credential-gated live smoke."
            },
            {
              "id": "A-W8-parity",
              "text": "A disposable repository completes unattended multi-track delivery from bounded intent with a dependency edge, two driver families, distinct configured models, fresh work and assembly verification, exact integration and a final target tree equal to the passed assembly. The Coach parity and failure corpus covers release/track worktrees, all five responsibilities, recovery, terminal/WebUI truth and telemetry non-interference while reporting quality, elapsed and orchestration-only time, invocations, tokens, protocol artefacts, retries and rework with explicit denominators and preserved unknown baselines."
            },
            {
              "id": "A-W8-product-release",
              "text": "Two independently materialized product-only copies with fresh caches pass full VCS-free checks and produce byte-identical binaries and normalized packages before and after a record-only status change. Technical readiness binds the passed assembly product digest and normalized payload manifest while retaining exact Git candidate, tree, ancestry and expected-target evidence. The final complexity budget is at most eight internal production packages, 15000 non-generated production Go lines, ten direct dependencies and a 25 MiB stripped Linux binary."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test ./...", "GOFLAGS=-buildvcs=false go test -race ./...", "GOFLAGS=-buildvcs=false go vet ./...", "bash -o pipefail -c 'unformatted=\"$(git ls-files -z -- \"*.go\" \":(exclude,top).baton/releases/**\" | xargs -0 -r gofmt -l)\"; test -z \"$unformatted\"'", "CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -mod=readonly -buildvcs=false -trimpath -ldflags='-s -w' -o /tmp/sworn-v0.3.0 ./cmd/sworn", "git diff --check"],
          "constraints": ["This gate proves technical readiness only; it does not authorize a tag, main merge, hosted deployment or sworn-web change."],
          "depends_on": ["W4-topology-recovery", "W5-production-adapters", "W6-operator-evidence"]
        }
      ]
    }
  ]
}
```

# Goal

Deliver Sworn v0.3 as the lean Go engine that runs Baton RC2 autonomously with
five clear responsibilities, safe parallel tracks, common multi-vendor drivers,
truthful recovery, observable quality and exact integration.

# Authority

The repository owner approves these exact bytes through the unique
`post-maintenance-bridge-v2` marker on
[swornagent/sworn#157](https://github.com/swornagent/sworn/issues/157).
The two bridge placeholders must first be replaced by the exact landed target.
That target OID and tree are reasserted immediately before plan installation.
Conversation, the rehearsal, the superseded proposal and every earlier digest
or approval marker grant no authority.

# Scope

Included are the RC2 authority core, local recovery journal, the role-neutral
driver ABI with four model-facing roles and portable Merge coverage, required
adapters, Coach topology, terminal/WebUI cockpit, local evals, opt-in OTLP and
real-binary parity. Excluded are Sworn-hosted inference or credentials,
workflow frameworks, legacy-kernel reuse, publication and website work.

# Acceptance

Metadata acceptance IDs are the gates. Evidence binds observable candidates,
refs, receipts, verdicts and deterministic local checks; raw check logs remain
local and digest-addressed.

# Ordered tracks and work

```text
T0 admission
  ├─ T1 authority ─┐
  └─ T2 driver ────┼─ T3 runtime: W3 -> W4 ── T5 operator: W6 ─┐
                   └─ T4 adapters ───────────────────────────────┼─ T6 release
```

Independent tracks have disjoint ownership. Work is serial within a track.

# Dependencies and touch surfaces

Metadata defines ordering and ownership. Unexpected cross-track overlap or
conflict stops for repair or a newly approved plan.

# Checks

Each work runs its focused boundary suite. W8 alone repeats whole-repository
test, race, vet, product-only format/build/package and diff checks. Raw outputs
stay local and are referenced by digest.

# Constraints

Baton records and Git own delivery truth; the journal owns runtime, evaluation
and cockpit facts. Verifiers are fresh and read-only. Effects are recorded
before execution and reconciled before retry. Reapproval is required for a
changed outcome, authority, product scope, safety boundary, topology, ownership
or externally observable acceptance—not internal schema shape, test names or
argument ordering.
