# Baton reference records

These helpers save Baton's important facts as one plan and small receipt
commits. A receipt is a short, machine-written note that says what happened and
connects it to the exact Git work it covered.

Projects should call these helpers rather than ask an AI agent to construct the
machine-readable record itself.

- `receipts.mjs` parses `baton-plan/v2`, derives stable slice-contract digests,
  encodes canonical `receipt-v1` trailers, binds exact detail bytes, and rejects
  malformed or product-mutating receipt history.
- `git.mjs` provides the fixed Git execution, direct-ref capture, object reads,
  product-tree identity, deterministic composition, and compare-and-set
  boundary.
- `actions.mjs` exposes only plan revision, exact consumed-track-base
  preparation, receipt append, assembly preparation, and exact
  passed-candidate merge actions.

An engine passes role decisions and evidence to the action layer. Each action
captures the applicable refs, validates the plan, receipt, and immutable Git
bindings, creates at most one bounded commit or effect, and compare-and-set
updates only its declared ref. An exact retry returns the existing receipt
rather than duplicating it.

Product candidates remain ordinary product commits. Receipt commits have one
parent and exactly the same tree as that parent. Captain and Verifier decisions
therefore bind immutable Git objects without mixing metadata into the product
tree, and merge can advance the target only to the exact candidate covered by
the current PASS.

`.baton/releases` is reserved Baton metadata. Product code MUST NOT read or
depend on it, including from build, test, package, deploy, hooks, or runtime.
Product identity structurally ignores exactly this fixed non-symlinked
directory. Plan product scope cannot include it, candidates must preserve it from
their exact implementation base, and only the confined record writer may
modify it. The reference layer does not pretend to detect semantic reads.

Plan scope is a commitment to owned behavioral and product surfaces, not a
candidate-path allowlist. The action layer derives the complete candidate diff
from Git and binds the product tree. It accepts ancillary support paths that
remain inside the approved outcome; independent verification decides whether
the actual diff satisfies the unchanged contract or exposes a material change
that must stop. Required plan checks are the minimum, while the candidate and
Verifier receipt `checks` digests bind the complete results actually supplied,
including additional focused checks.

There is no hand-authored status cursor or proof bundle in the reference path.
The board derives responsibility from the newest internally consistent plan,
receipt history, dependency input pins, and Git topology. Missing cached board
state, duplicate dispatch, or an interrupted procedural effect is recovered by
rescanning; it cannot create approval, `proceed`, `pass`, or `merged`.

Plan revisions retain stable release and slice identities. Unchanged contracts
and unchanged consumed product-tree pins retain their PASS. Only a changed
slice and the actual dependency closure whose input pins changed require a new
attempt. `prepareTrackBase({ release, slice })` composes the plan-ordered
current producer PASS receipts into only the consumer track ref. It is
idempotent and compare-and-set verifies the release, target, producer, and
consumer refs before any move.

Consumed-track and assembly composition always try ordinary deterministic Git
ancestry first. Only an ordinary conflict may retry the exact passed delta from
an authority-derived product base built from the approved target, ordered prior
slice PASS products, and exact consumed PASS bindings. An ambiguous base or a
real product conflict stops without moving a ref.

Every newly appended consuming design records `base` as the exact
pre-composition track or release authority and `inputs` as the reviewed
product-tree pins; its receipt parent is the deterministic composition of that
seed and the plan-ordered producer PASS authorities. A consuming candidate
records its implementation-start `base`, which activates exact preparation,
authority-ancestry, and linear-work checks.

Legacy designs without the `base` plus `inputs` marker and legacy candidates
without `base` remain readable. Their immutable ancestry may still project
reviewed pins, but it does not claim the new exact-preparation guarantee.
