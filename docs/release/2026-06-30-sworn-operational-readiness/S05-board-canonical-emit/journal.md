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
