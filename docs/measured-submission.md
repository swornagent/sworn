# Atomic reviewable submission

Sworn admits one dependency-free Standard work attempt as Baton `reviewable`
through a single Store-owned transaction. The command expresses only intent:

```json
{"work_id":"health-endpoint"}
```

Submission identity, digest, candidate, checks, evidence, timestamps, and
artifact references are derived from committed control truth. A caller cannot
submit those facts or invoke the pure reducer with a reviewable projection.

## One causal chain

The initial path is deliberately narrow:

1. An exact Baton plan selects one Standard work contract, canonical assurance
   policy, ordered local-check definitions, target, scope, and authority grants.
2. An authenticated historical approval and its complete source, proof, plan,
   and receipt closure are persisted immutably. This proves provenance; it is
   not a fresh permit to execute or integrate.
3. A succeeded builder effect binds its journal-derived run ID to the exact
   delivery, work attempt, contract digest, and retained Git candidate.
4. `checks.dispatch` re-resolves the plan and policy, rebinds the builder and
   configured content runtime, and creates the whole ordered check batch in one
   transaction. Work becomes internal `checking`; the public Baton board still
   reports `active`.
5. Before each pending `check.local` claim, the controller freshly resolves
   current authority for the exact work, builder, check definition, and runtime.
   The effect then runs serially over a freshly materialized candidate and exact
   staged runtime. Its typed result binds the semantic outcome to a canonical
   receipt. Receipt, definition, environment, stdout, and stderr are closed
   through raw CAS bytes before success can be recorded. An interrupted attempt
   can retry only after bound-result convergence or exact content-bound cleanup.
6. `submission.admit` anchors itself to the `checks.dispatched` event at the
   current revision. It reloads the complete effect batch by command and
   ordinal, requires every policy-selected check to be durably `succeeded` and
   semantically `pass`, and revalidates every typed result and artifact closure.
7. Admission reloads the exact plan and authenticated approval, rechecks
   approval grants and chronology, validates the embedded Baton snapshot and
   request-to-environment runtime binding, and asks the configured repository
   to rederive immutable Git objects, parent, tree, changed paths, scope, and
   candidate-retention facts.
8. `protocol.BuildSubmission` derives Baton checks and evidence from the
   policy-ordered receipts. It accepts no caller-projected check or evidence
   records and returns one RFC 8785-canonical `submission-v1` record.
9. The same SQLite transaction writes the accepted command, next engine state,
   `submission.admitted` event, canonical record, and journal-provenance identity
   row. It emits no effect. Only after all writes commit does the JSON board
   expose `reviewable`, the submission ID and digest, candidate commit, and
   `verify`.

An exact accepted-command replay returns the original result before performing
Git or proof work again. Reusing a command ID for different bytes fails closed.
A new identity collision is an error, never an implicit replay.

## Crash and storage semantics

After valid intent reaches the Store-owned gate, any gate or infrastructure
failure leaves work at `checking` and writes no admission command. Deterministic
invalid intent or state is instead a durable reducer rejection. A crash or
injected failure during admission leaves either every admission write or none
of them. The submission record may reuse identical immutable CAS bytes, but
`submission_records` is inserted strictly and is bound by foreign keys to the
same run, delivery, and applied admission command.

Schema v5 intentionally purges legacy `submission_records` rows created by the
removed structural writer. Those rows could not prove journal-backed admission
and are not promoted. Their canonical `records` bytes remain as unbound
archaeology.

## Initial capability

Reviewable admission supports only:

- one dependency-free Standard work contract without assurance packs;
- local `test` evidence at `component` or `assembled` boundary;
- a content-bound `sworn-local-environment-v2` runtime;
- no network, inherited environment, declared external input, live evidence,
  or attestation; and
- one policy-owned evidence declaration per required check.

A timeout, cancellation, output overflow, or non-zero exit remains a durable
non-pass receipt and cannot become submission evidence. Effect success alone is
insufficient: admission explicitly requires the typed semantic `pass` outcome.

The producer exposes only `RunLocalContentBound`; there is no host-runtime
producer fallback or public read-only host-runtime executor entry point. The
distinct writable builder path cannot create a qualifying v2 environment
receipt.

## What reviewable does not claim

The historical approval reloaded here is authenticated provenance, and
admission does not mint a current execution permit. The production controller
freshly authenticates the configured source immediately before builder and
check execution capabilities are granted. “Current” remains local: it proves a
fresh read not below the Store's highest observed signed version, not that an
external publisher did not withhold a newer source. The journal does not claim
a shared cryptographic chronology across independent source publication, effect
execution, and SQLite admission.

Admission performs its Git checks inside the Store operation, but Git cannot
participate in the SQLite commit. V1 therefore assumes exclusive engine
ownership of candidate-retention refs; a hostile concurrent process running as
the same host user is inside the trust boundary. The control database likewise
assumes Sworn is its sole writer.

`sworn run` is one bounded mutating command which stops here. There is still no
autonomous claim loop, independent verifier verdict, `PASS`, bounded repair
policy, multi-work scheduler, or integration edge. Those later gates must
consume this exact committed submission rather than create a second delivery
truth.

## Proof

Tests exercise real Git candidate capture and retention, authenticated plan
authority, public builder/check effect lifecycles, actual embedded snapshot
binding, ordered passing receipts, canonical submission persistence, board
projection, exact replay, non-pass and incomplete batches, missing repository
configuration, retention loss, schema migration, and injected identity-write
rollback followed by retry. The full suite also runs under the race detector;
the Linux capability suite exercises real Bubblewrap/systemd containment when
required by the test environment.

The production-command suite separately builds the real `sworn` binary and
checks its strict command and composition boundary without making a live OpenAI
model call. That is not evidence of provider model quality or an independent
verifier verdict.
