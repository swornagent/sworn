# S19-s02-v015-rollback proof bundle

## Scope

Preserve the ordinary S02 rollback from immutable start tree
`e61cb190736ee7483fb4ed1a993442b26ce3574c` (tree
`c57285e3f652e5f49aa8bb15e3ba65249b4a3db8`) without rewriting release
records, then repair the accepted AC-04 history gap. The repair retains the
final-tree S02 backstop and rejects every S02 release-record change on the T1
first-parent path from S19 start through current HEAD. It also validates the
immutable amendment schema/record, allows only the two declared S19 `spec.json`
transitions, and admits `index.md` only if a detached current-head `sworn render`
reproduces its committed bytes.

## Files changed

`git diff --name-only 640396fa8cc319229d6f96dedfdbef65dbe317fe HEAD` yields 59
paths: the immutable 45-path semantic rollback inventory plus the S19
lifecycle/proof records, the two planner-ratified contract records, and the
generated release index. The authoritative complete list is the
`files_changed` array in `proof.json`; no S02 record is in that list.

## Test results

- `make build` — PASS (0)
- `go test ./... -count=1` — PASS (0)
- `go vet ./...` — PASS (0)
- `git diff --exit-code e61cb190736ee7483fb4ed1a993442b26ce3574c HEAD -- . ':(exclude)docs/release/2026-07-15-baton-v0.15-conformance/**'` — PASS (0)
- `proof/check-rollback.sh --head 4b38887e666f7e4ab664bac4780535b080ad54eb --require-maintainability` — PASS (0)
- `proof/check-rollback.sh --head 4b38887e666f7e4ab664bac4780535b080ad54eb --require-maintainability --require-proof-bundle` — PASS (0)
- `bash -n proof/check-rollback.sh` — PASS (0)
- detached current-head `sworn render` byte comparison — PASS (0; performed by the checker without modifying a release ref or the validated checkout)
- disposable adversarial Git objects — PASS: AC-03's contract-correct descendant-head invocation rejected non-zero; an S02-record mutation followed by byte restoration rejected non-zero; release refs and worktrees were unchanged after cleanup
- deterministic `sworn verify` proof-bundle gate with the live S19 diff — PASS (0)
- `sworn llm-check -type ac-satisfaction ...` — PASS (0; one non-blocking handoff observation)

`sworn lint coverage --slice S19-s02-v015-rollback --release
2026-07-15-baton-v0.15-conformance --base
640396fa8cc319229d6f96dedfdbef65dbe317fe --json` is explicitly **not** a
passing gate for this slice: it exits 2 while trying to scan the intentionally
absent S02 test path `internal/baton/install_transaction_test.go`. The
planner-ratified amendment and AC-02/AC-04 prohibit restoring or replacing that
path with a persistent non-release harness. `check-rollback.sh` is therefore
the contract-owned executable integration proof; this exception is not a
weakened coverage claim.

Full live command output is retained in
[`proof/validation-output.md`](proof/validation-output.md) and
[`proof/complete-rollback-envelope.txt`](proof/complete-rollback-envelope.txt).

## Reachability artefact

The executable checker is an integration-level Git proof: it dynamically
enumerates every first-parent non-merge path from S02 start through the pinned
implementation head, validates recognized merge-only input, compares exact
mode/blob/absence tuples, validates the immutable contract record against its
schema, and checks the live S02/S20 lifecycle records. It also renders the
release board only inside a disposable detached current-head worktree and byte
compares that output to committed `index.md`. It exited 0 at the exact head
above. The built public CLI also exited 0 for `./bin/sworn capabilities`; its
configuration-safe result is captured in the validation record.

## Captain adjudication

`CAPTAIN-VERDICT: PROCEED`. The historical verifier’s first later-authority
claim is retained as historical evidence but does not establish an AC-03 or
AC-04 breach: it pinned `--head` to the implementation commit while executing
from an adversarial descendant. AC-03 explicitly requires `--head
<adversarial-descendant>`; the contract-correct disposable run at
`bdef578b3fce9e7327dad448704531c870724c91` exited non-zero. The mutation then
byte-restoration of an S02 record is the accepted AC-04 defect. The repair
therefore adds only first-parent S02-record transition inspection, while keeping
the final-tree check and ignoring parent-two-only propagation differences.

## ProofBundleVerificationGate

The role prompt's positional reference command was run first:

```text
$ sworn verify S19-s02-v015-rollback 2026-07-15-baton-v0.15-conformance
{
  "verdict": "BLOCKED",
  "failed_gate": "first_pass:spec",
  "rationale": "no path provided",
  "cost_usd": 0
}
exit: 2
```

The installed binary exposes the supported flag interface rather than positional
slice/release lookup. Its deterministic first pass was therefore run with the
same live S19 evidence and did not dispatch an agentic verifier:

```text
$ git diff --no-ext-diff 640396fa8cc319229d6f96dedfdbef65dbe317fe | \
    sworn verify --spec docs/release/2026-07-15-baton-v0.15-conformance/S19-s02-v015-rollback/spec.json \
      --diff - \
      --proof docs/release/2026-07-15-baton-v0.15-conformance/S19-s02-v015-rollback/proof.json
{
  "verdict": "PASS",
  "rationale": "",
  "cost_usd": 0
}
exit: 0
```

