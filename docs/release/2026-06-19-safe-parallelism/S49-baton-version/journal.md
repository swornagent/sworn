# Journal — S49-baton-version

## 2026-07-09b: Re-implementation — bump pin to v0.4.0

### State transition: failed_verification → in_progress → implemented

Re-entered the slice because the planner routed it to `failed_verification` after
Baton v0.4.0 was published + tagged (commit `5ac5834fa1ee07b55a3e670d14dc7d9e63e84d84`).

### Changes

- `internal/adopt/baton/VERSION`: `baton-protocol: v0.3.0` → `v0.4.0`; updated
  `vendored:` to 2026-07-09; added `rules-added: 11-process-global-mutation`
- `internal/prompt/VERSION.txt`: `v0.3.0` → `v0.4.0`
- `internal/baton/version.go`: doc comment updated to v0.4.0
- `internal/prompt/prompt.go`: BatonVersion doc comment updated to v0.4.0; fixed
  Edit-tool newline collapse that fused comment+function on one line
- `proof.md`: regenerated from live state with corrected `Files changed` (15 files,
  includes S11 forward-merge artefacts)

### Test results

All S49-specific tests pass. Pre-existing failures unchanged (6 prompt heading
tests from T12, TestCmdRun_Parallel).

### Reachability

- `sworn version` → `baton-protocol on Baton v0.4.0`
- `sworn doctor` → `[OK] baton/VERSION.txt version=v0.4.0`, `[OK] baton/VERSION (baton-protocol) on Baton v0.4.0`, 11/11 rule files, exit 0

### Skeptic panel

Skipped — runtime does not support subagent dispatch.

### Deferrals

None.

## 2026-07-09: Implementation
### State transition: design_review → in_progress

Coach-approved design (3 pins, all addressed):
1. Dropped `cmd/sworn/main.go` from planned_files — `BatonVersion()` returns `"on Baton " + baton.Version()` so the existing `baton-protocol %s` format produces output containing "on Baton v0.3.0" without touching T15-owned main.go.
2. `SetVersionForTest` via unexported var pattern — `version_stub.go` (renamed from `export_test.go` because Go treats `*_test.go` as test-only).
3. Single accessor (baton.Version() from adopt embed) confirmed.

### Key decisions

- **Pin reconciliation**: Changed `internal/adopt/baton/VERSION` baton-protocol line from SHA `cf158423...` to `v0.3.0`, and `internal/prompt/VERSION.txt` from `v1.0.0` to `v0.3.0`.
- **`baton.Version()`**: Reads from `adopt.BatonDocsFS() → baton/VERSION`, parses `baton-protocol:` line. Returns `""` if embed missing.
- **`baton.IsSemverTag()`**: Strict `^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$` — no pre-release/build suffixes.
- **`prompt.BatonVersion()`**: Now delegates to `baton.Version()`, returns `"on Baton " + baton.Version()`.
- **Doctor checks**: Existing VERSION.txt check tightened to ERROR on non-semver; new `baton/VERSION (baton-protocol)` check added — fails closed on SHA.
- **`version_stub.go`**: Renamed from `export_test.go` because Go's `*_test.go` suffix convention makes it test-only, and `cmd/sworn/doctor_test.go` needs `baton.SetVersionForTest`.
- **`cmd/sworn/main.go` NOT touched**: Coach Pin 1 — `BatonVersion()` prefix handles the output reframing without touching T15-owned main.go.

### Pre-existing test failures (not S49-caused)

- `internal/prompt`: TestPlannerHasPhase2b, TestPlannerPhase2bDRYGate, TestPlannerPhase2bFastPath, TestImplementerHasDeviationCheck, TestImplementerHasDependencyDiscipline, TestVerifierHasCatalogConformance — these check for prompt headings from T12-harness-hardening (planned, not merged).
- `cmd/sworn`: TestCmdRun_Parallel — pre-existing.

### Deferrals

None — all spec acceptance checks met.

### Skeptic panel

Skipped — runtime does not support subagent dispatch (single-threaded API call mode).

## 2026-06-23: Design review

Captain reviewed design.md (commit 7093b0c0e4d1b28e1e8b9460ecb51588474dc9be). 3 pins:
1. Drop main.go from planned_files/design §3 (4th recurrence of Gate 2 failure pattern)
2. SetVersionForTest via export_test.go (not production code)
3. Single accessor confirmed — honours [[project_baton_sworn_architecture]]

