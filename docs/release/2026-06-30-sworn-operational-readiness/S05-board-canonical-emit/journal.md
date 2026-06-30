# Journal — S05-board-canonical-emit

## 2026-07-01 — Registration remediation (/replan-release)

S05 was implemented directly in the T4 worktree (feat `565f909`) without going through planner
registration first — spec, board membership, intake, production code, and proof were collapsed
into a single commit on the track branch, so the planner→implementer→verifier lifecycle was
short-circuited. A fresh verifier correctly BLOCKED it (`blocked_needs_planner`): board
registration and start-commit/lifecycle are planner authority and an implementer cannot
self-clear them.

This `/replan-release` resolves it:
- Registered `S05-board-canonical-emit` under `T4-board-record-reconciliation` in `release-wt`'s
  `board.json` (T4 slices now `[S04, S05]`).
- Set `start_commit = 0d22f65` (the parent of the S05 feat commit `565f909`), so
  `git diff 0d22f65 -- internal/ cmd/` isolates S05's four production files.
- Cleared `verification.result` from `blocked` back to `pending` so the slice re-enters
  verification.
- Forward-synced the corrected board + planning artefacts into the T4 track branch (Step 6).

The implementation was not re-touched — only the planning/lifecycle artefacts were corrected.
S05 is now ready for a fresh `/verify-slice`.

## 2026-07-01 — Spec defect discovered (AC-02 stale) — routed to /replan-release

