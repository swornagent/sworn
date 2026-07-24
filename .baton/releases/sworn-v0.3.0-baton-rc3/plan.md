```baton-plan-v1
{
  "schema_version": "baton.plan/v1",
  "release": "sworn-v0.3.0-baton-rc3",
  "repository": "swornagent/sworn",
  "target_ref": "refs/heads/release/v0.3.0",
  "release_ref": "refs/heads/release-wt/sworn-v0.3.0-baton-rc3",
  "record_root": ".baton/releases",
  "approval_ref": "github://swornagent/sworn/issues/157#baton-plan-approval-sworn-v0.3.0-baton-rc3-v1",
  "tracks": [
    {
      "id": "T0-admission",
      "ref": "refs/heads/track/sworn-v0.3.0-baton-rc3/T0-admission",
      "depends_on": [],
      "touch_surfaces": [".gitattributes", ".github/workflows", "AGENTS.md", "README.md", "cmd/sworn", "go.mod", "go.sum", "internal", "tools/batonassets", "tools/batongolden"],
      "work": [
        {
          "id": "W0-reset-admission",
          "outcome": "Admit the exact maintenance bridge and immutable Baton v1.0.0-rc.3 assets into a new control lineage.",
          "scope": {
            "include": [".gitattributes", ".github/workflows", "AGENTS.md", "README.md", "cmd/sworn", "go.mod", "go.sum", "internal", "tools/batonassets", "tools/batongolden"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W0-base",
              "text": "Immediately before installation, refs/heads/release/v0.3.0 is exact commit 2c9ce0493971e0e833d4dec6c562b030315e33c9 with tree a10d213da750ece28a6dc066e2170c76fc959def and parents c32d6846a98aef59a33d0a4bca89a4fde434a1d1 and 045d6e9c56c5da523f5d8a149e29fefeb7c56f17. Any drift requires newly rendered and approved bytes."
            },
            {
              "id": "A-W0-lineage",
              "text": "The materialized sworn-v0.3.0 RC2 namespace, digest sha256:cf6e9103219c76a12834fbaf1eb9da8576b765dfe0602ebe416e756ed8ca10f8 and its statuses remain immutable archaeology. No record, approval, verdict or merge receipt is copied into this new release identity."
            },
            {
              "id": "A-W0-assets",
              "text": "Baton v1.0.0-rc.3 is pinned by annotated tag object 34324784694696a38d951061c2313363b405c1e4, peeled commit affaf16cc37f845b5dc43b22988d8b680ff1f212, tree a26078b7db4ee36bdae4f28a48447ff2df782f4f, normalized release archive sha256:4757078049d8e9f0ac3db2aee91e65f8df48f31b0cccf26478343ca3d79d5166 and installed support package sha256:e5927a82f7c8a0daf3aa1196e7aa56231044449bb141cc2d7efd1cc8cca209bd. The admission check independently reconstructs and verifies all five; any unresolved or mismatched value hard-stops installation and materialization. VCS-free twin builds from separate product copies and fresh caches are byte-identical."
            },
            {
              "id": "A-W0-budget-authority",
              "text": "Fresh protected approval of this exact raw plan digest ratifies the measured replacement baseline and budgets and explicitly supersedes the provisional issue #157 body target of at most 15000 non-generated production Go lines. The admitted W0 plus proposed W1 and W2 composition already contains 10464 non-generated runtime-production physical lines across 29 cmd/** and internal/** files; its normal W5 plus non-W5 combined ceiling is therefore 18064 and its two proof-backed Captain bands can reach 19264. Approval of a conversation summary, an earlier plan, or bytes that do not contain this supersession is insufficient."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test ./tools/batonassets/... ./tools/batongolden/... ./cmd/sworn/..."],
          "constraints": [".baton/releases is control authority only and never a product, model, check, workspace, candidate, build or package input. Official builds use -buildvcs=false and -trimpath."],
          "depends_on": []
        }
      ]
    },
    {
      "id": "T1-authority",
      "ref": "refs/heads/track/sworn-v0.3.0-baton-rc3/T1-authority",
      "depends_on": ["T0-admission"],
      "touch_surfaces": ["internal/baton", "internal/gitx", "tools/batongolden"],
      "work": [
        {
          "id": "W1-authority-core",
          "outcome": "Implement Baton v1.0.0-rc.3 authority, product identity and exact Git composition in pure Go.",
          "scope": {
            "include": ["internal/baton", "internal/gitx", "tools/batongolden"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W1-authority",
              "text": "Strict records, bindings, lifecycle, admitted actions, retry reconciliation, track composition, assembly preparation and exact-ref compare-and-set integration match the pinned RC3 goldens. Product identity excludes .baton/releases while exact candidate OID, tree, ancestry, expected target and record history remain mandatory. Callers cannot choose authority refs, paths, Git commands, parents or merge mode; stale, conflicting, consumed, symbolic or unknown ref state fails without moving a ref."
            },
            {
              "id": "A-W1-rebind",
              "text": "Candidate c8ddcd89cbfdaab6c94d601631f87840bf7fd7c2 may be replayed only as scoped implementation input after an RC3-bound design and Captain PROCEED. A fresh candidate on this plan's exact materialization, fresh proof and fresh adversarial verification must establish every acceptance item; no RC2 proof, status or verdict is accepted by reference."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/baton/... ./internal/gitx/... ./tools/batongolden/..."],
          "constraints": ["The JavaScript reference is a development oracle only. Git runs through one sanitized literal boundary, and models never resolve conflicts or advance authority."],
          "depends_on": ["W0-reset-admission"]
        }
      ]
    },
    {
      "id": "T2-driver",
      "ref": "refs/heads/track/sworn-v0.3.0-baton-rc3/T2-driver",
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
              "text": "One driver contract covers Planner, Implementer, Captain and Verifier invocations with an explicit driver and model. Portable fixtures retain model:null Merge coverage, while production Merge is deterministic, engine-owned and never dispatched. A fresh bounded process receives only its workspace and ordered digest-bound inputs through a read-only overlay; Baton records, journal, resolver, canonical worktree and engine logs are absent. One sealed submission binds the invocation and permits only the role's exact artifacts and decision."
            },
            {
              "id": "A-W2-isolation-usage",
              "text": "Cancellation, clean read-only verification, output limits and the shared conformance suite pass with the fake. Transport failure creates no Baton decision or verdict. Usage keeps reported, unavailable and legitimate zero distinct; Sworn never estimates tokens or cost."
            },
            {
              "id": "A-W2-rebind",
              "text": "Candidate 0136a96c4355e60c815b5cab043b54e860d00062, product digest sha256:a9148b61fa37c11c6abc293160e6b8c3d4b28830d04a1ac2cc913506b6d016bf and proof sha256:e90eef99febd5aa0f65b8aab1d44927a5903a7549bc9b592a73a5a87680a3338 are prior-lineage evidence only. Reuse requires exact scoped replay or rebinding onto this plan's materialization plus a fresh proof and fresh adversarial verification; the old PASS is not authority."
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
      "ref": "refs/heads/track/sworn-v0.3.0-baton-rc3/T3-runtime",
      "depends_on": ["T1-authority", "T2-driver"],
      "touch_surfaces": ["cmd/sworn", "go.mod", "go.sum", "internal/gitx", "internal/journal", "internal/runtime", "test/e2e"],
      "work": [
        {
          "id": "W3-walking-skeleton",
          "outcome": "Drive one approved track through all Baton responsibilities and exact integration.",
          "scope": {
            "include": ["cmd/sworn", "go.mod", "go.sum", "internal/gitx", "internal/journal", "internal/runtime", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W3-flow",
              "text": "Bounded intent produces a strict Planner proposal and pauses on its raw digest. Only an outside-runner approval resolver may admit protected external approval; its immutable evidence binds the exact raw plan digest, repository, target ref and OID, authorizer identity, protected provenance and approval bytes. Conversation, TTY presence, Planner output, the autonomous caller and every delivery actor are incapable of minting approval. Missing, changed, unavailable, self-authored or invalid evidence stops before installation; exact replay returns the same one install receipt without another commit. A typed internal/gitx seam creates disposable role worktrees, captures their exact identity and imports only the sealed permitted product result; it accepts no caller-supplied Git command, ref, parent, merge mode or arbitrary path, and model processes never receive a canonical authority worktree. Implementer design, distinct Captain review, resumed implementation and fresh adversarial work verification then complete through the fake. After work PASS, engine-owned Merge composes the track and prepares assembly; a separate clean read-only worktree and fresh assembly Verifier alone permit engine-owned exact target integration."
            },
            {
              "id": "A-W3-journal",
              "text": "SQLite with one serialized connection and BEGIN IMMEDIATE transactionally records replay-stable commands, finite claims, attempts, before-effect identities, immutable receipts, normalized usage, run identity and an append-only monotonic local event stream. It binds an exact application and schema identity; read-only access never creates or migrates a database. These facts own recovery, evaluation and cockpit replay but never synthesize Baton lifecycle. The journal exposes a transactionally bound immutable snapshot plus event offset and bounded opaque evidence/log references; runtime exposes replay-stable typed command receipts, multi-run discovery metadata and a bounded best-effort typed observer with a no-op default. The observer receives redacted facts only after durable decisions and cannot block, retry or control delivery; journal events are never an OTLP queue. Identical replay returns the same identity; conflicting bytes or uncertain effects stop before retry."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/gitx/... ./internal/journal/... ./internal/runtime/... ./cmd/sworn/... ./test/e2e/..."],
          "constraints": ["No delivery actor approves its own plan. Local events are not an OTLP queue. T3 owns the first post-baseline go.mod and go.sum update; the dependency-ordered T5 module update must compose from T3's exact result."],
          "depends_on": ["W1-authority-core", "W2-driver-core"]
        },
        {
          "id": "W4-topology-recovery",
          "outcome": "Add Coach-loop worktree topology, bounded parallel tracks and honest recovery.",
          "scope": {
            "include": ["cmd/sworn", "internal/journal", "internal/runtime", "test/e2e"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W4-topology",
              "text": "release-wt owns assembly while product-only track worktrees run dependency-ready tracks concurrently with one serial writer per track. Parking one track on an operational failure never stops another independent dependency-ready track; a deterministic fixture proves that independent progress while the release remains gated. The runtime consumes W1's single exact Snapshot and selects only the current structurally authoritative status proved by owner materialization or exact composition transfer, then reports that lineage's valid lifecycle state; it never ranks timestamps, completion order or a later-looking record from divergent or malformed authority and does not implement a second oracle. Composition is serial, plan-ordered, exact and engine-owned; a distinct fresh assembly Verifier alone gates release Merge."
            },
            {
              "id": "A-W4-recovery",
              "text": "Pause, resume, cancel, retry and takeover are typed replay-stable commands served through one narrow runtime API with durable receipts; they accept no shell, path, ref, Git command or direct Baton write. Each run durably fixes hard maxima of three total transport attempts per invocation, three fresh role invocations per unchanged Baton responsibility in one retry epoch, and two retry epochs per unchanged candidate and status. Only a new authoritative Baton status, candidate or successfully reconciled external-effect receipt is progress; timers, logs, restarts, transport errors, lease expiry, takeover and identical replay never reset a budget. Lease expiry alone never authorizes takeover: the prior process tree must be quiescent and its effect reconciled first. Uncertain effects reconcile before retry. Transport exhaustion emits operational NO_VERDICT, role exhaustion parks that track for Planner or operator attention without synthesizing a Baton verdict, and retry-epoch exhaustion stops automatic dispatch for Planner or human authority; only a newly approved work identity after replan starts a new budget. Independent ready tracks continue while one track is parked, its dependants remain gated and assembly remains gated. Captain REVISE and work FAIL return to Implementer; Captain ESCALATE, work BLOCKED and assembly FAIL or BLOCKED return to Planner. After materialization, replanning preserves lineage and uses new approved work and release identities. Moved targets and conflicts stop without changing the prior release ref."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/journal/... ./internal/runtime/... ./cmd/sworn/... ./test/e2e/..."],
          "constraints": ["Concurrency is bounded. Cancellation cleans process trees and never rewrites Baton records."],
          "depends_on": ["W3-walking-skeleton"]
        }
      ]
    },
    {
      "id": "T4-adapters",
      "ref": "refs/heads/track/sworn-v0.3.0-baton-rc3/T4-adapters",
      "depends_on": ["T2-driver"],
      "touch_surfaces": ["internal/driver"],
      "work": [
        {
          "id": "W5-production-adapters",
          "outcome": "Implement six production profiles behind the common role-neutral driver.",
          "scope": {
            "include": ["internal/driver"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W5-common",
              "text": "Codex CLI, Claude Code CLI, OpenAI-compatible, DeepSeek, Gemini and Bedrock are six profiles behind one adapter contract; DeepSeek is an OpenAI-compatible profile. Native profiles retain their exact-version agentic CLI loops under one outer runner policy. The four HTTP profiles share one bounded tool loop exposing an empty-environment contained workspace shell plus sealed sworn_submit, with role authority expressed only by outer read-only or read-write mounts. Driver and provider codecs do not own roles, gates, retries, planning, recovery or Merge. A private domain-separated CredentialRef binds a configured credential handle and private credential generation without retaining credential bytes or cleartext identifiers. It detects unexpected local credential-generation substitution but never claims provider-account identity unless an authenticated provider response explicitly asserts and binds that identity."
            },
            {
              "id": "A-W5-bounds",
              "text": "HTTP profiles use bounded non-streaming JSON request and response bodies; SSE and AWS eventstream are unsupported and rejected. Native CLIs keep their own agentic loop and bounded JSONL lifecycle events plus a schema-checked final result; native event transport is not treated as an HTTP stream. Every invocation has explicit body or event, turn, aggregate tool-call, output and time limits. Parent cancellation terminates the complete native process tree or in-flight HTTP request and every profile maps provider-reported usage, legitimate zero and unavailable usage into the same typed receipt without estimation. Any attempted sworn_submit call, whether accepted or rejected, terminates that invocation and permits no later model or tool turn."
            },
            {
              "id": "A-W5-codex",
              "text": "Codex runs the exact digest-pinned static vendor ELF, not the Node launcher, with structured final output. It is unattended with -a never and never uses --yolo or bypass flags. A tested nested restriction grants model commands only the role workspace while the parent may refresh one locked persistent auth file in place; model commands cannot list, open, copy or infer that file. User config and rules are ignored, Verifier memories and external-memory import are disabled, and a closed feature manifest disables skill_search, in_app_browser, shell_snapshot, MCP, apps, plugins, hooks, browser, computer-use, multi-agent and every other unapproved ambient surface. Unknown default-on surfaces fail certification."
            },
            {
              "id": "A-W5-claude",
              "text": "Claude runs the exact digest-pinned resolved native ELF in non-interactive structured-output mode with --bare and --safe-mode, slash commands disabled, strict empty MCP configuration, Chrome disabled, no session persistence and the exact built-in tool allowlist Bash, Read, Edit, Write, Glob and Grep. Web, Task, network-agent and configuration extension surfaces are absent. A credential-backed or provider-backed live run is forbidden until certification proves every allowed tool child is blind to auth and provider credentials. Until then Claude may pass only stub and fixture conformance and reports live readiness as NOT CERTIFIED."
            },
            {
              "id": "A-W5-http",
              "text": "OpenAI-compatible and DeepSeek use bounded Chat Completions JSON with an explicit configurable base URL; live non-loopback endpoints require HTTPS and fixture-only loopback HTTP is separately marked. DeepSeek fixtures prove tools, tool_calls and reasoning_content are preserved or normalized without leaking reasoning into tool arguments or final artifacts. Gemini uses bounded generateContent JSON and round-trips function declarations, functionCall and functionResponse parts. Bedrock uses bounded Converse JSON with SigV4. This v0.3 plan deliberately approves the narrower bedrock_explicit_v1 credential and region mode instead of claiming the complete standard AWS chain: an explicit CredentialRef resolves access key, secret, optional session token and an explicit region, while shared profiles, process credentials, web identity, ECS and IMDS are unsupported and never queried or used as fallback. Doctor and certification report that narrower mode. Bedrock fixtures close credential, region, service, date, signed-header and payload-hash framing and prove model IDs plus inference-profile ARNs are URI-escaped exactly once in the canonical path. Streaming, SSE and AWS eventstream inputs fail closed."
            },
            {
              "id": "A-W5-certification",
              "text": "Before any inference, the driver certification library checks that the exact configured profile and model advertise every requested tool, function-call, structured-result, system-message, usage and cancellation capability; unknown or incompatible capability state hard-stops without a provider request. It emits canonical manifests for adapter/profile settings, exact ELF and toolchain digests, required libraries, CA bundle, resolv.conf, hosts and NSS inputs without exposing secrets. It detects drift, missing runtime closure, unusable DNS or NSS, credential visibility, undeclared tools and unsupported features before live execution. The runner mounts no undeclared host tool or configuration surface."
            },
            {
              "id": "A-W5-live-complexity",
              "text": "Technical adapter readiness requires credential-backed live PASS from at least one native CLI and one HTTP profile; each other profile reports PASS, NOT CERTIFIED or NOT RUN separately and never inherits another profile's result. The admitted W0 plus proposed W1 and W2 product-only composition contains exactly 10464 non-generated runtime-production physical lines across 29 cmd/** and internal/** files; blank and comment lines count, while support tooling is 773 lines, tests 5247, fixtures 247 and generated Go 0. W5 growth is the added non-generated runtime-production physical lines under internal/driver against that exact composition: at most 2600 is the normal target, 2601 through 2800 requires explicit proof-backed Captain justification, and more than 2800 hard-stops for a newly approved replan. Generated, support-tooling, test and fixture growth is reported separately and cannot offset production growth."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/driver/..."],
          "constraints": ["Use lean direct Go drivers without provider SDKs, hosted inference, proxying, gateways or hosted credential custody. The common runner boundary, not provider-specific orchestration, owns workspace and tool policy."],
          "depends_on": ["W2-driver-core"]
        }
      ]
    },
    {
      "id": "T5-operator",
      "ref": "refs/heads/track/sworn-v0.3.0-baton-rc3/T5-operator",
      "depends_on": ["T3-runtime"],
      "touch_surfaces": ["cmd/sworn", "go.mod", "go.sum", "internal/cockpit", "internal/observe"],
      "work": [
        {
          "id": "W6-operator-evidence",
          "outcome": "Deliver a thin terminal and Web cockpit, deterministic local evals and privacy-safe opt-in OTel.",
          "scope": {
            "include": ["cmd/sworn", "go.mod", "go.sum", "internal/cockpit", "internal/observe"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-W6-cockpit",
              "text": "Terminal JSON and the single-file WebUI share one ref-aware oracle across releases, release-wt, tracks and worktrees. An authenticated loopback-only control endpoint exposes only typed pause, resume, cancel, retry and takeover commands; a per-run unguessable credential, same-origin validation and durable command receipt bind every request. There is no arbitrary shell, path, Git or direct Baton-record write surface. The cockpit discovers multiple local runs, reconnects from a transactionally bound snapshot plus event offset without gaps, and drills from a projected row into immutable evidence and bounded redacted local logs. Both views expose current responsibility, worker, gate, driver/model, duration, retries, usage availability, recovery and command receipts. Frozen scenarios for moved refs, stale events, corrupt records, reconnect gaps, restart and concurrent updates yield identical authority or explicit unknown/error, never inferred progress. A real browser proves desktop and mobile layout, keyboard operation, focus order and automated accessibility checks."
            },
            {
              "id": "A-W6-eval",
              "text": "A documented command emits versioned canonical JSON from one pinned journal snapshot. Rows bind outcomes, exact integration, roles, driver/model, gates, findings, rework, retries, recovery, protocol artifact count, orchestration and model time, digests and usage availability. Every aggregate reports numerator, denominator and unknown counts; immutable expected-outcome labels alone drive quality and false-result measures. Replay is deterministic and evaluation never controls delivery."
            },
            {
              "id": "A-W6-otel",
              "text": "Direct Go OpenTelemetry emits versioned sworn.* spans, events and fixed-enum low-cardinality metrics only when explicitly enabled. Provider, model and every other unbounded identifier may appear only on allowlisted traces or events and never as metric labels. Export is bounded, asynchronous and non-controlling; local status exposes queue depth, drops, failures and last success. Allowlist and hostile-canary tests exclude prompts, completions, source, diffs, paths, credentials, argv, arbitrary errors, finding prose and tool payloads. Collector outage, rejection or restart creates explained gaps without changing delivery."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test -race ./internal/cockpit/... ./internal/observe/... ./cmd/sworn/..."],
          "constraints": ["The cockpit projects authority and submits typed commands; it is never a scheduler or Baton writer. T5 may change go.mod and go.sum only from the exact dependency-complete T3 result and must record the ordered module delta. No OTel logs, embedded Collector, vendor telemetry SDK, remote scoring, pricing catalogue, LangChain, LangGraph, Langfuse or DBOS is added."],
          "depends_on": ["W4-topology-recovery"]
        }
      ]
    },
    {
      "id": "T6-release",
      "ref": "refs/heads/track/sworn-v0.3.0-baton-rc3/T6-release",
      "depends_on": ["T3-runtime", "T4-adapters", "T5-operator"],
      "touch_surfaces": ["README.md", "cmd/sworn", "docs/releases/v0.3.0", "test/e2e"],
      "work": [
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
              "text": "The built binary passes every autonomous-engine case enumerated by the immutable pinned Baton v1.0.0-rc.3 manifest through the real adapter. The manifest runner rejects missing, duplicate, skipped, NOT RUN or non-PASS cases. Every production profile passes shared fixture conformance; credential-gated live results remain separate."
            },
            {
              "id": "A-W8-driver-operations",
              "text": "The real binary wires and tests sworn driver inspect, sworn driver doctor and sworn driver certify to the W5 certification library. Their canonical secret-free output identifies the exact profile, ELF, toolchain, library, CA, DNS, hosts and NSS manifest, explains drift or missing closure, and prevents an uncertified live invocation. CLI fixtures cover healthy, drifted, incomplete, credential-visible and unknown-capability states."
            },
            {
              "id": "A-W8-parity",
              "text": "The binding Coach parity baseline matrix is the pinned Baton package file docs/captures/2026-07-24-coach-loop-parity-baseline.md with raw digest sha256:8ad596e72fefb1b4cb43fdcce8cf4a705f65ead7618bb575dca1675cb9c7c39c. Through the real binary, one disposable fixture completes unattended delivery across exactly three tracks, includes a dependency edge and at least two serial slices in one track, advances an independent ready track while another is parked, proves divergent release/owner-ref authority, and refuses both a wrong worktree and a dirty worktree without mutation. It uses two driver families, distinct configured models, all four model-facing roles, deterministic engine Merge, fresh work and assembly verification, exact integration and a final target tree equal to the passed assembly. Every baseline row, including timeout/no verdict, process death at every external-effect edge, stale target, Verifier FAIL and repair, BLOCKED specification, composition conflict, fresh assembly verification, multi-driver per-role models, truthful restart views and telemetry non-interference, must finish PARITY, SUPERSEDED with a stricter mapped mechanism and executable evidence, or REJECTED with exact protected owner ratification; zero rows may be MISSING, silently skipped or inferred from another scenario."
            },
            {
              "id": "A-W8-product-release",
              "text": "Two independently materialized product-only copies with fresh caches pass full VCS-free checks and produce byte-identical binaries and normalized packages before and after a record-only status change. The release evidence reports production package count, non-generated runtime-production physical lines, support-tooling/test/fixture/generated lines, direct and transitive module requirements, stripped binary size and SHA-256, and per-work growth against the recorded baseline. Technical readiness binds the passed assembly product digest and normalized payload manifest while retaining exact Git evidence."
            },
            {
              "id": "A-W8-growth",
              "text": "The post-W2 baseline is 10464 non-generated runtime-production physical lines across 29 cmd/** and internal/** files, zero direct non-indirect module requirements, and a 2662562-byte CGO-disabled Go 1.26.5 linux/amd64 stripped binary with sha256:8f7ffd80c77019e72eaa28394425870241d83252f6fb2fa6e7873f91a58f9c20. Using the same recorded byte-defined classifier, cumulative non-W5 growth attributable to W3, W4, W6 and W8 targets at most 5000 added runtime-production lines; 5001 through 6000 requires explicit proof-backed Captain justification and more than 6000 requires a newly approved replan. W5 independently targets at most 2600, requires Captain justification from 2601 through 2800, and requires replan above 2800. The final go.mod targets at most 8 direct non-indirect module requirements, requires Captain justification for 9 or 10, and requires replan above 10. The exact official build command targets at most 25165824 bytes; 25165825 through 33554432 requires Captain justification and more than 33554432 requires replan. Every band reports its exact baseline, final value and delta; generated, support-tooling, test, fixture or transitive-dependency counts cannot offset a production, direct-dependency or binary breach."
            }
          ],
          "checks": ["GOFLAGS=-buildvcs=false go test ./...", "GOFLAGS=-buildvcs=false go test -race ./...", "GOFLAGS=-buildvcs=false go vet ./...", "bash -o pipefail -c 'unformatted=\"$(git ls-files -z -- \"*.go\" \":(exclude,top).baton/releases/**\" | xargs -0 -r gofmt -l)\"; test -z \"$unformatted\"'", "CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -mod=readonly -buildvcs=false -trimpath -ldflags='-s -w' -o /tmp/sworn-v0.3.0 ./cmd/sworn", "git diff --check"],
          "constraints": ["This gate proves technical readiness only. It does not authorize a tag, main merge, hosted deployment or sworn-web change."],
          "depends_on": ["W4-topology-recovery", "W5-production-adapters", "W6-operator-evidence"]
        }
      ]
    }
  ]
}
```

