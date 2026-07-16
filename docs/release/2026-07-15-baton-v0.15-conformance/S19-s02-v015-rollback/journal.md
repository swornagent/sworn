# S19-s02-v015-rollback journal

## 2026-07-16T22:53:25+10:00 — Planned

- Mandatory ordinary rollback assigned after S02's fresh Gate-3 verification failure.
- Baseline: S02 start_commit `e61cb190736ee7483fb4ed1a993442b26ce3574c` (tree `c57285e3f652e5f49aa8bb15e3ba65249b4a3db8`).
- Current known envelope: 45 non-release semantic paths; the final envelope extends through this slice's verified implementation head.
- S20 is blocked until this slice is freshly verified and tree-equal.

## 2026-07-16T23:45:55+10:00 — Design TL;DR produced

- Entered `design_review`; no semantic path has been changed and `start_commit`
  remains unset until Captain review has acknowledged a PROCEED decision.
- The proposed proof derives the ordinary first-parent envelope dynamically
  through S19's final implementation head, then proves exact baseline
  mode/blob/absence equality while separately protecting the release-record
  root.
- Awaiting fresh Captain review before implementation.

## 2026-07-17T00:05:36+10:00 — Automatic Coach acknowledgement and Captain PROCEED

- Under the Coach's standing instruction to orchestrate this release, the
  Captain's `PROCEED` verdict in `review.md` is acknowledged. There are no
  `[escalate]` pins and no new Type-1 decision to seek.
- Apply pin 1 inline: before `implemented`, record a final Implementer
  maintainability PASS with a non-null `implementation_head`, run the
  envelope/equality checker at that exact head, and bind the fresh-verifier and
  S20-gate evidence to the same object identity.
- Apply pin 2 inline: preserve exact mode/blob/absence equality and independent
  fresh verification, acknowledging the byte-exact v0.13.1 parity precedent.
- The Captain's design-review LLM check is recorded as `NOT PASSED`; its two
  reported findings used a stale release-wt diff containing historical S01/S02
  changes. It is not claimed as a pass or used to weaken the S19 proof boundary.
- Proceed to `in_progress` only in a fresh Implementer session; that session
  must implement the accepted design and stop at `implemented` for fresh
  adversarial verification.

## 2026-07-17T00:47:14+10:00 — Implemented

- Derived the ordinary rollback envelope from live first-parent history through
  the sole semantic restoration commit
  `4b38887e666f7e4ab664bac4780535b080ad54eb`: 45 paths total, with 37 exact
  baseline blobs and eight exact baseline absences.
- Restored every derived semantic tuple to S02 start tree
  `c57285e3f652e5f49aa8bb15e3ba65249b4a3db8`; preserved S02 release evidence
  byte-for-byte and kept S20 planned/pending.
- Applied both Captain pins: the final Implementer maintainability PASS, report
  blob, proof checker, and future fresh-verifier gate bind to the same exact
  implementation head; equality includes modes, blobs, and absence.
- The committed Rule-6 checker and supported deterministic `sworn verify`
  first-pass gate pass. The final AC-satisfaction recheck passes after making
  the S20 transition require the full exact-head maintainability, proof-bundle,
  and fresh-verifier conjunction.
- No local Baton installation or S20 work was performed. Fresh adversarial
  verification remains required in a new `/verify-slice
  S19-s02-v015-rollback 2026-07-15-baton-v0.15-conformance` session; this
  Implementer intentionally stops at `implemented`.

### 2026-07-17T01:10:16+10:00 — fresh verifier

BLOCKED

Slice: `S19-s02-v015-rollback`

Reason: The independently configured semantic-coverage check returned blocking
F-01: AC-01 through AC-05 rely on `proof/check-rollback.sh`, but no
CI-enforced test executes and asserts its pass/fail semantics. A persistent
non-release test or CI hook cannot be added within the current contract because
AC-02 and AC-04 require exact non-release equality to the S02 start tree.

