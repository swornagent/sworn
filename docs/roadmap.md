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
before execution, accepting `PASS`, and integration remain later gates. See
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
the production binary. The current controller schedules and executes only the
builder; it does not claim checks or admit submissions.

## 3. Current-authority controller

- [x] Acquire crash-released process-exclusive ownership of the retained Store
  and its containing namespace.
- [x] Enforce recovery-only then active ownership phases at the Store boundary.
- [x] Reject raw build dispatch, generic build claims, and unowned recovery.
- [x] Re-resolve authenticated authority before build scheduling and again
  before pending build execution.
- [x] Bind authority to the exact controller, state revision, plan, work
  attempt, contract, source, and builder profile.
- [x] Seal privileged raw builder execution, cleanup, and reconciliation behind
  one-shot Store-issued capabilities before any public mutating loop.
- [x] Converge controlled dispatch from its durable command outcome after an
  ambiguous successful commit before any public mutating loop.
- [x] Configure immutable startup trust roots and a Linux atomic file-bundle
  resolver, selected by exact source reference and plan digest and reread at
  every current-authority gate.
- [ ] Extend current permits to checks, verifier dispatch, accepting `PASS`,
  and integration as those edges become executable.

This milestone is intentionally internal. `BuilderController` performs one
explicit build step and owns no poller, scheduler, public command, or retry
policy. Builder execution, bound-result cleanup, and unbound reconciliation now
cross only narrow Store-issued one-shot capabilities. Every value copy shares
the same atomic consumption state, and each synchronous worker callback retains
ownership until it returns, so successor recovery cannot overlap old external
work. Store owns the replayable proof which permits an exact unbound attempt to
return to pending. SQLite remains the only durable control truth; ownership,
permits, and worker-entry capabilities are process-local.

Controlled dispatch now resolves an exact durable outcome under active
ownership before fresh authority and in one bounded post-apply-error probe. The
caller must reuse its command ID across retry and restart. Replay observes
history; fresh dispatch and pending-build claim still require current authority.
The change reuses existing SQLite truth and adds no schema, framework,
scheduler, or runtime dependency.

The completed authority-source slice is internal resolver wiring, not an
authorizer or public command. It holds no signing capability and accepts no
prompt, helper command, environment, or stdin authority. See [Exact plan and
authenticated authority](authenticated-authority.md) and [ADR
0006](adr/0006-current-authority-controller.md).

The Codex-first feasibility gate in [ADR
0007](adr/0007-native-agent-boundary.md) now passes against a real static CLI and
a scripted, token-free Responses turn. The executor gained only two explicit
capabilities: one digest-pinned input may be the direct entrypoint, and nested
user namespaces require invocation plus executor admission. Ordinary
invocations retain the prior boundary. No agent adapter, engine path,
controller path, Store transition, or public command was added. Historical
replay and deterministic reviewable admission remain historical; current
authority gates external effects and transitions which grant effectful
capabilities.

## 4. Fresh independent verdict

- [x] Pass the pinned Codex native-agent boundary proof in ADR 0007 without
  adding an adapter or mutating the production engine, controller, Store, or
  CLI.
- [ ] Take the next production vertical through the built binary, real builder,
  current-authorized and recoverable checks, and deterministic admission to
  `reviewable`; do not expose a builder-only public command.
- [ ] Dispatch the independent verifier through a native CLI adapter.
- [ ] Keep authorizer capability and verifier identity outside builder scope.
- [ ] Bind each verdict to the exact dispatch, policy, submission, candidate,
  and evidence.
- [ ] Implement bounded retry epochs without treating `INCONCLUSIVE` as
  implementation failure.

## 5. Public loop and integration

- [ ] Compose the future public command with an external interactive or remote
  authorizer transport while keeping signing capability outside Sworn.
- [ ] Add manual latch release and compare-and-swap fast-forward integration.
- [ ] Pass the 18 Baton real-boundary cases through the built binary.

Recovery proof is necessary but not sufficient for unattended use or a
default-branch cutover. Native builder recovery, exclusive ownership, and
current build authority now pass internally. Current authority for every later
effect, independent verdict, bounded policy, integration, built-binary
conformance, real CLI configuration, and the public loop still gate either
decision.