Implementer session opened to land the strict-reader delta (Coach direction: "make the reader
object-only + invert the string tests; board migration is the AC-06 cutover step"). The first
cut (`565f909`) already landed schema/validator/writer object-only; the only code delta left is
the reader flip + test inversion.

Blocking discovery (Rule 8 — AC consistency break): **AC-02 was not amended** when the
2026-07-01 strict-reader replan landed. AC-02 still says a legacy string release is "read and
then written" and "self-heals to canonical on write", and names `TestRelease_StringReadEmitsCanonicalObject`
as its proof — which *requires a lenient reader*. This directly contradicts:
- AC-03 (amended): reader SHALL be object-only — "a bare string release SHALL fail closed".
- AC-06 (amended): string boards are MIGRATED at cutover, "rather than self-healed-on-write".
- the rationale: "MIGRATED at cutover (AC-06) rather than self-healed-on-write".

Under a strict reader AC-02 is unsatisfiable — a string board errors on read, so it can never
be "read and then written". A fresh verifier reads spec.json from disk (Rule 7, no conversation
access) and would FAIL the slice on AC-02 + its inverted named test. Implementer cannot edit
spec.json (planner authority). Stopping at `in_progress`; routing forward to /replan-release.

Required spec fix (planner): rewrite AC-02 to the strict-reader world — either fold it into
AC-06 (string boards migrated at cutover, not self-healed) or restate it as "a name-only Release
constructed in-process (StringRelease, the index.md migration path) SHALL emit the canonical
object form" (which the writer + TestRelease_StringReadEmitsCanonicalObject-renamed still prove).
AC-04 was already amended to reference the strict reader; only AC-02 is stale. Also update the
stale lenient-read comments in board.go (MarshalJSON, lines 70-74; UnmarshalJSON 48-49),
validator.go (178-181), and board-v1.json release.description (line 19) as part of the reader flip.

## 2026-07-01 — Re-entry: AC-02 defect still open, no self-heal path (block stands)

A second `/implement-slice` session was opened against S05. The blocking AC-02 ↔ AC-03/AC-06
contradiction recorded above is **still unresolved** — the routed `/replan-release` has not yet
landed. New diagnostics gathered this session, to save the planner the re-check:

- **Both branches carry the identical stale AC-02.** `release-wt`'s copy of `spec.json` AC-02 is
  byte-identical to the track branch's. `git log origin/release-wt --not HEAD -- <slice dir>`
  returns nothing, so there is **no release-wt→track forward-port gap** — this is NOT the
  Step 6 ↔ Step 0b deadlock, and the Step 0b self-heal (cherry-pick) path does not apply. The
  defect is genuinely unfixed upstream, not merely unpropagated.
- **Code is still the first cut (lenient reader).** `Release.UnmarshalJSON` accepts a bare string
  (board.go first branch; comment still reads "strict emit, lenient read"). The strict-reader
  delta was never landed because the prior session correctly halted on the spec defect. So the
  remaining code work (reader flip + test inversion) is unchanged and still pending the spec fix.
- Worktree clean (Gate -1 pass). Slice left at `in_progress`, `verification.result: pending`.

No code touched. Routing forward to `/replan-release 2026-06-30-sworn-operational-readiness` to
amend AC-02 to the strict-reader world (fold into AC-06, or restate as the in-process
StringRelease emit case). Implementation can resume only once AC-02 is consistent with
AC-03/AC-06.

## 2026-07-01 — /replan-release resolution: AC-02 REMOVED (strict-reader reconciliation)

`/replan-release` ran and the human ratified **Remove AC-02**. AC-02's lenient
"string release self-heals to canonical on write" required a lenient reader,
directly contradicting the amended strict reader (AC-03) and the cutover
migration (AC-06) — mutually unsatisfiable (Rule 8, AC consistency). AC-02 is
deleted: its emit half is already AC-01 (MarshalJSON emits the object form for a
name-only release), its migration half is AC-06 (operator string boards migrated
at cutover). The stale lenient-world `user_outcome` and the "Reader unchanged"
effort rationale were also corrected to the strict-reader world.

Landed on release-wt as `3fbb651` and forward-merged here. AC IDs kept stable
(AC-01, AC-03..AC-06) for traceability. No production code touched by the planner.

NEXT: S05 is unblocked and stays `in_progress` — resume `/implement-slice S05` to
land the remaining code delta (UnmarshalJSON object-only + invert the string-read
tests to assert a bare string fails closed), then a fresh `/verify-slice`.

## 2026-07-01 — Strict-reader delta landed → implemented

Resumed `/implement-slice S05` after the `/replan-release` AC-02 removal. Continuation handshake:
regenerated files-changed + test-results from live worktree state and reconciled against the prior
(first-cut) proof — confirmed the first cut (`565f909`) had already landed schema + validator +
writer object-only, leaving exactly the reader flip + test inversion the replan resolution named.

Landed this session (start_commit 0d22f65):
- `board.go` `Release.UnmarshalJSON` → object-only (strict). The bare-string branch is removed; a
  string release now errors ("not a canonical {name} object ... migrate it"). The `Release` doc
  comment + `MarshalJSON` comment updated from "lenient read" to the strict-read world.
- `validator.go` + `board-v1.json` description: comment-only — both were already object-only from
  the first cut; their prose still claimed "the reader tolerates a legacy bare string", now corrected.
- `board_release_test.go`: `TestRelease_StringForm` → `TestRelease_StringForm_FailsClosed`;
  `TestRelease_StringReadEmitsCanonicalObject` split into `TestRelease_BareStringRead_FailsClosed`
  (read fails closed, AC-03) + `TestStringRelease_EmitsCanonicalObject` (in-process StringRelease
  still emits the object form, AC-01 — the journal-suggested restatement). Stale `AC-07` label on the
  round-trip test corrected to `AC-01`; the S04-era "reads both forms" header updated to strict.

Rule-2 transparency — two fixtures edited BEYOND the four declared touchpoints:
- `internal/board/board_test.go` (`TestOracleReadBoard_BoardJSONFirst`) and
  `cmd/sworn/merge_test.go` (`setupMergeFixture`) both built board.json fixtures with the legacy
  string-form `release`. Under the strict reader these fail closed — a real regression the full-suite
  AC-05 gate surfaced (6 cmd/sworn merge tests + 1 board test). Each migrated to `{"name": ...}`.
  Both are test fixtures in package surfaces owned by T4 (internal/board, cmd/sworn), not a cross-track
  collision; one-line fixture migrations, not production logic. Surfaced here + in proof Divergence.

Operational consequence noted (NOT acted on — AC-06 cutover): the three operator string boards on
disk (op-readiness, conformance-foundation, release-hygiene) now fail closed under a strict-reader
binary. By AC-06 they are migrated at cutover, gated on every active session being on a canonical
binary — must NOT run mid-flight. The op-readiness board itself is among them, so the loop's own
`sworn board` would fail once a strict binary is installed before that migration; that sequencing is
AC-06's to own, not this slice's.

Verification:
- `go build ./...` exit 0; `go vet` (board+baton) exit 0.
- `go test ./internal/board/... ./internal/baton/...` ok.
- `go test ./... -timeout 300s` ALL GREEN (AC-05).
- AC-04 reachability: S05 binary reads the real coach OBJECT board in ~/projects/fired (exit 0).
- Deterministic first-pass (`release-verify.sh`): 18 pass / 1 residual FAIL = `spec.md missing`,
  a script/format mismatch (slice uses spec-v1 spec.json; verified sibling S04 has no spec.md either),
  not a slice gap. Model-backed `sworn verify` needs SWORN_ANTHROPIC_API_KEY (unset) — the verifier's to run.

State → implemented. Stopping per Rule 7; NEXT = fresh-context `/verify-slice S05`.
