# S01 owner-ref status projection journal

## 2026-07-18 - implementation and protocol recovery

- Anchored to swornagent/sworn#124 and related authority contracts #123 and #81.
- Recorded the failing public CLI fixture in commit `fd3cf540`. Before the fix,
  the projected winner was `shipped`, sourced from `working-tree`, and marked
  `uncommitted` instead of the committed blocked owner verdict.
- Landed the owner-ref, canonical-prefix, and named-source projection in commit
  `60ff1e59`.
- The release artefacts were reconstructed after those implementation commits at
  the orchestrator's request. The status `start_commit` deliberately remains the
  integration base so verification covers both checkpoints and does not erase
  their history.
- Targeted CLI and board tests, the full Go suite, and vet passed. A built feature
  binary was also run from a separate live consumer project checkout. It reported
  the selected release source and the blocked slice verdict from the exact owner
  track with committed durability. Consumer identifiers and content are omitted
  because this repository is public.
- No Baton vendoring, version bump, tag, publication, or merge was performed.
- The implementer will leave the slice at `implemented` with verification
  pending. A fresh verifier must certify or reject it.

## 2026-07-18 - deterministic gates

- `sworn lint ac` passed with 4 of 4 event-driven ACs well formed.
- `sworn lint trace` passed with 4 needs and 4 ACs traced.
- `sworn reqvalidate` and `sworn designfit` passed for the slice.
- The three focused compiled CLI fixtures passed in 7.588 seconds; the board
  package passed; the full Go suite, vet, and changed-file formatting checks
  exited 0.
- `proof.json` and `proof.md` were generated from the live base-to-branch diff.

## 2026-07-18 - maintainability readiness

- The released Sworn v0.2 command returned a model PASS with no findings but did
  not emit the v0.13.1 scope identity fields. A guaranteed-clean ephemeral Git
  worktree supplied a synthetic base containing the release records at their
  current bytes, so the model input contained exactly the four canonical
  semantic paths and no release-record noise. Cleanup removed the worktree and
  temporary branch before the command returned.
- The canonical `baton-maintainability-v1` manifest over base
  `68a578b10d9c8c69632aad96301e4fc04dff0de0`, proof checkpoint
  `0cffdd93c80a3b5930da4da2e733828f60b91ace`, and the four included paths hashes
  to `sha256:597016f746483bc29b8eadb60afe5080dd77f0cd2ffef17fa136469bef8723ac`.
- The durable full report is blob-pinned in `status.json`. Maintainability moved
  to `passed`, implementation head is pinned to `0cffdd93`, and the slice moved
  from `in_progress` to `implemented`. No semantic file changed after the PASS.
- Verification remains pending. The fresh verifier owns the authoritative
  maintainability report and final slice verdict.

## Verifier verdicts received

### 2026-07-18 - BLOCKED

BLOCKED

Slice: `S01-owner-ref-status-projection`

Reason: AC-04 requires `go test ./...` to pass, but the ratified committed-only
`DiscoverCatalog` behaviour breaks the existing TUI live-worktree-state
invariant while TUI changes are explicitly out of scope and absent from the
planned touchpoints.

Proposed spec.json amendment: Add `internal/tui/board.go`,
`internal/tui/releases.go`, and `internal/tui/tui_test.go` to touchpoints;
replace the broad committed-only `DiscoverCatalog` requirement with an explicit
committed-status projection mode used by aggregate and named `sworn board`
commands while preserving the TUI live-worktree overlay; add an acceptance
criterion requiring
`TestBoardViewLiveWorktreeStateNotMaskedByLastCommit` and the compiled CLI
owner-ref fixture to pass together.
