# Journal: S13-walkthrough-attestation

## Session log

### 2026-06-26 10:00 — session start

- **State**: planned → in_progress
- **Notes**:
  - Created `internal/journey/shipgate.go` with `CheckShipGate` and `ShipGateResult`, using the existing `Attestation`/`AttestationArtefact` model from S15's `walkthrough.go`.
  - Created `cmd/sworn/ship.go` — the `sworn ship <release> [project-root]` command.
  - Updated `cmd/sworn/main.go` — added `case "ship"` dispatch and usage text.
  - Updated `internal/adopt/baton/rules/10-customer-journey-validation.md` — added walkthrough attestation section documenting the ship gate.
  - All attestation completeness checks (WalkPass, WalkedBy non-empty, RealInfra=true, MocksOff=true) are enforced by `attestationComplete()`.
  - The ship gate reuses S12's impact analysis to determine touched journeys and S15's attestation loading.
  - Removed the earlier conflicted `attestation.go` that had a different model — integrated with the existing `walkthrough.go` model instead.

## Open questions

- None.

## Deferrals surfaced

- Provisional attestation fields track S11 journey schema — refined via /replan-release (acknowledged 2026-06-16).

## Verifier verdicts received

### 2026-06-19 — FAIL (round 1, fresh-context)

FAIL

Slice: `S13-walkthrough-attestation`

Violations:
1. Gate 2 — `cmd/sworn/ship_test.go` appears in the diff but is not listed in the spec's planned touchpoints and is not explained in proof.md "Divergence from plan".
   Evidence: `git diff --name-only affb5227..HEAD` includes `cmd/sworn/ship_test.go`; spec's "Planned touchpoints" does not list it; proof.md "Divergence from plan" mentions `shipgate_test.go` but not `ship_test.go`.
2. Gate 2 — Planned touchpoints `internal/journey/journey.go` and `internal/journey/walkthrough_test.go` are not in the diff and are not explained in proof.md "Divergence from plan".
   Evidence: spec lists both under "Planned touchpoints (attestation model)"; neither appears in `git diff --name-only affb5227..HEAD`; proof.md "Divergence from plan" cites `walkthrough.go` from S15 but does not name these two files as unchanged or explain why each was not needed.
3. Gate 2 — "Divergence from plan" section begins with "None. Implementation follows the spec exactly:" while the same section then notes "(instead of modifying `internal/state/state.go`)" — a self-contradictory claim that undermines verifiability of the divergence record.
   Evidence: proof.md lines 67–74.

Required to address:
1. Add `cmd/sworn/ship_test.go` to proof.md "Divergence from plan" with an explanation (integration test companion to `ship.go`, not in the original plan).
2. Explicitly state in proof.md "Divergence from plan" why `internal/journey/journey.go` was not changed (e.g., T1/S11 already created the journey model; nothing additional was needed for S13).
3. Explicitly state in proof.md "Divergence from plan" why `internal/journey/walkthrough_test.go` was not changed (e.g., S15 already created the attestation model in `walkthrough.go` and `walkthrough_test.go`; S13 uses them directly via `shipgate.go`).
4. Remove the contradictory "None." leading text from "Divergence from plan" — the section has real divergences and the claim of "None" is false.
### 2026-06-26 12:00 - re-entry: fix verifier violations (round 2)

- **State**: failed_verification -> in_progress -> implemented
- **Notes**:
  - No code changes needed - all 4 verifier violations were in the proof.md
    "Divergence from plan" section, not in the implementation.
  - Violation 1 (ship_test.go not explained): Added to "Files in the diff but
    not in the spec's planned touchpoints" - integration-test companion to
    ship.go exercising cmdShip end-to-end.
  - Violation 2 (journey.go not in diff): Added to "Files in the spec's planned
    touchpoints but not in the diff" - S11 created it, S13 reads it but does
    not modify it.
  - Violation 3 (walkthrough_test.go not in diff): Added to same section - S15
    created it, S13 uses the Attestation model directly.
  - Violation 4 (contradictory "None"): Replaced with an honest structured
    section listing every deviation with explanation.
  - Also surfaced `internal/state/state.go` deviation (spec listed it but
    implementation went into shipgate.go instead).
  - `actual_files` updated in status.json to include ship_test.go and the
    consumed model files (walkthrough.go is consumed; index.md was updated).
  - verification.result set to null (ready for fresh verifier).
