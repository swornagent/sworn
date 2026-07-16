# Proof Bundle — S04-typed-reference-ambiguity

## Scope

Deliver C-02 typed-reference ambiguity checking, fail-closed generic check
identity binding, and retirement of generic `maintainability-review` across the
gate, CLI, and MCP.

## Files changed

Generated from `git diff --name-only 8e8d078a29ca1198c25e339bc40800070a0c3e56 HEAD`:

- `cmd/sworn/llmcheck.go`
- `cmd/sworn/llmcheck_test.go`
- `docs/release/2026-07-15-baton-v0.15-conformance/S04-typed-reference-ambiguity/journal.md`
- `docs/release/2026-07-15-baton-v0.15-conformance/S04-typed-reference-ambiguity/proof.json`
- `docs/release/2026-07-15-baton-v0.15-conformance/S04-typed-reference-ambiguity/proof.md`
- `docs/release/2026-07-15-baton-v0.15-conformance/S04-typed-reference-ambiguity/status.json`
- `internal/gate/llmcheck.go`
- `internal/gate/llmcheck_blocking_test.go`
- `internal/gate/llmcheck_test.go`
- `internal/gate/spec_ambiguity.go`
- `internal/gate/spec_ambiguity_test.go`
- `internal/mcp/lint.go`
- `internal/mcp/lint_test.go`
- `internal/spec/references.go`
- `internal/spec/references_test.go`
- `internal/spec/spec.go`
- `internal/spec/spec_test.go`

## Test results

An initial disposable detached worktree outside `/home/brad` was clean at
`cc1373f` but used the source repository's shared Git common-dir. With an empty
`GOFLAGS`, its binary-reachability builds and `make build` failed with Go's
`error obtaining VCS status: exit status 128`; its scoped and full test commands
therefore exited `1`, while `go vet ./...` exited `0`. This was a host
common-dir metadata observation, not accepted as a passing result.

To isolate source behavior, a complete bundle at `cc1373f` was verified and
cloned into an independent temporary Git repository outside `/home/brad`. Its
working tree was clean, `GOFLAGS` was empty, and the required ordinary commands
all passed:

| Command | Exit | Result |
| --- | ---: | --- |
| `go test ./internal/spec ./internal/gate ./internal/mcp ./cmd/sworn` | 0 | `spec` 0.138s; `gate` 0.308s; `mcp` 0.155s; `cmd/sworn` 165.449s |
| `go test ./...` | 0 | all packages passed; `cmd/sworn` 133.188s; `internal/baton` 108.926s |
| `go vet ./...` | 0 | clean |
| `make build` | 0 | built `bin/sworn` normally |
| `git diff --check 8e8d078a29ca1198c25e339bc40800070a0c3e56 HEAD` | 0 | live T1 track worktree clean |

Both temporary repositories were clean at `cc1373f` after testing. The detached
worktree was removed via `git worktree remove`; the independent clone and bundle
were removed. The live T1 branch and worktree remained clean at `cc1373f`.

## Reachability artefact

`cmd/sworn/llmcheck_test.go:TestSpecAmbiguityTypedReferencesBinaryReachability`
builds and invokes the `sworn` binary over a real temporary Git fixture with
typed file, sibling-slice, and contract references. The independent-clone scoped
test command above passed it. `TestGenericCheckIdentityBinaryReachability` and
`TestGenericMaintainabilityReviewRetiredWithoutDispatch` cover the public CLI
failure paths; `internal/mcp/lint_test.go` covers the registered MCP equivalents.

## Delivered

- AC-01: `internal/spec/references.go` and
  `internal/gate/spec_ambiguity.go` implement the dedicated resolver and report
  contract; the built-binary ambiguity reachability test passed.
- AC-02: `TestReferenceResolutionFailureMatrixBeforeDispatch` and
  `TestAmbiguityCheckRendersSafeUnresolvedReferenceAndSkipsUnsafe` prove unsafe
  failures suppress dispatch and safe unresolved artifacts are deterministic.
- AC-03: `TestDedicatedAmbiguityReportContractFailureMatrix` proves duplicate,
  cross-schema, overlapping-fingerprint, and contradictory reports fail closed.
- AC-04: `TestGenericReportCanonicalCheckIdentity`,
  `TestCheckIdentityMismatchFailsClosed`, and the binary identity test prove
  requested/emitted generic identity matching.
- AC-05: the CLI and registered MCP retired-maintainability tests prove exact
  non-success guidance with no dispatch or record mutation.

## Not delivered

- Fresh-context verification and a `verified` state transition are deliberately
  not delivered: Rule 7 prohibits implementer self-certification. Tracking:
  S04's verification state and Baton Rule 7; the Coach has acknowledged this
  separation.
- The dedicated maintainability lifecycle is not delivered because it belongs to
  `S13-maintainability-engine-cutover`; that ownership boundary is acknowledged
  in the S04 spec.
- S20 is not updated or unblocked because it owns its fixture work and requires
  a fresh S04 verifier PASS. Tracking:
  `S20-v015-parity-portable-fixture`; the Coach has acknowledged the gate.

## Divergence from plan

None.
