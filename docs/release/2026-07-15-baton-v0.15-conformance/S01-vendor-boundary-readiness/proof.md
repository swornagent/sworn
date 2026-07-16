# Proof Bundle: `S01-vendor-boundary-readiness`

Generated from live repository state on 2026-07-16. The machine-readable twin
is `proof.json` (`proof-v1`).

## Scope

Repair the public Baton vendor boundary so exact v0.15.1 candidates are
materialised and validated before mutation, VERSION participates in the same
durable transaction, recovery fails closed, check mode is mutation-free, and
valid results map only to public exits 0, 1, or 2.

## Files changed

The immutable slice base is
`5e16d2b54b0793381b246a9e7b9f1eb2c8e5cb18`.

```text
$ git diff --name-only 5e16d2b54b0793381b246a9e7b9f1eb2c8e5cb18..HEAD
cmd/sworn/baton.go
cmd/sworn/baton_test.go
docs/release/2026-07-15-baton-v0.15-conformance/S01-vendor-boundary-readiness/journal.md
docs/release/2026-07-15-baton-v0.15-conformance/S01-vendor-boundary-readiness/spec.json
docs/release/2026-07-15-baton-v0.15-conformance/S01-vendor-boundary-readiness/status.json
docs/release/2026-07-15-baton-v0.15-conformance/index.md
docs/release/2026-07-15-baton-v0.15-conformance/intake.md
internal/baton/diff_test.go
internal/baton/transform.go
internal/baton/transform_test.go
internal/baton/validate_schema.go
internal/baton/validate_schema_test.go
internal/baton/vendor.go
internal/baton/vendor_test.go
internal/baton/vendor_transaction.go
internal/baton/vendor_transaction_test.go
internal/baton/version.go
internal/baton/version_test.go
```

This bundle's `proof.json`, `proof.md`, final `journal.md`, and final
`status.json` are committed with the completion checkpoint and included in the
machine-readable inventory.

## Test results

```text
$ go test ./internal/baton ./cmd/sworn -count=1  # exit 0
$ go test ./...                                  # exit 0
$ go vet ./...                                   # exit 0
$ make build                                     # exit 0; bin/sworn built
$ git diff --check                               # exit 0
```

The targeted tests cover every apply and rollback position across mapped files
and VERSION; recovery tamper, type, mode, traversal, identity, foreign-path,
and missing-material cases; public clean, drift, invalid, apply, rollback,
recovery, positional-flag, upstream-before-network, VERSION, and mode-only
outcomes; exact lexical script boundaries; and exact schema path semantics.

## Reachability artefact

- **Type:** CLI run through the built public command.
- **Command:** `bin/sworn baton vendor /home/brad/projects/baton --check`
- **Result:** exit 1 with only this deterministic 17-path drift list:

```text
changed: internal/adopt/baton/README.md
changed: internal/adopt/baton/architecture.json
changed: internal/adopt/baton/rules/07-adversarial-verification.md
changed: internal/adopt/baton/rules/10-customer-journey-validation.md
changed: internal/baton/schemas/board-v1.json
changed: internal/baton/schemas/llm-check-report-v1.json
changed: internal/baton/schemas/slice-status-v1.json
changed: internal/baton/schemas/spec-v1.json
changed: internal/prompt/baton/README.md
changed: internal/prompt/baton/llm-checks/README.md
changed: internal/prompt/baton/llm-checks/maintainability-review.md
changed: internal/prompt/baton/llm-checks/spec-ambiguity.md
changed: internal/prompt/baton/rules.md
changed: internal/prompt/baton/track-mode.md
changed: internal/prompt/implementer.md
changed: internal/prompt/planner.md
changed: internal/prompt/verifier.md
```

`cmd/sworn/baton_test.go:TestBatonVendorAtomicPreflightReachability` also
drives the same public command boundary through clean, drift, invalid-source,
apply-failure, incomplete-rollback, recovery-only, positional `--check`,
upstream-before-network, VERSION-drift, and mode-only cases.

## Proof-bundle first pass

```text
$ git diff 5e16d2b54b0793381b246a9e7b9f1eb2c8e5cb18 |
    bin/sworn verify -verifier-model claude-cli/sonnet \
      -spec docs/release/2026-07-15-baton-v0.15-conformance/S01-vendor-boundary-readiness/spec.json \
      -diff - \
      -proof docs/release/2026-07-15-baton-v0.15-conformance/S01-vendor-boundary-readiness/proof.json
{
  "verdict": "PASS",
  "rationale": "",
  "cost_usd": 0
}
# exit 0
```

The keyless CLI model identifier satisfies the pre-cutover command's model
construction; the deterministic first pass returned before any model dispatch.
The current default Anthropic construction was also attempted and failed closed
before verification because no API key is configured; no PASS is inferred from
that unavailable path.

## Delivered

- **AC-01:** lexical script detection accepts
  `board.json.shared_touchpoints` prose and rejects exact unmapped shell,
  Python, and module-script tokens before write. Evidence:
  `TestTransformScriptReferenceLexicalBoundaries`.
- **AC-02:** the untouched v0.15 board schema compiles through the bounded
  ECMA-pattern adapter with all named path and line-terminator semantics.
  Evidence: `TestCompileV015BoardSchemaWithoutSemanticWeakening`.
- **AC-03:** one instant constructs VERSION before mutation; every candidate
  shares ordered snapshot, atomic replacement, rollback, verification, and
  Git-admin-confined recovery authority. Evidence:
  `TestVendorTransactionFailureRestoresPrimaryWorktree`,
  `TestVendorRecoveryRecordRejectsUntrustedMaterial`, and
  `TestUpstreamPinReplacementUsesCapturedInstant`.
- **AC-04:** the public command exposes only exits 0, 1, and 2; check mode is
  mutation-free and diagnostics are path-only. Evidence:
  `TestBatonVendorAtomicPreflightReachability` and the live CLI run above.
- Candidate order is deterministic and linear through an MSD byte-radix pass,
  with no second hardcoded mapping authority for S02 to update.

## Not delivered

- **Exact v0.15.1 content, VERSION pin, and local Codex/Claude mirror update.**
  Why: the Coach-ratified boundary confines S01 to machinery and proof.
  Tracking: `S02-v015-parity-and-installs`. Acknowledged by the Coach in the
  design review and replan.
- **Automated v0.15 maintainability authority for this bootstrap slice.** Why:
  the conformant engine does not exist until cutover, so a current PASS cannot
  be manufactured. Tracking: `S13-maintainability-engine-cutover`.
  Acknowledged by the Coach's staged-bootstrap decision.

## Divergence from plan

- Complete recovery authority is published before the first replacement, not
  only after an incomplete rollback, so process death during apply is also
  recoverable under the same contract.
- Verified recovery authority is retired with one atomic whole-root rename;
  the next write scrubs deterministic staging or retired debris and returns
  exit 2 for a clean rerun after recovery maintenance only.
- Destination order uses an MSD byte-radix pass instead of comparison sorting,
  preserving the explicit linear-complexity contract without a second static
  path list.
