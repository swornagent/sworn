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
