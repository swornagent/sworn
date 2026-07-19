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
6. `protocol.BuildSubmission` takes an opaque `ExactPlan` and work ID. It
   derives target, scope, acceptance, assurance, contract, and authority facts
   from that capability; resolves the plan-selected `assurance-policy-v1`
   registry and its baseline definitions by digest; then rebinds approval,
   environment, receipts, streams, policy coverage, candidate, timestamps, and
   exact artifact bytes. It constructs and RFC 8785-canonicalizes the Baton
   record, but does not authenticate authority or prove that supplied run facts
   came from the effect journal.
7. `store.PutSubmission` accepts only that opaque prepared capability. In one
   transaction it verifies the complete resolved artifact closure, writes the
   canonical record, and reserves global submission, delivery/work/attempt,
   builder-run, and producer-run identities. The structurally checked approval
   receipt cannot reserve or preempt an authority identity; only authenticated
   authority persistence can do that. Exact retries are idempotent, while
   rebinding an owned identity fails closed.

Records accept only strict I-JSON already in RFC 8785 canonical form. JSON and
`+json` artifacts must be strict I-JSON too, but retain their exact original
bytes and raw-byte digest. Empty artifacts remain valid empty SQLite BLOBs.

The local environment artifact binds the admitted Baton snapshot, Go runtime,
OS and architecture, executor probe, containment-policy version, all effective
resource and output limits, read-only access, and no-network mode.

`RunLocal` retains that original evaluation-only shape: host `/usr` is read-only
but its executable, libraries, interpreter, and subtools can drift.
`RunLocalContentBound` instead requires an opaque runtime capability, executes a
private staged copy, and records `sworn-local-environment-v2` with the exact
runtime manifest digest. The receipt binds that environment transitively. This
closes runtime-byte ambiguity without claiming a hermetic toolchain or journal
provenance. The current internal caller still supplies the capability; the next
typed effect request must bind the configured digest before this fact can be
admission-eligible.

## Initial capability and fail-closed behavior

This slice accepts only dependency-free Standard work with:

- no assurance packs;
- local `test` evidence at `component` or `assembled` boundary;
- no live observation, attestation, inherited environment, declared external
  input, or network; and
- one evidence declaration per required baseline check.

Evidence semantics come from the policy-selected definitions, never subprocess
output. The plan binds the registry's RFC 8785 canonical digest, so Sworn's CAS
requires that registry to be stored as canonical JSON at that digest. Definition
locators remain source/audit metadata; construction resolves their exact raw bytes
by digest. Mocked evidence is component-only. The approval receipt must be
strict, CAS-resolved, and structurally match the exact plan's digest, authority
source and digest, repository, target, and ordered grants. Authentication and
journal provenance are deliberately not claimed here.

A timeout, cancellation, or output overflow stores a `not_admitted` execution
receipt and creates no Baton check, evidence, or submission. A non-zero exit is
also retained as `not_admitted`, not relabeled as a Baton `fail`: the current
executor does not yet prove whether every non-zero status was a normal target
exit, signal, or resource termination. Typed termination and repair routing
belong to the effect/reconciliation path; ambiguity cannot spend a verifier run.

Before this path can feed the reducer, authenticated authority and
journal-registered builder/producer effects must become opaque engine-owned
capabilities. Admission must require the content-bound environment explicitly;
the host-runtime environment can never qualify. Structural consistency is
groundwork, not provenance.

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

The next admission work binds builder and producer results to the effect journal;
effect reconciliation then makes recovery durable.
Until that wiring exists, these primitives are unreachable from the public
mutating surface and Sworn makes no unattended-delivery claim.

Exact plan parsing and historical signed approval are now implemented separately
from this structural path; see [Exact plan and authenticated authority](authenticated-authority.md).
