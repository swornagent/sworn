# Baton Conformance 1.0

Conformance is behavioral. Loading Baton prose into a prompt, using named roles,
or reproducing a particular Git workflow does not make an engine conforming.
An engine conforms only when it validates the current schemas and passes every
published model and engine scenario.

## Required behaviors

A conforming engine MUST demonstrate that it:

1. strictly parses, schema-validates, and content-addresses every record and the
   referenced assurance policy before dispatch;
2. rejects duplicate identifiers, dangling references, missing dependencies,
   and plan cycles;
3. authenticates approval using an authorizer capability unavailable to the
   caller and runners, rejects forged or runner-writable source and proof bytes,
   and records a receipt binding the exact plan digest, grants, authorizer, and
   approval time;
4. applies effective authority as the intersection of plan grants, the resolved
   receipt, and local policy; local policy can restrict but never grant;
5. derives actual Git facts, rejects unauthorized paths or effects, bounds host
   resources, and captures a clean exact candidate;
6. runs required checks and observations through registered producers in a fresh
   candidate materialization and durably stores their content-addressed receipts;
7. refuses builder-self-stamped evidence or verdicts and keeps verifier control
   input independent of candidate-local runner configuration;
8. requires acceptance-linked, candidate-current evidence before `PASS`;
9. keeps `FAIL`, `SPEC_BLOCK`, and `INCONCLUSIVE` semantically distinct;
10. invalidates a verdict when a bound contract, candidate, base, policy, or
    authority fact changes;
11. integrates only a same-repository descendant by fast-forward compare-and-swap,
    records the exact effect, and never overwrites a moved target;
12. derives board state from durable facts rather than accepting a status edit;
13. banks completed facts, preserves them across later authority changes, and
    resumes pending external effects idempotently; and
14. never projects `verified` or `integrated` when persistence or the claimed
    effect failed.

## Strict JSON and digests

Baton records and assurance policies are I-JSON. Engines MUST reject duplicate
object names, invalid Unicode scalar values, non-finite numbers, and integers
outside the exactly interoperable range `[-9007199254740991, 9007199254740991]`.
All current schema `format` annotations, including `date-time`, are assertions,
not optional hints. Baton's date-time profile excludes lexical leap seconds:
the seconds component MUST be `00` through `59`.

Baton record and policy digests use RFC 8785 JSON Canonicalization
Scheme bytes and SHA-256 encoded as `sha256:<64 lowercase hex characters>`. An
extracted work-contract digest covers its object in `delivery-plan-v1.work`.
Artifact-pointer digests cover exact raw bytes, including whitespace and a final
newline. Media types in Baton records are canonical lowercase ASCII without
parameters. An artifact declared `application/json` or with a `+json` structured
syntax suffix MUST parse as strict I-JSON, including duplicate-name rejection,
but its bytes are not reserialized.

## Assurance policy

The plan's policy reference MUST resolve to `assurance-policy-v1` and match its
canonical digest. Baseline check IDs are unique, and every entry in the non-empty
`checks` array resolves exactly once. Each `application/json` definition matches
its raw-byte digest, parses strictly, and supplies stable engine- or
project-defined semantics for that ID.

Pack IDs are unique, but only packs selected by the work contract are required to
resolve. Each selected definition resolves exactly once, matches its raw-byte
digest, and parses strictly. An unavailable unselected pack does not invalidate
otherwise conforming Standard work.

## Authority

An authority receipt is the strict `authority_approval` variant of
`control-receipt-v1`. The authority digest covers the plan's complete
`authority` object; the source digest covers the resolved source policy. The
receipt locator and raw-byte digest must match before dispatch, accepting
`PASS`, and a pending integration.

The engine MUST re-resolve the source and check digest, validity, and revocation
before builder dispatch, verifier dispatch, accepting `PASS`, and integration.
Integration additionally requires an `integrate` grant whose repository and
full target ref exactly match the plan. A command, config file, or UI action
without that grant cannot enlarge authority.

If current authority fails after submission but before a `PASS` is accepted, the
engine does not admit that success, retains the immutable submission, keeps its
row `reviewable`, raises a delivery-level attention latch, and dispatches no
further effect. A non-PASS result may still be banked as a truthful review
finding, but it cannot authorize an effect. The engine never manufactures
`SPEC_BLOCK`; that outcome requires a verifier result.

A successful integration effect receipt is the strict `integration` variant of
`control-receipt-v1`. It is immutable and content-addressed. Later expiry,
revocation, or policy change does not erase that completed fact. New or pending
effects still re-resolve current authority and fail closed.

## Repository and scope

Repository identity, full target ref, base commit, candidate commit, and tree are
bound facts. The engine MUST prove that base and candidate are objects in that
repository, base is an ancestor of candidate, and `changed_paths` exactly equals
their tree diff. Rename checks include both source and destination.

