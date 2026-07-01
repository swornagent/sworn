# Design TL;DR — S02-board-render

**Slice:** `S02-board-render` · **Track:** `T2-board-render` · **Release:** `2026-06-30-sworn-operational-readiness`
**State at authoring:** `planned` → (this doc) `design_review`

## User outcome

An operator runs `sworn render <release>` and gets an `index.md` deterministically
generated from `board.json` plus the slice records, so the board's human view is
never hand-authored (by a model or a human) and cannot drift from the record.

## Approach

Add a **pure, deterministic renderer** in `internal/board` that reads
`docs/release/<release>/board.json` plus each referenced slice's `spec.json` and
`status.json`, and produces an `index.md` string. A thin `cmd/sworn` verb writes
that string to `docs/release/<release>/index.md`. New files only — no existing
engine path changes (touchpoint-disjoint from T1/T3/T4, per the matrix).

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
   `status.effort_complexity.quadrant`).
4. **Touchpoint matrix** — every `spec.touchpoints` file × track, marked.
5. **Dependency graph** — code-fenced block derived from `tracks[].depends_on`.

## Key design choices + rationale

1. **Tolerant `board.json` decode inside the renderer — NOT `board.ReadBoard`.**
   The strict S05 reader (`BoardRecord.Release`, object-only) *fails* on this
   release's on-disk `board.json` (`"release": "<string>"`, per commit `8fadf68`).
   Verified live: `ReadBoard` returns `board release: not a canonical {name}
   object`. The renderer therefore defines a local `renderBoard` struct that
   **reuses `board.BoardTrack`** (already tolerant via `StringList` for
   `depends_on`) and reads `release` as `json.RawMessage`, accepting **string or
   `{name}`**. This keeps the renderer within its touchpoints (migrating
   `board.json` is T4's file, out of scope) and lets AC-05 pass. See Pin 1 — this
   is the one choice that needs a reviewer verdict.

2. **Determinism (AC-02):** tracks **sorted by track id**; slices kept in their
   declared `slices[]` order (AC-01 says "ordered slices" — the sequence is
   meaningful, so it is preserved, not sorted); touchpoint rows sorted by
   `(owning-track-id, file-path)` — reproduces the track-grouped layout *and* is
   input-order-independent. Columns = tracks in sorted-id order. `Render` builds
   one string from these stable orderings → byte-identical on repeat.

3. **Frontmatter validated by the existing validator (AC-03).** The test runs the
   rendered output through `board.ValidateIndex` (same package, `index.go`) and
   asserts zero errors — reusing the exact structural checks that guard against
   the frontmatter-fusion failure class this slice exists to kill.

4. **Fail closed (AC-04):** missing / malformed-JSON / structurally-invalid
   `board.json` (no tracks, or a track missing id/slices/state), or a referenced
   slice missing `spec.json`/`status.json`, → descriptive error, non-zero exit,
   **no `index.md` written**. Build-then-write guarantees no partial view. Note:
   render validates *structure*, and tolerates the dual `release` form (string or
   object) rather than enforcing strict board-v1 `release=object` — see Pin 1.

5. **Repo/release-dir resolution** mirrors `top`/`ship`: `filepath.Abs(projectRoot)`
   then `filepath.Join(absRoot, "docs", "release", release)`. No new git dependency.

## Pins for the Coach

- **Pin 1 — ESCALATE (spec-fidelity, AC-04 ↔ AC-05 tension).** AC-04 says fail
  closed when `board.json` is "invalid against board-v1"; board-v1 (vendored,
  S05) requires `release` to be an **object**. But the live board.json this
  release ships with has `release` as a **string** (commit `8fadf68`,
  "installed board-v1 shape (release=string)"), and AC-05 requires `sworn render`
  to succeed against *this* release. Strict board-v1 validation ⇒ AC-05 fails;
  AC-05 ⇒ render must tolerate the string form. **Recommendation:** render
  validates *structure* + tolerates the dual `release` form (string-or-object),
  and does **not** enforce `release=object` (that is the coach board's contract,
  and `board.json` migration is T4's touchpoint — out of scope for S02). This
  narrows AC-04's fail-closed teeth to genuine corruption. Needs Coach ack that
  this reading of AC-04 is intended, or a re-spec via `/replan-release`.

- **Pin 2 — MECHANICAL (note, no decision needed).** The golden test needs a
  `internal/board/testdata/render/` fixture (a small board.json + 2 slice dirs +
  golden index.md). These are inert test fixtures owned by `render_test.go`, not
  production files, so they sit outside the 3-file touchpoint list by design.

## AC → planned change traceability

| AC | Covered by |
|----|-----------|
| AC-01 | `Render` emits all four sections from board.json + spec/status records |
| AC-02 | stable sort orderings + golden + render-twice idempotency test |
| AC-03 | single-quoted frontmatter + `ValidateIndex`-parses test |
| AC-04 | build-then-write + structural validation + fail-closed tests |
| AC-05 | reachability: `sworn render 2026-06-30-...` reproduces T1+T2 tracks table + disjoint matrix (governed by Pin 1) |
| AC-06 | `go build ./...` + `go test ./internal/board/...` green |

## Design-level risks

- Pin 1 is the load-bearing risk: if the Coach wants strict board-v1
  (`release=object`) enforcement, AC-05 cannot be met without first migrating
  `board.json` (a T4 file) — that would re-scope the slice.
- Idempotency depends on **every** ordering being total and input-independent;
  the render-twice test is the guard.
