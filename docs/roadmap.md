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

- [ ] Bind immutable repository identity, base commit, target ref, and worktree.
- [ ] Add one contained subprocess executor with cancellation and process-tree
  cleanup.
- [ ] Produce a submission from measured Git facts and content-addressed local
  check evidence.
- [ ] Reconcile interrupted workspace and Git effects.

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
