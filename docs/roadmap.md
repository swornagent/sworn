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
are internal only: no mutating CLI command is enabled, no effect executor exists,
and an authority receipt digest is not treated as proof that the receipt is
valid. Those capabilities remain gated below.

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
- [ ] Admit a reviewable submission only from authenticated authority,
  journal-registered runs, and a content-bound check runtime.
- [ ] Reconcile interrupted workspace and Git effects.

The Git-truth boundary is implemented internally on `feat/exact-local-candidate`.
It has no CLI mutation path and does not yet persist its binding in runtime
configuration or record a candidate in the control store. See
[Exact local candidate](exact-candidate.md).

The contained executor now implements distinct read-only and writable-export
modes over one Linux path. Writable work uses a fresh copy on a finite tmpfs,
live cgroup resource bounds, post-quiescence measurement, generation-bound
validation and digest-independent cleanup. It is not yet connected to an engine
effect or adapter; see [Contained executor](contained-executor.md).

The prepared Standard path now rechecks a retained candidate, runs one
policy-bound local producer through read-only containment, stores one reusable
content-addressed receipt, builds strict canonical Baton bytes, and reserves
submission and run identities transactionally. It remains evaluation-only: the
host runtime is unbound, authority is not authenticated here, and runs are not
yet journal-registered. See [Prepared local submission](measured-submission.md).

Exact-plan and historical approval capabilities now survive restart through the
single control store. This is not current effect authority: source re-resolution
before dispatch, accepting `PASS`, and integration remains milestone 4. See
[Exact plan and authenticated authority](authenticated-authority.md).

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

Only after recovery proofs pass may the project consider unattended use or a
default-branch cutover.
