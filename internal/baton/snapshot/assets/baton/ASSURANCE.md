# Baton Assurance 1.0

Most work follows Baton's standard path. Risky work can require stronger proof
or an extra decision from the right person, but that does not make every other
delivery carry the same weight.

The five principles and responsibilities still apply to every delivery.

## Standard

Standard delivery requires:

- an exact externally approved plan revision;
- an Implementer design TL;DR reviewed by a distinct Captain;
- an exact candidate with acceptance-linked evidence and required checks;
- an adversarial Verifier thread that starts fresh, with every invocation
  read-only;
- exact composition and fresh whole-product verification when tracks combine;
  and
- expected-target Merge or an honest stop.

The evidence may live in tests, the diff, code, commits, raw outputs, or an
optional document. Its receipt MUST bind the exact immutable source. Naming a
check is not proof that it ran.

Required plan checks are a minimum, not a ceiling. An Implementer or Verifier
MAY run additional focused checks and bind their results as evidence without a
plan revision. A newly discovered check requires revision only when it exposes
a material change to the approved behavior, contract, product dependencies, or
authority.

## Heightened assurance

A plan or repository policy MAY require additional checks, evidence boundaries,
review questions, or external decisions for risky work such as security,
privacy, money, migration, public contracts, or hard-to-reverse architecture.

Heightened policy SHOULD define its deterministic trigger, required
observations, stronger evidence boundary, Verifier questions, and externally
owned decisions. Those requirements belong in the approved plan revision.

Standard delivery follows the
[Protocol's direct-repair continuation rule](PROTOCOL.md#direct-repair-continuation).
Heightened policy MAY require a new Verifier thread after every repair.

An engine or Verifier may request stronger assurance but cannot weaken approved
requirements. If the contract or authority must change, the Planner proposes a
forward-only revision under the same release and stable slice identities unless
the goal, target, or authority itself is replaced.

## Evidence admission

Shape-valid receipts and a board projection are not action authority. Guided
and autonomous systems resolve protected approval and clean Verifier-dispatch
evidence outside the candidate, verify provenance, and bind them to the exact
objects being acted on.

A work `PASS` covers one slice candidate. Assembly `PASS` separately covers the
exact composed candidates and complete product. Neither substitutes for the
other.

## Admission rule

A universal Baton requirement is justified only when removing it would break
trust for nearly every delivery. Otherwise it belongs in a deterministic
reference or engine invariant, a repository check, explicit heightened policy,
or nowhere when independent verification and cheap retry already contain the
risk.