Scope uses the literal prefix semantics in `PROTOCOL.md`: case-sensitive Git
paths, exact-or-descendant matching, `.` as whole repository, and exclusions
winning. Symlink targets, submodule contents, or external inputs used by checks
must be separately bound by policy; path scope alone does not authorize them.

## Freshness and evidence

The engine creates a distinct verifier dispatch only after the submission is
immutable. It supplies no builder transcript and links the strict
`verifier_dispatch` control receipt to the verdict. Verifier instructions,
plugins, hooks, tool discovery,
and capabilities come from an immutable engine-controlled context. The candidate
is exposed as read-only review data; candidate-local runner instructions or
configuration are not activated. The workspace has no writable target refs,
remotes, or inherited write credentials. The engine rechecks candidate identity
and cleanliness after review.

Every builder, producer, and verifier subprocess runs with local-policy limits on
process count, memory, CPU, output, wall time, and writable temporary storage.
Exhausting a limit is a typed control or environment failure, never success.

Evidence boundary order is `component < assembled < live`:

- `component` exercises an isolated leaf;
- `assembled` enters through the product integration point from a clean
  candidate; and
- `live` observes a candidate-revision-bound deployed or operational instance.

Evidence meets an acceptance criterion only at the same or a stronger boundary.
Evidence declaring mocks cannot satisfy `assembled` or `live`. Every evidence and
check receipt binds the candidate tree, producer run, capture time, concrete
environment reference, durable artifact locator, and artifact digest. The bytes
must still resolve and match immediately before `PASS` and integration.

Every `producer_run_id` names an engine-registered run represented in the
submission's `checks`; it MUST NOT name the builder run. A live observation uses
a controlled observer registered for that boundary. An attestation producer
records the attester identity and admits exact supplied bytes; its passing check
means admission succeeded, not that the attested claim is true. The verifier
still assesses sufficiency.

If a required baseline check fails, the engine banks the receipt and routes a
bounded builder repair before submission. It does not spend a verifier run or
create a delivery verdict for an already-known deterministic failure.

Before accepting `PASS`, the engine also confirms:

- work and acceptance IDs are unique within the plan; `(work ID, attempt)` is
  unique across submissions; check and evidence IDs are unique within the
  submission; finding and pack IDs are unique within the verdict; submission and
  verdict IDs are globally write-once; approval-receipt IDs, builder, producer,
  verifier-dispatch, and integration-effect identities are not reused for
  different bytes or effects; the same exact approval receipt MAY be referenced
  by several submissions;
- every contract acceptance appears exactly once with `pass` and at least one
  valid evidence reference;
- every policy-required check is present exactly once and passes;
- policy locator and digest exactly match the referenced plan;
- every required versioned pack appears exactly once with `pass` and its required
  evidence;
- all acceptance, pack, and evidence references resolve without ambiguity;
- no blocking finding exists; and
- verifier run and dispatch differ from the builder run.

## Board projection

Every plan work ID appears exactly once on the board. Row state, exact submission
and verdict identifiers and digests, and next action obey the lifecycle matrix.
`attention` is a pre-submission control stop; `blocked` requires a bound
`SPEC_BLOCK` verdict. A current `PASS` without the plan's exact integration grant
is `verified` with `replan`; with that grant but a pending local or manual latch,
it is `ready_to_integrate` with `integrate`.
For repeated verification of one submission, durable engine event order selects
the current write-once verdict; record timestamps do not. An integrated row's
effect receipt MUST bind the exact projected submission and current verdict.
Verifier transport failure creates no verdict and remains `reviewable`; `retry`
requires a bound `INCONCLUSIVE` verdict.
A post-submission authority or policy stop leaves the row's factual state intact
and raises delivery-level `attention`. Delivery state is aggregated from rows,
effect receipts, and observed Git facts. A row is `integrated` only when a valid
effect receipt exists and its exact candidate equals or is an ancestor of the
observed target; later serial candidates do not erase that result. Delivery
`integrated` means every required row is integrated. Regenerating a board from
the same durable source revision is byte-for-byte deterministic; render time is
not a semantic field, and edited boards are never commands.

Aggregation is exact: a non-empty delivery-level attention latch or any
`attention`/`blocked` row yields `attention`; all `waiting` yields `planned`; all
`integrated` yields `integrated`. `integrating`, `ready_to_integrate`, or
`verified` is projected only when every row is either that state or `integrated`
and at least one row has that state. Every other mixture is `active`.

## Conformance artifacts

`conformance/manifest.json` indexes three classes: schema fixtures, executable
cross-record model cases, and real-boundary engine cases. The local checker runs
the first two. A delivery engine must additionally run every engine case through
its real binary, Git implementation, persistence store, and subprocess boundary.

Sworn is the reference implementation, not a privileged interpretation. Prompt
text, command names, model choice, token counts, directory layout, and internal
state names are non-normative.
