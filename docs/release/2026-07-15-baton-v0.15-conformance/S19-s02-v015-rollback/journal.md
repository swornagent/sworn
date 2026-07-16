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