# Goal

Deliver Sworn v0.3 as a lean Go engine that runs immutable Baton
v1.0.0-rc.3 autonomously: four model-facing roles, deterministic Merge,
parallel track worktrees, common multi-vendor drivers, truthful recovery,
observable quality and exact integration.

# Authority

This is a non-authoritative Plan proposal. The repository owner must approve
these exact raw bytes through the fresh protected marker
`baton-plan-approval-sworn-v0.3.0-baton-rc3-v1` on
swornagent/sworn#157. The old RC2 plan, approval, statuses and conversation
grant no authority.

That fresh exact-byte approval also explicitly supersedes issue #157's
provisional target of at most 15,000 non-generated production Go lines. The
measured replacement baseline is already 10,464 runtime-production lines, and
the separately gated normal W5 and non-W5 allowances make the normal combined
ceiling 18,064. Without protected approval of these exact replacement bytes,
neither the supersession nor any larger ceiling is authorized.

# Scope

Included are RC3 authority, the role-neutral driver contract and six provider
profiles, Coach topology and recovery, thin terminal/Web cockpit, deterministic
evals, opt-in OTLP and real-binary parity. Excluded are hosted inference or
credential custody, provider-specific orchestration, workflow frameworks,
legacy-kernel reuse, publication and website work.

