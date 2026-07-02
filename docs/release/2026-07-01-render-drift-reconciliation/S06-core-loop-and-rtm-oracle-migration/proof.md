---
title: Slice proof bundle — S06-core-loop-and-rtm-oracle-migration
description: Rule 6 proof bundle, scoped to one slice. Generated from live repo state, not recollection. Verifier reads this; do not paraphrase.
---

# Proof Bundle: `S06-core-loop-and-rtm-oracle-migration`

Rendered from `proof.json` (proof-v1). First implementation pass.

## Scope

The autonomous loop (`run.RunParallel`) finds its tracks and correctly detects
shared touchpoint files across tracks for any current-format (board.json-backed)
release instead of hard-erroring "no tracks found" or silently disabling the
shared-file invariant; and Rule 8's RTM carries the real release benefit / org
objective from board.json instead of empty strings.

## Files changed

```
$ git diff --name-only d427d181bc725199aadbd9f3fc17b22247b1b8d2
docs/release/2026-07-01-render-drift-reconciliation/S06-core-loop-and-rtm-oracle-migration/journal.md
docs/release/2026-07-01-render-drift-reconciliation/S06-core-loop-and-rtm-oracle-migration/proof.json
docs/release/2026-07-01-render-drift-reconciliation/S06-core-loop-and-rtm-oracle-migration/proof.md
docs/release/2026-07-01-render-drift-reconciliation/S06-core-loop-and-rtm-oracle-migration/reachability-runparallel-output.txt
docs/release/2026-07-01-render-drift-reconciliation/S06-core-loop-and-rtm-oracle-migration/status.json
docs/release/2026-07-01-render-drift-reconciliation/index.md
internal/router/router_test.go
internal/rtm/rtm.go
internal/rtm/rtm_test.go
internal/run/cold_start_test.go
internal/run/parallel.go
internal/run/parallel_test.go
```

`start_commit` is `d427d181` (current HEAD when the slice moved to `in_progress`,
first-set, never overwritten). All changes are inside T5's six declared
touchpoints plus this slice's docs — **no `internal/board` edit** (T1-drift-guard's
exclusive touchpoint).

## Test results

### Go

```
$ go build ./...
(no output, exit 0)

$ go test ./internal/run/... ./internal/rtm/... ./internal/router/... -count=1 -timeout 300s
ok  	github.com/swornagent/sworn/internal/run	5.100s
ok  	github.com/swornagent/sworn/internal/rtm	0.023s
ok  	github.com/swornagent/sworn/internal/router	0.053s

$ go vet ./internal/run/... ./internal/rtm/... ./internal/router/...
(no output, exit 0)

$ gofmt -l internal/run/parallel.go internal/run/parallel_test.go internal/run/cold_start_test.go internal/rtm/rtm.go internal/rtm/rtm_test.go internal/router/router_test.go
(empty — all formatted)
```

(Full-suite verification is the merge gate's responsibility; this bundle runs the
slice-relevant packages only, per Rule 6.)

## Reachability artefact

- **Type**: live command run captured to file (AC-06, Coach-ratified option (a))
- **Path**: `docs/release/2026-07-01-render-drift-reconciliation/S06-core-loop-and-rtm-oracle-migration/reachability-runparallel-output.txt`
- **What it proves**: `TestRunParallel_AC06_RealReleaseBoardResolvesTracks` drives
  `run.RunParallel` against THIS repo's own live 5-track
  `2026-07-01-render-drift-reconciliation/board.json` (copied into an isolated
  temp workspace with the worktree paths redirected, a pausing router injected,
  a faked planned-files reader, and an in-memory DB — zero side effects on the
  real in-flight track worktrees). The captured stderr shows
  `sworn run --parallel: loaded 5 tracks in 1 phases` and all five tracks
  (`T1-drift-guard`, `T2-tui`, `T3-mcp`, `T4-cli-merge-regress`,
  `T5-core-loop-rtm-rescrape`) resolving and pausing — where the old
  frontmatter parser returned zero tracks and hard-errored "no tracks found in
  release board".

## Delivered