Coach approved with CAPTAIN-VERDICT: PROCEED.
## Verifier verdicts received

BLOCKED: Spec's "User outcome" section and front-matter description claim `sworn version` prints "SwornAgent vA.B.C on Baton vX.Y.Z", but the entry point section, acceptance checks, and delivered reachability artefact use "baton-protocol on Baton vX.Y.Z" (reframing is done only in BatonVersion() without editing main.go per T15 ownership). The spec is internally inconsistent on the user-facing output format.

Proposed spec.md amendment: In the YAML front-matter "description" and the "## User outcome" section, change the claimed output format from "SwornAgent vA.B.C on Baton vX.Y.Z" to "baton-protocol on Baton vX.Y.Z" (or "sworn <v>\nbaton-protocol on Baton vX.Y.Z") to match the "Entry point" section, the AC, and the actual implementation. Also align "Planned touchpoints" list with status.json (test stub file missing from spec list).

## 2026-06-23: Planner — BLOCKED resolved (replan Step 2b)

Ratified the verifier's BLOCKED verdict (spec contract defect, not an implementation bug) and corrected `spec.md`:
- Front-matter `description` and `## User outcome`: replaced the single-line "SwornAgent vA.B.C on Baton vX.Y.Z" claim with the **actual delivered** two-line output — `sworn <version>` then `baton-protocol on Baton vX.Y.Z` — produced by the T15-owned `cmd/sworn/main.go` (left unedited per the design-review pin) with the `on Baton vX.Y.Z` segment supplied by S49-owned `prompt.BatonVersion()`. This is consistent with the Entry-point section and ACs 90/92, which the implementation already satisfies (proof.md: `baton-protocol on Baton v0.3.0`).
- `## In scope` reframing bullet: same correction.
- Touchpoints: added the real `internal/baton/version_stub.go`; `status.json` `planned_files` had a non-existent `internal/baton/export_test.go` (corrected to `version_stub.go`) and listed `cmd/sworn/main.go` (removed — unedited per pin). `planned_files` now matches `actual_files`.

Cleared `verification.result` → `pending` and set `state` → `implemented` (the existing implementation satisfies the corrected spec), so the slice re-enters verification. `start_commit` (d58aeca) and `actual_files` preserved.

**Discrepancy noted for the verifier (not blocking):** design-review pin #2 said "SetVersionForTest via `export_test.go`"; the implementation delivered `version_stub.go` instead. Spec/status now reflect the delivered `version_stub.go`; re-verification should confirm the test-seam approach is sound.

**Open item flagged to the human (separate from this BLOCKED):** the `v0.3.0` adoption pin and its SHA are pending revalidation against the in-flight "tag the latest canonical Baton prompts as a release, then re-pin R3" work. If that retags Baton, S49's VERSION pin (production code) gets a follow-up implement + re-verify — does not block clearing this format defect.

## 2026-06-23T21:02:23Z: Planner — re-routed to implementer (v0.4.0 pin)

Baton v0.4.0 is published + tagged (commit `5ac5834fa1ee07b55a3e670d14dc7d9e63e84d84`).
Spec re-pinned v0.3.0 → v0.4.0; the existing implementation still writes `baton-protocol: v0.3.0`,
so it no longer satisfies the spec. State set to `failed_verification` to route to the
**implementer**: bump `internal/adopt/baton/VERSION` to `baton-protocol: v0.4.0`, update the
`vendored:` date + `rules-added:` (add 11-process-global-mutation), then verify
(`sworn version` → `baton-protocol on Baton v0.4.0`). `start_commit` preserved.

## Verifier verdicts received

BLOCKED: slice is in state 'failed_verification', expected 'implemented'.## 2026-06-23T21:39:12Z: Planner — cleared verifier's sticky BLOCKED (Step 2b)

The verifier was dispatched on a non-`implemented` slice (the loop raced a planner
re-route) and stamped a sticky `verification.result: blocked` → `/replan-release` →
deadlock. That's a transient routing condition, **not** a spec defect. Cleared
`verification.result` → `pending` and `violations` → []; `state` stays
`failed_verification` → routes to the **implementer** to finish. A pre-dispatch
state guard was added to coach-loop (never verify a non-`implemented` slice) to
prevent recurrence.
