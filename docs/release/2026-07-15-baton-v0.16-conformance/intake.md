---
title: 'Release intake: Baton v0.16 conformance'
description: 'Planning record that preserves the in-flight Baton v0.15.1 bootstrap and adds exact Baton v0.16.0 parity and portable board-oracle conformance.'
---

# Release Intake: `2026-07-15-baton-v0.16-conformance`

## Release goal

Preserve the in-flight Baton `v0.15.1` bootstrap records truthfully, then adopt
the exact Baton `v0.16.0` protocol delta before later release-mode consumers
depend on its board projection. Shipped means an operator can drive Sworn's
public release commands through the bounded Implementer,
fresh-Verifier, Coach, Track Integrator, re-plan, merge, release and ship paths;
the same committed semantic inputs produce the same identity and reusable
evidence; malformed, stale or rewritten evidence fails closed; deterministic
rollback and shared-file composition protect integration; and the installed
Codex and Claude Baton mirrors report the same pinned protocol as the binary.

## Needs

- N-01: **Immutable bootstrap parity.** Sworn preserves the exact Baton
  `v0.15.1` content and lifecycle already started by the bootstrap slices,
  including their normative JSON bytes and supported local-install obligations.
- N-02: **Typed planning and report contracts.** Sworn preserves and validates
  typed `spec.references`, `board.shared_touchpoints`, the dedicated
  `spec-ambiguity-report-v1` contract, the required generic check identity, and
  the complete `slice-status-v1.maintainability` record.
- N-03: **Canonical semantic identity.** One reusable operation constructs the
  Baton semantic path set, manifest, fingerprint and normalized prompt diff from
  immutable commits, modes and Git object identities with all canonical
  exclusions and provenance checks.
- N-04: **Durable report authority.** Full reports and compact status ledgers
  remain append-only, pin committed report paths/blob OIDs, validate their
  identity and history, and fail closed after deletion, rewriting or mismatch.
- N-05: **Bounded role lifecycle.** Implementer, fresh Verifier and Coach
  adapters enforce Baton's phase order, role authority, dispatch budget,
  explicit human adjudication inputs, committed handoffs, single cycle-1 resume
  within immutable permitted touchpoints and mandatory re-slicing outcome.
- N-06: **Deterministic integration evidence.** Track scope and freshness compose
  active authoritative intervals and rollback-backed retired ownership without
  treating historical ownership as PASS evidence or losing disjoint sibling
  work.
- N-07: **Rollback-backed recovery.** Ordinary and post-sync rollback paths
  compare the correct full-through-rollback-head or post-sync candidate envelope
  to its exact committed baseline; unexpected later ordinary history requires
  reconstruction rather than a silently preserved carve-out.
- N-08: **Configuration-independent shared-file composition.** Declared shared
  touchpoints are validated from `board.json` and composed from committed blobs
  identically regardless of merge drivers, attributes, filters or local config.
- N-09: **One integration-ready gate.** Re-plan, track merge, release merge and
  ship adapters call shared lifecycle, provenance, rollback and readiness
  validators on every success and idempotent path, distinguishing valid
  unstarted deferral and rollback-backed terminal deferral from raw `deferred`.
- N-10: **Restart-safe public proof.** Integration tests exercise Sworn's public
  entry points, verdict/exit behavior, durable commits, exact dispatch counts,
  idempotency and recovery after process restart.
- N-11: **Safe active-plan migration.** Sworn provides a Planner-governed
  migration operation and proves the exact transformation against the pristine
  `2026-07-15-local-first-account-safety` records without fabricating historical
  evidence. After this conformance release is integrated, a fresh Planner
  session applies that migration and reruns the v0.15 trace and spec-ambiguity
  gates before any downstream implementation begins.
- N-12: **OpenAI strict-envelope compatibility.** Generic LLM checks retain
  exact Baton report semantics when only the two explicit OpenAI strict-output
  transports need a deterministic provider envelope; canonical schemas,
  prompts, local validation, emitted-check equality, and all other provider
  paths remain unchanged and fail closed.
- N-13: **Direct OpenRouter tool-call compatibility.** A release operator can
  use the explicitly selected `openrouter/z-ai/glm-5.2` model for a generic
  structured check only through a direct, forced-function transport that keeps
  the canonical Baton schema and local semantic gate authoritative. Sworn's
  hosted proxy, Ollama, and every unprofiled provider remain default-deny.
- N-14: **Sanitized bounded proof-receipt recovery.** A release operator can
  retain only a strict metadata-only proof receipt, reserve it atomically
  before dispatch, and make at most one classified recovery attempt without
  exposing provider data, broadening ordinary retry behavior, or weakening
  canonical report validation and the independent S20 gate.
- N-15: **Exact v0.16 protocol and installer parity.** After the immutable
  v0.15.1 bootstrap reaches a lawful boundary, Sworn vendors exact Baton
  `v0.16.0` content, including its board-oracle schema, and proves its native
  Codex and Claude installations match the tagged installer inputs without
  mutating a real user home.
- N-16: **Portable board-oracle projection.** Sworn emits and validates the
  Baton `board-oracle-v1` aggregate and named-release compatibility shapes,
  derives topology from Git rather than optional convenience fields, and fails
  closed before a malformed projection or unsafe `sourceRef` can drive a
  mutable release operation.

## Source of truth

