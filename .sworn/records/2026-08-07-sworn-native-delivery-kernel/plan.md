```baton-plan-v2
{
  "schema_version": "baton.plan/v2",
  "release": "2026-08-07-sworn-native-delivery-kernel",
  "revision": 2,
  "previous_plan": "9293aa5e3d065d0781a73957090e7cf889303181",
  "repository": "sworn",
  "target_ref": "refs/heads/main",
  "approval_ref": "operator://2026-08-07-sworn-native-delivery-kernel/2",
  "tracks": [
    {
      "id": "T1-kernel",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-native-authority",
          "outcome": "Sworn owns its delivery roles, trust rules, receipts, and transition authority without requiring a separately installed, released, or certified Baton product, while existing Baton-authored history remains safely readable.",
          "scope": {
            "include": [
              "Sworn-owned Planner, Implementer, Captain, Verifier, and deterministic Merge operation authority",
              "Sworn-owned plan, receipt, candidate, transition, and merge validation",
              "legacy Baton plan and receipt read compatibility",
              "one thin globally installed Sworn agent skill and retirement of standalone Baton role skills",
              "Sworn CLI, TUI, MCP, diagnostics, documentation, and runtime identity"
            ],
            "exclude": [
              "rewriting committed historical records",
              "deleting or changing the published receipt-v1 schema bytes",
              "maintaining Baton as a separately released protocol or executable",
              "live streams, detached workers, parallel dispatch, caching, expanded telemetry, and in-flight adjustment"
            ]
          },
          "acceptance": [
            {
              "id": "A1",
              "text": "A clean offline build and first run use only Sworn-owned embedded contracts and never require a Baton installation, checkout, release lookup, version match, conformance command, network fetch, or driver certification before delivery can start."
            },
            {
              "id": "A2",
              "text": "Role separation, fresh read-only verification, merge-only-the-exact-PASS-candidate, no verdict on operational failure, exact-head repair continuation, target-advance tolerance, and deterministic engine-owned Merge remain enforced through one Sworn command and reducer path."
            },
            {
              "id": "A3",
              "text": "Fixtures and real repositories prove supported Baton-era plans and receipts remain readable with their original provenance, malformed or ambiguous history fails closed, and newly written authority identifies a Sworn-owned contract without rewriting prior commits."
            },
            {
              "id": "A4",
              "text": "Operator surfaces consistently describe one Sworn product and one release authority; no user-facing error instructs the operator to install, restore, upgrade, or certify Baton, while diagnostics distinguish legacy history from active Sworn authority."
            },
            {
              "id": "A5",
              "text": "Sworn can evolve its operation wording and implementation with an ordinary Sworn release without byte-identical external package checks, generated-payload ceremony, conformance publication, or semantic word-budget ratchets."
            },
            {
              "id": "A6",
              "text": "A supported install exposes one thin Sworn agent skill that recognizes governed work, locates or starts the local Sworn service, prefers its MCP tools for headless operation, and presents exact operator choices; it contains no independent lifecycle reducer, receipt writer, role authority, verification verdict, or merge procedure."
            },
            {
              "id": "A7",
              "text": "Previously installed baton-plan, baton-implement, baton-design-review, baton-verify, and baton-merge skills are removed or replaced by bounded migration stubs that direct the caller to Sworn and cannot continue the old standalone prose workflow; installation and upgrade tests prove stale copies cannot remain earlier on an agent's skill path."
            },
            {
              "id": "A8",
              "text": "Planner, Implementer, Captain, and Verifier semantic instructions remain versioned Sworn-owned prose assets bound to and delivered with the exact responsibility invocation; changing those assets is an ordinary Sworn product change, and no globally installed skill can substitute different role instructions or grant authority."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false go test -count=1 ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go test -count=1 -parallel=1 -timeout=20m ./test/e2e",
            "GOFLAGS=-buildvcs=false go test -count=1 -race ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go vet ./...",
            "git diff --check"
          ],
          "constraints": [
            "Retirement changes ownership, not the substance of the trust boundary.",
            "Historical Baton identities remain historical facts and are never relabelled as Sworn-authored facts.",
            "Keep https://baton.sawy3r.net/schemas/receipt-v1.json serving its exact published bytes for existing receipts.",
            "Do not perform a broad package or file rename unless required by an acceptance boundary; remove duplicate authority before cosmetic terminology.",
            "Installed agent skills are discovery and transport adapters only; Sworn's binary and command service remain the sole lifecycle authority.",
            "No builder may certify its own work and no model may perform Merge."
          ],
          "depends_on": [],
          "consumes": []
        },
        {
          "id": "S2-slice-artifacts",
          "outcome": "A Sworn delivery is defined by one compact release manifest plus one immutable contract per slice, with explicit dependency, consumed-input, evidence, and parallel touchpoint information that can be validated and retained independently.",
          "scope": {
            "include": [
              "Sworn-native release manifest and slice contract formats",
              "per-slice content digests and safe relative paths",
              "optional human-readable evidence bundles",
              "explicit track touchpoint ownership and overlap validation",
              "legacy plan-v2 and plan-v3 read compatibility",
              "Planner, CLI, TUI, MCP, and repository discovery projections"
            ],
            "exclude": [
              "mutable status.json, journal.md, or proof.md lifecycle authority",
              "candidate-path allowlists",
              "word-count or prose-layout gates",
              "running dependency-ready tracks concurrently"
            ]
          },
          "acceptance": [
            {
              "id": "A1",
              "text": "Every new release manifest names each slice's stable ID, one-line outcome, safe relative contract path, and exact digest; missing, substituted, mismatched, duplicated, escaping, or malformed slice artifacts fail closed before approval or dispatch."
            },
            {
              "id": "A2",
              "text": "Each slice artifact contains only its promised outcome, scope, acceptance, minimum checks, constraints, dependencies, and consumed products, and an unchanged slice can be retained mechanically across a forward-only plan revision while the actual changed dependency closure is invalidated."
            },
            {
              "id": "A3",
              "text": "The optional touchpoint matrix assigns every declared product path to its owning track, uses prefix-aware overlap, compares the slices that actually declare a shared path, accepts only explicitly ordered sharing, and leaves genuinely disjoint dependency-ready tracks eligible for parallel execution."
            },
            {
              "id": "A4",
              "text": "Evidence bundles can inventory screenshots, recordings, command output, traces, and other human-reviewable proof without becoming lifecycle authority, and every bundle remains bound to the exact release, slice, attempt, candidate, tree, checks, and verifier result it describes."
            },
            {
              "id": "A5",
              "text": "Planner output, repository admission, CLI, TUI, and MCP read and present the same canonical manifest and slice contracts; supported legacy plans remain readable, but newly proposed releases use only the Sworn-native format."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false go test -count=1 ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go test -count=1 -parallel=1 -timeout=20m ./test/e2e",
            "GOFLAGS=-buildvcs=false go test -count=1 -race ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go vet ./...",
            "git diff --check"
          ],
          "constraints": [
            "The plan is a commitment, not an inventory of predicted implementation files or operational bookkeeping.",
            "Use the corrected slice-to-slice touchpoint ordering behavior merged to Baton main after RC15.3, not the defective tagged comparison of each track's first slice.",
            "Separate files improve stable ownership and review; they cannot weaken exact-byte approval or permit partial mutable plans.",
            "One canonical parser and validator own every presentation and runtime consumer.",
            "No format migration may silently activate historical, remote-only, malformed, or ambiguous material."
          ],
          "depends_on": [
            "S1-native-authority"
          ],
          "consumes": [
            "S1-native-authority"
          ]
        },
        {
          "id": "S3-semantic-loop",
          "outcome": "Sworn turns a human goal into useful, separately reviewable slices, resolves only genuine meaning gaps with the human, and carries the confirmed plan through Implementer design and adversarial Captain review using durable resumable authority.",
          "scope": {
            "include": [
              "repository-aware Planner discovery and summary-before-plan flow",
              "typed durable human-only meaning confirmation",
              "slice decomposition, dependency, touchpoint, acceptance, and evidence design",
              "Implementer acceptance-to-approach design handoff",
              "Captain adversarial review, replan routing, and delegated approval within recorded remit",
              "restart-safe continuation through TUI, MCP, and headless operator surfaces",
              "one canonical headless MCP lifecycle over the existing Sworn command and projection services"
            ],
            "exclude": [
              "fixed questionnaires, mandatory clarification turns, or wording snapshots",
              "Captain expansion of its own remit",
              "human meaning confirmation being treated as plan approval",
              "detached worker survival, multi-track concurrent dispatch, or general in-flight adjustment"
            ]
          },
          "acceptance": [
            {
              "id": "A1",
              "text": "Before asking a person, the production Planner inspects current Git, applicable plans, code, tests, documentation, and useful history; it asks only for a missing human choice that could materially change the promised result and never asks the human to rediscover available repository facts."
            },
            {
              "id": "A2",
              "text": "When meaning is clear, Planner first presents a concise result, scope, acceptance, evidence, inputs, and limits summary; a genuine material ambiguity creates one exactly bound human-only turn, and only the resumed responsibility after that answer may emit approval-ready manifest and slice bytes."
            },
            {
              "id": "A3",
              "text": "Every proposed slice has one independently reviewable result, an acceptance boundary that can fail through the real product, its minimum real-binary E2E proof, real dependencies and consumed products, and explicit touchpoint ownership; the plan reuses existing production owners and does not invent parallel functions, fields, schemas, or components."
            },
            {
              "id": "A4",
              "text": "Before implementation, Implementer maps every acceptance ID to an approach and proposed evidence, identifies existing owners and risks, and stops without code when a required behavior is absent or contradictory."
            },
            {
              "id": "A5",
              "text": "Captain independently attempts to disprove the outcome, acceptance, exclusions, dependencies, consumed inputs, touchpoints, reuse claims, risks, and proposed proof; it returns PROCEED or REVISE within the exact approved contract and escalates only the precise decision outside its recorded remit."
            },
            {
              "id": "A6",
              "text": "After an exact plan exists, Captain may authorize or revise it without pausing for the human only when the recorded delegation covers that decision; new scope, changed target, destructive or high-stakes work, ambiguity, protocol migration, or remit expansion still requires explicit human approval and produces an informational operator event."
            },
            {
              "id": "A7",
              "text": "Human questions and answers are durably bound to the exact release, track, slice, role, responsibility, invocation, authority snapshot, turn, generation, and park checkpoint; stale, conflicting, replayed, wrong-scope, or structurally corrupted answers fail closed across restart without granting plan, Git, candidate, receipt, or merge authority."
            },
            {
              "id": "A8",
              "text": "The local product MCP exposes the supported headless start or attach, status and current responsibility, control, human answer, exact approval, Captain delegation, and role-result admission capabilities needed for the serial loop; every tool calls the same command or projection owner as TUI and CLI, uses strict typed input and structured output, and cannot bypass an unavailable or rejected authority transition."
            },
            {
              "id": "A9",
              "text": "A real coding-agent host using only the installed Sworn skill and MCP completes the same semantic journey and produces the same durable authority as an interactive TUI run; killing the MCP client or reconnecting it cannot duplicate, lose, or invent responsibility, submission, approval, verdict, receipt, or merge authority."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false go test -count=1 ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go test -count=1 -parallel=1 -timeout=20m ./test/e2e",
            "GOFLAGS=-buildvcs=false go test -count=1 -race ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go vet ./...",
            "git diff --check"
          ],
          "constraints": [
            "Extend the existing driver yield, journal attention, continuation, recovery, approval, delegation, Git, workspace, and presentation owners; add no second lifecycle path.",
            "Carry forward the proven human-turn implementation and the uncommitted exact-park-checkpoint repair only after rebasing them onto the passed S2 product and re-verifying them; do not merge the obsolete RC15-conformance candidate or receipt.",
            "Planning, building, reviewing, verifying, and merging remain separate responsibilities even though their contracts are Sworn-owned.",
            "MCP is a transport over canonical Sworn services, not a second orchestrator, lifecycle model, event store, or source of role semantics.",
            "All driver and model selections remain explicit and provider-neutral; no model default, silent fallback, or preflight certification gate is introduced.",
            "Prompts, answers, repository facts, source, diffs, tool data, and raw errors remain project-local customer content."
          ],
          "depends_on": [
            "S2-slice-artifacts"
          ],
          "consumes": [
            "S2-slice-artifacts"
          ]
        },
        {
          "id": "S4-kernel-proof",
          "outcome": "The exact assembled native kernel completes a fresh real-agent Sworn delivery from goal discovery through planning, approval, implementation, independent verification, deterministic merge, interruption recovery, and truthful operator presentation.",
          "scope": {
            "include": [
              "slice-level real-binary end-to-end proof",
              "cumulative T1 kernel end-to-end proof",
              "fresh exact-assembly real-agent system proof",
              "legacy project migration and clean-project journeys",
              "failure, restart, direct-repair, and target-advance scenarios"
            ],
            "exclude": [
              "mock-only or scripted-submission release certification",
              "dependency-ready multi-track parallel execution",
              "live in-flight adjustment and detached worker survival",
              "macOS packaging and first-run release proof"
            ]
          },
          "acceptance": [
            {
              "id": "A1",
              "text": "Each preceding slice has an independently verified real-built-binary E2E result bound to its exact candidate, and their exact composition passes one cumulative T1 journey before assembly."
            },
            {
              "id": "A2",
              "text": "A fresh real-agent run in a clean repository enters through the installed Sworn skill and local MCP, accepts a human goal, discovers repository facts, obtains only necessary human meaning, emits separate slice artifacts, receives exact approval or in-remit Captain authorization, implements the slices, obtains a fresh read-only Verifier PASS, and deterministically merges only the covered candidate."
            },
            {
              "id": "A3",
              "text": "A second real run starting from supported Baton-era history resumes through the same Sworn-native owners without installing Baton or rewriting history and ends with new Sworn-owned authority and truthful provenance."
            },
            {
              "id": "A4",
              "text": "Crash cuts around human parking and answering, Captain decisions, candidate publication, verification, direct repair, target advance, and Merge recover to exactly one authorized continuation and final mutation with no duplicate verdict, effect, receipt, commit, or ref movement."
            },
            {
              "id": "A5",
              "text": "The exact final assembly passes the product, serial E2E, race, vet, diff, legacy-compatibility, and fresh semantic system gates, recording the assembly commit and tree, binary digest, plan and slice identities, predecessor products, runtime schema, event cursor, and final target equality."
            },
            {
              "id": "A6",
              "text": "Sworn's conformance suite proves observable behavioral parity across installed skill to MCP, direct MCP, TUI, CLI, and configured driver paths, including rejection and recovery cases; prose identity, a tool-list snapshot, schema parsing, unit tests, or command exit status alone cannot certify the product contract."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false go test -count=1 ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go test -count=1 -parallel=1 -timeout=60m ./test/e2e",
            "GOFLAGS=-buildvcs=false go test -count=1 -race ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go vet ./...",
            "git diff --check"
          ],
          "constraints": [
            "Evidence is hierarchical and fail-closed: slice E2E, then cumulative track E2E, then fresh release E2E.",
            "Every consumer uses the exact passed predecessor product through its production contract; fixtures, copied types, or test-only adapters cannot substitute.",
            "The independent final Verifier starts fresh, remains read-only, and verifies the exact immutable assembly rather than prior conversational claims.",
            "Conformance belongs to Sworn's public behavior and adapter boundaries; it is not a second implementation of a separately published prose protocol.",
            "This release proves the dependable serial kernel; it cannot claim the later parallel autonomous-delivery outcome."
          ],
          "depends_on": [
            "S3-semantic-loop"
          ],
          "consumes": [
            "S3-semantic-loop"
          ]
        }
      ]
    }
  ]
}
```

