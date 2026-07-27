# Baton reference records

The reference record layer keeps authored protocol state to one plan and small
machine-written receipt commits.

- `receipts.mjs` parses `baton-plan/v2`, derives stable slice-contract digests,
  encodes canonical `receipt-v1` trailers, binds exact detail bytes, and rejects
  malformed or product-mutating receipt history.
- `git.mjs` provides the fixed Git execution, direct-ref capture, object reads,
  product-tree identity, deterministic composition, and compare-and-set
  boundary.
- `actions.mjs` exposes only plan revision, receipt append, assembly
  preparation, and exact passed-candidate merge actions.

An engine passes role decisions and evidence to the action layer; it does not
ask a model to construct protocol JSON. Each action captures the applicable
refs, validates the plan, receipt, and immutable Git bindings, creates at most
one bounded commit or effect, and compare-and-set updates only its declared
ref. An exact retry returns the existing receipt rather than duplicating it.

Product candidates remain ordinary product commits. Receipt commits have one
parent and exactly the same tree as that parent. Captain and Verifier decisions
therefore bind immutable Git objects without mixing metadata into the product
tree, and merge can advance the target only to the exact candidate covered by
the current PASS.

There is no hand-authored status cursor or proof bundle in the reference path.
The board derives responsibility from the newest internally consistent plan,
receipt history, dependency input pins, and Git topology. Missing cached board
state, duplicate dispatch, or an interrupted procedural effect is recovered by
rescanning; it cannot create approval, `proceed`, `pass`, or `merged`.

Plan revisions retain stable release and slice identities. Unchanged contracts
and unchanged consumed product-tree pins retain their PASS. Only a changed
slice and the actual dependency closure whose input pins changed require a new
attempt.