- **Human stakeholder**: repository owner / Coach
- **Tracking issue / epic**: [sworn#122](https://github.com/swornagent/sworn/issues/122)
- **Normative upstream release for the v0.16 tail**: [Baton v0.16.0](https://github.com/sawy3r/baton/releases/tag/v0.16.0)
- **Normative upstream peeled tag commit for the v0.16 tail**: `aae82d1cb8c28085ab20668c720f0282048dcc09`
- **Normative bootstrap release retained by S20**: [Baton v0.15.1](https://github.com/sawy3r/baton/releases/tag/v0.15.1), peeled commit `3fb4d275ae8a151f6287e7b9279d71628b12eea0`, source archive SHA-256 `8f0839ea897374eb10d6db2a789939714727739621babef1117d74cbf4488d2f`
- **Authoritative handoff**: [sworn#122 final v0.15 handoff](https://github.com/swornagent/sworn/issues/122#issuecomment-4978801054)
- **Upstream design history**: [Baton PR #76](https://github.com/sawy3r/baton/pull/76) and [Baton issue #75](https://github.com/sawy3r/baton/issues/75)
- **Downstream dependent plan**: `docs/release/2026-07-15-local-first-account-safety/`
- **Related memory entries consulted**: none committed; live repository and tagged upstream records are authoritative.

## Users and their gestures

- **Sworn release operator**: starts or resumes a planned slice through the
  public Sworn commands and receives only Baton-valid transitions, bounded model
  calls, durable handoffs and fail-closed exits.
- **Implementer**: reaches one stable-diff maintainability preflight, performs at
  most one remediation and closure review in the active cycle, commits the
  resulting handoff, and never certifies the authoritative verdict.
- **Fresh Verifier**: performs exactly one final read-only maintainability gate
  over the pinned semantic identity and cannot repair or rerun its own result.
- **Coach**: adjudicates a cycle-0 authoritative failure as either the single
  `resume_in_scope` cycle or `re_slice`, explicitly supplying rationale,
  identity and resume-only permitted touchpoints; a later failure has no waiver
  path.
- **Track Integrator**: merges a track only when committed scope, report history,
  freshness, rollback, shared-file composition and integration readiness all
  pass the same canonical validators; the sole recoverable post-sync
  invalidation is committed locally without push and routed to re-planning.
- **Planner/re-planner**: preserves v0.15.1 bootstrap records from their exact
  owner track, then appends v0.16.0 tail work without resetting failed or
  started history; maintainability remains an opaque authority object.
- **Protocol maintainer**: vendors the exact upstream tag, proves byte parity,
  refreshes local Codex/Claude installs and sees the same version through
  `sworn doctor` and parity commands.

## What's currently broken or missing

- Sworn is pinned to Baton `v0.13.1`; v0.14 typed references and ambiguity
  reporting and v0.15 maintainability/integration contracts are not adopted.
- `RunLLMCheck` assumes the earlier generic report shape, and generic reports do
  not consistently carry the required check identity.
- The current spec record reader ignores typed references, so normative inputs
  cannot be resolved and confined as Baton requires.
- Board and status readers/writers drop `shared_touchpoints` and
  `maintainability`, which would silently erase protocol authority.
- The current schema compiler cannot compile a board-v1 expression that uses a
  negative lookahead unsupported by Go's regular-expression engine.
- The vendoring transform can mistake prose containing
  `board.json.shared_touchpoints` for a shell source path, so upstream sync is
  not yet atomic or safely path-aware.
- No shared Sworn operations yet construct the canonical semantic manifest,
  committed report ledger, lifecycle-history FSM, evidence intervals, rollback
  equality, shared-blob composition or integration-ready predicate.
- Existing command adapters do not enforce the v0.15 retry/authority budget or
  re-gate idempotent merge/release/ship paths.
- The embedded protocol and public board projection do not yet carry the
  v0.16.0 `board-oracle-v1` schema or its reusable portable topology validator.
- The repository contains historical closed slice records without v0.15
  maintainability evidence and active planned records that need a legitimate
  Planner migration before implementation.

## Constraints and non-negotiables

- Each started v0.15.1 bootstrap record remains governed solely by Baton
  `v0.15.1`; the new S23/S24 tail is governed solely by exact Baton `v0.16.0`.
  No record is reinterpreted across that boundary, and earlier issue comments
  are historical context only where wording differs from the relevant tag.
- Planning and implementation target `release/v0.2.0`; `main` remains production.
- This Planner session writes only this release's artefacts. It does not modify
  production code, tests, other releases or local Baton installations.
- Sworn remains one native Go binary with no required external runtime and no
  new third-party dependency without an accepted ADR.
- Every construction, validation, dispatch, history, provenance, blob or
  integration ambiguity fails closed with non-zero exit behavior.
- The same semantic bytes/modes/object identities within one role session reuse
  evidence without another model call; every semantic change changes identity.
- Release records, generated output and lockfile-only changes are excluded only
  by Baton's canonical rules, never by broad local heuristics.
- `start_commit`, pinned implementation/report identities, report prefixes,
  terminal re-slice history and Coach adjudication retain Baton's immutability.
- An Implementer never certifies its own authoritative result; the final
  Verifier remains fresh-context and read-only.
- Public artefacts contain only public-safe technical requirements. No private
  strategy, pricing, customer or provider-negotiation material enters this repo.
- Upgrade and install operations never report success with partial writes.
  Recoverable failure restores the exact starting snapshot; an incomplete
  rollback is explicit, preserves recovery material and blocks later write-mode
  success until restoration.
- Integration tests must enter through Sworn's public command dispatch and pair
  protocol outcome with exit behavior; leaf-only tests are insufficient proof.

## Adjacent / out of scope

- **Local-first account safety implementation**: remains planned in
  `2026-07-15-local-first-account-safety` and begins only after this conformance
  release is integrated. **Why deferred**: implementing it against v0.13 records
  would immediately create invalid lifecycle evidence. **Tracking**: sworn#121
  and its release board. **Acknowledged**: Coach, 2026-07-15, by approving this
  prerequisite release.
- **Autonomous loop, web board and notification delivery implementation**:
  remains owned by `2026-07-14-autonomous-operations`. **Why deferred**: this
  release supplies protocol correctness and gates, not the operations UI or
  durable event/outbox product. **Tracking**: sworn#109 and its release board.
  **Acknowledged**: Coach, 2026-07-15, through the ratified sequencing.
- **New Baton protocol design**: Sworn will not reinterpret or simplify the
  adopted Baton contracts. **Why deferred**: protocol changes belong upstream
  and this release is a
  downstream conformance implementation. **Tracking**: Baton issues/PRs for any
  newly discovered defect. **Acknowledged**: Coach, 2026-07-15, by accepting the
  tagged release as normative.
- **Commercial or hosted-control-plane strategy**: no business or private
  strategy is part of this public technical release. **Why deferred**: it is a
  separate private decision surface and is not required for protocol correctness.
  **Tracking**: outside this public repository. **Acknowledged**: Coach,
  2026-07-15, under the repository's public-doc discipline.

## Decisions made during planning

### 2026-07-15 — Ratify a dedicated v0.15 conformance prerequisite

- **Context**: The local-first account-safety plan validates structurally only
  after v0.15, while current Sworn code and installed Baton mirrors are v0.13.1.
- **Options considered**: implement trust-safety work first; fold the upgrade
  into its first slice; plan a dedicated conformance prerequisite.
- **Decision**: plan and deliver `2026-07-15-baton-v0.16-conformance` first.
- **Why**: v0.15 changes executable record, lifecycle, evidence and integration
  contracts; treating it as a content-only bump would make every downstream
  proof untrustworthy.

### 2026-07-15 — Version-gate historical records as a read-only archive

- **Context**: The repository contains pre-v0.15 records that cannot truthfully
  satisfy v0.15's maintainability ledger, report identity and authoritative PASS
  requirements because those artefacts did not exist when the work was done.
- **Options considered**: preserve historical records under a pinned original
  protocol and require migration before execution; bulk-rewrite every record;
  accept missing maintainability data through a permissive legacy shim.
- **Decision**: Historical records remain immutable and read-only under their
  deterministically resolved original protocol version. Any planning record that
  enters a mutating, verification, integration, release or shipping workflow
  must first receive an explicit Planner migration to v0.15 and pass the v0.15
  gates. An unresolvable version fails closed rather than selecting a default.
- **Why**: This preserves the audit record without inventing PASS reports,
  invocation identities, fingerprints or blob OIDs, while keeping every live
  success path strictly conformant instead of embedding an ambiguous bypass.
- **Implementation obligation**: Protocol selection must be derived from
  committed repository evidence, not folder names, displayed state or a loose
  "legacy" heuristic. Original-version validation grants archival inspection
  only; it never grants current mutation or integration authority.

### 2026-07-15 — Give maintainability one dedicated public command authority

- **Context**: Sworn currently exposes `maintainability-review` through the
  generic `llm-check` command, but v0.15 makes it a stateful operation that owns
  phase order, role authority, dispatch budgets, semantic identity, committed
  reports and lifecycle transitions.
- **Options considered**: add a dedicated `maintainability` command namespace;
  extend generic `llm-check` with lifecycle flags; keep the operation internal
  and require another tool or direct record editing for Coach decisions.
- **Decision**: Make `sworn maintainability review` the sole public review
  operation and `sworn maintainability adjudicate` the sole public Coach
  transition. The review command accepts only Baton-valid role/phase pairs:
  Implementer `preflight` or `closure`, and Verifier `authoritative`. The
  adjudication command accepts only the exact Baton decisions
  `resume_in_scope` or `re_slice`. The Coach supplies decision, non-empty
  rationale and identity plus resume-only permitted touchpoints; committed
  evidence derives cycle, reports, semantic scope, invocation/finding identities
  and current authority, while the command captures the timestamp once. User
  flags cannot assert or override derived authority.
- **Why**: A dedicated stateful namespace gives public reachability and recovery
  without mixing a durable protocol FSM into one-shot quality checks or leaving
  a second executable authority path.
- **Compatibility obligation**: Retire generic
  `sworn llm-check --type maintainability-review` as an executable path. If the
  old spelling is recognized at all, it exits non-zero and points to the
  dedicated command; it never dispatches a model or mutates lifecycle state.
- **Adapter obligation**: `sworn loop`, verifier orchestration, routing and every
  integration adapter call the same internal maintainability authority. No role
  or merge command may reconstruct the lifecycle independently.

### 2026-07-15 — Separate deterministic scope, ledger and integration authorities

- **Context**: v0.15 combines Git semantic identity, report/history lifecycle
  authority and integration/rollback composition. Current Sworn success paths
  distribute related logic across `gate`, `state`, `board`, `router`, `run`, CLI
  merge and MCP merge adapters.
- **Options considered**: focused domain packages with one-way dependencies; one
  broad maintainability package; extend the existing packages with local helper
  functions.
- **Decision**: Create `internal/maintainability/scope` for authored-path
  discovery, canonical exclusions, normalized diff, manifest and fingerprint;
  create `internal/maintainability/ledger` for full-report/blob identity,
  append-only committed history, role budgets and Coach adjudication; and create
  `internal/integration` for shared-touchpoint validation/composition, sync
  provenance, active and retired evidence intervals, rollback equality and the
  integration-ready predicate. A thin root `internal/maintainability`
  coordinator may compose those authorities and dispatch through the canonical
  driver interface, but owns no duplicate Git or FSM rules.
- **Why**: These boundaries match Baton's independent authorities, support
  focused adversarial tests and a one-way dependency graph, and stop CLI, loop
  and MCP adapters from growing divergent local success predicates.
- **Carrier obligation**: Existing `internal/state`, `internal/board` and
  `internal/spec` packages become lossless record carriers. They preserve every
  v0.15 field but do not infer lifecycle transitions. `internal/git` gains only
  typed raw-byte/object plumbing; Baton semantics remain in the domain packages.
- **Dependency obligation**: Low-level record/Git packages feed scope and
  ledger; integration may consume their validated results; the thin coordinator
  may consume integration; public adapters consume the coordinator and shared
  validators. Reverse imports and adapter-specific reimplementations are
  prohibited.

### 2026-07-15 — Prove live and historical protocol authority from committed evidence

- **Context**: Five existing releases contain status records authored under
  more than one Baton version. Release name, planning date, displayed state,
  missing fields and the running binary's current pin therefore cannot establish
  the protocol that governed an individual historical record.
- **Options considered**: exact live marker plus per-record historical Git
  evidence; one release-name/version registry; inference from record shape or
  the running binary.
- **Decision**: Every live or migrated release carries a Sworn-owned committed
  `docs/release/<release>/protocol.json` record that pins the protocol name,
  exact version, upstream commit SHA, source digest and upstream root `VERSION`
  blob OID. Before any dispatch, write, verification, integration, release or
  ship operation, the marker must be committed and identical on every
  participating authority ref. Separately, each ref's committed
  `internal/adopt/baton/VERSION` manifest blob must agree across participants,
  and its parsed tag/SHA/digest must equal the marker and running binary.
- **Historical rule**: For read-only archive inspection, resolve each requested
  record's blob at the authority ref and walk first-parent history newest to
  oldest while that path has the exact current blob. Select the oldest commit in
  that uninterrupted equal-blob suffix, so deletion/reintroduction chooses the
  most recent introduction, and read `internal/adopt/baton/VERSION` from that
  exact tree. Return record, evidence commit, version path/blob and pin as
  inspection metadata. This evidence never authorizes a current operation.
- **Why**: The two-layer scheme gives live work an explicit immutable authority
  and preserves the truth of mixed-era history without a manually curated
  mapping or a structural legacy heuristic.
- **Fail-closed obligation**: Missing, dirty-only, deleted, divergent,
  unsupported, malformed or binary-mismatched live markers block before side
  effects. Missing objects, shallow history, conflicting version files or an
  otherwise unresolvable archive record fail structured inspection; an explicit
  raw mode may emit exact committed bytes but cannot claim validation.
- **Read-only obligation**: Archive inspection must not call current record
  writers or lazily create `board.json`. The working tree and refs are
  byte-identical before and after inspection.

### 2026-07-15 — Make the stateful command own the complete durability boundary

- **Context**: Baton requires maintainability failures and Coach decisions to be
  durable committed handoffs. A valid result left in a dirty worktree or a
  machine-local commit can disappear across the exact session/machine boundary
  the track branch is designed to survive.
- **Options considered**: command writes, commits and pushes atomically; command
  commits and the role session pushes; command returns data and the role session
  authors all records and Git operations.
- **Decision**: `sworn maintainability review` and
  `sworn maintainability adjudicate` resolve the authoritative owner track,
  require a clean starting tree/index, validate the current committed lifecycle,
  write the full report, compact status ledger and journal transition as one
  coherent change, commit it, and push the exact track ref before reporting
  protocol success. A valid maintainability FAIL is also durably committed and
  pushed before its FAIL exit is returned.
- **Why**: The command that owns the transition is the only layer able to make
  record identity, lifecycle mutation and Git durability one restartable
  operation instead of a multi-actor convention.
- **Recovery obligation**: If the report and transition commit succeed but the
  push fails, a rerun validates and reuses that exact committed report, performs
  no model dispatch and completes the missing push. Rewritten or partial local
  evidence fails closed rather than being repaired heuristically.
- **Authority obligation**: Stateful operations expose no `--dry-run`,
  `--no-push`, `--force`, waiver or lifecycle-override flag. Caller-controlled
  flags cannot change phase, cycle, head, invocation identity, fingerprint,
  findings, timestamp or authority ref.

### 2026-07-15 — Migrate only pristine planned records in place

- **Context**: Some pre-v0.15 releases are complete historical evidence, while
  `2026-07-15-local-first-account-safety` contains seven untouched planned
  slices with null starts, no implementation artefacts, no maintainability
  reports and pending verification.
- **Options considered**: allow in-place migration only before implementation
  begins; migrate every unshipped record; archive every pre-v0.15 plan and
  recreate even untouched work under new release/slice IDs.
- **Decision**: An in-place Planner migration is legal only when every affected
  slice is `planned`, has `start_commit: null`, empty actual files and reports,
  null adjudication and pending verification. The migration preserves the source
  authority commit and record blob identities in a committed receipt, adds the
  exact v0.15 pending lifecycle and typed references, writes the live protocol
  marker, validates every output, and reruns the v0.15 trace and ambiguity gates
  before implementation may start.
- **Why**: The boundary coincides with the first execution evidence. Untouched
  intent can be translated truthfully; once implementation starts, retroactive
  lifecycle evidence would be invented rather than migrated.
- **Started-history obligation**: Any started, implemented, verified, deferred
  or shipped pre-v0.15 slice remains an immutable archive record. Continuing or
  replacing its functionality requires a new v0.15 slice ID under a live marked
  plan; the old record is never reset, narrowed or assigned synthetic reports.
- **Confirmed eligibility**: All seven slices in
  `2026-07-15-local-first-account-safety` satisfy the pristine-planned
  preconditions and are the first required downstream migration after this
  conformance release is integrated.

### 2026-07-15 — Separate pre-cutover readiness from post-deployment truth

- **Context**: Current `sworn ship` validates human journey attestations and
  reports whether cutover may proceed; it does not deploy or persist a `shipped`
  transition. Baton v0.15 separately requires the post-deployment state change
  to revalidate lifecycle, report, provenance, rollback and readiness evidence.
- **Options considered**: preserve `ship` and add `mark-shipped`; make `ship`
  both validate and transition; leave the shipped transition to an external
  role/skill without a deterministic binary entry point.
- **Decision**: Keep `sworn ship <release>` as the pre-cutover gate and prepend
  the canonical v0.15 lifecycle, provenance, rollback and integration-ready
  validation to its existing journey checks. Add
  `sworn mark-shipped <release>` as the explicit post-deployment operation that
  revalidates the same committed authority and atomically transitions eligible
  `verified` slices to `shipped`. The native adapter accepts an optional deployed
  commit and deploy note, requires the clean primary integration worktree,
  fetches origin and blocks if local is behind/divergent, revalidates terminal
  authority, and first determines whether any `verified` slice remains. With no
  eligible slice it returns Baton's exact no-evidence no-op. Otherwise the
  deployed commit is mandatory: an existing conventional release-merge identity
  must be contained, while an older release with no such identity follows
  Baton's recorded legacy containment-skip path. The transition writes exact
  last-updated metadata plus the Baton `ship` block,
  preserves the pure-plan board byte-identically, derives state/count/activity
  views from statuses into the rendered index, and commits only those status/index paths once
  locally. It never pushes; success returns Baton's push-and-cleanup handoff.
  Neither command deploys code or executes cleanup.
- **Why**: "safe to deploy" and "deployed" are different facts. Separate
  operations prevent a successful pre-cutover check from becoming false
  production history while giving the terminal transition a public proof and
  recovery surface.
- **Idempotency obligation**: Re-running `ship` revalidates every gate.
  Re-running `mark-shipped` after no `verified` slice remains still performs
  Baton's primary-worktree, origin freshness, schema, lifecycle, provenance,
  rollback and terminal-state gates, then returns the exact successful
  nothing-to-do result without requiring or resolving deployment evidence,
  release-merge identity or a timestamp and without rewriting or re-handing-off
  existing `ship` records. There is no push-only recovery.

### 2026-07-15 — Keep re-plan judgment human-led and make application engine-backed

- **Context**: Requirements, deferrals and replacement boundaries require
  Planner/Coach judgment, while v0.15 re-plan mechanics require exact owner-track
  status seeding, immutable lifecycle preservation, narrowly authorized rollback
  linkage and safe propagation. Current role and MCP paths can edit records
  without one shared mutation authority.
- **Options considered**: conversational Planner plus deterministic engine
  application; keep every mutation in the role session; move requirements
  decomposition and re-planning judgment into the binary.
- **Decision**: The human and conversational Planner continue to decide the
  meaning of a revised plan, including slice boundaries, deferrals, rollback and
  replacement work. A shared engine operation applies only a ratified plan
  delta: it resolves the exact owner track/ref and status blob, validates schema,
  report identity and committed lifecycle history, preserves the seeded
  `maintainability` object byte-for-byte except for an explicitly ratified
  `rollback_slice_id`, commits release records atomically, and propagates them to
  affected track refs under Baton's conflict rules.
- **Why**: This keeps qualitative product/spec judgment in the proper human
  planning boundary while making it impossible for CLI, skill or MCP adapters
  to corrupt or bypass protocol authority during application.
- **Adapter obligation**: Planner/CLI and MCP entry points call the same engine
  operation. Direct MCP `plan_release`, `create_slice`, `set_track`, and
  `update_intake` authoritative writes are retired with Planner/delta guidance;
  replacement delta tools call the identical engine operations. The binary
  validates and applies a decision but never invents decomposition, deferral
  rationale or Coach approval.
- **Recovery obligation**: Every application records its source ref/object IDs
  and committed target transaction so restart can distinguish not-started,
  locally committed and fully propagated outcomes without replaying Planner
  judgment or narrowing lifecycle history.

### 2026-07-15 — Bootstrap conformance in stages under the exact tagged protocol

- **Context**: Sworn cannot use a v0.15 engine operation to implement the first
  pieces of that same engine. Building everything outside the release would put
  code before its slice contracts; running early slices under v0.13 and
  migrating them later would violate the pristine-only migration boundary.
- **Options considered**: staged v0.15 self-bootstrap; out-of-band monolithic
  engine implementation followed by retrospective planning; early v0.13 work
  followed by in-place migration of started records.
- **Decision**: Author every release record against the exact v0.15 contract
  from planning onward. The first delivery activity repairs the atomic vendor
  boundary, then vendors the exact v0.15 tag and refreshes both supported local
  Baton installations from that pin. Until Sworn's new engine authorities are
  reachable, early slices follow the tagged v0.15 role prompts manually, use
  fresh Implementer/Verifier contexts, and commit schema-valid v0.15 reports and
  lifecycle transitions without claiming engine automation.
- **Cutover rule**: The engine-cutover slice must run the new binary against all
  earlier release records, reports, Git scopes and transitions. Any mismatch is
  a blocking defect repaired under the original slice authority; no report is
  synthesized or grandfathered. Only after that independent revalidation passes
  do later role and adapter slices use the automated stateful operations as
  their execution authority.
- **Why**: Staged self-bootstrap preserves specification-first delivery and the
  exact new protocol while acknowledging the temporary absence of its reference
  implementation. The cutover turns manual evidence into engine-validated
  evidence rather than retrospectively inventing it.
- **Install obligation**: Local Codex and Claude mirrors are updated only after
  exact vendored source parity is established. Canonical managed outputs are
  generated by the exact pinned upstream installers in empty isolated homes,
  including all Codex wrapper/frontmatter transformations. `sworn doctor
  --sync-baton` uses those canonical outputs plus Sworn VERSION sentinels to
  repair both installations as one rollback-protected transaction.

### 2026-07-16 — Close the per-slice Gate-8 bootstrap gap

- **Trigger**: S01's first genuinely fresh v0.15.1 verifier passed deterministic
  Gates 1–7 and then returned BLOCKED because `maintainability` remained pending
  with a null `implementation_head`. The earlier planning decision correctly
  prohibited fabricated evidence but incorrectly treated S13 revalidation as a
  substitute for each slice's required Implementer and Verifier reports.
- **Options considered**: keep the S13 deferral as an informal waiver; move the
  complete S06–S13 engine ahead of S01; change Baton upstream to add a bootstrap
  exception; or use a bounded planning-authority adapter to execute the exact
  tagged operation and persist real reports while leaving generalized automation
  and cutover with S06–S13.
- **Decision**: Use the bounded adapter. Invocation spelling is non-normative,
  but semantic scope, fingerprint, prompt bytes, output schema, role isolation,
  committed report blob, ledger identity and fail-closed behavior are the exact
  Baton v0.15.1 contracts. S01 adds a fifth AC for its two reports; the same
  adapter governs S02–S13 until cutover.
- **Why**: An informal waiver violates the tagged protocol, and moving the whole
  engine forward destroys the release decomposition. The bounded adapter creates
  the missing evidence without claiming the public command, generalized merge
  handling, reuse, adjudication, rollback or activation behavior that later
  slices still own.
- **Authority**: Brad's standing instruction was to continue with the
  orchestrator's recommendation. The orchestrator recommends this bounded exact
  adapter after the independent verifier and a separate read-only planning agent
  both confirmed the circular dependency and rejected synthesized evidence.

### 2026-07-15 — Decompose conformance into 18 proof-bounded slices

- **Context**: v0.15 adoption spans atomic vendoring, exact installed parity,
  lossless record carriers, ambiguity checking, protocol provenance, raw Git
  plumbing, canonical semantic scope, report identity, lifecycle history,
  deterministic integration, role recovery, release adapters, re-plan and
  public conformance proof.
- **Options considered**: 18 proof-bounded slices; approximately 12 consolidated
  slices; approximately 22 micro-slices.
- **Decision**: Use the 18 slices below, each with one operator or maintainer
  outcome and one independent proof boundary. Keep every slice under the
  15–25-file ceiling except the ratified 42-file S02 parity/install/doctor
  transaction: the normative mapped surface, complete offline installer input,
  and its one repair/recovery authority are indivisible.
- **Why**: The decomposition separates deterministic primitives from lifecycle,
  integration and adapter authorities without inventing coordination seams that
  have no independently verifiable outcome. It also preserves the engine
  cutover as an explicit boundary between manually governed bootstrap evidence
  and automated v0.15 authority.

### 2026-07-16 — Regroup the release into five dependency-safe tracks

- **Context**: The original seven-track draft left S03 and S06–S12 verifiable
  only through leaf tests because their public adapters landed later, and S07
  consumed S10's C-07 authority across a track boundary. The 18 slices contain
  one genuine delivery parallel seam only after engine cutover.
- **Options considered**: retain seven tracks and add public diagnostic
  commands; add scaffold slices; merge contract owners; serialize the engine
  bootstrap behind the already-planned maintainability command.
- **Decision**: Use five tracks without adding or merging slices.
  `T1-foundation` owns S01–S05;
  `T2-engine-bootstrap` owns S06, S10, S07, S08, S09, S11, S12, then S13 and
  depends on T1;
  `T5-role-loop` owns S14 and depends on T2;
  `T6-release-adapters` owns S15–S17 and depends on T2; and
  `T7-conformance-proof` owns S18 and depends on T5 and T6.
- **Why**: S06 introduces the final maintainability command as a fail-closed
  scaffold and each later T2 slice extends that real binary path before failing
  at the next unavailable gate. This satisfies Rule 1 without public API bloat,
  puts C-07 before C-04, preserves every contract owner and file ceiling, and
  reduces manual planning-authority integration to T1 then T2. S13 remains the
  explicit automation boundary and downstream parallelism remains intact.
- **Execution obligation**: `board.shared_touchpoints` is `{}`. Any newly
  discovered cross-track write collision blocks implementation and returns to
  re-planning rather than being accepted silently.

### 2026-07-16 — Close the fresh-context ambiguity batch and stage protocol activation after T2

- **Context**: Two independent v0.15 ambiguity reviews found that the slice
  outcomes were sound but several adapter-level outcomes were still implicit:
  vendor exits, parity ownership, typed-reference resolution, semantic manifest
  bytes, lifecycle rows, release-record synchronization, later-authority
  rollback, deployment evidence, re-plan wire records, crash recovery, and the
  one-way `planning` to `current` transition.
- **Options considered**: leave the tagged upstream prose implicit; copy exact
  decision tables into the owning specs and one referenced normative artifact;
  consolidate the behavior into larger implementation slices.
- **Decision**: Keep the 18 slices and add the exact decision tables and schemas
  under this release's `planning/` directory. Every affected spec references
  those files directly because typed references are non-recursive. Add C-13 as
  the explicit activation contract owned by S13.
- **Activation order**: The active bootstrap set implements and verifies under
  the ratified manual v0.15 adapter. After the Gate-3 amendment this means S01,
  S03 through S13, S19 and S20, while original S02 remains a C-09-valid
  rollback-backed terminal deferral. T1 then T2 merge canonically into
  `release-wt` while
  the marker remains `planning`. On that clean local/remote-equal release-wt, before
  any T5 or T6 ref exists, `sworn maintainability cutover <release>` revalidates
  every gate, changes only `protocol.json.authority` to `current`, commits and
  pushes once, and permits downstream track materialisation. Completed T1/T2
  track refs are historical evidence rather than current participants.
- **Why**: This lets the self-hosting S13 implementation receive an independent
  manual fresh-context verdict before its own engine becomes authoritative,
  avoids a marker conflict during T2 integration, and gives every automated
  downstream operation the same activated assembly base.
- **Recovery obligation**: Only an exact coherent commit may resume its missing
  push. Dirty partial evidence, divergent refs, alternate targets, force,
  downgrade, waiver, or auto-promotion fail unchanged with zero dispatch.
- **Ratification**: Selected under the Coach's 2026-07-16 instruction to proceed
  with the orchestrator's recommendation; the Type-1 choices are copied into
  S13, S16, and S17 machine-readable design records.

### 2026-07-16 — Remove self-reference and separate migrated activation

- **Context**: The second fresh ambiguity pass proved that a delta committed at
  its own declared source commit would be self-referential, that receipt ID/time
  were not deterministic, and that native C-13 could not authorize a migrated
  downstream release.
- **Decision**: Put every delta on the derived
  `plan/<release>/<delta-id>` ref whose parent is the separately pinned source;
  derive the receipt ID, Planner ref/commit and normalized ratification time;
  and split protocol migration into `replan migrate` (pristine source to
  migrated planning authority) followed by `replan activate` (receipt-bound
  planning to current). C-13 remains the sole native self-hosting cutover.
- **Fixture**: `planning/local-first-migration-manifest.json` is now a complete
  schema-valid 17-mutation delta containing every before OID, exact after byte
  payload and after OID, including the downstream contracts-registry consumer
  repair for migrated S04's C-02/C-04 references. Its seven status mutations
  also persist the Coach's completed requirements-validation ratification while
  leaving the existing scenarios and benefit hypotheses unchanged. The isolated
  proof places those exact bytes at the canonical downstream delta path; the
  live downstream release remains a tracked post-integration Planner action.
- **Why**: Source, judgment, transaction, receipt, and activation identities
  are now independently derivable from Git without circular data or one
  release borrowing another release's activation authority.
- **Ratification**: This is the fail-closed refinement of the already ratified
  S17 engine-backed, pristine-only migration choice under the Coach's
  2026-07-16 instruction to proceed with the orchestrator recommendation.

### 2026-07-16 — Close final conformance contradictions before implementation

- **Context**: Final independent reviews against exact Baton v0.15 found six
  adapter contradictions: moving Git heads were mixed into reusable report
  identity; the Baton report had no place for Sworn provenance/disposition;
  Coach inputs were partly inferred; ordinary rollback excluded later paths;
  recognized sync invalidation lacked a durable owner; and `mark-shipped`
  incorrectly pushed despite Baton's local-bookkeeping boundary.
- **Report decision**: Keep every upstream prompt/schema byte exact. Validate
  maintainability output first against Baton and then the referenced Sworn
  additive overlay. Blocking findings emit structured `remediate_in_scope` or
  `re_slice` disposition plus required touchpoints; the engine adds top-level
  scope provenance/freshness while closed Baton fields remain unchanged.
- **Identity decision**: Immutable release/slice/status/start/base/review-head
  manifest identity governs reuse. Track and release-wt heads are only a moving
  freshness frontier whose intervening history is revalidated.
- **Lifecycle decision**: Coach explicitly supplies decision, rationale,
  identity and resume-only permissions. Evidence derives report/finding
  identities; the command captures time. Cycle-1 structured re-slice or resumed
  path expansion transitions immediately with zero downstream dispatch.
- **Integration decision**: Ordinary rollback covers every authored path through
  the verified rollback implementation head and restores the original start
  tree. Eligible recognized-sync invalidation is committed locally by S15 with
  no push; later overlap, unrecognized provenance or shipped authority blocks
  with no mutation.
- **Shipping decision**: `mark-shipped` creates one local integration
  bookkeeping commit containing exact last-updated metadata, Baton `ship`
  blocks and the rendered index while preserving the pure-plan board
  byte-identically, then returns Baton's exact handoff.
  It never pushes, merges, builds, deploys or cleans; an already-shipped rerun
  passes the complete read-only upstream gate before the exact no-op.
- **Registry decision**: All live CLI operations and MCP tools receive an
  exhaustive pre-handler policy. Direct MCP planning writers are retired in
  favour of the shared delta engine.
- **Ratification**: These are the orchestrator's recommended fail-closed
  resolutions, ratified by the Coach's instruction to proceed.

### 2026-07-16 — Preserve exact upstream edge semantics

- **Context**: A final pinned-source review found that a Sworn-specific
  deployment object, dirty-byte-preserving loop restart, and ordinary dispatch
  on empty semantic scope each contradicted an explicit Baton v0.15 edge.
- **Shipping correction**: The native `mark-shipped` adapter now writes the
  exact upstream last-updated metadata and `ship` block, preserves the
  pure-plan board byte-identically, and re-renders the status-derived index.
  Every run fetches and checks origin and applies the complete upstream gate;
  only then is a run with no verified slices Baton's exact no-deployment-evidence
  no-op. A transition requires the deployed commit and containment only when a
  conventional release-merge identity exists; a legacy release with no such
  identity records Baton's exact containment-skip note.
- **Restart correction**: Standalone stateful commands still fail unchanged on
  ambiguous uncommitted evidence. A resumed loop has stronger ownership: it
  target-asserts the owner worktree, hard-resets to validated committed state,
  cleans untracked debris, proves cleanliness, then reconstructs the next legal
  phase without losing committed progress.
- **Empty-scope correction**: The exact header-only semantic scope produces a
  deterministic persisted PASS with zero Git-diff invocation and zero model
  dispatch, including stable restart identity.
- **Offline-input correction**: S02 embeds one byte-pinned complete installer
  input tar generated by exact `git archive`, rather than asking the mapped
  Sworn subset to regenerate files it does not contain. Doctor rollback failure
  now preserves snapshots plus a rollback-incomplete sentinel and admits only
  recovery runs until exact restoration.
- **Schema correction**: Raw model output uses its own committed disposition
  constraint which forbids engine provenance; the persisted overlay remains a
  separate post-injection schema. Both accept only exact 40- or 64-hex OIDs.
- **Transaction correction**: C-12 pins complete unsigned Git commit-object
  construction, timezone bytes and LF-terminated subject, with golden SHA-1
  identities for the 17-mutation local-first fixture.
- **Re-plan topology correction**: `ratified_at` must preserve a numeric RFC3339
  offset, the Planner delta path is never a transaction mutation, each original
  and rollback slice ID is unique across rollback links, and the receipt path
  differs from every mutation and canonical delta path. All four predicates
  fail before object creation or ref mutation.
- **Bootstrap/reachability correction**: Native operation policy applies only
  to Sworn handlers. T1 then T2 use the exact pinned manual Track Integrator
  transaction under planning authority, including deterministic projection and
  compare-and-swap push recovery. The serial T2 public-command scaffold makes
  S06–S12 independently reachable through built-binary tests before S13
  completes the coordinator and cutover.
- **Ratification**: Exact pinned Baton v0.15 behavior controls when an earlier
  adapter recommendation conflicts with upstream protocol bytes or role rules.

### 2026-07-16 — Close the final execution-boundary ambiguities

- **Context**: The last fresh-context plan review found four places where an
  implementer could satisfy one artefact while violating another: installer
  proof commands could touch live homes, absent frontier heads had no valid
  wire representation, a legal planned-intent deferral looked non-terminal to
  shipping, and S15's intentional local/remote split had no C-12 parent rule.
- **Installer proof decision**: The S02 board command invokes the dedicated Go
  parity test. That test owns the exact pinned-checkout assertion and creates
  empty isolated `HOME`, `CODEX_HOME`, `AGENTS_HOME`, and `CLAUDE_HOME` targets;
  no board proof command invokes an upstream installer against developer state.
- **Frontier decision**: Both validated frontier-head members remain required
  so absence is explicit, but each accepts either a full repository-format OID
  or `null`, matching the normative missing-identity rule.
- **Shipping decision**: A slice is release-terminal when it is verified,
  shipped, or satisfies exactly one of C-09's two legal deferral predicates.
  An unstarted planned-intent deferral therefore remains `planned` and is
  preserved rather than contradicting the terminal-state gate.
- **Re-plan parent decision**: Every C-12 target records separate expected local
  and remote heads. They must be equal except for the exact validated S15
  one-parent local invalidation, where local is that commit and remote is its
  parent. The propagation commit retains local as parent 1 and compare-and-swap
  advances each ref from its separately pinned pre-state, preserving both the
  invalidation history and fail-closed recovery.
- **Ratification**: These are mechanical consistency repairs within the
  already-ratified fail-closed architecture; no slice or track boundary changes.

### 2026-07-16 — Put the upstream VERSION pin inside S01's atomic transaction

- **Context**: Captain review commit
  `1bc4d7508960d83182e2177a18374df530c632fc` returned `NEEDS_COACH` because
  S01 AC-03/AC-04 required the public exit map, upstream VERSION write and
  restart-authoritative recovery beyond the declared touchpoints.
- **Options considered**: keep the pin as a post-vendor write; exclude upstream
  write mode from S01; or construct the pin candidate before mutation and make
  it an ordinary member of the mapped-file transaction.
- **Coach decision**: include `cmd/sworn/baton.go`, `internal/baton/version.go`
  and `internal/baton/version_test.go` in S01. Public vendor outcomes are 0 for
  clean/success, 1 only for deterministic check drift, and 2 for invalid,
  operational, apply, rollback or recovery failure. Upstream VERSION bytes are
  constructed from one captured invocation instant and join the same fully
  materialised snapshot/apply/rollback/recovery transaction as mapped vendor
  destinations. Excluding the pin write is rejected because it preserves a
  partial-success state.
- **Recovery decision**: one deterministic owner-only record and snapshot tree
  beneath the current worktree's resolved Git administrative directory is the
  sole restart authority. Repository/path tuples and original
  bytes/modes/existence are integrity-checked; traversal, foreign, symlinked,
  missing or tampered material exits 2 in recovery-only mode without ordinary
  vendor writes.
- **Scope boundary**: S01 changes machinery and tests but does not advance the
  actual v0.15.1 pin/content/install state. S02 still executes and proves that
  update. S01 remains in `design_review`; its design must be revised and
  re-reviewed before implementation.

### 2026-07-16 — Keep the diff parity fixtures compatible with S01 preflight

- **Context**: All three pre-existing parity tests in
  `internal/baton/diff_test.go` call write-mode `Vendor` to seed temporary
  repositories. S01's exact Git-admin-confined recovery preflight now requires
  those repository fixtures to provide a fake or real `.git` administrative
  directory.
- **Coach decision**: Add only `internal/baton/diff_test.go` to S01's
  touchpoints and planned files so the owned test fixtures can satisfy that
  mechanically required precondition.
- **Boundary**: This is test-fixture compatibility, not new behavior. There is
  no `internal/baton/diff.go` production change, acceptance-criterion or user
  outcome change, new dependency, track/topology change, contract change, or
  shared-touchpoint exception.
- **Lifecycle**: Preserve the exact committed T1 owner lifecycle at
  `dc9835e4cb66a7e5f51f8ad5f6e64ffcc48a2488`, including `in_progress`, the
  immutable `start_commit`, and the complete maintainability and verification
  objects; update only the planned-file boundary and planner metadata.

### 2026-07-16 — Resolve S02 archive authority and transaction ownership

- **Context**: Fresh Captain review found that the planned offline archive had
  no binary embed owner, no public `baton diff` owner, and no place in S01's
  mapped-bytes-plus-VERSION repository recovery set. The design also placed
  hostile archive handling and three-root recovery implicitly in the 1,316-line
  doctor adapter, misstated the exact tagged command count as seven, and did not
  record the structural Rule-9 decisions.
- **Options considered**: write the tar separately and accept split recovery;
  place all new behavior in `doctor.go`, `source.go`, or `manifest.go`; or expand
  the one repository transaction and assign focused internal owners.
- **Coach decision**: Expand the existing repository transaction so the exact
  installer tar is materialised, snapshotted, applied, rolled back, and
  restart-recovered with mapped bytes and VERSION. Use one explicit embed in
  `internal/adopt`, public archive parity in `internal/baton/diff.go`, bounded
  archive generation/validation in `internal/baton/installer_archive.go`, and
  bounded three-root install rollback/recovery in
  `internal/baton/install_transaction.go`; keep CLI files thin.
- **Rule-9 record**: One embedded archive, the expanded repository transaction,
  whole-root rollback across `agents_home`, `codex_home`, and `claude_home`, and
  bounded helper placement are Type-1 decisions selected by the Coach under
  Brad's instruction to follow the orchestrator's recommendation. Path-only
  diagnostics are the recorded Type-2 default.
- **Mechanical correction**: The exact v0.15.1 tag installs eight commands,
  including `design-review.md`; both native trees derive that complete inventory
  from the validated archive.
- **Boundary**: S02 is now an explicit forty-two-file bootstrap exception and a
  high-effort/high-complexity beast. It remains one slice because C-01 requires
  the binary embed, public parity, repository pin, and both supported installs
  to converge before any success claim. No vendoring or real-home mutation is
  authorized until the revised design receives fresh Captain PROCEED.

### 2026-07-16 — Separate VERSION identities and make install recovery crash-safe

- **Context**: The next fresh Captain confirmed all seven ownership pins were
  resolved, then proved that `5f1dd0af59642311ee04e018a0023562d4dde008`
  is the upstream tag's root `VERSION` blob containing exactly `v0.15.1` plus
  LF, while Sworn's `internal/adopt/baton/VERSION` is a different multi-line
  manifest required by the running parser. The same review found that upstream
  installer modes inherit umask and that environment-selected install roots
  could alias or crash between replacements before recovery authority existed.
- **Coach decision — identity**: Rename the strict marker field to
  `upstream_version_blob_oid` and keep `5f1dd...` as upstream source identity.
  Resolve the actual committed Sworn manifest blob separately on every
  participating ref, require those blobs equal, and parse/compare their
  tag/SHA/digest to the marker and binary. Neither identity substitutes for the
  other.
- **Coach decision — modes**: Run both independent exact-script oracles under
  umask `0022`; canonical managed-tree directories are `0755` and regular files
  are `0644`. A hostile inherited umask must not change the oracle.
- **Coach decision — recovery**: Physically resolve and require pairwise-disjoint
  `agents_home`, `codex_home`, `claude_home`, and recovery roots; reject equal,
  nested, aliased, symlinked, special-file, or recovery-overlapping topology
  before mutation. Durably publish complete owner-only snapshots, manifest, and
  sentinel before the first replacement. Any sentinel presence makes later sync
  recovery-only until all three complete pre-run roots are restored.
- **Golden cascade**: The corrected migrated marker changes one of the 17
  local-first mutation blobs, so the delta, Planner, receipt, transaction, and
  activation blob/tree/commit identities were independently reconstructed and
  all nine section-11 goldens refreshed. No live downstream release was
  modified.
- **Fresh ambiguity gate**: A context-isolated read-only reviewer returned PASS
  after checking the two VERSION identities, deterministic modes, root topology,
  pre-replacement recovery authority, S05 provenance, all 17 S17 mutations, and
  the refreshed nine-object cascade. It found no materially divergent conforming
  implementation.
- **Rule-9 record**: Identity separation and crash-safe root topology are Type-1
  Coach choices. Umask `0022` is the deterministic Type-2 default. Production,
  vendored, archive, and real-home writes remain blocked pending revised design
  and a fresh Captain `PROCEED`.

### 2026-07-16 — Align embedded-schema valid fixtures inside S02

- **Context**: After the Captain-approved implementation atomically vendored the
  exact v0.15.1 `slice-status-v1` schema, the complete `internal/baton` package
  exposed two stale literals in one existing test file. Both were labelled as
  valid status examples but omitted the newly required `start_commit` and
  `maintainability` members.
- **Coach decision**: Add `internal/baton/validate_schema_test.go` to S02's
  touchpoints and update only those two positive fixtures to the minimum valid
  v0.15.1 shape. Do not weaken the embedded schema and do not migrate or
  reinterpret active records; those authorities remain with S03–S05 and S17.
- **Why now**: A red package suite caused by examples that falsely claim
  conformance directly contradicts S02's exact-schema parity outcome. The same
  file was already touched by verified S01 in this serial track, so S02 owns
  only the schema-version alignment on top of that ancestry.
- **Boundary**: This is a single-path mechanical scope correction, taking the
  ratified bootstrap exception from 41 to 42 files. Implementation remains
  paused at clean pushed checkpoint `7b57f64fbfe9d0540737034d1794100b80aeec3b`
  until the amended planner gates pass and the delta is merged into T1.

### 2026-07-16 — Pull forward the status carrier needed by the repository gate

- **Context**: At clean pushed S02 checkpoint
  `60dcd6291ddbe3491e16a05d2ed98d896d714165`, the S02-owned packages, build,
  vet, formatting, and public parity checks passed, but `go test ./...` exposed
  ten stale positive tests in `internal/gate`, `internal/run`, and
  `internal/state`. Five generic-check fixtures omitted v0.15's required
  `check` identity. More importantly, two exact-schema reachability tests could
  not pass honestly because `state.Status` had no carrier for a supplied
  `maintainability` object, so `Read` dropped it and `Write` could not emit it.
- **Independent scope audit**: A read-only fresh subagent reproduced the failures
  and returned `BLOCKED` for a fixture-only repair. Weakening validation,
  patching bytes after `Write`, or deleting the exact-schema assertions would
  conceal a real production carrier gap.
- **Coach decision**: Expand S02 by exactly five paths: the three affected test
  owners plus `internal/state/state.go` and its focused test. Pull forward only
  an optional opaque/lossless maintainability carrier that preserves a supplied
  object through read/write. Tests must supply their own valid `start_commit`
  and maintainability facts. S02 may not default, infer, transition, migrate, or
  rewrite lifecycle state.
- **Ownership boundary**: S03 retains complete absent-versus-null start-commit
  semantics, typed maintainability fields, additive-field preservation,
  exact-schema atomic writers, record sweeps, board/spec carriers, rendering,
  and doctor coverage. S04 retains requested/emitted check matching, mismatch
  rejection, ambiguity separation, and retirement of generic maintainability
  dispatch; S02 changes only its canned schema-valid responses. S05 and S17
  retain protocol-selection and active-record migration authority.
- **Ratification**: This is the orchestrator's recommended fail-closed sequencing
  repair under Brad's standing 2026-07-16 instruction to follow that
  recommendation. The S02 bootstrap exception is now 47 files. Implementation,
  maintainability review, and real-home installation remain paused until the
  amended planner gates pass, the release record is synchronized into T1, the
  Implementer reconfirms effort, and a fresh Captain returns `PROCEED`.

### 2026-07-16 — Seal pre-C12 bootstrap reconciliation provenance

- **Context**: S02 Gate-8 scope construction proved all semantic paths and both
  synchronization topologies exact, but stopped before model dispatch because
  merges `b8df1857…` and `7696c9bf…` contain manual release-record resolutions
  forbidden by normative clarification section 6. A separate audit found the
  same defect in S01's already committed report frontier at `36d1bd56…` and
  `d062d055…`; rejecting the class now would invalidate durable S01 evidence.
- **Options considered**: A partial S02 rewrite or re-slice leaves S01 invalid.
  Rebuilding T1 far enough back changes S02's immutable start object, rewrites
  pushed evidence, and requires reissuing S01 reports. A generic exception would
  weaken fail-closed synchronization provenance.
- **Coach decision**: Record a sealed, schema-validated, exact-object historical
  manifest for only the four merges consumed by S01/S02 immutable intervals.
  Pin every ordered parent/base and eight exceptional B/P1/P2/result path tuples,
  plus the exact consumer slice, immutable start, exact review head,
  start-exclusive/head-inclusive first-parent membership, and permitted purpose;
  require ordinary section-6 validation everywhere else, and forbid pattern,
  subject, prefix, wildcard, caller-supplied, recomputed, cross-slice,
  endpoint-widened, or future use.
- **Boundary**: Three other audited non-compliant merges lie below the relevant
  immutable starts and remain unrecognized. The manifest is available only
  while authority is planning, only for historical scope/freshness/bootstrap
  evidence, and is revalidated then retired by C-13. It grants no merge creation
  or lifecycle authority.
- **Ratification**: Brad's standing instruction selected the orchestrator's
  recommendation. The sealed manifest contains four entries/eight paths and is
  pinned at SHA-256
  `f9e0de63c0a5ecf15cdb6058a52166ff0a609fa0d0cf2ecdf81d7955030b1943`.
  S02 preflight remains paused until this release-only provenance amendment is
  synchronized through an ordinary section-6-compliant merge and fresh planner
  ambiguity validation passes.

### 2026-07-16 — Repin the sealed S02 closure-review endpoint

- **Trigger**: The first authorized S02 Implementer preflight at exact review
  head `60097cfa65dc39d9a0ab174be7c627fde2d3f7d5` returned in-scope `FAIL`.
  Blocking `F-01` required phase-specific private transaction states in
  `internal/baton/install_transaction.go`; advisory `F-02` identified the inert
  `needManifest` parameter. The report and mandatory pending transition are
  committed at `bce66675862c18286dfee5d59f462ee50359abb1` and `fdae1cc…`.
- **Bounded remediation**: A fresh Captain returned `PROCEED` for a mechanical
  four-phase typestate extraction confined to the single required touchpoint.
  Commit `4377d71a23a2252d4bbb6bb3784692171b0329da` authors only that file,
  removes `preparedInstall`, `createdPaths`, and `needManifest`, preserves public
  APIs/faults/error classes/transaction identity, and passes targeted tests,
  full `internal/baton`, `go test ./...`, `go vet ./...`, and `make build`.
- **Coach decision**: Replace only the two S02 authorization `review_head`
  values in the sealed manifest with exact remediation head `4377d71…`. S01's
  two envelopes, both S02 starts, all four merge OIDs, eight exceptional paths,
  every Git tuple, interval semantics, purposes, schema, and all prohibitions
  remain byte-identical. This is endpoint replacement after an authorized
  remediation, not an entry/path/purpose extension.
- **Ratification**: Brad's standing instruction selected the orchestrator's
  recommendation. The replacement manifest is pinned at SHA-256
  `2fabdcbf60ea0d81f77259bcaa08258a0e804f4cf1e23b8ba33eb2a7d47f5666`.
  The sole closure dispatch remains prohibited until fresh ambiguity review,
  deterministic planner gates, and ordinary section-6 synchronization into T1
  all pass.

### 2026-07-16 — Preserve pre-capture crash disposition and repin S02 again

- **Trigger**: Independent final-byte review of the authorized typestate
  remediation found one behavioral regression at the existing `paths-ready`
  fault point: the synthetic process-crash error had moved into the captured
  phase and would have been converted into `process-crashed`, unlike the
  established direct pre-capture result.
- **Bounded correction**: Commit
  `2a17443d67d39cf681dba117a57673714a916d7f` authors only
  `internal/baton/install_transaction.go`. It marks the pre-capture boundary
  explicitly and returns the original `paths-ready` cause unchanged, while
  retaining the four phase-specific private values and the existing
  post-capture crash disposition. A direct injected-crash check, the targeted
  fault/identity suite (132.644s), full `internal/baton` suite (403.226s),
  repository suite (including `internal/baton` at 411.401s), `go vet ./...`,
  `make build`, formatting/diff checks, and public binary reachability all
  passed. A repeat independent audit returned PASS.
- **Coach decision**: Replace only the same two S02 authorization
  `review_head` values from `4377d71…` to exact final head `2a17443…`.
  The S01 envelopes, S02 starts, four merge OIDs, exceptional paths, Git
  tuples, interval semantics, purposes, schema, and prohibitions remain
  unchanged. This is a second endpoint-only replacement after a verified
  in-scope preservation correction, not a scope extension.
- **Ratification**: Brad's standing instruction continues to select the
  orchestrator's recommendation. The replacement manifest is pinned at
  SHA-256 `23ca47fe790e5f8d4e9022b5b0df819de9972938d581e014a7ffd9c0dc16227e`.
- **Review and gates**: A fresh no-history authority review returned PASS.
  Draft 2020-12 schema validation, digest reproduction, first-parent ancestry,
  one-production-file delta checks, all 109 ACs, trace, requirements
  validation, design-fit, spec-quality, board oracle, and diff checks passed.
  Closure dispatch remains prohibited until ordinary section-6 synchronization
  into T1 passes.

### 2026-07-16 — Seal the exact historical index defect and replacement closure

- **Trigger**: A fresh independent all-four-merge reconstruction found that
  `7696c9bf9c235fffb937d3ed7e4be5a8a2bbda2a` does not satisfy the ordinary
  deterministic `index.md` rule that the v1 manifest still required. Its
  committed blob `998b44c05390a0e1cef9d37ccd312669610e9474`
  omits S02/T1 ownership of `internal/run/slice_test.go`. Replaying the exact
  merge's renderer against P2 `dcd6386…`, T1 at the merge, and absent dependent
  track refs produces blob `9bf11cda0fb35ef8bb1995ab3ba3087e9b5e056e`,
  SHA-256
  `7c06230ba73add2f99ce3982931092c30253ba8be1e007540f118bfb4a5c319d`,
  22,468 bytes. The other three sealed merges render byte-identically.
- **Fail-closed hold**: A concurrent session dispatched closure invocation
  `f4ef4f75-4dc8-48d1-a37c-d37d5f83c5ff` before this provenance defect was
  repaired. Its PASS report is committed at `1c46bccc…`, blob `31172c04…`, and
  its lifecycle transition at `0d593202…`; hold commit
  `3ac42c01cca6e9227bb93092ed13813bad95aa08` removes it from the authoritative
  ledger and restores S02 to in-progress/pending cycle 0 with null
  implementation head and pending verification. The file remains permanent
  forensic evidence. The physical dispatch and temporary historical ledger
  append remain recorded facts, but the transition was invalid and cannot
  supply response, report, verdict, lifecycle, Verifier, or cutover authority.
- **Independent design recommendation**: Retain v1 unchanged for audit and
  replace live recognition with a closed v2 schema. Preserve the exact four
  ordered merges and eight exceptional release-record paths. For each S02
  entry, bind a non-dispatch `historical_report` envelope to original preflight
  head `60097cfa…`, invocation `ff6145c0…`, report blob `6f95a34a…`, and
  fingerprint `sha256:4d58ca…`, plus a separate `live_review` envelope at final
  semantic head `2a17443…`. Only `7696c9bf…` may carry the exact actual-versus-
  expected projection record, and its stale bytes are never current authority.
- **Replacement decision**: Authorize exactly one new fresh Implementer
  closure at S02 start `e61cb190…`, head `2a17443…`, cycle 0, only after the v2
  schema/manifest/digests and ordinary release-wt-to-T1 synchronization pass on
  a clean local/remote-equal track containing hold `3ac42c01…`. The replacement
  runs only through the manual bootstrap Implementer adapter, not the native
  current-only command. It must use a new invocation and recompute scope from
  Git; it cannot reuse the orphan response, report, verdict, fingerprint, or
  lifecycle transition. Before any model request, the adapter creates a
  one-parent commit changing only the immutable claim record, whose digest
  fields equal the live manifest/v2-schema/claim-schema/receipt-schema bytes.
  The claim stores only the SHA-256 of a random 32-byte continuity token whose
  preimage remains in process memory. Creating that commit consumes the sole
  replacement dispatch and issues a non-resumable permit; it must be pushed
  before use. A local-only or pushed claim without a token-matching receipt and
  authoritative report routes to the Coach with zero redispatch. The matching
  receipt reveals the token and binds the claim commit, invocation, report blob
  and verdict; only that report consumes the ordinary C-06 closure row. It does
  not authorize a Verifier by itself, and any invalid or exhausted attempt
  returns to the Coach without a second automatic dispatch.
- **Ratification**: Brad's standing instruction selects the orchestrator's
  recommendation. The v2 schema is bound at SHA-256
  `daa13bd5cb8dd3d5c0f7473ee132b9d15d083405a1e89bfeedc7f8e298bbbbad`, the claim schema at SHA-256
  `32023df8e953640b266d9113c9055b9cd601cceb26e842def080dc1491563746`, the receipt schema at SHA-256
  `3678f1ac208e0d9a04a3bc01ad9d9e61fa8bc5472402a05ae376f21ca8022a52`, and the sealed manifest at SHA-256
  `3d0e0da7fa57a0d754b8e0b6a0faae90f47bea72c100ea2dbf0ba4c68c486dc1`.
  No S01/S02 spec, report, proof, semantic code, lifecycle state, Verifier,
  installation, or branch authority is changed by this planning amendment.

### 2026-07-16 — Retire failed S02 through exact rollback and fresh replacement

- **Trigger**: The fresh artefact-only Verifier failed Gate 3 because
  `cmd/sworn/doctor_test.go:TestDoctorAndBatonDiffV015BinaryReachability`
  passed the developer-specific `/home/brad/projects/baton` checkout to the
  built binary. The test was green only on the maintainer's host and was not a
  clean-CI reachability proof.
- **Lifecycle constraint**: S02's final Implementer maintainability PASS froze
  semantic head `2a17443d67d39cf681dba117a57673714a916d7f`. Baton therefore forbids
  repairing the test under the same slice id. Original S02 remains terminal
  `re_slice_required`, preserves its complete reports/claim/receipt/proof and
  Gate-3 failure, changes overall state only to rollback-backed `deferred`, and
  records one immutable rollback link.
- **Decision**: Insert `S19-s02-v015-rollback` and
  `S20-v015-parity-portable-fixture` after original S02 and before S03. S19
  restores the complete first-parent non-merge semantic envelope through its
  verified implementation head to S02 start commit
  `e61cb190736ee7483fb4ed1a993442b26ce3574c` exactly. S20 may start only after
  that rollback is freshly verified and becomes the active C-01 owner.
- **Portable proof**: S20 re-delivers the exact 45-path v0.15.1 semantic result
  and adds one test-only authenticated Git-bundle fixture. The bundle is the
  exact 2,505,826-byte canonical bundle-v2 stream: the fixed tag header plus Git 2.43.0 `pack-objects --stdout --revs --window=0 --depth=0 --compression=9 --threads=1 --no-reuse-delta --no-reuse-object --no-sparse --delta-base-offset` output, pinned
  at SHA-256 `cba3796ed382623f35abc568183e3a5a0d4a82335cebd4589989d0ae41b43ad5`
  and blob `77e5b4cc7210a41ce8779bc352a1f487101fb80e`; it contains annotated tag
  `3ba5f704…` and peeled commit `3fb4d275…`. The test verifies complete history,
  clones with `--no-checkout` beneath `t.TempDir()`, then detach-checkouts
  `v0.15.1^{commit}` before evaluating HEAD. No sibling checkout, synthetic commit, weakened pin
  predicate, or runtime bundle dependency is authority. The committed bytes and
  pinned identities are normative; cross-version regeneration is not required.
- **Topology**: T1 order is S01, original S02, S19, verified S04, planned S21,
  planned S22, blocked S20, S03, S05. The release now contains 22
  proof-bounded slices;
  T2/T5/T6/T7 dependencies and the pure-plan `shared_touchpoints: {}`
  authority are unchanged.
- **Ratification**: Brad was told the mandatory rollback/replacement consequence,
  accepted the orchestrator's recommendation, and instructed the release process
  to continue. Real Codex and Claude homes remain untouched until S20 receives an
  independent fresh Verifier PASS.

### 2026-07-17 - Make S04 the immediate prerequisite for blocked S20

- **Trigger**: The exact `ac-satisfaction` prompt emits only `verdict` and
  `findings`, but the upgraded generic `llm-check-report-v1` requires emitted
  `check`. The configured raw response therefore failed closed even when it
  stated PASS.
- **Decision**: Move `S04-typed-reference-ambiguity` immediately before S20 in
  T1. S04's existing AC-04 is the sole correction for requested/emitted generic
  check identity. S20 AC-05 explicitly excludes that behavior, so no S20
  acceptance criterion, implementation, or workaround is widened.
- **Lifecycle**: S20 remains `blocked` at
  `a7229cae11ea342eab5677269c24f03754e6b6b9`; its immutable start
  `08dd38f81e466d3288ff4bf64953cfc90ea6063c` and implementation commits
  `edad0fa8a75ab3b4a1938bdaf856c7973be72107` and
  `f3da6a49c3f89f0883e265befd30d1eb099d6a90` remain reachable and unmodified.
- **Handoff**: A fresh S04 verification PASS is required before S20 may resume.
  The resumed S20 session reruns its readiness and maintainability evidence; it
  does not inherit, fabricate, or bypass those gates.

### 2026-07-17 - Add a closed-world OpenAI strict envelope before S20 resumes

- **Trigger**: At clean T1 head
  69238f0b011b7e2965ede64231e17ba373a510dd, the exact canonical
  llm-check-report-v1 request reaches the configured OpenAI structured-output
  endpoint but is rejected before model emission because the top-level allOf
  conditionals are unsupported. This is a transport incompatibility, not
  evidence that the canonical report contract or S04 requested/emitted identity
  check is wrong.
- **Decision**: Add planned S21-openai-structured-envelope immediately after
  verified S04 and before blocked S20. It owns only a deterministic
  llm-check-report-v1-openai-envelope for openai/ Responses and
  openai-completions/ native response-format calls. It selects the canonical
  generic report only by exact $id
  https://baton.sawy3r.net/schemas/llm-check-report-v1.json plus source SHA-256
  ed38b77823af1b329c1dc7d8427b08849f15690d5afa9625e196505bdfa5b65b.
  Unknown/digest-mismatched generic-report identities and
  spec-ambiguity-report-v1 must fail locally with zero HTTP; xAI, tool-call,
  and all unprofiled paths retain their existing schema handling.
- **Authority boundary**: Exact vendored schemas and prompt bytes,
  internal/gate/llmcheck.go semantic authority, local canonical validation, and
  S04 requested/emitted equality are unchanged. The envelope neither
  synthesizes check nor reconstructs ambiguity maps, retries without structure,
  or falls back to raw text. This is a non-Type-1 technical correction
  ratified under the Coach standing orchestration authority.
- **Lifecycle**: S04 remains verified. S20 remains blocked with immutable start
  08dd38f81e466d3288ff4bf64953cfc90ea6063c, semantic commits
  edad0fa8a75ab3b4a1938bdaf856c7973be72107 and
  f3da6a49c3f89f0883e265befd30d1eb099d6a90, resume
  bef712dbc629678d7bf2579d3beb560e2b025c0a, and all existing blocked
  evidence preserved. It may resume only after a fresh S21 verifier PASS, then
  must rerun its own readiness and maintainability evidence and perform a
  credentialed exact-base OpenAI smoke that produces an accepted emitted
  check: ac-satisfaction result.

### 2026-07-17 - Add a direct-only OpenRouter tool route before S20 resumes

- **Trigger**: The Coach selected `openrouter/z-ai/glm-5.2` as the replacement
  model for the required S20 provider-readiness evidence. The model catalogue
  reports tool support, and OpenRouter's published
  chat-completions contract supports a forced named function. Sworn currently
  treats `openrouter/` as structured-output unsupported, so the public command
  fails locally before an HTTP request; substituting the model flag alone would
  not produce evidence.
- **Decision**: Add `S22-openrouter-tool-structured-output` after verified S21
  and before blocked S20. It profiles only **direct** `openrouter/` routing for
  the existing one-forced-function wire: the full canonical schema is supplied
  as the function parameters and `emit_structured_output` is forced. It does
  not use OpenAI's strict envelope or response format. `SWORN_DIRECT=1` is
  mandatory for the release proof, so Sworn's hosted proxy remains
  structured-output default-deny for OpenRouter IDs. A direct-only
  `SWORN_OPENROUTER_BASE_URL` override provides a local fake endpoint for the
  built-command reachability test; ordinary direct use continues to use
  `https://openrouter.ai/api/v1`.
- **Authority boundary**: The canonical schema bytes are supplied unchanged to
  `ChatStructuredJSON`; S04's full local canonical validation and
  requested/emitted check equality remain the semantic authority. No provider
  default, model catalogue claim, raw-text fallback, schema projection,
  synthetic report, retry, or proxy-wide enablement is authorised. Ollama and
  every other direct or proxied provider retain their current default-deny
  structured capability unless separately planned and proven.
- **Lifecycle**: The required dedicated `spec-ambiguity` check for this new
  slice cannot run through the current binary before S22 because that
  default-deny failure is the slice's subject. This is an explicit temporary
  planning deferral: **why**, the direct OpenRouter structured route does not
  yet exist; **tracking**, S22 AC-06 requires one direct
  `openrouter/z-ai/glm-5.2` `spec-ambiguity` result after deterministic
  implementation evidence and before S22 may become implemented; and
  **acknowledgement**, the Coach selected the model and directed this route on
  2026-07-17. A fresh S22 verifier PASS then gates S20. S20's immutable start,
  semantic commits, previous non-secret failures, and no-real-home boundary
  remain unchanged; its later direct readiness smoke uses the selected model
  only after that PASS.

### 2026-07-17 - Recover S22 after the unreceipted AC-06 invocation

- **Trigger**: The earlier once-authorized S22 AC-06 direct invocation did not
  yield a usable sanitized receipt. It is therefore neither a PASS nor a FAIL,
  and it is not a fresh verifier verdict. The advisory audit also found two
  unpinned deterministic transport cases: JSON `null` tool arguments and a
  returned tool call whose `type` is not `function`.
- **Coach decision**: Re-scope only started S22, preserving its immutable
  start commit `a09b0e46df465862d00469d4aef2a997442b3d5b` and all existing T1
  code. Add explicit AC-07 JSON-null and AC-08 non-function-type rejections
  at the existing `internal/model/oai.go` and
  `internal/model/structured_test.go` touchpoints. S22 remains after verified
  S21 and before blocked S20; no other slice, track, or S20 fact changes.
- **New direct proof authority**: After both guard fixes and every deterministic
  S22 gate pass, the Coach authorizes exactly one new direct
  `openrouter/z-ai/glm-5.2` `spec-ambiguity` proof. This is an explicit
  recovery action, not a silent retry. The receipt may contain only check
  identity, model ID, immutable start commit, process exit code, and a
  PASS/FAIL/BLOCKED/UNPARSEABLE result. Raw provider/model output is
  private-temporary then destroyed, or never retained.
- **Boundary**: There is no fallback and no further retry. The cleared external
  evidence block returns only S22 to a fresh Implementer. S20 remains blocked,
  unmodified, and cannot resume until the one new receipt is PASS and a fresh
  S22 verifier separately records PASS. Credentials, request bodies, model
  output, and provider diagnostics are neither inspected nor retained by the
  planner.

### 2026-07-18 - Close S22 public-error and atomic-finalization audit findings

- **Trigger**: Pre-live adversarial audit found two fail-closed gaps in the
  started S22 recovery: `sworn.llm_check` can wrap a provider error whose
  message derives from the provider response body, and a second write failure
  while restoring a receipt reservation after post-rename finalization failure
  can leave a final model verdict trusted. It also found that S22's upstream
  S21 preflight did not mechanically prove the recorded fresh-context evidence.
- **Coach decision**: Replan only S22. Add `internal/mcp/lint.go` and
  `internal/mcp/lint_test.go` to its T1-owned touchpoints and require the
  registered MCP tool to retain error/non-success semantics while exposing
  exactly `llm_check: provider request failed` for provider/model errors. Add
  deterministic reachability and leak-canary coverage. Require the receipt
  lifecycle to retain or surface only durable `receipt_failure` if restoration
  itself fails, never trust final verdict bytes, and reject every absent or
  mismatched S21 identity, status reference, immutable start, PASS, verdict
  time, or fresh-context fact before dispatch.
- **Boundary**: This is a confidentiality and fail-closed correction, not a
  routing or product-policy change. No provider call, default model, proxy
  capability, fallback, third dispatch, or S20 activity is authorized. The
  prior Captain review is superseded; fresh Captain PROCEED and acknowledgement
  remain required after deterministic implementation evidence.
- **Release mechanics**: The release assembly was first forward-merged from
  `release/v0.2.0` cleanly. T1 is dirty only with S22 in-progress proof/spec
  artefacts, so the replan is not propagated into that worktree; the next fresh
  Implementer session must self-heal from the committed release assembly
  without discarding the preserved work.

## Schema-vs-spec audit notes

- The v0.15 `slice-status-v1` schema requires a non-null `maintainability`
  object even for a planned slice; missing data is not an Implementer reset.
- The v0.15 `board-v1` schema adds machine-authoritative
  `shared_touchpoints`; the rendered matrix is not an alternate authority.
- The v0.14/v0.15 `spec-v1` contract makes `references` the sole normative
  discovery surface for contract, slice and file inputs.
- `llm-check-report-v1` and `spec-ambiguity-report-v1` are distinct contracts;
  the latter cannot be parsed as the older generic findings array.
- Existing closed records cannot be assigned PASS report ledgers without real
  committed reports, invocation identities and blob OIDs.

## Ratified slice decomposition

- `S01-vendor-boundary-readiness`: A maintainer can run the upstream v0.15
  vendor/check workflow without false script-reference matches, unsupported
  schema-expression failures or any partial write being reported as success.
- `S02-v015-parity-and-installs`: Historical failed original. Its immutable
  evidence explains the Gate-3 failure and its terminal deferral is valid only
  through the verified S19 rollback link; it no longer owns active C-01.
- `S19-s02-v015-rollback`: The complete ordinary semantic envelope is restored
  mode-for-mode, object-for-object, and absence-for-absence to S02's immutable
  start tree before replacement begins.
- `S20-v015-parity-portable-fixture`: The binary, vendored normative content and
  supported Codex and Claude installations report and byte-match exact Baton
  v0.15.1, with built-binary reachability proven from a verified exact-tag Git
  bundle clone rather than a developer checkout. It remains blocked behind a
  fresh S22 PASS and retains all existing immutable lifecycle evidence.
- `S03-lossless-record-carriers`: `sworn doctor` proves state, board and spec
  records round-trip maintainability, shared touchpoints and typed references
  without loss against the exact v0.15 schemas.
- `S04-typed-reference-ambiguity`: The Planner runs the dedicated ambiguity
  check over typed, workspace-confined references and every generic check emits
  the required canonical check identity.
- `S21-openai-structured-envelope`: Only the explicit OpenAI strict-output
  response-format paths compile the exact generic report into a deterministic
  small envelope; the canonical schema and S04 semantic authority remain local
  and unchanged.
- `S22-openrouter-tool-structured-output`: Only direct `openrouter/` routing
  uses the existing forced-function transport for the selected GLM proof path;
  it passes the canonical schema to the tool unchanged, leaves proxy and other
  provider routes default-deny, and retains the full S04 local gate.
- `S22-openrouter-tool-structured-output` configured-values recovery (ratified
  2026-07-18): preserve attempts 1 and 2 as immutable historical receipts, then
  permit one separately authorised attempt 3 that resolves the verifier model
  only through the standard current config with no proof-specific model flag.
  Record the resolved model ID but no config path, endpoint, credential, or
  payload. Attempt 3 is terminal and no fourth dispatch or fallback exists.
- `S05-protocol-provenance-archive`: An operator can inspect historical records
  read-only with committed version evidence, while every live operation fails
  before side effects unless its exact protocol marker matches the binary and
  all authority refs.
- `S23-v016-parity-and-installs`: After the started bootstrap prefix reaches its
  lawful boundary, the binary, vendored protocol, tagged source bundle and
  isolated Codex/Claude installation outputs prove exact Baton v0.16.0 parity,
  including the `board-oracle-v1` schema and eight-command installer surface.
- `S24-board-oracle-v1-projection`: The public aggregate board catalog and
  named-release compatibility view validate one portable topology contract;
  malformed ownership/dependencies or unsafe `sourceRef` values fail before a
  later mutable consumer can derive a Git object or path.
- `S06-exact-git-object-plumbing`: `sworn maintainability review` first reaches
  the NUL-safe committed-Git boundary, then fails closed at unavailable scope.
- `S10-shared-touchpoint-composition`: The same command recognizes shared-path
  synchronization only through canonical committed-blob composition.
- `S07-canonical-semantic-scope`: The same command constructs the exact path
  set, exclusion record, manifest, fingerprint and normalized prompt diff, then
  fails closed before report authority.
- `S08-report-ledger-identity`: The same command validates/reuses only exact
  committed Baton-plus-overlay report identity, then fails closed before
  lifecycle authority.
- `S09-lifecycle-fsm-adjudication`: The same command enforces committed-history
  phase order, dispatch budget, Coach authority and immutable lifecycle before
  the coordinator exists.
- `S11-track-evidence-freshness`: The same command revalidates active/retired
  ownership and synchronization freshness before reuse or dispatch.
- `S12-rollback-readiness`: `sworn maintainability cutover` reaches ordinary or
  post-sync rollback equality and the one canonical readiness predicate, then
  fails closed before S13's activation transaction.
- `S13-maintainability-engine-cutover`: Public review and adjudication commands
  atomically persist, commit and push lifecycle transitions, recover without
  redispatch, produce the exact empty-scope zero-dispatch PASS, and revalidate
  every active S01/S03-S13/S19/S20 bootstrap record plus terminal original S02's
  historical ledger, rollback link, and exact S19 equality, including S13's own
  verifier-certified evidence without self-awarding PASS, before automation
  becomes authoritative.
- `S14-role-lifecycle-recovery`: Loop, router, Implementer and Verifier paths use
  the shared command authority with exact dispatch counts, cycle routing and
  target-asserted reset/clean restart recovery.
- `S15-unified-track-merge`: CLI, autonomous-loop and MCP track-merge paths use
  the same provenance, freshness, composition and readiness authorities on
  normal and idempotent execution.
- `S16-release-ship-transitions`: Release merge, pre-cutover ship and
  post-deployment mark-shipped paths share canonical gates while preserving
  readiness, exact Baton ship/status/index evidence plus unchanged board, local bookkeeping and human
  push as distinct facts.
- `S17-engine-replan-migration`: Planner and MCP adapters share one engine
  authority for ordinary application, pristine migration, and the separate
  migrated activation edge; source, receipt, transaction and recovery identity
  are Git-derived and lifecycle authority remains opaque.
- `S18-public-conformance-proof`: Real-binary temporary-Git tests prove complete
  PASS, failure, tamper, restart, merge, re-plan and ship behavior through public
  entry points.

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Whether strict v0.15 validation applies retroactively to closed historical releases or only to records whose committed protocol version predates v0.15 | Record loading, trace, board oracle, merge/release/ship gates, and active-plan migration | Ratified: version-gated read-only archive; explicit Planner migration before any current workflow |
| A-02 | Exact public command spelling for maintainability operations and Coach adjudication | CLI reachability tests and adapter ownership | Ratified: dedicated `sworn maintainability review` and `sworn maintainability adjudicate`; generic `llm-check` execution retired |
| A-03 | Whether canonical v0.15 operations fit existing packages or require new focused internal packages | Touchpoints, tracks and file ceilings | Ratified: focused `maintainability/scope`, `maintainability/ledger`, and `integration` authorities with thin adapters and lossless carriers |
| A-04 | How a current operation proves v0.15 authority while historical releases contain mixed protocol eras | Protocol selection, archive inspection, migration and every mutation/success path | Ratified: exact committed `protocol.json` for live authority plus per-record Git evidence for read-only history |
| A-05 | Whether the maintainability command or the surrounding role session owns report/status commits and the track push | Crash recovery, report reuse, machine-to-machine handoff and public exit semantics | Ratified: stateful command owns atomic records, commit and push; interrupted push resumes without model dispatch |
| A-06 | Which pre-v0.15 records may receive an in-place Planner migration | Local-first plan activation and protection of started historical evidence | Ratified: only pristine planned records with null start and no execution evidence, through C-12 migrate then activate; started or terminal work requires new v0.15 IDs |
| A-07 | Whether readiness validation and the deployed `shipped` transition are one command or distinct facts | Public command semantics, deployment truth and idempotent terminal validation | Ratified: keep `sworn ship` as pre-cutover gate; native `mark-shipped` performs exact Baton status/index bookkeeping while preserving the pure-plan board, hands push/cleanup to the human, and no-ops when nothing remains verified |
| A-08 | Whether re-plan requirements judgment and authoritative record mutation belong in the same layer | Re-slicing, rollback linkage, owner-track seeding, MCP writes and recovery | Ratified: Planner owns meaning; one engine operation validates, commits and propagates the ratified mutation |
| A-09 | How the release can obey v0.15 before the conformant Sworn engine exists | Early slice governance, first install activity, evidence integrity and engine cutover | Ratified: staged manual v0.15 bootstrap followed by mandatory engine revalidation before automated authority |
| A-10 | How deeply to decompose the v0.15 conformance body | Slice independence, proof boundaries, track parallelism and file ceilings | Ratified and amended after Gate 3: 22 proof-bounded slices, normally under 25 files, with the historical S02 exception followed by an explicit 45-path S19 rollback, verified S04 identity gate, planned OpenAI envelope S21, planned direct OpenRouter tool route S22, and 46-path S20 replacement including one authenticated test-only Git bundle |
| A-11 | How to group the 22 slices for safe parallel delivery | Track dependencies, file ownership, worktree materialisation, Rule-1 reachability and the S13 cutover | Ratified: five tracks; T1 orders original S02 → S19 rollback → verified S04 → planned S21 → planned S22 → blocked S20 → S03 → S05, serial T2 runs S06 through S13, T5/T6 parallel after T2 cutover, T7 final, and no shared-touchpoint exceptions |
| A-12 | How exact v0.15 adapter decisions and the planning-to-current boundary become executable | Vendor exits, reference resolution, semantic identity, lifecycle, integration, deployment, re-plan, recovery and downstream track activation | Ratified: direct normative planning references plus C-13 post-T2 release-wt activation before T5/T6 materialisation |
| A-13 | How a committed delta avoids naming its own commit and how a migrated marker becomes current | Re-plan source identity, receipt bytes, crash recovery and downstream protocol migration | Ratified: canonical Planner ref parented by the source, deterministic receipt fields, and distinct C-12 migrate/activate edges; C-13 stays native-only |
| A-14 | Which owners publish, embed, compare, generate, install, and recover S02's offline archive | Repository atomicity, public parity, binary authority, local-home safety, file ceiling and Rule-9 records | Ratified: one expanded repository transaction; explicit `internal/adopt` embed; focused archive and install-transaction helpers; `internal/baton/diff.go` public parity; thin CLI adapters; eight-command inventory |
| A-15 | Whether the upstream root VERSION blob and Sworn's adopting-repository VERSION manifest share one identity, and when local-install recovery becomes authoritative | Protocol marker schema, live/archive provenance, oracle modes, root topology, crash recovery and migration goldens | Ratified: `upstream_version_blob_oid` names only upstream `v0.15.1\n`; committed Sworn manifest blobs and parsed pins are separate per-ref evidence; umask 0022 is fixed; roots are disjoint; complete recovery authority is durable before first replacement; all affected goldens are refreshed |
| A-16 | How OpenAI can accept a strict generic-check transport schema without becoming semantic authority | Generic LLM check Responses/completions transport, S04 identity gate, dedicated ambiguity contract, provider isolation, and S20 recovery | Ratified: exact-id-plus-digest closed-world OpenAI envelope below S04; canonical bytes and local validation remain authoritative; unsupported report identities fail before HTTP; xAI/tool-call/non-OpenAI paths remain raw; fresh S21 PASS gates S20 separate real smoke |
| A-17 | How the selected OpenRouter GLM model can produce structured evidence without broadening proxy or provider authority | Direct OpenRouter wire, local public-command testing, S04 semantic gate, hosted proxy isolation, and S20 recovery | Ratified: direct-only forced named function using canonical parameters, `SWORN_DIRECT=1` for the proof, a local endpoint override only for public-command testing, and fresh S22 PASS before the separate S20 GLM smoke; hosted proxy, Ollama, and all other unprofiled routes remain default-deny |
| A-18 | Historical S22 receipt-recovery decision | Exact direct-tool fail-closed cases, immutable started work, receipt minimisation, retry authority, and S20 sequencing | Superseded by A-19 after the audit identified that a shell-style five-field receipt cannot prove atomic dispatch, preserve raw-data boundaries across normal rendering, or distinguish an eligible environmental recovery from an opaque failure. The historical raw output remains destroyed. |
| A-19 | How S22 can recover a missing proof receipt without adding a generic retry or exposing provider data | Native CLI contract, atomic evidence, typed error classification, generic output sanitation, direct-only transport, and S20 sequencing | Ratified: record the original invocation only as attempt 1 receipt_failure/UNPARSEABLE with immutable release/slice/check/model/start binding; add a separate strict metadata-only native proof receipt reserved atomically before dispatch; a mismatched release/slice/check/model/start receipt rejects before dispatch and cannot consume or reuse retry budget; allow attempt 2 only for rate_limit, upstream, transient, network, deadline, runner_failure, or receipt_failure; final PASS/FAIL/BLOCKED and 400/401/402, unknown, parse/schema/identity/malformed-tool/opaque/untrusted outcomes do not retry; no third call, raw data, fallback, or S20 action before fresh S22 PASS. Fresh Captain review is required. |
| A-20 | Whether a sanitized receipt is sufficient when MCP can wrap provider-response-derived errors, or when a second receipt recovery write fails | Public MCP error surface, atomic finalization, S21 freshness proof, retry authority, and S20 sequencing | Ratified: extend only S22 with C-17's fixed MCP provider-error diagnostic and deterministic canaries; make post-rename restoration failure fail closed without trusting a final verdict; mechanically bind the S21 identity/start/PASS/time/fresh-context facts. No model/provider policy or proof budget changes. |
| A-21 | Whether the active release keeps a v0.15-derived name after Baton v0.16.0 is released | Operator clarity, release refs/worktrees, board discovery, and historical evidence | Ratified 2026-07-18: rename the active release, release-wt ref and T1 ref/worktree to `2026-07-15-baton-v0.16-conformance`; retain the old identifier only in Git history and the explicit migration record. The rename alters no source claim, started status, or verifier result. |
| A-22 | Whether to rewrite the in-progress S20 v0.15.1 bootstrap in place for v0.16.0 | Lifecycle immutability, rollback evidence, S04 overlap, and exact tagged parity | Ratified 2026-07-18: do not rewrite S20. It has a non-null start and committed implementation interval, so changing its pin would fabricate lifecycle evidence and requires an unsafe topology reconstruction. Complete or fail S22/S20 under their existing contract, then append S23 and S24 as fresh v0.16.0 tail slices. |
| A-23 | Whether S22 may bypass its exhausted hard-coded GLM receipt budget by using the verifier values currently configured for Sworn | Receipt history, model authority, provider dispatch count, confidentiality, and S20 sequencing | Ratified by Brad 2026-07-18: correct S22's blocked contract in place because it is unmerged and the existing implementation does not yet satisfy the new recovery. Preserve attempts 1-2 byte-for-byte; authorize exactly attempt 3 only after fresh Captain review and deterministic/proof gates; resolve `verifier.model` via `config.Load` plus `ResolveVerifierModel("", cfg)` with no CLI model override; persist only the resolved model ID in the strict receipt; preflight-reject unsupported/unconfigured values with zero dispatch; make every attempt-3 outcome terminal; forbid fallback, a fourth dispatch, and S20 activity before a fresh S22 PASS. |

## Screenshots / references

- No screenshots supplied; the normative tagged records and linked handoff are
  the durable evidence for this release.
