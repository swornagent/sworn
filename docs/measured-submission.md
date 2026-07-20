# Prepared local submission

Sworn can now structurally prepare one Baton `submission-v1` across the real
Git, executor, artifact, and protocol boundaries. This is an evaluation-only
construction path, not yet a reviewable submission source. No CLI or reducer
transition can admit it.

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
5. An unambiguous exit-zero completion returns only that canonical receipt with
   a `pass` outcome. The producer does not mint a second Baton check or evidence
   projection. Prepared construction rebinds its temporary Baton inputs to the
   stored receipt and exact definition; final admission will derive those facts
   from the journal instead of trusting a caller.
6. `protocol.BuildSubmission` takes an opaque `ExactPlan` and work ID. It
   derives target, scope, acceptance, assurance, contract, and authority facts
   from that capability; resolves the plan-selected `assurance-policy-v1`
   registry and its baseline definitions by digest; then rebinds approval,
   environment, receipts, streams, policy coverage, candidate, timestamps, and
   exact artifact bytes. It returns only the RFC 8785-canonical Baton record,
   not a second submission view or dependency list, but does not authenticate
   authority or prove that supplied run facts came from the effect journal.
7. Structural submission persistence and its parallel builder/producer run
   identity registry have been removed. The future final admission transaction
   will reverify the complete artifact closure and be the sole writer of the
   canonical submission and its global submission and work-attempt identities.

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
closes runtime-byte ambiguity without claiming a hermetic toolchain.

The internal `check.local` effect worker uses only `RunLocalContentBound`; there
is no host-runtime fallback. Its immutable request binds the builder effect,
check definition, and runtime-manifest digest. Its typed result contains only
the semantic outcome and receipt reference. The worker materializes the
candidate from the succeeded builder result, and the executor stages and
remeasures both the candidate workspace and exact runtime before execution.

Before the journal binds the result, the store matches the receipt's candidate
identifiers to that builder result, validates the immutable CAS closure for the
receipt, definition, environment, stdout, and stderr, matches the definition's
execution fields and measured runtime/output ceilings, and requires
runtime-manifest equality between the environment and request. It does not
repeat Git materialization or workspace/runtime
measurement, and it does not compare the environment's protocol-snapshot
digest. The effect row provides the invocation and attempt identity, avoiding a
second identity registry or duplicated result facts. Final admission remains
responsible for the complete protocol and submission closure.

Result binding and effect completion are separate journal transitions. The same
closure validator gates ordinary completion and reconciliation to `succeeded`.
Neither a missing result nor an otherwise valid receipt left in content-addressed
storage proves safe retry or successful execution.

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

The reducer can now schedule one bounded, policy-ordered check batch after the
store transaction rebinds the exact plan, policy, definitions, historical
approval, succeeded builder, and configured content runtime. Effect claims are
serial within that batch. The production binary still exposes no mutating
command or worker loop, and historical approval is not a current execution
permit. Admission must require the content-bound
environment explicitly; the host-runtime environment can never qualify. The
remaining path is one atomic admission that revalidates authority, typed journal
results, and the complete artifact closure. Structural consistency alone is not
provenance.

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
  manifest drift, producer-binding mutations, and missing/aliased artifacts;
  and
- repeatable empty-artifact handling and exact prepared-submission construction.

Typed builder and local-check results now bind to the effect journal, including
recovery through the same validator. Plan-derived check scheduling exists
internally; final atomic admission, current execution authority, and the public
mutating surface do not, so Sworn makes no unattended-delivery claim.

Exact plan parsing and historical signed approval are now implemented separately
from this structural path; see [Exact plan and authenticated authority](authenticated-authority.md).
