# Baton Conformance 1.0

Conformance is behavioral. Loading Baton prose, naming five agents, or drawing
a workflow does not establish it.

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
6. start verification in fresh context with read-only candidate access;
7. keep operational failure distinct from `FAIL` and `BLOCKED`;
8. retain unchanged slices across plan revisions and invalidate only changed
   slices plus the dependency closure whose consumed inputs changed;
9. append design and implementation attempts without erasing earlier Git
   history;
10. derive the board from the plan, receipts, and Git rather than store another
    lifecycle cursor;
11. reconcile duplicate dispatch, stale projection, interruption, and known Git
    effects without manufacturing trust facts; and
12. compose and integrate only exact candidates covered by current `PASS`.

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
approval, uses a distinct Captain, starts a fresh read-only Verifier, records
compact receipts through a machine writer, and stops when a required trust fact
cannot be established.

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
`PROCEED` and `REVISE`, implementation retry after `FAIL`, fresh work and
assembly `PASS`, exact consumed-input preparation, exact composition, and final
Merge.

Recovery cases cover missing derived status, stale board output, duplicate
dispatch, runner interruption, skipped procedural cursor, and reconcilable Git
effects without a new model role or human approval.

Negative cases cover missing or substituted approval; self-review; changed
plan, design, candidate, proof, product tree, or target; ambiguous authority;
runtime events presented as role outcomes; missing, stale, or ambiguous
consumed authority; unsafe dependency reuse; composition conflict; and forged
Merge results.

## Board and engine handoff

The reference oracle, terminal view, and WebUI consume the same bounded
projection. They may report runtime diagnostics but cannot create approval,
`PROCEED`, `PASS`, or `MERGED`.

Sworn is the reference autonomous implementation, not a privileged
interpretation of Baton.
