# Journal — S14-blocked-terminal

## 2026-07-11 — implementer session (design_review → implemented)

**Session start.** Coach acknowledgement verified committed (captain-proceed.md
@31c3290, verdict PROCEED, all 5 pins dispositioned). Worktree clean at 31c3290.
Step 0b: `verification.result` = pending — no BLOCKED guard. All four preceding
T4 slices (S05–S08) verified.

**in_progress transition (79fce54).** D1–D6 recorded in
`status.json.design_decisions` per Coach pin 2, sibling record shape. D3 folds
in the CRITICAL pin-1 correction (persist `supervisor.StateFailed`, never
`releaseTrack("blocked")` — `supervisor.Release` coerces unknown states to
`done`). `effort_complexity.confirmed_by_implementer: true` (low/high, puzzle —
agreed: small surface, threads through the just-landed S06/S07 state machine).

**Implementation (6c5866a).** Landed exactly the five surgical changes from
design.md, with every pin applied inline:

1. `internal/driver/driver.go` — additive `Result.BlockedReason` + StatusBlocked
   semantics binding (terminal, never inferred from prose). Zero-value default;
   no driver emits it this slice (spec out_of_scope 4).
2. `internal/orchestrator/blocked.go` (new) — `BlockedLaneSentinel` (value
   byte-identical to run's private prefix) + `BlockedLaneRouteSuffix`.
3. `internal/implement/implement.go` — early return on StatusBlocked before
   spec-record/proof/implemented transition (also fixes the latent
   spurious-certification bug design.md identified).
4. `internal/run/slice.go` — implement-leg blocked-terminal branch after the
   dispatch-ledger record (economics survive, S08 posture): writes
   `verification.result=blocked`, `routing=needs_planner`, violations =
   [BlockedReason] VERBATIM (fallbacks ResultText → "(no blocker reason
   provided)"), commits, notifies `blocked` once, returns the sentinel error
   with the `/replan-release` directive BEFORE any triage — resolveCount /
   modelIdx untouched. `errVerdictBlockedPrefix` aliased to the shared sentinel.
5. `internal/scheduler/worker.go` — `WorkerOptions.RecordBlocked` +
   `blockedLaneTerminal` helper wired at all three RunSliceFn-error sites
   (router implement/verify, redesign, legacy), checked after the pause
   sentinels and before breaker fingerprinting; `releaseTrack(supervisor.StateFailed)`
   (pin 1); skips breaker fingerprint + duplicate `track_failed` notify (D6).
6. `internal/run/parallel.go` — mutex-guarded blocked-lane collector wired as
   RecordBlocked on both launch paths; `renderBlockedVsFailReport` renders
   BLOCKED lanes (verbatim blocker + `-> /replan-release <release>`) vs FAIL
   lanes; byte-identical legacy error when no lane is blocked (D5).

Flag dispositions: (a) trim chosen — worker trims the route suffix from the
recorded reason so the report renders the directive once (asserted in the
AC-05 test); (b) declared in proof.md; (c) fused-comment grep + gofmt + vet
clean on all changed files; (d) full suite run before the transition.

**Tests (all in NEW files — zero existing test files edited, AC-06).**
`internal/run/blocked_terminal_test.go` (AC-01/02/03),
`internal/run/blocked_report_test.go` (AC-05),
`internal/scheduler/blocked_lane_test.go` (AC-04, also anchors pin 1 by
asserting the supervisor row is `failed`, not `done`). Full
`go test -count=1 -timeout 300s ./...`: every package ok.

**Gates.**
- `sworn lint ac 2026-06-28-driver-contract`: PASS (S14: 6 well-formed EARS
  ACs; release: 75/75, 0 violations).
- `sworn designfit 2026-06-28-driver-contract`: exits 2 on a PRE-EXISTING
  defect in S11-baton-revendor (quadrant "beast", invalid enum) before
  evaluating any other slice. Out of S14's touchpoints (track collision rule —
  T7 planner artefact); filed and tracked as **sworn#90**; not absorbed.
- `sworn coverage` / `sworn llm-check`: commands do not exist in this branch's
  binary, and no provider credentials are present in this environment (same
  posture as review.md flag (e), acknowledged by the Coach via pin-4
  catch-all). AC↔test coverage is instead traced manually in proof.json
  `delivered` (one named test per AC); the fresh-context Verifier (Rule 7)
  backstops. Declared in proof.json `not_delivered`.
- Proof-bundle first-pass gate: `sworn verify` (deterministic) run against
  spec.json + start_commit diff + proof.md — output captured in proof.json
  `first_pass` and below.

**Decisions / trade-offs beyond design.md (all minor):**
- The blocked branch writes the status record BEFORE `Stage(".") + Commit` so
  the commit always has content even when the blocked dispatch left no file
  edits (design.md's commit-first phrasing assumed edits exist; same intent —
  clean tree for the caller).
- `verification.routing` uses the literal `"needs_planner"` with a
  vocab-binding comment (verdict.Result.Routing doc / board.BlockedNeedsPlanner)
  rather than importing internal/board into slice.go for the constant.
- Declared touchpoints deliberately NOT changed, as designed:
  `internal/run/resolve.go`, `internal/run/parallel_test.go`,
  `internal/verify/`, `cmd/sworn/run.go`, `cmd/sworn/run_test.go`.

**Rule 2 deferrals (each with why + tracking + acknowledgement):** see
proof.json `not_delivered`.
