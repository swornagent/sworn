---
title: 'Proof bundle — S01-all-ref-board-catalog'
description: 'Deterministic all-ref board discovery with source and durability provenance. Implemented, not yet Rule-7 verified.'
date: 2026-07-17
---

# Proof bundle — S01-all-ref-board-catalog

> Rendered from `proof.json` (proof-v1). The JSON record is the source of truth.

## Scope

The SwornAgent board command now discovers every locally available release board through one
read-only catalog oracle, selects a deterministic topology source and farthest valid slice
evidence, and reports explicit committed or uncommitted provenance while keeping named-board
output compatible.

## Files changed

- `cmd/sworn/board.go`, `cmd/sworn/board_test.go`
- `internal/git/git.go`, `internal/git/git_test.go`
- `internal/board/discovery.go`, `internal/board/discovery_test.go`, `internal/board/oracle.go`,
  `internal/board/status_evidence_test.go`
- This slice's `journal.md`, `status.json`, `proof.json`, and `proof.md`

## Test results

- Targeted packages: `env GOFLAGS=-buildvcs=false /usr/local/go/bin/go test ./internal/git
  ./internal/board ./cmd/sworn` → exit 0.
- Compiled-CLI acceptance fixtures (seven cases) → exit 0.
- Coverage: `sworn lint coverage --slice S01-all-ref-board-catalog --release
  2026-07-17-ref-aware-board-discovery` → 6/6 acceptance checks covered, exit 0.
- Full suite: `env GOFLAGS=-buildvcs=false /usr/local/go/bin/go test ./...` → exit 0.
- Static checks: `go vet ./...` → exit 0; `gofmt -l .` → empty output, exit 0.
- AC-satisfaction: configured `openrouter/google/gemini-3.5-flash` returned structured `PASS`
  with no findings.

## Reachability artefact

**type: e2e-test.** `TestBoardCLIAllRefsCatalogStateEvidenceReachability` compiles the CLI and
runs `sworn board --json` from a temporary consumer Git repository where checked-out `HEAD` has no
release board. It asserts two non-HEAD releases, source/state provenance, and byte-identical
snapshots of HEAD, branch, refs, porcelain status, and process directory before and after the CLI
call.

Required mutation transcript:

1. Temporarily force discovery to consider only `HEAD`.
2. Run `TestBoardCLIAllRefsCatalogStateEvidenceReachability` → fails: `releases=0, want 2`
   (exit 1).
3. Restore all-ref discovery with `apply_patch`.
4. Rerun the same compiled-CLI test → passes (exit 0; latest run 2.51s).

## Delivered

- **AC-01:** all-ref catalog discovers non-HEAD releases, emits sorted keys, selected `sourceRef`,
  and elected state provenance. Evidence: `TestBoardCLIAllRefsCatalogStateEvidenceReachability`;
  `board.DiscoverCatalog`.
- **AC-02:** deterministic canonical/local/remote source-ref rank order. Evidence:
  `TestDiscoverCatalogSourceRefRanking`; `TestBoardCLIAllRefsCatalogSourceRef`.
- **AC-03:** canonical skew fails closed. Evidence:
  `TestDiscoverCatalogCanonicalSkewFailsClosed`; `TestBoardCLIAllRefsCatalogCanonicalSkewFailsClosed`.
- **AC-04:** shared state election preserves lifecycle, attention, and durability provenance.
  Evidence: state-evidence tests and `TestBoardCLIStateEvidenceProvenance`.
- **AC-05:** named queries preserve their envelope while agreeing with aggregate evidence.
  Evidence: `TestBoardCLINamedReleaseJSONShapeCompatibility` and
  `TestBoardCLINamedAndCatalogStateEvidenceAgree`.
- **AC-06:** all-ref reachability is read-only and the Go checks pass. Evidence:
  `TestBoardCLIAllRefsCatalogReadOnly`, `TestRepoListRefsReadOnly`, and the test results above.

## Not delivered

None.

## Divergence from plan

None.

## First-pass verdict

`git diff --binary 130a304a4cf108734a026f8037bc645718e99363..HEAD | sworn verify --spec
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
