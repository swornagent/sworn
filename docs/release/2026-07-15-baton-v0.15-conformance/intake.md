---
title: 'Release intake: Baton v0.15 conformance'
description: 'Planning record for exact Baton v0.15 adoption, maintainability lifecycle enforcement, deterministic integration, and fail-closed recovery.'
---

# Release Intake: `2026-07-15-baton-v0.15-conformance`

## Release goal

Make Sworn faithfully execute and enforce the Baton `v0.15.0` protocol before
any later release depends on its records or lifecycle. Shipped means an operator
can drive Sworn's public release commands through the bounded Implementer,
fresh-Verifier, Coach, Track Integrator, re-plan, merge, release and ship paths;
the same committed semantic inputs produce the same identity and reusable
evidence; malformed, stale or rewritten evidence fails closed; deterministic
rollback and shared-file composition protect integration; and the installed
Codex and Claude Baton mirrors report the same pinned protocol as the binary.

## Needs

- N-01: **Immutable upstream parity.** Sworn vendors and reports exact Baton
  `v0.15.0` content, preserves normative JSON bytes, and updates both supported
  local Baton installations from that same pinned source.
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
  committed handoffs, single cycle-1 resume and mandatory re-slicing outcome.
- N-06: **Deterministic integration evidence.** Track scope and freshness compose
  active authoritative intervals and rollback-backed retired ownership without
  treating historical ownership as PASS evidence or losing disjoint sibling
  work.
- N-07: **Rollback-backed recovery.** Ordinary and post-sync rollback paths
  compare the correct candidate envelope to the correct committed baseline and
  preserve later or sibling bytes that the failed interval did not own.
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
- N-11: **Safe active-plan migration.** Planning records that will be executed
  after the upgrade, including `2026-07-15-local-first-account-safety`, are
  migrated by the Planner to the exact v0.15 record shape and re-run through the
  v0.15 trace and spec-ambiguity gates without fabricating historical evidence.

## Source of truth

- **Human stakeholder**: repository owner / Coach
- **Tracking issue / epic**: [sworn#122](https://github.com/swornagent/sworn/issues/122)
- **Normative upstream release**: [Baton v0.15.0](https://github.com/sawy3r/baton/releases/tag/v0.15.0)
- **Normative upstream tag commit**: `16a3b304f360ec9b6a0f2cc5544d019058ac687c`
- **Normative source archive SHA-256**: `8acfaaabe27d93cfd6eeb0d8d9fba37261095e9e702826cc9e678f9ab8c3343b`
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
  `resume_in_scope` cycle or `re_slice`; a later failure has no waiver path.
- **Track Integrator**: merges a track only when committed scope, report history,
  freshness, rollback, shared-file composition and integration readiness all
  pass the same canonical validators.
- **Planner/re-planner**: emits v0.15 records, seeds started status from the exact
  owner track, preserves maintainability as an opaque authority object, and
  creates rollback/replacement slices without resetting failed history.
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
- The repository contains historical closed slice records without v0.15
  maintainability evidence and active planned records that need a legitimate
  Planner migration before implementation.

## Constraints and non-negotiables

- Baton `v0.15.0` is the sole normative protocol contract. Earlier issue
  comments are historical context only where wording differs from the tag.
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
- Upgrade and install operations must be atomic enough that a failed fetch,
  transform or validation cannot partially overwrite the primary worktree or a
  supported local install.
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
- **New Baton protocol design**: Sworn will not reinterpret or simplify v0.15.
  **Why deferred**: protocol changes belong upstream and this release is a
  downstream conformance implementation. **Tracking**: Baton issues/PRs for any
  newly discovered defect. **Acknowledged**: Coach, 2026-07-15, by accepting the
  tagged release as normative.
- **Commercial or hosted-control-plane strategy**: no business or private
  strategy is part of this public technical release. **Why deferred**: it is a
  separate private decision surface and is not required for v0.15 correctness.
  **Tracking**: outside this public repository. **Acknowledged**: Coach,
  2026-07-15, under the repository's public-doc discipline.

## Decisions made during planning

### 2026-07-15 — Ratify a dedicated v0.15 conformance prerequisite

- **Context**: The local-first account-safety plan validates structurally only
  after v0.15, while current Sworn code and installed Baton mirrors are v0.13.1.
- **Options considered**: implement trust-safety work first; fold the upgrade
  into its first slice; plan a dedicated conformance prerequisite.
- **Decision**: plan and deliver `2026-07-15-baton-v0.15-conformance` first.
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
  `resume_in_scope` or `re_slice`. Both derive cycle, reports, semantic scope,
  invocation identities and current authority from validated committed state;
  user flags cannot assert or override them.
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
  exact version, upstream commit SHA, source digest and vendored `VERSION` blob
  OID. Before any dispatch, write, verification, integration, release or ship
  operation, the marker must be committed, identical on every participating
  authority ref and exactly equal to the running binary's embedded pin.
- **Historical rule**: For read-only archive inspection, resolve each requested
  record's blob at the authority ref, walk the first-parent history to the commit
  that introduced the current path/blob identity, and read the canonical Baton
  `VERSION` blob from that exact tree. Return the record blob, evidence commit,
  version path/blob and resolved pin as inspection metadata. This evidence never
  authorizes a current operation.
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

## Proposed slice decomposition (draft)

Pending structured discovery and ratification. The upstream handoff recommends
three layers: immutable parity, reusable deterministic primitives, and role plus
command adapters. Those layers are inputs to decomposition, not pre-approved
slice boundaries.

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Whether strict v0.15 validation applies retroactively to closed historical releases or only to records whose committed protocol version predates v0.15 | Record loading, trace, board oracle, merge/release/ship gates, and active-plan migration | Ratified: version-gated read-only archive; explicit Planner migration before any current workflow |
| A-02 | Exact public command spelling for maintainability operations and Coach adjudication | CLI reachability tests and adapter ownership | Ratified: dedicated `sworn maintainability review` and `sworn maintainability adjudicate`; generic `llm-check` execution retired |
| A-03 | Whether canonical v0.15 operations fit existing packages or require new focused internal packages | Touchpoints, tracks and file ceilings | Ratified: focused `maintainability/scope`, `maintainability/ledger`, and `integration` authorities with thin adapters and lossless carriers |
| A-04 | How a current operation proves v0.15 authority while historical releases contain mixed protocol eras | Protocol selection, archive inspection, migration and every mutation/success path | Ratified: exact committed `protocol.json` for live authority plus per-record Git evidence for read-only history |

## Screenshots / references

- No screenshots supplied; the normative tagged records and linked handoff are
  the durable evidence for this release.
