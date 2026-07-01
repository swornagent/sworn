# Design review — S02-board-render (RE-REVIEW, post-cutover)

**Reviewer:** Captain · **Date:** 2026-07-01 · **State on entry:** `design_review`
**Supersedes:** the prior review (`DECISION: NEEDS_COACH`, escalate Pin 1 = AC-04↔AC-05 contradiction). Prior version preserved in git history.

## Context — what changed since the prior review

The prior review escalated Pin 1: AC-04 (fail closed when `board.json` is invalid
against board-v1) vs AC-05 (render must succeed against *this* release's board)
were mutually unsatisfiable **because the live board carried `release` as a bare
string** while the S05 reader is object-only. The Coach resolved it (2026-07-01)
by executing the **S05 AC-06 cutover** rather than adding tolerance:

- Canonical strict `sworn` installed globally (verified fail-closed on string boards).
- `board.json` `release` migrated string → `{"name": …, "integration_branch": …}` on
  `release-wt` and forward-merged to `track/T2`.
- Verified live: `board.ReadBoard` now **succeeds** on this release's object board.

So the escalated contradiction is **gone**: with the board object-form, AC-04 and
AC-05 are consistent *as written*. The design decision is settled (canonical strict
reader, no tolerance). **The design.md, however, still describes the pre-cutover
approach** and must be brought into line before code. That is the load-bearing
finding of this re-review.

## Pins

### 1. [escalate → resolved-direction] Design Choice 1 / Pin 1 / Choice 4 — replace the local tolerant `renderBoard` decoder with canonical `board.ReadBoard`.

**Observed:** design.md Key Choice 1 says *"Tolerant `board.json` decode inside the
renderer — NOT `board.ReadBoard` … defines a local `renderBoard` struct … reads
`release` as `json.RawMessage`, accepting string or `{name}`."* Choice 4 and the
AC-05 traceability row inherit this ("tolerates the dual `release` form … governed
by Pin 1"). This premise is **inverted by the cutover**: `board.json` is now
object-form and `board.ReadBoard` (`internal/board/board.go:126`, reads the on-disk
`docs/release/<release>/board.json` via the strict S05 `Release.UnmarshalJSON`)
**succeeds** against it — verified live (`sworn board --release <this> --json` →
exit 0, 4 tracks).

**Direction (Coach already decided — not a fresh escalation):** the renderer SHALL
decode `board.json` via canonical `board.ReadBoard`. It SHALL NOT define a local
tolerant `renderBoard` struct or accept the string form. S05 AC-03 is explicit —
"legacy operator string boards are migrated, **not read-tolerated**"; a second
tolerant reader is exactly the reader-divergence surface
`feedback_releaseverify_specmd_false_fail` warns against. AC-04's fail-closed teeth
now land on genuine invalidity (a still-string or corrupt board fails closed via
ReadBoard), and AC-05 passes because the board is object-form.

**Why this drives IMPLEMENTER_FIX, not PROCEED:** the reader choice is **invisible
to the Verifier** — a tolerant decoder and `ReadBoard` both satisfy every AC test
(AC-05 renders, AC-04 fails on corruption). Only design review sees the divergence
(Rule 9). Left as an apply-inline directive against a design.md that still says
"define a local renderBoard decoder," an implementer could faithfully build the
forbidden reader and the Verifier would pass it. The design.md must be corrected
and re-checked.

### 2. [mechanical] `ReadBoard` lazy-migration vs AC-04 "missing board.json → fail closed".

**Observed:** `ReadBoard` (board.go:126-145) lazy-migrates when `board.json` is
**absent** — it reconstructs a `BoardRecord` from `index.md` frontmatter and writes
a new `board.json`. For a slice whose contract is *"index.md is derived from
board.json, never hand-authored"* (the user outcome), relying on that fallback
would **invert the data flow** and mask AC-04's "if board.json is missing … fail
closed, no index.md written."

**Direction:** the revised design must fail closed on a missing `board.json`
explicitly (e.g. `os.Stat` the path first, or reject the migration branch) rather
than let `ReadBoard` reconstruct from `index.md`. This interaction is introduced
*by* the Pin 1 correction, so it belongs in the revised design.md.

### 3. [mechanical] Rule 9 — record the Type-1 design decision in `status.json`.

**Observed:** `status.json.design_decisions` is `null`. The cutover resolution
(canonical strict `ReadBoard` + board migrated to object, over the rejected
tolerant-reader option) is an architecturally-significant Type-1 choice.

**Direction:** at `in_progress`, record it in `status.json.design_decisions` —
chosen option (strict `ReadBoard`, board object-form via AC-06 cutover), rationale,
and the rejected option (local tolerant decoder). Carries forward the prior
review's mechanical pin.

### 4. [mechanical] Drift — T2 is behind `release-wt` by 10 commits (S03 merged).

**Observed:** `git rev-list --count track/…/T2..release-wt/…` = 10; the delta is
sibling-track T3 (S03-sworn-self-ignore: implemented + verified + merged to
release-wt) plus release-wt bookkeeping. **None of it touches S02's `spec.json`
(byte-identical on both branches) or `design.md` (present only on T2).** So the
review is **not** stale — the authoritative artefacts are T2's. But before
`/merge-track`, T2 must forward-merge `release-wt/` to pull S03's merged content
(the implementer's/verifier's drift gate — not a Captain action). Note: local
`release-wt` (`5fefbe1`) is ahead of `origin` (`bd72c3f`); the T3 merges are
unpushed.

### 5. [memory-cited] Widen test scope beyond `./internal/board/...`.

**Observed:** AC-06 names only `go test ./internal/board/...`, but the slice adds
`cmd/sworn/render.go`. Per `feedback_releaseverify_specmd_false_fail`, a
reader/contract change regressed fixtures in `cmd/sworn` before (the S05 strict
reader). **Direction:** also run `go test ./cmd/sworn/...`, plus a full
`go test ./...` **with a timeout** per `project_newline_eating_edit_corruption`
(the newline-eating hang). Apply inline; the /merge-track affected-package gate
also backstops. Citation: `feedback_releaseverify_specmd_false_fail`,
`project_newline_eating_edit_corruption`.

### 6. [memory-cited] Frontmatter-fusion kill — confirmed sound.

**Observed:** Choice 3 (`board.ValidateIndex`, index.go:48, + single-quoted YAML
scalars, AC-03) directly targets `project_index_frontmatter_corruption_false_ready`
— deterministic render removes the newline-eating hand-edit path that caused the
false merge-ready. **Direction:** confirm the rendered output *replaces* the
hand-authored index.md (AC-05 reachability), and keep the sworn#20 lint drift-guard
deferral tracked (Rule 2 — the design scopes it out correctly). Citation:
`project_index_frontmatter_corruption_false_ready`.

## Summary

**6 pins — 3 [mechanical], 2 [memory-cited], 1 [escalate].**
Critical: **Pin 1** — the design.md still specifies the forbidden pre-cutover
tolerant decoder; shipping from it would recreate the reader-divergence the cutover
eliminated, and the Verifier cannot catch it. Pins 2–3 fold into the same design
revision; Pins 4–6 are apply-inline / confirmations. The design is otherwise sound
and fully AC-traced (pure `Render`/`RenderToFile`, stable orderings, `ValidateIndex`
reuse, build-then-write fail-closed, `top`/`ship`-mirrored verb).

<!-- CAPTAIN-VERDICT
DECISION: IMPLEMENTER_FIX
CONSTITUTIONAL: no
REASON: Design Choice 1 still specifies the pre-cutover tolerant renderBoard decoder; the cutover settled the direction (canonical strict ReadBoard, board object-form) but the reader choice is Verifier-invisible (Rule 9), so design.md must be revised (use ReadBoard, guard the lazy-migration/AC-04 interaction, record the Type-1 decision) and re-checked before code.
-->
