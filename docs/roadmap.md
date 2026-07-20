# Sworn v1 walking skeleton

Each milestone must remain runnable and preserve one control truth. Later work
may extend a record or reducer path, but may not introduce a second engine.

## 0. Protocol and repository boundary

- [x] Start from a fresh clone on disconnected orphan branch `release/v1.0.0`.
- [x] Preserve and restoration-test v0 archaeology.
- [x] Embed and checksum the admitted Baton snapshot.
- [x] Install v1-specific CI before any kernel implementation.

## 1. Transactional control core

- [x] Choose the SQLite driver in an ADR and add forward-only migrations.
- [x] Store commands, immutable events, records, artifacts, and pending effects
  in one database.
- [x] Implement the pure reducer with expected revisions and idempotency keys.
- [x] Derive a read-only JSON board from committed truth.
- [x] Prove duplicate-command, crash-before-effect, crash-after-effect, and
  unknown-state recovery behavior.

No agent subprocess or remote mutation is permitted in this milestone.

Implemented on `feat/transactional-control-core`. The state machine and store
remain internal: no mutating CLI command is enabled, and an authority receipt
digest is not treated as proof that the receipt is valid. Later internal effect
work does not change that public boundary.

## 2. Exact local candidate

- [x] Bind immutable repository identity, base commit, target ref, and worktree.
- [x] Create exact single-parent candidates and retain safe engine refs.
- [x] Add the read-only Linux containment foundation with immutable staging,
  cancellation, resource/output ceilings, and process-tree cleanup.
- [x] Add bounded writable builder handoff and safe measured workspace export;
  keep it inside the same executor boundary.
- [x] Prepare canonical submission bytes from measured Git facts and
  content-addressed local check evidence.
- [x] Parse and persist exact canonical Baton plans and work-contract digests.
- [x] Authenticate exact plan approval with a pinned Ed25519 root and persist
  its complete source/proof/receipt closure atomically.
- [x] Derive Standard submission facts from the exact plan and its strict,
  canonical, digest-selected assurance-policy registry rather than caller
  projections.
- [x] Add an explicit content-bound local-check runtime that stages,
  remeasures, executes, and receipts one exact runtime tree without a
  host-runtime producer fallback.
- [x] Add one immutable typed result slot per effect, with lease-bound binding,
  shared completion/reconciliation validation, and fail-closed unknown recovery.
- [x] Add an internal content-bound-only `check.local` worker whose minimal
  result rebinds receipt candidate identifiers to the builder result and
  validates the definition, environment, and output CAS closure, including
  requested runtime-manifest equality.
- [x] Derive `check.local` dispatch from the exact plan in the reducer.
- [x] Admit a reviewable submission only from authenticated authority,
  journal-registered runs, and a content-bound check runtime.
- [x] Reconcile interrupted workspace and Git effects with kind-specific,
  attempt-bound external evidence before any autonomous retry.

The recovery item is complete for the only native writable effect. A build
claim records an exact attempt identity before execution. The native worker
prepares but does not publish a candidate; after its typed result is bound,
Store establishes candidate and attempt refs before journal success. On
restart, a bound result converges to that success, while an unbound build can
return to pending only after Store prevalidation and a composite opaque proof
of absent publication, executor quiescence, and attempt-root cleanup. Legacy,
corrupt, stale, cross-Store, or ambiguous attempts remain stopped. See [ADR
0004](adr/0004-bound-result-recovery-contraction.md) and [ADR
0005](adr/0005-native-builder-recovery.md).

The Git-truth boundary and native builder worker are implemented internally and
bound into immutable Store configuration. A narrow composition service proves
the real builder-to-checks-to-admission path, but it owns neither claims nor a
loop. There is still no CLI mutation path or agent-CLI adapter. See [Exact local
candidate](exact-candidate.md).

The contained executor implements distinct read-only and writable-export modes
over one Linux path. Writable work uses a fresh copy on a finite tmpfs, live
cgroup resource bounds, post-quiescence measurement, deterministic attempt
ownership, process-shared serialization, and machine-proved cleanup. The
read-only content-bound path is used by an internal local-check effect worker. A
bounded reducer transition schedules its exact policy-selected batch after
builder success, but no production command or claim loop executes it; see
[Contained executor](contained-executor.md).

The Standard path rechecks a retained candidate, runs policy-bound producers
through read-only content-bound containment, and stores canonical receipts. One
Store-owned admission transaction now derives checks and evidence from the
exact journal batch, revalidates the whole protocol and Git closure, and is the
sole submission writer. See
[Atomic reviewable submission](measured-submission.md).

Exact-plan and authenticated historical approval facts survive restart through
the single control store. Check scheduling and admission reload their exact
relational closure, but it is not current effect authority: source re-resolution
before execution, accepting `PASS`, and integration remain milestone 4. See
[Exact plan and authenticated authority](authenticated-authority.md).

Submission construction resolves policy and check definitions from the exact
plan and rejects caller-selected substitutes. The intent-only admission command
binds that constructor to typed effect provenance and the content-bound runtime
in one transaction; see
[ADR 0003](adr/0003-reviewable-admission-contraction.md).

The effect journal now binds one immutable kind-specific result before
completion. Its `check.local` worker admits only a content-bound runtime and
stores a minimal outcome plus receipt. The worker materializes the builder
candidate, and the executor remeasures the workspace and runtime. The store then
rebinds receipt candidate identifiers to the succeeded builder result, validates
the immutable receipt/definition/environment/output CAS closure, and checks the
requested runtime-manifest equality when binding and reconciling success.
Effect completion does not repeat Git or runtime measurement or compare the
embedded protocol snapshot; admission closes those final facts before
reviewable. An absent bound result or orphan receipt cannot authorize autonomous
retry or success.

The plan-derived edge resolves one dependency-free Standard policy and all of its
definitions, rebinds the exact succeeded builder and process-configured runtime,
and creates the complete ordered check batch in one transaction. Work becomes
`checking` and claims serialize. A second transaction admits reviewable only
after every exact check passes and the complete authority, artifact, runtime,
snapshot, chronology, and Git closure revalidates. Both remain unreachable from
the production binary until a current-authority gate and mutating command
service exist.

## 3. Fresh independent verdict

- [ ] Dispatch builder and verifier through native CLI adapters.
- [ ] Keep authorizer capability and verifier identity outside builder scope.
- [ ] Bind each verdict to the exact dispatch, policy, submission, candidate,
  and evidence.
- [ ] Implement bounded retry epochs without treating `INCONCLUSIVE` as
  implementation failure.

## 4. Authority and integration

- [ ] Resolve authenticated interactive and standing authority sources.
- [ ] Revalidate authority before dispatch, accepting `PASS`, and integration.
- [ ] Add manual latch release and compare-and-swap fast-forward integration.
- [ ] Pass the 18 Baton real-boundary cases through the built binary.

Recovery proof is necessary but not sufficient for unattended use or a
default-branch cutover. Native builder recovery now passes; current authority,
independent verdict, bounded policy, integration, built-binary conformance, and
the exclusive public controller still gate either decision.
