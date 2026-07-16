# S19-s02-v015-rollback journal

## 2026-07-16T22:53:25+10:00 — Planned

- Mandatory ordinary rollback assigned after S02's fresh Gate-3 verification failure.
- Baseline: S02 start_commit `e61cb190736ee7483fb4ed1a993442b26ce3574c` (tree `c57285e3f652e5f49aa8bb15e3ba65249b4a3db8`).
- Current known envelope: 45 non-release semantic paths; the final envelope extends through this slice's verified implementation head.
- S20 is blocked until this slice is freshly verified and tree-equal.

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
