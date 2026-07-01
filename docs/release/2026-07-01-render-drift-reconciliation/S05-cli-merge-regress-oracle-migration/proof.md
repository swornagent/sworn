---
title: Slice proof bundle — S05-cli-merge-regress-oracle-migration
description: Rule 6 proof bundle, scoped to one slice. Generated from live repo state, not recollection. Verifier reads this; do not paraphrase.
---

# Proof Bundle: `S05-cli-merge-regress-oracle-migration`

Rendered from `proof.json` (proof-v1).

## Scope

`sworn merge-release` and `sworn regress` succeed against any current-format
(board.json-backed) release instead of hard-erroring with "release_worktree_path
not found in index.md frontmatter".

## Files changed

```
$ git diff --name-only c5634da7d8006072946fbc6f6f1e9fe1819864dd
cmd/sworn/merge.go
cmd/sworn/merge_test.go
cmd/sworn/regress.go
cmd/sworn/regress_test.go
docs/release/2026-07-01-render-drift-reconciliation/S05-cli-merge-regress-oracle-migration/journal.md
docs/release/2026-07-01-render-drift-reconciliation/S05-cli-merge-regress-oracle-migration/status.json
docs/release/2026-07-01-render-drift-reconciliation/index.md
internal/board/oracle.go
internal/board/oracle_test.go
```

(`proof.json` and this `proof.md` land with the bundle commit.)

## Test results

### Go

```
$ go build ./...
(no output, exit 0)

$ go test ./internal/board/... ./cmd/sworn/... -timeout 120s
ok  	github.com/swornagent/sworn/internal/board	0.123s
ok  	github.com/swornagent/sworn/cmd/sworn	41.437s

$ go test ./... -timeout 280s
ok  	github.com/swornagent/sworn/cmd/sworn	37.848s
ok  	github.com/swornagent/sworn/internal/account	(cached)
... (41 packages total, all ok — internal/baton/schemas and internal/verdict
     have no test files; full list matches `go list ./...` output)

$ go vet ./internal/board/... ./cmd/sworn/...
(no output, exit 0)

$ gofmt -l cmd/sworn/merge.go cmd/sworn/regress.go cmd/sworn/merge_test.go cmd/sworn/regress_test.go internal/board/oracle.go internal/board/oracle_test.go
(no output — clean)
```

`./internal/board/...` was added to the AC-06-scoped command per design-review
pin 4 (memory-cited: `feedback_releaseverify_specmd_false_fail` — a prior
strict board-v1 reader change regressed fixtures in `internal/board` while
`cmd/sworn` alone stayed green). Full `go test ./...` was also run, per the
same memory's "ALWAYS run full `go test ./...` before trusting a 'verified'
release" guidance and `project_newline_eating_edit_corruption`.

## Reachability artefact

- **Type**: live command run against a real, committed release
- **Path**: `docs/release/2026-07-01-render-drift-reconciliation/S05-cli-merge-regress-oracle-migration/reachability-regress-output.txt`
- **User gesture**: an operator runs `sworn regress --release 2026-06-30-sworn-operational-readiness`
  from a checkout of this release (board.json carries `release_worktree_path`;
  its rendered `index.md` carries zero `release_worktree_path` occurrences —
  confirmed by grep) and the command resolves the worktree path and runs the
  regression suite instead of hard-erroring on the missing frontmatter key.
  Exit 0 (Go tests PASS, TypeScript SKIP — no package.json in that worktree,
  Golden fixtures PASS).
- **AC-05 retarget** (design-review pin 1): the spec's originally-named
  example release `2026-07-01-loop-cli-ux` carries no `release_worktree_path`
  anywhere (verified live — 0 grep hits in both board.json and index.md) and
  cannot demonstrate the fix; retargeted to `2026-06-30-sworn-operational-readiness`,
  which does carry the field. `sworn merge-release` was also considered and
  rejected as the vehicle: this repo's own release board has non-terminal
  slices, so merge-release's gate 1 ("all slices terminal") fails before ever
  reaching the code this slice changes.

## Delivered

- **AC-01** — `merge-track`/`merge-release`'s release-worktree resolution now
  calls `oracleAdapter.ReadReleaseWorktreePath(rel)` — the `OracleReaderAdapter`
  both commands already build for gate 1 — instead of the frontmatter scraper
  `resolveReleaseWorktree` (deleted, along with its private `extractFrontmatterBody`
  copy). Evidence: `cmd/sworn/merge.go`; `TestMergeTrack_AllVerified`,
  `TestMergeTrack_OracleRouting`, `TestMergeRelease_Pass` all pass against
  board.json-backed, `board.RenderToFile`-generated fixtures (not hand-authored
  frontmatter).
- **AC-02** — `regress`'s default (non-`--worktree`) resolution now calls
  `board.ReadBoard(".", *releaseName).ReleaseWorktreePath` instead of the
  deleted `extractReleaseWorktreePath` frontmatter scraper. Evidence:
  `cmd/sworn/regress.go`; new `TestRegressDefaultResolution_BoardJSON` — this
  path had **no test at all** before this slice; only the `--worktree`
  override's fail-closed guard was tested.
