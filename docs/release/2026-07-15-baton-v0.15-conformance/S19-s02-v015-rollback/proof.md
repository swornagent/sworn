# S19-s02-v015-rollback proof bundle

## Scope

Restore every ordinary S02-authored semantic path to the immutable S02 start
tree `e61cb190736ee7483fb4ed1a993442b26ce3574c` (tree
`c57285e3f652e5f49aa8bb15e3ba65249b4a3db8`) without rewriting release
records, then bind the final Implementer maintainability PASS to the sole
semantic restoration commit `4b38887e666f7e4ab664bac4780535b080ad54eb`.

## Files changed

`git diff --name-only 640396fa8cc319229d6f96dedfdbef65dbe317fe HEAD` yields
the 45 dynamically derived semantic envelope paths plus this slice's status,
maintainability record, checker, proof bundle, and journal. The authoritative
complete list is the `files_changed` array in `proof.json`; no S02 record is in
that list.

## Test results

- `make build` — PASS (0)
- `go test ./... -count=1` — PASS (0)
- `go vet ./...` — PASS (0)
- `git diff --exit-code e61cb190736ee7483fb4ed1a993442b26ce3574c HEAD -- . ':(exclude)docs/release/2026-07-15-baton-v0.15-conformance/**'` — PASS (0)
- `proof/check-rollback.sh --head 4b38887e666f7e4ab664bac4780535b080ad54eb --require-maintainability` — PASS (0)

Full live command output is retained in
[`proof/validation-output.md`](proof/validation-output.md) and
[`proof/complete-rollback-envelope.txt`](proof/complete-rollback-envelope.txt).

## Reachability artefact

The executable checker is an integration-level Git proof: it dynamically
enumerates every first-parent non-merge path from S02 start through the pinned
implementation head, validates recognized merge-only input, compares exact
mode/blob/absence tuples, and checks the live S02/S20 lifecycle records. It
exited 0 at the exact head above. The built public CLI also exited 0 for
`./bin/sworn capabilities`; its configuration-safe output is captured in the
validation record.

## CompleteRollbackEnvelope

The checker reports 45 derived envelope paths: 37 baseline-present exact tuples
and eight baseline absences. Every tuple is printed in
[`proof/complete-rollback-envelope.txt`](proof/complete-rollback-envelope.txt),
including the baseline and implementation-head mode/blob values. Its final
summary is:

```text
ROLLBACK_CHECK PASS
BASE e61cb190736ee7483fb4ed1a993442b26ce3574c tree=c57285e3f652e5f49aa8bb15e3ba65249b4a3db8
IMPLEMENTATION_HEAD 4b38887e666f7e4ab664bac4780535b080ad54eb
ENVELOPE_PATHS 45 baseline-present=37 baseline-absent=8
```

## ExactStartTreeEquality

The same dynamic inventory checks the exact Git mode and blob OID for every
baseline-present path and exact absence for every S02-added path. It also runs a
whole-tree non-release diff backstop against the immutable S02 start commit.
Neither check allows a path allow-list to reduce the discovered envelope.

## FailClosedRollbackChecks

The checker refuses an un-restored S19 start head and refuses a later
release-record checkpoint as semantic authority. Captured negative output is in
[`proof/fail-closed-checks.md`](proof/fail-closed-checks.md). It also rejects an
unrecognized merge, parent-two semantic mismatch, authored/merge overlap,
mode/blob/absence drift, S02 record drift, or premature S20 state transition.

## PreservedReleaseHistory

At the S19 start checkpoint, S02 was already a deferred terminal record. The
checker compares the current track head to that checkpoint and fails if any S02
release-record byte changes. It permits only S19's lifecycle/proof records under
the physical release root, while the non-release whole-tree diff is exact to the
immutable S02 baseline.

## Delivered

- The complete dynamically derived S02 semantic envelope was restored exactly
  at `4b38887e666f7e4ab664bac4780535b080ad54eb`.
- The preservation and fail-closed proof is committed with live tuple evidence.
- The final Implementer maintainability PASS is bound to that exact commit via
  the report blob and status ledger.
- S02 remains rollback-backed/deferred and S20 remains planned/pending.

## Not delivered

An independent fresh Verifier PASS is intentionally not claimed here. Rule 7
requires a new `/verify-slice S19-s02-v015-rollback
2026-07-15-baton-v0.15-conformance` session, which must run the checker with
`--require-fresh-verifier` only after it records its own PASS. This is tracked by
S19's verification gate before `S20-v015-parity-portable-fixture`, and the Coach
acknowledgement is recorded in `journal.md`.

## FreshVerification

Fresh verification remains pending. The checker contains a strict
`--require-fresh-verifier` option that requires `state: verified`, a PASS verdict,
a fresh-context flag, and a verdict timestamp bound to the same implementation
head. It also allows S20 to leave planned/pending only after the complete AC-05
conjunction is present: the exact-head Implementer PASS/report binding, complete
proof bundle, and that independent fresh verifier evidence. Otherwise it fails
closed. It is deliberately not run as a passing condition by this Implementer.

## Divergence from plan

None.
