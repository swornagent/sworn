# Captain review — S04b-tui-live
Date: 2026-06-21
Design commit: 305e544c5b7f0d1889e040b4e888dc9c4d96d6ea

## Pins

1. [mechanical] §1 / schema — `started_at` is in `tracks` table, NOT `events` table (CRITICAL)
   What I observed: Design §1 says "Elapsed time since `started_at` (from DB events table)." Verified the live schema in `internal/db/db.go`: `started_at TEXT NOT NULL DEFAULT ''` is a column on the `tracks` table (line 38). The `events` table has only `id`, `track_id`, `release`, `event`, `detail`, `ts` — no `started_at` column.
   What to ask the implementer: Query `tracks.started_at` directly (e.g. `SELECT id, current_slice, state, started_at FROM tracks WHERE release = ?`). The events table is not needed for elapsed time unless you want sub-second granularity via the first "started" event — but that would be a design change. Confirm you read from `tracks.started_at` and update the §1 wording in the proof.md description to reflect the correct table.

2. [mechanical] Risks — WAL verification not formally recorded in design
   What I observed: Spec Risk says "Verify WAL is enabled before implementing." Design Decision 1 states "SQLite WAL mode (enabled in S01's `internal/db/db.go`)" — the file is cited but no explicit audit line is recorded. Verified from live repo: WAL is set at `internal/db/db.go:69` — `PRAGMA journal_mode=WAL`.
   What to ask the implementer: Record the audit result explicitly in proof.md ("WAL confirmed at `internal/db/db.go:69`"). The spec Risk requires the audit be documented before proceeding. The underlying fact is correct; just capture it formally.

3. [mechanical] §2 Decision 1 — "read-only connection" claim vs `db.Open()` actual semantics
   What I observed: Design §2 Decision 1 says "The TUI opens its own read-only connection to this DB." However, `internal/db` exports only `Open(dbPath string)` — which runs `PRAGMA journal_mode=WAL`, `PRAGMA foreign_keys=ON`, and schema migrations (CREATE TABLE IF NOT EXISTS). These are write operations. There is no exported `OpenReadOnly()` function. The spec says "polls the SQLite DB (via `internal/db`)".
   What to ask the implementer: Two acceptable resolutions: (a) use `db.Open()` as-is — the WAL mode makes it safe for concurrent TUI reads + scheduler writes, and the migrations are idempotent; update the design claim from "read-only connection" to "read-write connection that only issues SELECT queries"; (b) open sqlite directly with `file:path?mode=ro` URI and bypass the `internal/db` package — but then `planned_files` must include `internal/db/db.go` or a new helper file. Option (a) is lower risk. Choose one and document it; "read-only connection" as written contradicts the only available API.

4. [mechanical] Step 2b — `design_decisions` absent from status.json
   What I observed: S04b `status.json` has no `design_decisions` field. Design §2 lists 5 decisions. `sworn designfit` passes (skips slices with empty design_decisions), but the artefact is incomplete vs. the S04a pattern (which has a full design_decisions array). All 5 decisions appear Type-2.
   What to ask the implementer: Populate `status.json.design_decisions` for all 5 §2 decisions before or during implementation. Use Type-2 + human_decision: "" for each (Type-2 choices don't require a human decision, but the field should be present). This matches S04a's established pattern and keeps the designfit audit trail complete.

5. [mechanical] §6 Q2 — credit balance refresh answer is in spec
   What I observed: Q2 asks "static or re-read every N ticks?" Spec In-scope says "reads `~/.config/sworn/credits.json` cache" — "cache" implies static load. Spec AC4 requires only that the balance is shown/absent correctly, with no requirement for dynamic refresh.
   What to ask the implementer: Q2 is answered by the spec. Load once at TUI startup. No human decision needed — proceed with your stated approach.

6. [mechanical] §6 Q3 — auto-transition vs offer `l` answer is in spec
   What I observed: Q3 asks whether to auto-transition or require `l`. Spec AC1 says "the TUI transitions to (or offers `l` to view)" — listing auto-transition as the primary behavior with `l` as an alternative. The design implements both: auto-transition AND `l`/`b` toggle. This is additive and spec-compliant.
   What to ask the implementer: Q3 is answered by the spec. Both behaviors are within scope. No human decision needed — your auto-transition + toggle approach is correct.

## Summary

Pins: 6 total — 6 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: Pin 1 — if `started_at` is queried from the `events` table (which has no such column), elapsed time display will return empty strings or a query error; the running track row will show no elapsed time.

## Smaller flags (not pins, worth one-line ack)

(a) **macOS credits path**: Design says `os.UserHomeDir()` + `.config/sworn/credits.json`, which is Linux XDG. On macOS, `config.json` lives at `~/Library/Application Support/sworn/config.json`. The spec prescribes `~/.config/sworn/credits.json` explicitly, so the design is spec-correct; but if a future issue is filed to unify the credits path with the config path on macOS, that context will matter. No action required here.

(b) **Smoke step fixture specificity**: §5 step 3 says "fixture repo (or primary repo)." The S02b history shows verifiers expect concrete smoke step commands with actual output documented in proof.md. Nail down the fixture repo choice before writing proof.md — vague references fail Gate 4.

(c) **`test_commands` in status.json is empty**: Implementer should populate with `["go test ./internal/tui/... -v -count=1", "go build ./...", "go vet ./..."]` on transition to in_progress. Not a gate issue now; a reminder.

## Suggested ack reply
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
