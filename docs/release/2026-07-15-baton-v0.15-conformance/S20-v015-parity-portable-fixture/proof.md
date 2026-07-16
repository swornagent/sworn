# S20-v015-parity-portable-fixture proof bundle

## Scope

Re-deliver exact Baton v0.15.1 protocol, archive, schema, state-carrier, and
isolated-install parity with a committed authenticated Git-bundle clean-CI
oracle instead of a developer-specific sibling checkout.

## Files changed

`git diff --name-only 08dd38f81e466d3288ff4bf64953cfc90ea6063c` supplies the
authoritative complete path list in [`proof.json`](proof.json). The only
non-release source authority newly introduced by S20 is the committed
`baton-v0.15.1.bundle`; its bytes are test-only and are authenticated before
any temporary checkout becomes evidence.

## Test results

- Built-binary clean/diff/error and invalid-bundle reachability tests — PASS.
- Exact bundle identity, full archive/mirror parity, transaction recovery, and
  internal Baton suite — PASS.
- `go test ./internal/gate ./internal/run ./internal/state -count=1`,
  `go test ./... -count=1`, `go vet ./...`, `make build`, and `git diff --check`
  — PASS.
- Live `bin/sworn baton diff` against a verified temporary bundle clone — PASS
  (0).
- Live `bin/sworn doctor --sync-baton` in explicit disposable homes with no
  PATH tools — repair PASS (2), immediate exact re-run PASS (0).
- Deterministic `bin/sworn verify` proof-bundle first pass with the live S20
  diff — PASS (0).

## Reachability artefact

`TestDoctorAndBatonDiffV015BinaryReachability` constructs source evidence from
the committed bundle only after validating size, SHA-256, Git blob, header,
complete history, exact annotated tag/peeled commit, VERSION object/bytes,
fsck, and clean status. It runs the built `sworn baton diff` in an isolated
repository fixture and the built `sworn doctor --sync-baton` in contained,
pairwise-disjoint temporary homes. The negative companion test proves missing,
truncated, and byte-corrupt bundles cannot create a temporary evidence clone.

## Delivered

All seven S20 acceptance criteria are evidenced in [`proof.json`](proof.json):
exact protocol/archive/schema parity, public 0/1/2 diff exits, three-way
installer parity, durable isolated install recovery, lossless state carriage,
portable exact-tag bundle reachability, and invalid-bundle fail-closed handling.

## Not delivered

The two explicitly out-of-scope follow-ons are recorded with owning slices in
[`proof.json`](proof.json). No untracked deferral was introduced.

## Divergence from plan

None.