This working-tree form was rerun after the `implemented` status transition and
generated release index were present, so the first pass covers the complete
handoff diff. It remains a structural preflight, not a fresh-verifier verdict.

For the Captain-authorized first-parent S02 history repair, the same supported
flag invocation returned `PASS` before the `implemented` transition. Its live
output is retained in
[`proof/validation-output.md`](proof/validation-output.md#first-parent-s02-record-history-repair).
It remains a structural preflight, not a fresh-verifier verdict.

The structural PASS is a Rule-6 proof-bundle preflight only; it does not change
the independent fresh-verifier requirement.

## CompleteRollbackEnvelope

The checker reports 45 derived envelope paths: 37 baseline-present exact tuples
and eight baseline absences. Every tuple is printed in
[`proof/complete-rollback-envelope.txt`](proof/complete-rollback-envelope.txt),
including the baseline and implementation-head mode/blob values. Its repaired
summary also records the amendment and deterministic-index gates:

```text
ROLLBACK_CHECK PASS
BASE e61cb190736ee7483fb4ed1a993442b26ce3574c tree=c57285e3f652e5f49aa8bb15e3ba65249b4a3db8
IMPLEMENTATION_HEAD 4b38887e666f7e4ab664bac4780535b080ad54eb
ENVELOPE_PATHS 45 baseline-present=37 baseline-absent=8
CONTRACT_AMENDMENT PASS schema=b62d48f698059fc0151ea0a3b9da18dfe1e507f5 record=9e298676129ee628714ffa80caa8c02bcea244f7
S19_SPEC_HISTORY PASS first=c0d7d672fe14090655fea7db3f5bf0e22dfd29f9 second=2c25021305b62d4b1e1f75bf1c7e0e6db504651b
S02_RECORD_HISTORY PASS first-parent-commits=19
RENDERED_INDEX PASS
```

## ExactStartTreeEquality

The same dynamic inventory checks the exact Git mode and blob OID for every
baseline-present path and exact absence for every S02-added path. It also runs a
whole-tree non-release diff backstop against the immutable S02 start commit.
Neither check allows a path allow-list to reduce the discovered envelope.

## FailClosedRollbackChecks

The checker refuses an un-restored S19 start head and refuses a later
release-record checkpoint as semantic authority. Captured negative output is in
[`proof/fail-closed-checks.md`](proof/fail-closed-checks.md). Live disposable
objects also reject arbitrary index bytes, all other S19 spec blobs, amendment
record tamper, an unrecognized merge/parent-two drift, authored/merge overlap,
mode/blob/absence drift, a surviving added path, an AC-03-correct later
authority descendant, a transient S02-record mutation even when its final bytes
are restored, and a premature S20 state transition.

## PreservedReleaseHistory

At the S19 start checkpoint, S02 was already a deferred terminal record. The
checker preserves that final-tree comparison and additionally compares every
post-start T1 first-parent commit to its parent one, failing on every S02
release-record transition. It deliberately does not compare merge parent two:
valid release propagation can differ there while retaining T1's exact S02 bytes.
It permits only S19's lifecycle/proof records under the physical release root,
the two exact schema-validated S19 spec transitions, and a generated `index.md`
whose isolated current-head render is byte-identical. The non-release whole-tree
diff remains exact to the immutable S02 baseline.

## Delivered

- The complete dynamically derived S02 semantic envelope was restored exactly
  at `4b38887e666f7e4ab664bac4780535b080ad54eb`.
- The preservation and fail-closed proof is committed with live tuple evidence.
- The final Implementer maintainability PASS is bound to that exact commit via
  the report blob and status ledger.
- The repaired proof validates both planner-pinned S19 spec transitions and
  their provenance, while arbitrary spec edits and amendment-record drift fail
  closed.
- The repair keeps the S02 final-tree check and rejects every changed S02 record
  on the T1 first-parent history, including a disposable mutation then exact
  restoration; valid parent-two-only propagation remains accepted.
- The release index is admitted only after a disposable detached render
  byte-compares it; arbitrary index bytes fail closed.
- S02 remains rollback-backed/deferred and S20 remains planned/pending.
- The supported deterministic `sworn verify` proof-bundle first pass passed.

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

The role prompt's positional `sworn verify <slice-id> <release>` reference is
not implemented by the installed CLI: it ignores positional values and blocks on
a missing `--spec` path. The same deterministic first pass passed through the
installed flag interface with the S19 spec, proof, and non-empty live diff after
the final status/index records were present. No agentic verifier was invoked and
no fresh-verifier evidence is claimed.

The generic `sworn lint coverage` scanner also cannot be used as a passing gate
for this historical rollback because it tries to scan an intentionally absent
S02 test path. The planner-ratified amendment explicitly names the committed
checker as the executable integration proof and prohibits a replacement
persistent non-release harness; the non-pass is captured above rather than
suppressed.
