# Proof: S01 owner-ref status projection

## Scope

Board discovery projects exact committed owner-track status through canonical
Git documentation paths and reports matching topology and slice provenance in
aggregate and named CLI output.

## Files changed

The live diff from `68a578b10d9c8c69632aad96301e4fc04dff0de0` includes:

- `internal/board/discovery.go`
- `cmd/sworn/board.go`
- `cmd/sworn/board_test.go`
- `docs/captures/2026-07-18-owner-ref-status-projection-plan.md`
- this release's intake, board, design, review, acknowledgement, journal, spec,
  status, and proof records

## Test results

- EARS lint: PASS, 4 of 4 ACs well formed.
- Trace: PASS, 4 needs and 4 ACs traced.
- Requirements validation: PASS, 1 of 1 slice validated.
- Design fit: PASS.
- Three compiled board CLI fixtures: PASS.
- `go test ./internal/board -count=1`: PASS.
- `go test ./... -count=1`: PASS.
- `go vet ./...`: PASS.
- `gofmt` check for changed Go files: PASS with empty output.

All Go commands used `GOFLAGS=-buildvcs=false` to avoid linked-worktree VCS
stamping affecting an otherwise unrelated build.

## Reachability artefact

`TestBoardCLIOwnerRefStatusProjectionThroughLogicalDocsSymlink` executes the
compiled public CLI from a temporary consumer repository. Its Git tree contains
a logical docs symlink and canonical release records. The selected release,
owner track, non-owner track, and dirty launch file deliberately disagree.

Before the implementation, the test failed because the CLI projected `shipped`
from `working-tree` with `uncommitted` durability. After commit `60ff1e59`, both
aggregate and named commands exit 0 and project the implemented blocked owner
verdict from the exact owner ref with `committed` durability.

A separate built-binary run from a live consumer-project checkout reproduced the
same owner-ref and durability result. Private consumer identifiers and content
are omitted from this public technical record.

## Delivered

- AC-01: exact owner-ref authority through a canonical documentation path.
- AC-02: selected release-ref fallback when owner status is unavailable.
- AC-03: matching aggregate and named source provenance.
- AC-04: targeted, repository-wide, trace, design, vet, and formatting gates.

## Not delivered

Nothing deferred.

## Divergence from plan

The Baton release records were recovered after implementation checkpoints
`fd3cf540` and `60ff1e59`. The status keeps the integration base as the immutable
start so proof and fresh verification cover the full implementation history.

## Verification state

Implementation evidence is complete. Verification remains pending and must be
performed by a fresh context.
