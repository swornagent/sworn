# Journal — S04b-tui-live

## State transitions

- **2026-06-28**: `design_review → in_progress` — Coach approved design via `approved-ack.md` (6 pins, PROCEED). Captain verdict: all 6 pins mechanical; pins 5 and 6 (Q2/Q3) closed by spec. Implemented per approved-ack directives.

## Decisions

- **DB connection**: Coach option (a) — use `db.Open()` as-is (read-write, migrations run, WAL-safe). Dropped "read-only" language. See approved-ack pin 3.
- **started_at source**: Read from `tracks.started_at`, not `events` table (events has no `started_at` column). Approved-ack pin 1 (CRITICAL).
- **WAL audit**: Confirmed at `internal/db/db.go:69` — `PRAGMA journal_mode=WAL`. Recorded in proof.md per approved-ack pin 2.
- **Auto-transition**: When release is selected (Enter) and the release has tracks in `in_progress` state in the SQLite DB, the TUI auto-transitions to `viewLive`. User can also toggle with `l`/`b`. Approved-ack pin 6 (Q3 closed by spec).
- **Credit balance**: Loaded once at TUI startup from `~/.config/sworn/credits.json`. Static load per spec ("cache" language). Approved-ack pin 5 (Q2 closed by spec).
- **Polling via tea.Tick**: Used Bubble Tea's `tea.Tick` chained message pattern instead of a goroutine. Stays within Bubble Tea's single-goroutine model.

## Coach directives incorporated

1. **started_at from tracks table** (CRITICAL) — Query: `SELECT id, current_slice, state, started_at FROM tracks WHERE release = ? AND state != 'planned' AND state != 'verified'`
2. **WAL audit** — Recorded in proof.md: WAL at internal/db/db.go:69
3. **db.Open() option (a)** — Use as-is, dropped "read-only" claim
4. **design_decisions** — 5 Type-2 entries in status.json
5. **Q2 closed** — Static credit load
6. **Q3 closed** — Auto-transition + `l`/`b` toggle

## Deferrals

- Blocked-slice TL;DR panel — deferred to S04c (spec §Out of scope, acknowledged by Coach in design review)
- Notification of state changes — deferred to S07 (spec §Out of scope)
- Credits purchase flow — deferred to S06b (spec §Out of scope)
- Web dashboard — deferred (spec §Out of scope)
- Mouse support — deferred (same pattern as S04a)