# Goal

Make Sworn the single owner of its delivery contract and prove that its complete
serial loop works. Baton becomes read-only design history and legacy provenance,
not a separately installed, versioned, certified, or conformed product.

# Authority

The human operator controls
`operator://2026-08-07-sworn-native-delivery-kernel/1`. Approval must bind these
exact revision-1 bytes and the then-current `refs/heads/main` target. Planning
does not approve itself. Captain delegation may act only after an exact proposal
exists and only within its separately recorded envelope.

# Scope

This release absorbs the valuable Baton trust rules into Sworn, replaces the
standalone Baton role skills with one thin Sworn-to-binary/MCP adapter,
introduces a Sworn-native manifest with separate immutable slice contracts and
evidence bundles, restores useful semantic planning, completes the serial
headless MCP lifecycle, and proves the resulting kernel through fresh real-agent
system journeys. It retires external Baton packaging and conformance ceremony.
It deliberately does not claim the later streaming, worker-survival, parallel
scheduling, caching, expanded OTel, evaluation, adjustment, or macOS-on-ramp
outcomes.

# Acceptance

S1 proves that Sworn owns one trustworthy authority path, needs no external
Baton product, and is entered through one thin Sworn skill rather than five
standalone role protocols. S2 proves independently digest-bound slice artifacts
and correct parallel-touchpoint declarations. S3 proves repository-aware
semantic planning, durable human-only meaning, useful decomposition,
Implementer design, Captain review/delegation, and TUI/CLI/MCP command parity.
S4 proves the exact composition from real entry point to deterministic merge,
including legacy history and crash recovery.

