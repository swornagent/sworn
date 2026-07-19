# Prepared local submission

Sworn can now structurally prepare and persist one Baton `submission-v1` across
the real Git, executor, artifact, protocol, and SQLite boundaries. This is an
evaluation-only construction path, not yet a reviewable submission source. No
CLI, reducer transition, verifier, or effect worker can invoke it.

## One causal chain

The initial Standard path is deliberately narrow:

1. `repo.MaterializeCandidate` rederives candidate objects, parent, tree diff,
   changed paths, and retention-ref identity. It preflights explicit byte and
   entry ceilings, rejects symlinked candidate content, checks out the exact
   retained tree into a fresh plain workspace, and mints its structural
   manifest. A moved target branch does not rewrite that immutable candidate.
2. A `sworn-local-check-v1` definition is resolved by its exact raw-byte digest.
   It admits one explicit argv, no inherited environment, no inputs, no network,
   the workspace root as working directory, and one definition-owned evidence
   declaration.
3. `internal/producer` passes the minted manifest to the read-only contained
   executor. The executor stages, remeasures, and compares every entry, byte,
   type, and permission before starting the check. Candidate stdout and stderr
   are hostile opaque bytes; both are stored separately in SQLite.
4. Sworn stores one canonical `sworn-local-check-receipt-v1`. It binds the
   definition, producer run, candidate commit and tree, workspace manifest,
   local environment, exact argv, timestamps, exit and control flags, and both
   output artifacts.
5. An unambiguous exit-zero completion creates one Baton check and one
   acceptance-linked evidence entry. Both point to that same receipt. There is
   no duplicate evidence bundle and no human command string that could erase
   argv boundaries.
6. `protocol.BuildSubmission` structurally rebinds pre-admitted work, approval,
   definitions, environment, receipts, streams, policy coverage, candidate,
   timestamps, and exact artifact bytes. It constructs and RFC
   8785-canonicalizes the Baton record, but does not authenticate authority or
   prove that supplied run facts came from the effect journal.
7. `store.PutSubmission` accepts only that opaque prepared capability. In one
   transaction it verifies the complete resolved artifact closure, writes the
   canonical record, and reserves global submission, delivery/work/attempt,
   builder-run, and producer-run identities. Approval identity remains reusable
   only for the same exact receipt. Exact retries are idempotent; rebinding an
   identity fails closed.

Records accept only strict I-JSON already in RFC 8785 canonical form. JSON and
`+json` artifacts must be strict I-JSON too, but retain their exact original
bytes and raw-byte digest. Empty artifacts remain valid empty SQLite BLOBs.

The local environment artifact binds the admitted Baton snapshot, Go runtime,
OS and architecture, executor probe, containment-policy version, all effective
resource and output limits, read-only access, and no-network mode.

It also records an important reproducibility limitation: host `/usr` is the
runtime trust root and is not content-bound. Read-only mounting prevents check
writes, but the executable, libraries, interpreter, and subtools can drift.
Runtime pinning remains an assurance gate before unattended use.

## Initial capability and fail-closed behavior

This slice accepts only dependency-free Standard work with:

- no assurance packs;
- local `test` evidence at `component` or `assembled` boundary;
- no live observation, attestation, inherited environment, declared external
  input, or network; and
- one evidence declaration per required baseline check.

Evidence semantics come from pre-admitted policy facts, never subprocess
output. Mocked evidence is component-only. The approval receipt must be strict,
CAS-resolved, and structurally match the admitted plan, authority source,
repository, target, and builder grants. Authentication against a capability
outside autonomous write scope is deliberately not claimed here.

A timeout, cancellation, or output overflow stores a `not_admitted` execution
receipt and creates no Baton check, evidence, or submission. A non-zero exit is
also retained as `not_admitted`, not relabeled as a Baton `fail`: the current
executor does not yet prove whether every non-zero status was a normal target
exit, signal, or resource termination. Typed termination and repair routing
belong to the effect/reconciliation path; ambiguity cannot spend a verifier run.

Before this path can feed the reducer, authenticated authority and
journal-registered builder/producer effects must become opaque engine-owned
capabilities. A content-bound runtime is also required for reproducible
unattended assurance. Structural consistency is groundwork, not provenance.

## Proof

Tests cover:

- the complete Git candidate to canonical prepared-record chain, including a
  target move after capture and full artifact-closure reread/rehash;
- the real Linux containment boundary when systemd, Bubblewrap, cgroup v2, and
  user namespaces are available;
- retained non-pass receipts that cannot become evidence;
- strict JSON, RFC 8785 number and UTF-16 ordering vectors, and the admitted
  example submission digest;
- candidate-ref loss and explicit reconciliation, materialization ceilings,
  manifest drift, producer-binding mutations, missing/aliased artifacts, and
  global identity reuse; and
- repeatable empty-artifact and exact-submission persistence.

The next admission work binds authority and run provenance; runtime pinning and
effect reconciliation then make assurance reproducible and recovery durable.
Until that wiring exists, these primitives are unreachable from the public
mutating surface and Sworn makes no unattended-delivery claim.