# Acceptance

Metadata acceptance IDs are the observable gates. Evidence binds exact refs,
candidates, receipts, verdicts, manifests and deterministic local checks. Raw
logs remain local and digest-addressed. Coach parity binds the pinned Baton
package file `docs/captures/2026-07-24-coach-loop-parity-baseline.md` at digest
`sha256:8ad596e72fefb1b4cb43fdcce8cf4a705f65ead7618bb575dca1675cb9c7c39c`.
Every row receives an explicit terminal classification and zero rows may remain
missing.

# Ordered tracks and work

```text
T0 admission
  ├─ T1 authority ─┐
  └─ T2 driver ────┼─ T3 runtime: W3 -> W4 ── T5 operator: W6 ─┐
                   └─ T4 adapters: W5 ──────────────────────────┼─ T6 release: W8
```

Independent tracks own disjoint surfaces. Work is serial within a track.

# Dependencies and touch surfaces

Metadata owns ordering and paths. The `go.mod` and `go.sum` overlap is
intentional and ordered: T0 establishes the baseline, T3 adds SQLite, and only
after all of T3 completes may T5 add direct Go OpenTelemetry dependencies.
Every module delta binds its exact predecessor. Any other unexpected
cross-track overlap, undeclared host capability or shared-file conflict stops
for repair or a newly approved plan.

# Checks

Each work runs its focused suite. W8 repeats whole-repository test, race, vet,
format, build, reproducibility and diff checks. Raw outputs are retained
locally by digest. Release checks reproduce physical-line manifests, direct
module counts and the CGO-disabled stripped linux/amd64 build, reporting exact
baseline, final value and delta against each normal, Captain and replan band.

# Constraints

Baton records and Git own delivery truth; the journal owns runtime, evaluation
and cockpit facts. Verifiers are fresh and read-only. Effects are recorded
before execution and reconciled before retry. Reapproval is required for a
changed outcome, authority, product scope, safety boundary, topology,
ownership or observable acceptance.
