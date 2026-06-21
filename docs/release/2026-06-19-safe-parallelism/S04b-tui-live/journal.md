# Journal — S04b-tui-live

## State transitions

- **2026-06-28**: `design_review -> in_progress` — Coach approved design via `approved-ack.md` (6 pins, PROCEED). Captain verdict: all 6 pins mechanical; pins 5 and 6 (Q2/Q3) closed by spec. Implemented per approved-ack directives.
- **2026-06-28**: `in_progress -> implemented` — Proof bundle generated, release-verify.sh first-pass PASS (23/23). Skeptic panel skipped — runtime does not support subagent dispatch.

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

- Blocked-slice TL;DR panel — deferred to S04c (spec Out of scope, acknowledged by Coach in design review)
- Notification of state changes — deferred to S07 (spec Out of scope)
- Credits purchase flow — deferred to S06b (spec Out of scope)
- Web dashboard — deferred (spec Out of scope)
- Mouse support — deferred (same pattern as S04a)

## Summary

- 14 tests pass (5 existing + 9 new)
- All 6 Coach pins addressed inline during implementation
- First-pass verification: PASS (23/23)

## Verifier verdicts received

### 2026-06-28 — verifier verdict: FAIL (2 violations)

- **Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant)
- **Slice**: S04b-tui-live → state: `failed_verification`

**Violation 1 (Gate 1 + Gate 3 — CRITICAL): `Model.Update()` drops `tickMsg`; live view is static after initial poll.**
`internal/tui/model.go` lines 56–64: `Update()` handles only `tea.KeyMsg` and `tea.WindowSizeMsg`; all other messages fall to `return m, nil`. Bubble Tea delivers `tickMsg` to the root model when the tick fires — since there is no `tickMsg` case, the message is silently dropped. The tick chain (started by `lv.Init()`) terminates after its first fire. The DB is polled only once, synchronously, during `StartLiveView`; the live view shows stale data forever.
Spec acceptance check #2 — "The concurrent status table updates its elapsed time column every ~1 second" — is not met in the running TUI.
Rule 1 violation: `TestConcurrentStatusPoll` calls `lv.Update(tickMsg{})` **directly on `LiveView`**, bypassing `Model.Update()`. The leaf-level test passes while the integration path (model receives tick → forwards to LiveView) is broken and untested.

**Violation 2 (Gate 2): `internal/tui/styles.go` changed but not in spec's planned touchpoints, and proof.md "Divergence from plan" does not mention it.**
The file adds `LiveTitle`, `LiveRow`, and `DividerLine` styles consumed by `concurrent.go`. The change is legitimate but undisclosed in "Divergence from plan" as required by Gate 2.

**Required to address:**
1. Add a `tickMsg` case to `Model.Update()` (or a delegating else-branch) that forwards the message to `m.Live.Update(msg)` when `m.state == viewLive && m.Live != nil`, and chains the returned `tea.Cmd` so the next tick is scheduled.
2. Add an integration-level test that sends `tickMsg{}` through `Model.Update()` (not directly to `LiveView.Update()`), and asserts both that `m.Live.TickCount` increases and that `m.Live.Rows` is populated.
3. Add a brief entry in proof.md "Divergence from plan" for `styles.go` (touches not in planned touchpoints; adds `LiveTitle`, `LiveRow`, `DividerLine` styles for the live view).
### 2026-06-28 — re-implementation: all 3 verifier items addressed

- **Violation 1 fixed**: Added `tickMsg` case to `Model.Update()` (model.go:56-69) that forwards to `m.Live.Update(msg)` when `m.state == viewLive && m.Live != nil`, and chains the returned `tea.Cmd`. The tick chain now stays alive: `tickCmd -> tea.Tick -> tickMsg -> Model.Update() -> LiveView.Update() -> poll + chain next tickCmd`.
- **Integration test added**: `TestModelTickForwarding` sends `tickMsg{}` through `Model.Update()` (not directly on `LiveView.Update()`), asserts TickCount increases and Rows are populated across two consecutive ticks.
- **Violation 2 fixed**: Added `styles.go` entry to proof.md "Divergence from plan" disclosing `LiveTitle`, `LiveRow`, `DividerLine` additions.
- **Status updated**: `failed_verification -> implemented`, verification result cleared (stale from prior round).
- **Re-implementation session**: 2026-06-28 implementer on track T2-monitoring worktree.
- **15 tests pass** (5 existing + 10 new).
### 2026-06-21 — verifier verdict: PASS

- **Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant)
- **Slice**: S04b-tui-live → state: `verified`
- **Verified against**: `b53ba535672b29aee77630945b1563acfa02dbf8`

All six gates passed:
- **Gate 1**: `viewLive` state wired in root `model.go`; auto-transition through `handleReleasesKey` when `HasInProgressTracks` returns true; `l` key in `handleBoardKey`. Entry point is live user-reachable code.
- **Gate 2**: All 3 planned touchpoints changed. `styles.go` addition disclosed in Divergence from plan. Forward-merged docs files (S21, S27, S28, index.md, implementer.md, rules/10-...) are Step-0 drift-gate noise, not slice scope.
- **Gate 3**: All 4 required tests present and passing. `TestModelTickForwarding` sends `tickMsg` through `Model.Update()` (not just `LiveView.Update()`) — Rule 1 satisfied. 15/15 PASS on re-run.
- **Gate 4**: Proof names explicit user gestures (navigate to release, observe auto-transition, toggle b/l). Tests are integration-level (real SQLite, root Model path) — not just "tests pass."
- **Gate 5**: No TODO/FIXME/deferred markers in production code. All deferrals carry why/tracking/acknowledged.
- **Gate 6**: All 5 acceptance checks verifiable from code and test evidence.
