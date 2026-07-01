---
title: 'S17 — Declare three Rule-10 critical journeys in .sworn/journeys.json'
description: 'Declare and human-ratify three Rule-10 critical journeys (keyless-full-loop, loop-verifier-negative, ship-a-release) in .sworn/journeys.json using the canonical journeys-v1 shape from S16; entitlement/credits no-mock boundary declared on J1.'
---

# Slice: `S17-journeys-declare`

## User outcome

`.sworn/journeys.json` exists in the repo and is human-ratified; it declares three journeys with entitlement/credits no-mock boundaries; the `journey.Check()` gate passes on merge-release (satisfying S05's journey gate requirement).

## Entry point

`sworn journey --generate --release <release-name>` (or equivalent) generates the journey stubs; human (Brad) reviews and ratifies via `sworn journey --ratify`; the resulting `.sworn/journeys.json` is committed.

## In scope

- `.sworn/journeys.json`: declare three journeys using the canonical journeys-v1 shape (S16 must be implemented first):
  - **J1 — keyless-full-loop**: user type = Coach (keyless subscription); steps = plan via /plan-release → `sworn run --release <name>` (full implement+verify loop) → merge; no-mock boundary = `entitlement/credits` (the subscription/credits check cannot be mocked — must cross the real entitlement boundary); reachability_test_path = TBD (manual attestation at ship)
  - **J2 — loop-verifier-negative**: user type = Coach; steps = submit a deliberately thin implemented slice to the loop verifier → assert the verifier does NOT advance to `verified` (the negative path); no-mock boundary = `loop-verifier` (real agentic verifier must run, not the stateless judge); reachability_test_path = TBD
  - **J3 — ship-a-release (surface-seam)**: user type = Coach; steps = /plan-release (Driver 1) → `sworn run` (Driver 3) → observe via TUI/MCP (Driver 2) → escalate and resolve a BLOCKED slice via /implement-slice → merge and /mark-shipped (Driver 1); no-mock boundary = `real-board/real-gates` (all three drivers operate against a real board.json and real gate suite); reachability_test_path = TBD
- Ratify the artefact with `ratified_by: "brad@sawyer.net.au"` and `ratified_at: <date>`
- The ratified `.sworn/journeys.json` is committed to the integration branch (not a track branch — it is a cross-release artefact)
- `journey.Check()` returns true after this slice is implemented

## Out of scope

- Walking any journey against real infra (that happens at ship cutover)
- Automated test harness for journeys (future release)
- Eliciting additional CLI journeys (onboard/init, develop-feature) — deferred

## Planned touchpoints

- `.sworn/journeys.json` (new file)

## Acceptance checks

- [ ] `.sworn/journeys.json` exists in the repo root (or `.sworn/` directory) after this slice
- [ ] The artefact parses as valid journeys-v1 JSON against the embedded schema (S16)
- [ ] The artefact contains exactly 3 journeys: `keyless-full-loop`, `loop-verifier-negative`, `ship-a-release`
- [ ] Each journey has a declared `no_mock_boundary` field naming its boundary (entitlement/credits, loop-verifier, real-board/real-gates)
- [ ] `artefact.IsRatified` is `true` (human-ratified during this slice)
- [ ] WHEN `journey.Check(projectRoot)` is called after this slice, THE SYSTEM SHALL return `(result.Pass, artefact, nil)` where `result.Pass` indicates the artefact exists and is ratified
- [ ] `sworn merge-release` does not BLOCK on the journey gate after this slice

## Required tests

- **Unit**: `internal/journey/journey_test.go` — add a test that calls `Check()` with a repo containing the newly declared `.sworn/journeys.json` and asserts it returns `Pass`
- **Reachability artefact**: `sworn journey --check` (or `go test ./internal/journey/... -v -run TestCheck`) exits 0 with the new file in place; manual smoke step: `sworn merge-release --dry-run` does not BLOCK on the journey gate

## Risks

- This slice depends on S16 (journeys-v1 nested shape) being implemented first; if S16 is not yet merged when T4 reaches S17, the implementer must work with whatever shape is current
- `.sworn/journeys.json` commits to the integration branch directly (not a track branch); this is correct since journeys are a cross-release artefact and must be visible to all track branches

## Deferrals allowed?

Yes — `reachability_test_path` for each journey is TBD (manual attestation at ship cutover). Rule 2: Why = real-infra walk requires a deployed release, which happens after merge. Tracking = /mark-shipped ceremony for this release. Acknowledged = Brad, 2026-06-27.
