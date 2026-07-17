---
title: 'Proof bundle — S01-all-ref-board-catalog'
description: 'Deterministic all-ref board discovery with source and durability provenance, including recovery from malformed noncanonical historical records. Implemented, not yet Rule-7 verified.'
date: 2026-07-17
---

# Proof bundle — S01-all-ref-board-catalog

> Rendered from `proof.json` (proof-v1). The JSON record is the source of truth.

## Scope

The SwornAgent board command discovers every locally available valid release board through one
read-only catalog oracle, ignores malformed noncanonical historical topology records, preserves
canonical release-worktree fail-closed semantics, and reports deterministic source and durability
provenance in aggregate and named output.

## Verifier-remediation evidence

The fresh verifier found that a bare-string legacy `board.json` on
`refs/heads/audit/2026-07-02-conformance-gap-closure` aborted both user-facing commands before
they could produce output. The correction batch-reads direct topology artifacts, validates them
before source-ref ranking, skips invalid noncanonical candidates, and leaves canonical candidates
strict.

After commit `2cca001`:

```text
$ GOFLAGS=-buildvcs=false /usr/local/go/bin/go run ./cmd/sworn board --json
release_count: 17
target.sourceRef: refs/heads/release-wt/2026-07-17-ref-aware-board-discovery
exit: 0

$ GOFLAGS=-buildvcs=false /usr/local/go/bin/go run ./cmd/sworn board \
    --release 2026-07-17-ref-aware-board-discovery --json
release: 2026-07-17-ref-aware-board-discovery
tracks: 1
top_level_releases: false
exit: 0
```

`TestBoardCLIAllRefsCatalogSkipsInvalidNoncanonicalTopology` is the compiled-CLI regression: it
places a lexically earlier malformed noncanonical bare-string board beside a valid lower-priority
record, asserts the valid source wins, asserts an invalid-only historical release is omitted, and
asserts an unrelated named query remains reachable. Canonical malformed/missing/identity-mismatched
cases still fail closed in `TestBoardCLIAllRefsCatalogCanonicalSkewFailsClosed`.

## Files changed

- `cmd/sworn/board.go`, `cmd/sworn/board_test.go`
- `internal/git/git.go`, `internal/git/git_test.go`
- `internal/board/discovery.go`, `internal/board/discovery_test.go`, `internal/board/oracle.go`,
  `internal/board/status_evidence_test.go`
- This slice's `journal.md`, `status.json`, `proof.json`, and `proof.md`, plus the rendered
  release `index.md`

## Test results

- Targeted packages: `env GOFLAGS=-buildvcs=false /usr/local/go/bin/go test ./internal/git
  ./internal/board ./cmd/sworn` → exit 0.
- Compiled-CLI acceptance fixtures (eight cases, including malformed noncanonical recovery) →
  exit 0.
- Coverage: `sworn lint coverage --slice S01-all-ref-board-catalog --release
  2026-07-17-ref-aware-board-discovery` → 6/6 acceptance checks covered, exit 0.
- Full suite: `env GOFLAGS=-buildvcs=false /usr/local/go/bin/go test ./...` → exit 0.
- Static checks: `go vet ./...` → exit 0; `gofmt -l .` → empty output, exit 0.
- AC-satisfaction: configured `openrouter/google/gemini-3.5-flash` returned structured `PASS`
  with no findings.

## Reachability artefact

**type: cli-run.** The live project-root commands above are the user-facing affordance the fresh
verifier found unreachable; they now exit 0 from this committed track branch. The compiled fixture
keeps that recovery testable without depending on this repository's historical refs.

Required mutation transcript:

1. Temporarily force discovery to consider only `HEAD`.
2. Run `TestBoardCLIAllRefsCatalogStateEvidenceReachability` → fails: `releases=0, want 2`
   (exit 1).
3. Restore all-ref discovery with `apply_patch`.
4. Rerun the same compiled-CLI test → passes (exit 0).

## Delivered

- **AC-01:** all-ref catalog discovers non-HEAD releases, emits sorted keys, selected `sourceRef`,
  and elected state provenance without being poisoned by invalid noncanonical history.
- **AC-02:** deterministic canonical/local/remote source-ref rank order among valid direct records.
- **AC-03:** canonical skew still fails closed.
- **AC-04:** shared state election preserves lifecycle, attention, and durability provenance.
- **AC-05:** named queries preserve their envelope while agreeing with aggregate evidence.
- **AC-06:** all-ref reachability is read-only and the Go checks pass.

## Not delivered

None.

## Divergence from plan

None.

## First-pass verdict

`git diff --binary 130a304a4cf108734a026f8037bc645718e99363 | sworn verify --spec
docs/release/2026-07-17-ref-aware-board-discovery/S01-all-ref-board-catalog/spec.json --diff -
--proof docs/release/2026-07-17-ref-aware-board-discovery/S01-all-ref-board-catalog/proof.json`
returned:

```json
{
  "verdict": "PASS",
  "rationale": "",
  "cost_usd": 0
}
```

This is the deterministic Rule-6 first pass. It is not a fresh-context Rule-7 verifier verdict.