Proposed `spec.json` amendment: Amend AC-01 through AC-05 to name
`docs/release/2026-07-15-baton-v0.15-conformance/S19-s02-v015-rollback/proof/check-rollback.sh`
as the required executable integration test, require a fresh verifier to run it
against live Git history plus adversarial bad Git objects, and state explicitly
that no persistent non-release test or CI hook is required or permitted for this
historical rollback proof. Replace each AC `test_refs` entry with that executable
checker path and its named invocation.

Evidence: fresh verification re-ran build, uncached repository tests, vet,
whole-tree equality, deterministic proof verification, AC-satisfaction, and the
rollback checker. The checker rejected real-form synthetic blob drift, a
surviving added path, unrecognized merge provenance, authored/merge overlap,
later ordinary authority, and absent fresh-verifier evidence. The semantic
coverage LLM check itself returned `FAIL`/`F-01`.

Next: `/replan-release 2026-07-15-baton-v0.15-conformance`
## 2026-07-17T01:25:52+10:00 — Planner ratification of executable-proof contract correction

- Reconciled the fresh verifier's owner-track BLOCKED verdict at
  `d6bfd4578d367aa6bac2ed3243a6f2c909c183ce`: Gate 4b correctly found that
  AC-01 through AC-05 relied on the committed rollback checker without naming
  that executable integration proof or its adversarial execution boundary.
- The proposed amendment is factual but exposes a bounded checker defect: the
  committed checker blob `ba1ef2323418ffd9e019ef2602ec1a1149b743b1` allows
  only S19 lifecycle/proof records after `640396fa8cc319229d6f96dedfdbef65dbe317fe`.
  A normal post-start `spec.json` amendment would therefore fail closed. This
  ratification is an exception with exact evidence, not a mutable-spec waiver.
- `proof/contract-amendment.json`, validated by
  `proof/contract-amendment-v1.schema.json`, fixes the only permitted transition:
  `spec.json` blob `1e2f7a3ee70164320fa7dd30d6aba749fc5de47d` to
  `ae05d118cbb0eb47c6ad7595c25f93df5c14417e`, from the named blocked source,
  after release-wt head `deb090fcef736ca2e61b2c2136283beec4743e45`, under the
  exact planner commit subject `docs(release): ratify S19 executable rollback
  proof contract`.
- AC-01 through AC-05 now make `proof/check-rollback.sh` the required
  integration proof. The next Implementer must repair that checker to retain
  its original allowlist, accept only this exact reachable spec-blob transition
  after validating the amendment record, and reject every other post-start S19
  spec change while retaining byte-for-byte S02-record immutability.
- A fresh Verifier must run the repaired checker on live T1 history and against
  the six named disposable adversarial Git-object cases. No persistent
  non-release test, source, CI workflow, hook, or harness is permitted because
  the non-release tree must remain exactly equal to the S02 start tree.
- Returned S19 to `failed_verification` with `verification.result: pending` and
  owner `implementer`. Its immutable start commit, actual-file inventory, proof
  references, exact-head Implementer maintainability PASS, and S20 block are
  preserved. No production code, test code, checker implementation, or fresh
  verification was performed by this Planner replan.

## 2026-07-17T01:46:23+10:00 — Implementer BLOCKED: contract amendment omits rendered index lineage

- Resumed the failed-verification repair without changing S19's immutable
  `start_commit` `640396fa8cc319229d6f96dedfdbef65dbe317fe`. The generic start
  step was intentionally not used to re-anchor that rollback base.
- Reproduced the expected pre-repair checker failure at pinned semantic head
  `4b38887e666f7e4ab664bac4780535b080ad54eb`: its original allowlist rejects
  the ratified `S19/spec.json` transition. The proposed repair is otherwise
  mechanically concrete: the amendment pins
  `1e2f7a3ee70164320fa7dd30d6aba749fc5de47d` to
  `ae05d118cbb0eb47c6ad7595c25f93df5c14417e` under planner commit
  `c0d7d672fe14090655fea7db3f5bf0e22dfd29f9`.
