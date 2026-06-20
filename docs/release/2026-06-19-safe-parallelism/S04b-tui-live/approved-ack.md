<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Solid design — clean scope, all ACs covered, §4 deferrals correctly cited. 6 mechanical pins:

1. **`started_at` table (CRITICAL).** Read from `tracks.started_at`, not the events table — `events` has no `started_at` column. Query: `SELECT id, current_slice, state, started_at FROM tracks WHERE release = ?`. Correct the §1 description in proof.md.
2. **WAL audit record.** In proof.md, record: "WAL confirmed at `internal/db/db.go:69` (PRAGMA journal_mode=WAL)." Spec Risk requires the audit to be documented.
3. **Read-only claim vs `db.Open()`.** `internal/db.Open()` runs migrations (not truly read-only). Pick one: (a) use `db.Open()` as-is and drop the "read-only connection" language — WAL makes it safe; or (b) open sqlite with `file:path?mode=ro` and add a helper to `internal/db/`. Option (a) is lower risk. Document your choice.
4. **Populate `status.json.design_decisions`.** Add all 5 §2 decisions as Type-2 entries (matching S04a's pattern) on transition to in_progress.
5. **§6 Q2 closed by spec.** Static load at startup — "cache" in spec means once. No escalation needed.
6. **§6 Q3 closed by spec.** Auto-transition + `l`/`b` toggle is spec-compliant. No escalation needed.

Flags (not pins): (a) macOS credits path is spec-compliant but diverges from config.go convention — no action now; (b) smoke step in §5 needs a concrete fixture repo command with actual output for proof.md (S02b Gate 4 lesson); (c) populate `test_commands` in status.json when transitioning to in_progress.

§2 decisions all ack as Type-2. §6 questions 2 and 3 closed above; Q1 (DB connection lifecycle) proceeds with per-view connection using `db.Open()` (option (a) above).

Address pins 1–4 inline during implementation (5 and 6 are closed). Proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 6 pins are apply-inline mechanical corrections; pin 1 (schema table) is critical but has an unambiguous fix; no authority-boundary decisions required.
-->
