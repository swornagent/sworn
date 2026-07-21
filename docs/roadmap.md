# Sworn architectural v1 walking skeleton

Each milestone must remain runnable and preserve one control truth. Later work
may extend a record or reducer path, but may not introduce a second engine.

Architectural v1 is the greenfield kernel and schema generation, not Sworn's
SemVer major. v0.2.0 packages the completed control, candidate, authority, and
bounded builder-to-checks-to-`reviewable` path below. The next release line is
v0.3.0, beginning with fresh independent verification and bounded verdict
routing. Existing `*-v1` schemas and reference identifiers remain unchanged.

## 0. Protocol and repository boundary

- [x] Start from a fresh clone on the disconnected construction branch
  `release/v1.0.0`; package its first release as v0.2.0.
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

At this milestone the state machine and Store remained internal: no mutating
CLI command was enabled, and an authority receipt digest was not treated as
proof that the receipt was valid. Later milestones compose through the same
state machine and Store rather than changing that historical boundary.

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
- [x] Reconcile interrupted workspace, Git, and content-bound check effects with
  kind-specific, attempt-bound external evidence before any autonomous retry.

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

The Git-truth boundary and native builder worker are bound into immutable Store
configuration. The production Codex adapter and bounded command now compose
that same builder-to-checks-to-admission path; neither owns a second journal or
scheduler. See [Exact local candidate](exact-candidate.md).

The contained executor implements distinct read-only and writable-export modes
over one Linux path. Writable work uses a fresh copy on a finite tmpfs, live
cgroup resource bounds, post-quiescence measurement, deterministic attempt
ownership, process-shared serialization, and machine-proved cleanup. The
read-only content-bound path is used by the local-check effect worker. The
bounded production controller claims that exact policy-selected batch serially,
with fresh current authority for each pending check; see [Contained
executor](contained-executor.md).

The Standard path rechecks a retained candidate, runs policy-bound producers
through read-only content-bound containment, and stores canonical receipts. One
Store-owned admission transaction now derives checks and evidence from the
exact journal batch, revalidates the whole protocol and Git closure, and is the
sole submission writer. See
[Atomic reviewable submission](measured-submission.md).

Exact-plan and authenticated historical approval facts survive restart through
the single control store. Check scheduling and admission reload their exact
relational closure, but it is not current effect authority: source re-resolution
now occurs immediately before each builder or check execution. Accepting
`PASS` and integration remain later gates. See [Exact plan and authenticated
authority](authenticated-authority.md).

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

The plan-derived edge resolves one dependency-free Standard policy and all of
its definitions, rebinds the exact succeeded builder and process-configured
runtime, and creates the complete ordered check batch in one transaction. Work
becomes `checking` and claims serialize. The controller freshly authorizes and
executes each check. A second, deterministic historical transaction admits
reviewable only after every exact check passes and the complete authority,
artifact, runtime, snapshot, chronology, and Git closure revalidates.

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
- [x] Extend current permits to each pending local-check execution.
- [ ] Extend current permits to verifier dispatch, accepting `PASS`, and
  integration as those edges become executable.

`Controller` owns one explicit bounded convergence and no poller, scheduler, or
retry policy. Builder and check execution, bound-result cleanup, and unbound
reconciliation cross only narrow Store-issued one-shot capabilities. Every
value copy shares the same atomic consumption state, and each synchronous
worker callback retains ownership until it returns, so successor recovery
cannot overlap old external work. Store owns the replayable proofs which permit
only exact, machine-reconciled unbound attempts to return to pending. SQLite
remains the only durable control truth; ownership, permits, and worker-entry
capabilities are process-local.

Controlled dispatch now resolves an exact durable outcome under active
ownership before fresh authority and in one bounded post-apply-error probe. The
caller must reuse its command ID across retry and restart. Replay observes
history; fresh dispatch and pending-build claim still require current authority.
The change reuses existing SQLite truth and adds no schema, framework,
scheduler, or runtime dependency.

The configured authority source is a resolver, not an authorizer. It holds no
signing capability and accepts no prompt, helper command, environment, or stdin
authority. The bounded command consumes already published signed bundles; it
does not create approval. See [Exact plan and authenticated
authority](authenticated-authority.md) and [ADR
0006](adr/0006-current-authority-controller.md).

The Codex-first feasibility gate in [ADR
0007](adr/0007-native-agent-boundary.md) now passes against a real static CLI and
a scripted, token-free Responses turn. The executor gained only two explicit
capabilities: one digest-pinned input may be the direct entrypoint, and nested
user namespaces require invocation plus executor admission. Ordinary
invocations retain the prior boundary. That feasibility milestone itself added
no adapter or command; the following production vertical composes the exact
profile through the existing controller and Store. Historical replay and
deterministic reviewable admission remain historical; current authority gates
external effects and transitions which grant effectful capabilities.

## 4. Fresh independent verdict

- [x] Pass the pinned Codex native-agent boundary proof in ADR 0007 without
  adding an adapter or mutating the production engine, controller, Store, or
  CLI.
- [x] Compose the sole bounded `sworn run` through the exact production profile,
  current-authorized recoverable checks, and deterministic admission; expose no
  builder-only command.
- [x] Prove one successful ready-to-reviewable run at the built process boundary
  with the exact pinned Codex artifact; make any live-provider token cost
  explicit. The opt-in `gpt-5.4` release proof passed on 2026-07-21; its second
  process invocation converged without another model turn.
- [x] Add the narrow v3 executor capability required by a trusted verifier CLI:
  exact executable and credential, host network, nested sandbox, and a
  physically read-only candidate, without widening either existing entry point.
- [x] Add strict verifier-dispatch, model-assessment, and engine-stamped verdict
  contracts with Baton-derived pure cross-record validation. This establishes
  shape and binding, not durable admission or an executable verifier edge; see
  [Independent verifier protocol boundary](verifier-protocol.md).
- [ ] Dispatch the independent verifier through a native CLI adapter.
- [ ] Make verifier turns memoryless: use ephemeral Codex sessions, disable
  history persistence, ignore user configuration and rules, and never resume a
  prior session. Disable approval prompts with `-a never` while retaining the
  narrow nested workspace sandbox wherever CLI-managed authentication is
  mounted.
- [ ] Keep authorizer capability and verifier identity outside builder scope.
- [ ] Carry the pure verdict bindings through verifier-effect recovery and
  durable Store admission, including current artifact and authority closure.
- [ ] Implement bounded retry epochs without treating `INCONCLUSIVE` as
  implementation failure.

## 5. Public loop and integration

- [ ] Compose the future public loop with an external interactive or remote
  authorizer transport while keeping signing capability outside Sworn.
- [ ] Add manual latch release and compare-and-swap fast-forward integration.
- [ ] Pass the 18 Baton real-boundary cases through the built binary.

Recovery proof is necessary but not sufficient for unattended use. Native
builder and local-check recovery, exclusive ownership, current execution
authority, and a strict bounded-run configuration compose in v0.2.0 without a
second engine. The token-free real-Codex boundary proof and the separate live
`gpt-5.4` built-process delivery both passed; the latter reached `reviewable`
and then converged on a second invocation without another model turn.

v0.2.0 deliberately stops at `reviewable`. Independent verification, bounded
repair policy, integration, the 18 Baton real-boundary cases, and the public
loop remain later gates. v0.3.0 starts with the memoryless verifier and exact
verdict binding rather than widening the builder path.
