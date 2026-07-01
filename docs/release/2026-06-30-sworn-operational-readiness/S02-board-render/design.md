# Design TL;DR — S02-board-render (REVISED, post-cutover)

**Slice:** `S02-board-render` · **Track:** `T2-board-render` · **Release:** `2026-06-30-sworn-operational-readiness`
**State at authoring:** `design_review` (revised in place after `DECISION: IMPLEMENTER_FIX`)
**Supersedes:** the pre-cutover draft (Choice 1 = local tolerant `renderBoard` decoder,
Pin 1 = ESCALATE). That draft is preserved in git history. This revision brings the
design into line with the executed **S05 AC-06 cutover** per `review.md` Pins 1–3.

## What changed since the pre-cutover draft

The pre-cutover draft chose a **local tolerant `renderBoard` decoder** reading
`release` as `json.RawMessage` (string-or-object), because the live `board.json`
carried `release` as a bare string and the strict S05 reader rejected it. Pin 1 of
the design review was ESCALATE on the resulting AC-04 ↔ AC-05 contradiction.

That contradiction is **gone**. The Coach authorised and executed the S05 AC-06
cutover (journal, 2026-07-01): the canonical strict `sworn` is installed globally,
this release's `board.json` `release` field is migrated string → `{ "name": …,
"integration_branch": … }` on `release-wt` and forward-merged to `track/T2`, and
`board.ReadBoard` now **succeeds** against this release's object-form board
(verified live: `sworn board --release 2026-06-30-sworn-operational-readiness --json`
→ exit 0, 4 tracks). So the design decision is settled — **canonical strict reader,
no tolerance** — and this design.md is corrected to match it.

## User outcome

An operator runs `sworn render <release>` and gets an `index.md` deterministically
generated from `board.json` plus the slice records, so the board's human view is
never hand-authored (by a model or a human) and cannot drift from the record.

## Approach

Add a **pure, deterministic renderer** in `internal/board` that reads
`docs/release/<release>/board.json` (via the canonical strict reader) plus each
referenced slice's `spec.json` and `status.json`, and produces an `index.md`
string. A thin `cmd/sworn` verb writes that string to
`docs/release/<release>/index.md`. New files only — no existing engine path
changes (touchpoint-disjoint from T1/T3/T4, per the matrix).

Shape:

- `internal/board/render.go`
  - `func Render(projectRoot, release string) (string, error)` — **pure**: returns
    the full `index.md` markdown or a descriptive error. No file writes, so the
    golden test drives it directly.
  - `func RenderToFile(projectRoot, release string) error` — calls `Render`, and
    only on success writes `index.md` (build-then-write: never a partial file).
- `internal/board/render_test.go` — golden-file + idempotency + fail-closed +
  frontmatter-parse + disjoint-matrix tests over a `testdata/` fixture release.
- `cmd/sworn/render.go` — `command.Register` a `render` verb; signature
  `sworn render <release> [project-root]`, mirroring `sworn top` / `sworn ship`
  (positional release, optional project-root defaulting to `.`).

## Rendered layout (AC-01)

Fixed Markdown, sections in this order:

1. **Frontmatter** — `title` / `description` as **single-quoted YAML scalars** (AC-03).
2. **Tracks table** — id · ordered slices · depends_on · state.
3. **Slice table** — id · track · one-line outcome (from `spec.user_outcome`) ·
   state (from `status.state`) · effort_complexity quadrant (from
   `spec.effort_complexity.quadrant`).
4. **Touchpoint matrix** — every `spec.touchpoints` file × track, marked.
5. **Dependency graph** — code-fenced block derived from `tracks[].depends_on`.

## Key design choices + rationale

1. **Decode `board.json` via canonical `board.ReadBoard` — strict, object-only.**
   `render.go` lives in `package board`, so it calls `ReadBoard` directly. The
   renderer does **NOT** define a local `renderBoard` struct and does **NOT**
   accept the bare-string `release` form. This is the settled cutover direction
   (review.md Pin 1, Coach-decided): S05 AC-03 is explicit — "legacy operator
   string boards are migrated, **not** read-tolerated." A second tolerant reader
   would recreate exactly the reader-divergence surface the cutover eliminated
   (`feedback_releaseverify_specmd_false_fail`), and — critically — the choice is
   **invisible to the Verifier** (a tolerant decoder and `ReadBoard` both pass
   every AC test), so it must be fixed here in design, not left to implementation.
   AC-05 passes because the board is now object-form; AC-04's fail-closed teeth
   now land on genuine invalidity (a still-string or corrupt board fails closed
   *through* `ReadBoard`'s strict `Release.UnmarshalJSON`, board.go:54-62).

2. **AC-04 missing-`board.json` guard — `os.Stat` before `ReadBoard`, fail closed;
   never lazy-migrate from `index.md`.** `ReadBoard` (board.go:126-142) lazy-migrates
   when `board.json` is **absent**: it reconstructs a `BoardRecord` from `index.md`
   frontmatter (`migrateFromIndex`) and writes a fresh `board.json`. For a slice
   whose entire contract is *"index.md is derived from board.json, never
   hand-authored"*, letting that fallback fire would **invert the data flow** —
   render would reconstruct the record from the very file it is meant to generate,
   and AC-04's "if board.json is missing … fail closed, no index.md written" would
   be silently violated. So `Render` **`os.Stat`s the `board.json` path first**;
   if it does not exist, it returns a descriptive AC-04 error immediately and does
   not call `ReadBoard`. A present-but-malformed / still-string / structurally-invalid
   board fails closed via `ReadBoard`'s own error. Build-then-write guarantees no
   partial `index.md` in any failure path.

3. **Determinism (AC-02):** tracks **sorted by track id**; slices kept in their
   declared `slices[]` order (AC-01 says "ordered slices" — the sequence is
   meaningful, so it is preserved, not sorted); touchpoint rows sorted by
   `(owning-track-id, file-path)` — reproduces the track-grouped layout *and* is
   input-order-independent. Columns = tracks in sorted-id order. `Render` builds
   one string from these stable orderings → byte-identical on repeat.

4. **Frontmatter validated by the existing validator (AC-03).** The test runs the
   rendered output through `board.ValidateIndex` (same package, `index.go:48`) and
   asserts zero errors — reusing the exact structural checks that guard against
   the frontmatter-fusion failure class this slice exists to kill. Frontmatter
   scalars are emitted **single-quoted** so the output cannot reproduce the
   `state: merged---` fence-fusion class (`project_index_frontmatter_corruption_false_ready`).

5. **Repo/release-dir resolution** mirrors `top`/`ship`: `filepath.Abs(projectRoot)`
   then `filepath.Join(absRoot, "docs", "release", release)`. No new git dependency.

## Type-1 design decision (Rule 9 — to record in `status.json.design_decisions` at `in_progress`)

The reader choice is **architecturally-significant** (it defines whether a second
board-decode path exists in the codebase), hence **Type-1**. The human decision
already exists — the Coach authorised the S05 AC-06 cutover — so at the
`planned → in_progress` transition the implementer records it (the model is not
recording a *fresh* Type-1 judgement; it is transcribing a Coach-made one):

- **Chosen:** canonical strict `board.ReadBoard` against an **object-form**
  `board.json` (board migrated via the S05 AC-06 cutover). One board-decode path
  in the codebase.
- **Rejected:** a local tolerant `renderBoard` decoder accepting string-or-object
  `release`. Rejected because it recreates a divergent second reader (S05 AC-03:
  migrated, not read-tolerated) and is invisible to the delivery Verifier.
- **Rationale:** single strict contract; AC-04/AC-05 are consistent as written once
  the board is object-form; no reader-divergence surface.

## Pins for the Coach

- **Pin 1 — MECHANICAL (was ESCALATE; now resolved-direction).** The reader choice
  is settled by the executed cutover: canonical strict `ReadBoard`, no local
  tolerant decoder, no string-form acceptance. No open escalation remains. Recorded
  as the Type-1 decision above.
- **Pin 2 — MECHANICAL (note).** The golden test needs an
  `internal/board/testdata/render/` fixture (a small **object-form** board.json + 2
  slice dirs + golden index.md). These are inert test fixtures owned by
  `render_test.go`, not production files, so they sit outside the 3-file touchpoint
  list by design.
- **Pin 3 — MECHANICAL (test scope, apply inline).** AC-06 names only
  `go test ./internal/board/...`, but the slice adds `cmd/sworn/render.go`. Per
  `feedback_releaseverify_specmd_false_fail` a reader/contract change has regressed
  `cmd/sworn` fixtures before, so also run `go test ./cmd/sworn/...` and a full
  `go test ./...` **with a timeout** (`project_newline_eating_edit_corruption`).

## AC → planned change traceability

| AC | Covered by |
|----|-----------|
| AC-01 | `Render` emits all four sections from `board.ReadBoard` + each slice's spec/status records |
| AC-02 | stable sort orderings + golden + render-twice idempotency test |
| AC-03 | single-quoted frontmatter + `ValidateIndex`-parses test |
| AC-04 | `os.Stat` missing-board guard (Choice 2) + strict `ReadBoard` error on malformed/string board + build-then-write; fail-closed tests assert no `index.md` written |
| AC-05 | reachability: `sworn render 2026-06-30-sworn-operational-readiness` reproduces T1+T2 tracks table + disjoint matrix via canonical `ReadBoard` against the (now object-form) live board |
| AC-06 | `go build ./...` + `go test ./internal/board/...` green (plus `./cmd/sworn/...` and full-suite-with-timeout per Pin 3) |

## Design-level risks

- **Lazy-migration masking (mitigated by Choice 2).** If the `os.Stat` guard were
  omitted, a missing `board.json` would silently reconstruct from `index.md` and
  invert the data flow. The guard + a fail-closed test on a board-less fixture dir
  is the mitigation.
- **Idempotency depends on every ordering being total and input-independent;** the
  render-twice byte-identity test is the guard.
- (Resolved) The pre-cutover strict-vs-tolerant escalation is closed by the S05
  AC-06 cutover; no residual spec-fidelity risk.
