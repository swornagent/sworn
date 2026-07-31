# Baton Conformance 1.0

Following Baton means the system actually protects the five handoffs. Merely
loading these documents, naming five agents, or drawing the workflow is not
enough.

The requirements below explain the behavior an implementation must prove.

## Portable protocol profile

Every implementation MUST:

1. preserve one externally approved plan lineage with stable release and slice
   identities;
2. bind each plan revision and compact responsibility receipt to exact immutable
   objects;
3. keep Planner, Implementer, Captain, Verifier, Merge, and external-authorizer
   authority distinct;
4. require an applicable Captain `PROCEED` before implementation;
5. derive repository, candidate, tree, ancestry, checks, and Merge facts rather
   than trust an unsupported claim;
6. start each Verifier thread in fresh context, keep every invocation
   read-only, and, when retaining one, enforce the
   [Protocol's direct-repair rule](PROTOCOL.md#direct-repair-continuation);
7. keep operational failure distinct from `FAIL` and `BLOCKED`;
8. retain unchanged slices across plan revisions and invalidate only changed
   slices plus the dependency closure whose consumed inputs changed;
9. append design and implementation attempts without erasing earlier Git
   history;
10. derive the board from the plan, receipts, and Git rather than store another
    lifecycle cursor;
11. reconcile duplicate dispatch, stale projection, interruption, and known Git
    effects without manufacturing trust facts; and
12. compose and integrate only exact candidates covered by current `PASS`; and
13. treat approved behavior, product surfaces, minimum checks, semantic limits,
    authority, and real product dependencies as commitments while keeping
    ancillary path discovery, additional checks, evidence correction, and
    procedural recovery as candidate or engine facts.

`.baton/releases` is structurally reserved for Baton metadata: product code,
build, test, package, deploy, hooks, and runtime MUST NOT read or depend on it;
only Baton's record writer may modify it; product identity ignores exactly this
directory; plan product scope cannot include it; and candidates preserve it from
their exact implementation base.

A consuming design MUST be reviewed against an exact base containing the
applicable producer `PASS` authorities. Before design and again before
implementation, the engine MUST prepare those current authorities in plan
order without moving any unrelated ref. Product-tree identity is the review
pin: a different or absent product requires a fresh design, while a new
candidate with the same product retains the review.

The portable receipt uses existing fields to make that guarantee explicit:
the consuming design `base` is the immediately prior valid track receipt or
current plan-install authority, `inputs` are its reviewed product pins, and the
design commit parent is the deterministic composition of that seed and those
producer `PASS` authorities. The consuming candidate `base` is its exact
implementation start.

Receipt serialization, branch names, record paths, worktree layout, locks, and
retry algorithms are reference-kit or engine choices. A conforming portable
representation remains strict, bounded, deterministic, and safe for untrusted
repository input.

## Guided profile

A guided implementation conforms when it presents exact plan bytes for external
approval, uses a distinct Captain, starts an independent Verifier thread fresh,
keeps every invocation read-only, records compact receipts through a machine
writer, and stops when a required trust fact cannot be established. Permitted
continuation follows the Protocol's direct-repair rule; starting a new thread
also conforms.

It may rely on a person to choose eligible operations and recover procedure.
Procedural recovery does not require another role decision when all applicable
trust facts already exist.

## Autonomous-engine profile

An autonomous engine additionally MUST demonstrate through its real binary and
boundaries:

- protected approval is unavailable to delivery roles;
- instructions, credentials, workspaces, and process lifetimes are isolated;
- at most one active writer owns a track;
- dispatch and effect identities are durable and write-once;
- resources are bounded;
- timeout, cancellation, crash, malformed output, and retry exhaustion never
  manufacture a Baton outcome;
- interrupted external effects reconcile before retry and complete
  idempotently;
- plan revision and consumed-input changes produce deterministic selective
  invalidation;
- projection replay yields the same state from the same plan, receipts, and
  Git; and
- track composition and release Merge recheck exact candidates and expected
  targets.

Provider names, prompt bytes, token counts, and internal event names are not
conformance requirements.

## Required cases

Positive cases cover plan approval and revision, stable slices, design
`PROCEED` and `REVISE`, implementation retry after `FAIL`, independent work
verification, fresh assembly `PASS`, exact consumed-input preparation,
exact composition, and final Merge. An implementation that retains Verifier
threads also covers permitted direct-repair continuation.

Recovery cases cover missing derived status, stale board output, duplicate
dispatch, runner interruption, skipped procedural cursor, and reconcilable Git
effects without a new model role or human approval. They also cover discovering
ancillary test or oracle paths, running additional focused checks, and
correcting evidence under the same approved plan and stable slice identity. An
implementation that supports exact-head refresh also covers the bounded
recovery defined by the
[Protocol](PROTOCOL.md#exact-head-refresh) without fabricating `FAIL`.

Negative cases cover missing or substituted approval; self-review; changed
bound plan, design, proof, or target; candidate or product-tree movement not
admitted and re-receipted by the
[exact-head refresh rule](PROTOCOL.md#exact-head-refresh); candidate movement
after Verifier dispatch; ambiguous authority; runtime events presented as role
outcomes; missing, stale, or ambiguous consumed authority; unsafe dependency
reuse; composition conflict; and forged Merge results. A material behavioral,
consumed-product, contract, authority, or external-decision change still crosses
the applicable decision boundary.

## Board and engine handoff

The reference oracle, terminal view, and WebUI consume the same bounded
projection. They may report runtime diagnostics but cannot create approval,
`PROCEED`, `PASS`, or `MERGED`.

Sworn is the reference autonomous implementation, not a privileged
interpretation of Baton.
