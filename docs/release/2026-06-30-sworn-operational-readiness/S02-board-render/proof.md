# Proof bundle — S02-board-render

**Release:** 2026-06-30-sworn-operational-readiness · **Track:** T2-board-render
**State:** implemented · **start_commit:** db14b95
_Rendered from proof.json (proof-v1). Generated from live repo state._

## Scope

Add `sworn render <release>`, a pure deterministic renderer that generates
`index.md` from `board.json` plus each slice's `spec.json`/`status.json`, so the
board's human view is never hand-authored (by a model or a human) and cannot
drift from the record.

## Files changed (`git diff --name-only db14b95` + new files)

- `internal/board/render.go` — pure `Render` / `RenderToFile` + section writers
- `internal/board/render_test.go` — golden, idempotency, fail-closed, frontmatter, disjoint-matrix tests
- `cmd/sworn/render.go` — self-registering `render` verb
- `internal/board/testdata/render/…` — fixture release (board.json + 2 slices) + `rel-fixture.golden.md`
- `docs/release/2026-06-30-sworn-operational-readiness/index.md` — regenerated (reachability, AC-05)
- `docs/release/2026-06-30-sworn-operational-readiness/S02-board-render/status.json` — state transitions + Type-1 decision

## Test results (live runs)

| Command | Result |
|---------|--------|
| `go build ./...` | PASS (exit 0) |
| `go test ./internal/board/...` | PASS (`ok`) |
| `go test ./cmd/sworn/...` | PASS (`ok`, 37s) |
| `go test ./... -timeout 300s` | PASS (all packages `ok`, no FAIL, no hang) |
| `go vet ./internal/board/ ./cmd/sworn/` | PASS (exit 0) |
| `sworn designfit 2026-06-30-sworn-operational-readiness` | PASS (Rule 9 Type-1 gate — 6 slices, all clear) |

Deterministic first-pass = `designfit` PASS + build/tests green. The model-backed
`sworn verify` could not run in this session (`SWORN_ANTHROPIC_API_KEY not set`);
full adversarial verification is the fresh `/verify-slice` session's job (Rule 7).

## Reachability artefact (AC-05)

`sworn render 2026-06-30-sworn-operational-readiness <worktree>` → exit 0,
regenerated `docs/release/2026-06-30-sworn-operational-readiness/index.md`,
replacing the hand-authored file:

- Tracks table contains **T1-operational-unblock** and **T2-board-render** (all 5 tracks).
- Touchpoint matrix: **31 file rows, 0 collisions** — no file marked under two
  tracks (the disjointness the matrix exists to prove).
- Frontmatter scalars single-quoted (AC-03).
- Re-running render produced a **byte-identical** `index.md` (idempotent, AC-02).

## Delivered

- **AC-01** — `render` verb reads board.json + slice records and writes the four
  sections (tracks table, slice table, touchpoint matrix, dependency graph).
  Evidence: `cmd/sworn/render.go`, `internal/board/render.go`, golden fixture.
- **AC-02** — deterministic + idempotent (sorted-by-id tracks, declared slice
  order, matrix rows sorted by (owning-track, path)). Evidence: `TestRenderGolden`
  (renders twice, asserts equal + golden) + live real-release idempotency re-run.
- **AC-03** — single-quoted frontmatter, passes `ValidateIndex`. Evidence:
  `writeFrontmatter` + `TestRenderFrontmatterValidates`.
- **AC-04** — fail closed: `os.Stat` missing-board guard (no lazy-migration
  fallthrough), strict `ReadBoard` on malformed/string board, missing slice
  record → error, no partial `index.md`. Evidence: three fail-closed tests.
- **AC-05** — rendered output replaces the hand-authored `index.md`; T1+T2 present;
  matrix disjoint. Evidence: regenerated `index.md` + `TestRenderReproducesTracks`.
- **AC-06** — `go build ./...` + `go test ./internal/board/...` green (plus
  `./cmd/sworn/...` + full `./...` with timeout per review Pin 3).
- **Rule 9** — Type-1 decision (strict `board.ReadBoard` over the rejected local
  tolerant decoder) recorded in `status.json.design_decisions`; `designfit` PASS.

## Not delivered (Rule 2 deferrals)

- **Lint drift-guard** (committed `index.md` vs fresh render). Why: explicitly
  out of scope in `spec.json` rationale. Tracking: **sworn#20**. Ack: scoped out
  by planner + confirmed in `review.md` Pin 6.
- **Planner skill calling `sworn render`.** Why: private-harness change, out of
  scope in `spec.json`. Tracking: follow-up harness task. Ack: named out of scope
  by the planner; this slice delivers the reusable engine.
- **Model-backed `sworn verify` first-pass verdict.** Why: no
  `SWORN_ANTHROPIC_API_KEY` in this session. Tracking: the fresh `/verify-slice`
  session owns the model-backed run (Rule 7). Ack: surfaced here, not a silent skip.

## Divergence from plan

- The rendered `index.md` includes a `# Release board: <release>` heading and a
  one-line provenance note above the four required sections — presentation chrome
  matching the prior hand-authored index.md's title. Additive to AC-01's four
  required sections (all present); no functional deviation.