- **AC-01** — `RunParallel` resolves tracks via `board.ReadBoard` (the oracle) +
  the local `trackInfosFromBoardTracks` converter, replacing
  `board.ParseTracks(extractFrontmatter(...))`. Evidence:
  `internal/run/parallel.go`; `TestRunParallel_AC06_RealReleaseBoardResolvesTracks`
  ("loaded 5 tracks", no "no tracks found"); the existing `TestRunParallel_*`
  suite stays green through `board.ReadBoard`'s lazy migration of their legacy
  `index.md` fixtures.
- **AC-02** — `extractReleaseWorktreePath` is deleted; the release worktree path
  comes from `br.ReleaseWorktreePath`, and the cold-start default now fires only
  when board.json records none (not unconditionally). Evidence:
  `internal/run/parallel.go`; `TestRunParallel_Basic`,
  `TestRunParallel_ReleaseWorktreePathMissing`.
- **AC-03** — the weaker local `parseDocumentedSharedFiles` (explicit-marker only)
  is deleted and delegated to `router.ParseDocumentedShared` (explicit marker
  **and** ≥2-checkmark inference), fail-open when no matrix. Evidence:
  `internal/run/parallel.go`; `TestInvariant2_DocumentedSharedFromRenderedBoard`
  builds a **genuinely `board.Render`-produced** index.md whose shared file
  carries ≥2 checkmarks and no annotation, then asserts no `INVARIANT-2` block
  fires and both tracks run. Red confirmed: forcing `docShared = nil` makes the
  test FAIL (invariant-2 wrongly blocks a track); restoring the delegation makes
  it PASS.
- **AC-04** — `rtm.Build` reads `release.vertical_trace.benefit`/`.org_objective`
  from board.json via `readBoardVerticalTrace` (a sibling of `releaseDir`, so
  `Build`'s signature and its sole caller `internal/implement/ready.go` are
  untouched); the markdown-heading parse is retained as the no-board.json legacy
  fallback. Evidence: `internal/rtm/rtm.go`;
  `TestBuild_VerticalTraceFromBoardJSON` (board.json benefit wins over a
  deliberately-different index.md benefit; `OrgObjective` empty because the key
  is genuinely unauthored) and `TestBuild_VerticalTraceLegacyFallback`.
- **AC-05** — `router_test.go` gains `TestParseDocumentedSharedFromRenderedBoard`,
  exercising `ParseDocumentedShared`/`parseTouchpointMatrix` against a real
  `board.Render`-generated index.md (not the pre-migration
  `2026-06-27-conformance-foundation` fixture, which `TestParseDocumentedSharedFromFile`
  still covers, unchanged). Evidence: `internal/router/router_test.go`.
- **AC-06** — reachability artefact captured via the Coach-ratified Go-level
  `run.RunParallel` substitute; see **Reachability artefact** above and
  `reachability-runparallel-output.txt`.
- **AC-07** — `go build ./...` succeeds and
  `go test ./internal/run/... ./internal/rtm/... ./internal/router/...` passes.
  Evidence: **Test results** above.

## Not delivered

None. Every acceptance check is delivered.

## Divergence from plan

- **AC-06 substitute (Coach decision).** The reachability artefact is the
  Coach-ratified Go-level `run.RunParallel` invocation (review.md "Coach
  decision", option (a)), not the literal `sworn loop --parallel` CLI — the
  literal CLI would dispatch live work against sibling in-flight tracks (T2/T3)
  and trip [swornagent/sworn#46]. The substitute exercises the exact fixed logic
  (`board.ReadBoard` / `trackInfosFromBoardTracks` / documented-shared detection)
  against real board.json data. This is a plan-conformant divergence, recorded
  for the verifier.
- **llm-check (Rule 2 deferral).** `sworn llm-check --type ac-satisfaction` was
  not run in the implementer session — no `SWORN_ANTHROPIC_API_KEY` credential is
  available here. Tracking: the fresh-context `/verify-slice` pass is the
  model-backed check for this slice, consistent with the sibling S04 slice.
  Acknowledgement: surfaced in the implementer's session-end output.
