# S02-board-render — journal

## 2026-07-01 — Implementer session halted at design-review gate (Rule 9)

**State on entry:** `design_review`, `verification.result: pending`, `design_decisions: null`.
`review.md` terminal verdict was `DECISION: NEEDS_COACH` (escalate Pin 1), no PROCEED
acknowledgement present. Per the Session-start handshake I did not write code, did not
transition to `in_progress`, and did not record the Type-1 decision (Rule 9 forbids the
model recording it).

**Blocker verified live (not recalled):** AC-04 and AC-05 are mutually unsatisfiable as
written.
- AC-04 (unwanted): render must fail closed when `board.json` is invalid against board-v1.
- AC-05 (event-driven): render must succeed against this release's `board.json`.
- Live `board.json` `release` field is the bare string `"2026-06-30-sworn-operational-readiness"`
  (`jq` → `type=string`).
- Canonical board-v1 reader rejects a bare-string release: `internal/board/board.go:54-62`
  — "not a canonical {name} object (a bare string release is no longer read — migrate it
  to {\"name\":...})".

Root cause is a **board-v1 contract skew**, not an S02 design flaw: the in-tree
`board.go` was tightened (T4 / S05, verified+merged) to reject string releases, while the
live `board.json` still carries the string shape and commit `8fadf68` deliberately set it
to string to match the **installed** `sworn` binary. The installed binary and the in-tree
contract disagree about board-v1.

**Coach decision (via /implement-slice session, 2026-07-01):** *Reconcile the contract
skew first.* Neither S02 option (tolerant render reader / migrate-then-strict). Pause S02;
decide the canonical `release` shape once (string vs object); align the installed binary +
`board.json` + `board.go` to that single contract; then re-spec S02 against the settled
contract.

**Next step (not an implementer action):** route to `/replan-release
2026-06-30-sworn-operational-readiness` (or a dedicated contract-reconciliation session).
The Planner records the settled shape + this decision into `status.json.design_decisions`
as the Type-1 record and revises AC-04/AC-05 so they are consistent. S02 remains
`design_review` until then.

## 2026-07-01 — CORRECTION: shape already decided; blocker is the S05 AC-06 cutover

The earlier entry framed this as "decide the canonical shape (string vs object)". That is
WRONG and is corrected here: the canonical `release` shape is **already decided and
documented — OBJECT / strict** — in T4:
- S05-board-canonical-emit AC-03 (Coach-ratified 2026-07-01, "no-wild-data"): reader is
  object-only, a bare string release fails closed; "legacy operator string boards are
  migrated, not read-tolerated — see AC-06".
- S05 AC-06: string boards are migrated `release:"X"` -> `{"name":"X"}` as a one-time
  CUTOVER, applied only once every active session is on a canonical (S04/S05) binary;
  "hold this for the operational-readiness cutover — the op-readiness board itself is among
  the boards to migrate."

So `board.go` rejecting the string is INTENDED, and the string board.json is a
known-deferred un-migrated artefact — not undecided contract. Verified live: the installed
binary is pre-S05 string-tolerant (`8fadf68`, 2026-06-30T14:25:32Z — reads the string board
exit=0); board.json is still string on release-wt too.

**S02 is blocked on the S05 AC-06 operational-readiness cutover, NOT a design decision:**
(1) build+install canonical (S05) `sworn`; (2) get all in-flight sessions (T3, loop) on it
per AC-06 sequencing; (3) migrate this release's board.json to object form; (4) then S02's
AC-04/AC-05 are consistent as written — re-run `/design-review` (expect PROCEED, render via
canonical ReadBoard) then implement. No AC revision needed; a `/replan-release` is only to
record the cutover dependency if desired. Cutover execution is a Coach/operator action
(blast radius: breaks pre-S05 string-binary sessions if mis-sequenced).

## 2026-07-01 — S05 AC-06 cutover EXECUTED (Coach-authorised) — S02 board-level unblock

Coach authorised running the cutover. Done + verified (all live):
- Built canonical `sworn` from release-wt HEAD (bd72c3f, has S05 strict reader);
  confirmed it FAILS closed on the string board (exit 2, "not a canonical {name}
  object"). Installed globally to /home/brad/go/bin/sworn — byte-identical to the
  verified-strict build; definitively strict (rejects the still-string
  2026-07-01-release-hygiene board, exit 2).
- Migrated board.json `release` string -> {"name": ..., "integration_branch":
  "release/v0.1.0"} on release-wt/2026-06-30 (commit bd72c3f) AND forward-merged
  release-wt -> track/T2 so this track's board.json is object too. Global `sworn
  board --release 2026-06-30-...` now reads it (exit 0, 4 tracks).

Effect: S02's AC-04<->AC-05 tension is resolved at the board level — render can use
canonical ReadBoard against a valid object board. S02 is still `design_review`; next
step is re-run `/design-review S02-board-render` (escalate should clear to PROCEED,
no second reader), then `/implement-slice`.

Remaining fleet-cutover items (operator-coordinated, NOT S02, surfaced Rule 2):
- release/v0.1.0 integration-branch copies (2026-06-30 primary tree, plus
  2026-07-01-release-hygiene and 2026-06-27-conformance-foundation boards) are still
  string; they now fail on the GLOBAL strict binary but still work on the local
  pre-S05 ./bin/sworn (76c657b). Full cutover of those releases = migrate their boards
  + rebuild their binaries, intersecting their own in-flight work.
- 2026-06-30 primary-tree board reconciles to object automatically at /merge-release.
- Local ./bin/sworn (76c657b) is now inconsistent with the global strict binary;
  rebuild or remove once the integration branch carries S05.

## 2026-07-01 — design.md REVISED per DECISION: IMPLEMENTER_FIX (re-review post-cutover)

**State on entry:** `design_review`, `verification.result: pending`. The re-review
(`review.md`, 2026-07-01, Captain) returned **`DECISION: IMPLEMENTER_FIX`**, not
PROCEED — because `design.md` still described the **pre-cutover** approach (Choice 1
= local tolerant `renderBoard` decoder reading `release` as `json.RawMessage`). Per
`captain.md` an IMPLEMENTER_FIX verdict returns the design to the implementer for
revision; Rule 9 forbids writing code from a design that specifies the forbidden
reader (the reader choice is Verifier-invisible — both readers pass every AC test).

**No production code written this session.** Revised `design.md` only, addressing the
three review pins that fold into the design:
- **Pin 1** (was ESCALATE → resolved-direction): replaced Choice 1. The renderer now
  decodes via canonical strict `board.ReadBoard` (object-only `release`); no local
  tolerant struct, no string-form acceptance. Justified live: board is object-form
  post-cutover, `ReadBoard` succeeds (`sworn board --release … --json` exit 0).
- **Pin 2** (mechanical): added Choice 2 — `Render` `os.Stat`s `board.json` first and
  fails closed on absence, so `ReadBoard`'s lazy-migration-from-`index.md`
  (board.go:126-142 → `migrateFromIndex`) never fires and cannot invert the data flow
  AC-04 exists to protect. Verified the lazy-migration path in live code before writing.
- **Pin 3** (mechanical): documented the Type-1 design decision (strict `ReadBoard` +
  object-form board, over the rejected local tolerant decoder) as a new design.md
  section, to be transcribed into `status.json.design_decisions` at `in_progress`.
  Type-1 human decision already exists (Coach authorised the cutover); the implementer
  transcribes, does not originate it.

Also carried Pin 3-test-scope (run `./cmd/sworn/...` + full suite with timeout) and
Pin 2-fixture (object-form testdata board) into the revised design's Pins section.

**Slice stays `design_review`.** A design revised after a non-PROCEED verdict must be
re-reviewed (Rule 9 — no jump to code). **Next step:** fresh `/design-review
S02-board-render 2026-06-30-sworn-operational-readiness` — expect PROCEED now that the
design specifies the strict reader and the missing-board guard.

## 2026-07-01 — design review PROCEED + implementation → `implemented`

Re-review returned `DECISION: PROCEED` (review.md, commit 986c26f) and the Coach
pasted the acknowledgement reply ("Address pins 1–6 inline … then proceed to
in_progress"). Rule 9 gate satisfied. Transitioned design_review → in_progress
(start_commit db14b95), recorded the Type-1 decision, implemented, → implemented.

**Built (within the 3 declared touchpoints + inert testdata):**
- `internal/board/render.go` — pure `Render(projectRoot, release) (string,error)`
  + `RenderToFile` (build-then-write). Decodes via canonical strict
  `board.ReadBoard` (Pin 1); `os.Stat` missing-board guard so a missing board.json
  fails closed instead of `ReadBoard` lazy-migrating from index.md (Pin 2). Emits
  single-quoted frontmatter + tracks table + slice table + touchpoint matrix +
  dependency graph, all from stable orderings (tracks sorted by id; slices in
  declared order; matrix rows by (owning-track, path)).
- `cmd/sworn/render.go` — self-registering `render` verb (init() → command.Register),
  mirroring `top`/`ship` (positional release + optional project-root). Chose
  self-registration NOT an edit to commands.go, which is outside the touchpoints —
  a track collision avoided (board.go/route.go/merge.go use the same pattern).
- `internal/board/render_test.go` + `testdata/render/` — golden (renders twice,
  byte-identical), frontmatter-ValidateIndex, disjoint-matrix, and three
  fail-closed tests (missing board / bare-string board / missing slice record).

**Pins applied inline:** Pin 3 test-scope (ran `./internal/board/...`,
`./cmd/sworn/...`, and full `./... -timeout 300s` — all green, no newline-hang);
Pin 5 (ValidateIndex + single-quoted frontmatter kept); Pin 6 (rendered index.md
replaces the hand-authored one — reachability). Pin 4 forward-merge is a
pre-/merge-track step, not an implementer action.

**Gates:** `designfit` PASS (Rule 9 Type-1 recorded). `specquality`/`lint ac` fail
release-wide because every slice uses spec.json/EARS not spec.md/acceptance-examples
(known false-negative, `feedback_releaseverify_specmd_false_fail`; verified siblings
S03/S04/S05 share it) — not introduced by S02. `sworn verify` model gate could not
run (no SWORN_ANTHROPIC_API_KEY) → deferred to the fresh /verify-slice (Rule 7),
surfaced in proof.json not_delivered.

**Reachability (AC-05):** ran `sworn render <this release>` → regenerated index.md,
T1+T2 present, 31-file matrix with 0 collisions, byte-identical on re-render.

Terminal state `implemented`. Next: fresh `/verify-slice S02-board-render
2026-06-30-sworn-operational-readiness`.