# Ordered tracks and slices

One serial bootstrap track is intentional. Each slice changes the contract
consumed by the next, so parallel implementation would either duplicate owners
or let consumers build against an unpassed authority format. S1 establishes
native ownership, S2 establishes the artifact contract, S3 consumes both for the
semantic loop, and S4 verifies their exact composition. The next autonomous
release uses S2's touchpoint and dependency model to run independent tracks in
parallel.

# Dependencies and inputs

The target baseline is the merged `2026-08-04-provider-neutral-authorization`
product at `refs/heads/main`. S1 also consumes the final Baton handover knowledge
from tag `v1.0.0-rc.15.3`, corrected touchpoint commit
`66b81bdb0e6e8f3a77dd41ca800df62d19470f53`, and shelving decision commit
`f3c479f5bd336478747b185dea79634ffa446eb2` as design inputs, not runtime
dependencies. The prior autonomous-loop requirements remain preserved at Git
blob `892523ba1d7c1f6fece7ea60ecc98874c596459b`. The superseded
`2026-08-05-baton-rc15-conformance` history and its evidence remain immutable;
only its useful unmerged source work may be rebased and freshly proven.

# Checks

Every implementation slice runs the product, serial E2E, race, vet, and diff
gates listed in its contract. Slice PASS additionally requires a real-built-
binary E2E scenario for that slice's named outcome. Before S4, the exact T1
composition must pass a cumulative real-binary journey. S4 then builds once from
the exact immutable assembly and performs fresh clean-project, legacy-project,
semantic, authority, recovery, and merge journeys against that binary.

# Constraints

Preserve one owner for every authority, state, driver, journal, recovery,
workspace, Git, approval, presentation, and merge seam. Reuse or extend existing
functions, types, schemas, fields, components, styles, and tests; a new or
overlapping owner requires exact Captain justification. Do not bring across
Baton's separate publication machinery, byte-identical package gating,
generated-payload release ceremony, or semantic word budgets. Historical
provenance and published schema bytes remain truthful and stable.