- **AC-03** — releases with no `board.json` still resolve via the legacy
  index.md frontmatter fallback. Evidence (unit level):
  `internal/board/oracle.go` `Oracle.ReadReleaseWorktreePath`'s fallback
  branch, `TestReadReleaseWorktreePath_LegacyIndexMDFallback`,
  `TestReadReleaseWorktreePath_LegacyIndexMD_MissingKeyFailsClosed`.
  Evidence (integration level, both test files per design.md's traceability
  table): `TestMergeTrack_LegacyIndexMDFallback`,
  `TestRegressDefaultResolution_LegacyIndexMDFallback`.
- **AC-04** — `merge_test.go`'s `setupMergeFixture`/`writeMergeStatus` and
  `regress_test.go`'s `setupRegressFixture` now generate `index.md` through
  the real `board.RenderToFile` path — writing `spec.json` + `status.json`
  for every `board.json`-referenced slice first, since `RenderToFile` fails
  closed unless both are present — instead of hand-authored frontmatter.
  Confirmed the real renderer emits zero `release_worktree_path:` keys, which
  is exactly the bug this slice fixes; a hand-authored fixture carrying that
  key (the pre-fix state of both test files) is how the bug shipped
  undetected. The genuinely-legacy (no-`board.json`) fixtures stay
  hand-authored by design — `board.RenderToFile` cannot produce output
  without a `board.json` to render from.
- **AC-05** — see Reachability artefact above.
- **AC-06** — `go build ./...` exits 0; `go test ./internal/board/... ./cmd/sworn/...`
  passes (widened per design-review pin 4); full `go test ./...` (41
  packages) also passes.

## Not delivered

None. Every acceptance check is delivered.

## Divergence from plan

- AC-05's reachability target was retargeted from the spec's named example
  (`2026-07-01-loop-cli-ux`) to `2026-06-30-sworn-operational-readiness` —
  see Reachability artefact above. Design-review pin 1, applied inline; see
  `journal.md`.
- One new package-level method (`Oracle.ReadReleaseWorktreePath` +
  `OracleReaderAdapter.ReadReleaseWorktreePath`) was added to
  `internal/board/oracle.go`, outside `spec.json`'s literal touchpoint list
  (which names only the two `cmd/sworn` files and their tests) — the shared
  board.json/index.md dual-path resolution engine both `merge.go` call sites
  need already lives there (`readTrackInfos`); duplicating it inside
  `cmd/sworn` would recreate the drifting-second-copy bug class this release
  exists to close. Recorded as a Type-2 design decision in `status.json` (per
  `design.md` DC-1), not escalated.
- `release-verify.sh`'s deterministic first-pass reports a residual
  "spec.md missing" FAIL — a known false negative for spec-v1 (`spec.json`)
  slices (`feedback_releaseverify_specmd_false_fail` memory); no `spec.md`
  was manufactured to silence it. Same posture as S01/S04.
- No `approved-ack.md` marker convention exists in this repo yet; the
  design-review acknowledgement is recorded in `journal.md` instead of a
  separate marker file, per Rule 9's human-owned gate — same posture as S01.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S05-cli-merge-regress-oracle-migration 2026-07-01-render-drift-reconciliation
release-verify.sh
  slice:       S05-cli-merge-regress-oracle-migration
  slice dir:   docs/release/2026-07-01-render-drift-reconciliation/S05-cli-merge-regress-oracle-migration
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  FAIL  spec.md missing
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  could not determine integration branch from docs/release/2026-07-01-render-drift-reconciliation/index.md; skipping drift check

== Diff vs start_commit (verifier base) ==
  diff base: start_commit c5634da7d8006072946fbc6f6f1e9fe1819864dd
  PASS  9 file(s) changed vs diff base
  (first 20)
    cmd/sworn/merge.go
    cmd/sworn/merge_test.go
    cmd/sworn/regress.go
    cmd/sworn/regress_test.go
    docs/release/2026-07-01-render-drift-reconciliation/S05-cli-merge-regress-oracle-migration/journal.md
    docs/release/2026-07-01-render-drift-reconciliation/S05-cli-merge-regress-oracle-migration/status.json
    docs/release/2026-07-01-render-drift-reconciliation/index.md
    internal/board/oracle.go
    internal/board/oracle_test.go

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  deferrals (proof 'Not delivered' + spec 'Out of scope') carry concrete tracking refs
  PASS  proof.md 'Files changed' count (~9) consistent with diff vs start_commit (9)

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 19
  checks failed: 1

FIRST-PASS FAIL
Address the failures above before invoking the LLM verifier session.
See /home/brad/.claude/baton/adversarial-verification.md for the verifier protocol.
```

The single failure (`spec.md missing`) is the known `feedback_releaseverify_specmd_false_fail`
false negative for spec-v1 (`spec.json`) slices — S01 and S04's verified siblings
have no `spec.md` either. Not a real gap; the canonical gate is the model-backed
fresh-context `/verify-slice` pass.