- The same planner ratification changes the release-wide rendered
  `docs/release/2026-07-15-baton-v0.15-conformance/index.md`, which is outside
  the schema's exact six-entry S19-only `preserved_allowlist`. Its direct
  transition is parent `deb090fcef736ca2e61b2c2136283beec4743e45`, blob
  `8aeef3849541106b6cb5503daed1874b0fb4d31f`, to ratification blob
  `b3ba13ef1b9d958e67b035f662e39dd0175a82a7` at `c0d7d672...`.
- Live T1 propagation is also deterministically different from that direct
  planner blob: merge `c7d56c10f62c5583b5aeb27fda5aa9c8de50b81d` combines
  parent-one blob `37b0d6ebb72445802eab1ab336e3b4f5b7a8e7d5` and parent-two
  blob `b3ba13ef1b9d958e67b035f662e39dd0175a82a7` into rendered blob
  `1614b35035b685d7d9d3f9451c98fa350a91033f`. The merge result is not
  parent-two exact, so accepting it generically would weaken the original
  release-record and merge-provenance gates.
- The amended schema says to retain the original allowlist and admit only the
  S19 spec transition. It supplies no exact index path/blob/provenance
  exception. Implementing one here would modify the planner contract and
  violate AC-04's non-S19 release-record rejection.
- Required planner action: run `/replan-release
  2026-07-15-baton-v0.15-conformance` and amend only the contract record and
  its schema to authorize this one exact c0-originated index lineage (direct
  parent/blob, ratification commit, and exact live propagation render), then
  add that bounded identity to the allowlist. It must not create a generic
  `index.md` exception or relax any S02, S19-spec, semantic, or S20 gate.
- No checker, product code, S20 record, local installation, semantic rollback
  evidence, proof claim, or fresh-verifier claim was changed after discovering
  this contract gap. The existing 45-path evidence and final Implementer
  maintainability binding remain intact.

## 2026-07-17T01:59:17+10:00 — Planner correction: deterministic rendered-index condition

- Reconciled the live T1 BLOCKED handoff at
  `07c8d65d50e0400c8efa1cac61947d0aca215a08`. The blocker is factual: the
  original contract retained only S19 lifecycle/proof paths plus the first
  precise spec transition, even though the planner's required rendered board
  changed `index.md`.
- Preserved the observed history as audit-only evidence in the contract record:
  direct planner transition `deb090f...` / `8aeef3849541106b6cb5503daed1874b0fb4d31f`
  to `c0d7d67...` / `b3ba13ef1b9d958e67b035f662e39dd0175a82a7`, and propagated
  T1 merge `c7d56c1...` with parent-one
  `37b0d6ebb72445802eab1ab336e3b4f5b7a8e7d5`, parent-two
  `b3ba13ef1b9d958e67b035f662e39dd0175a82a7`, and result
  `1614b35035b685d7d9d3f9451c98fa350a91033f`. These blobs are not a current or
  future allowlist.
- The authoritative rule is executable and current: the repaired checker must
  create a disposable detached Git worktree at its current T1 HEAD, run
  `sworn render 2026-07-15-baton-v0.15-conformance
  <ephemeral-project-root>`, byte-compare the emitted `index.md` with the
  current committed index, then remove the copy without mutating the validated
  checkout or any release ref. An isolated run at `07c8d65...` correctly
  exposed the prior drift (`blocked` status versus `failed_verification` view),
  proving the condition is reproducible and fail-closed.
- The contract now permits exactly the original c0 S19 spec transition and this
  planner correction's second exact transition. It rejects arbitrary index
  bytes, every other S19 spec transition, and every other non-S19/non-lifecycle
  release record. S02 immutability remains unchanged.
- Returned S19 to `failed_verification` with `verification.result: pending` for
  a fresh Implementer. Preserve the immutable `start_commit`, actual-file
  inventory, existing proof bundle, exact-head maintainability PASS, and S20
  block. The next Implementer repairs only `proof/check-rollback.sh` and proof
  evidence; no production/test/CI harness, semantic rollback, or S20 change is
  authorized.
