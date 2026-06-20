# S26-telemetry — Journal

## Session 1: Implementation (2026-06-21)

### State transitions
- `design_review` → `in_progress` (commit 208ec07)
  - Transitioned to in_progress after Coach approved via approved-ack.md
  - Added open_deferrals entry for AC1/AC2 (T3/S09 cross-track dependency)
  - Cleared verification.result (stale from prior round)
  - set start_commit

### Coach decisions applied (from approved-ack.md)
1. **Pin 1** — AC1/AC2 Rule 2 deferral: Added to status.json open_deferrals
2. **Pin 2** — main.go four-way collision: Added §4 note in design.md
3. **Pin 3** — dispatch() version/help cases: Confirmed both return 0 explicitly
4. **Pin 4** — Meta-command exclusion: Coach pick (a) — exclude sworn telemetry *; version/help still fire
5. **Pin 5** — Config path: Coach pick (a) — hardcode ~/.config/sworn/
6. **Pin 6** — ShowDisclosure neutrality: Intentional — only shows in neutral state
7. **Pin 7** — ShowConsent contract: Documented signature ShowConsent(r io.Reader, w io.Writer) (bool, error); added TestShowConsent tests
8. **Pin 8** — sworn_version delivery: Coach pick (a) — Fire(cmd, sub, version string, durationMS int64, exitCode int)

### Flags applied
- (a) Renamed TestIsEnabled_Neither → TestIsEnabled_OptedIn_NoOverrides
- (b) go_version trimmed to major.minor in trimGoVersion()
- (c) sworn version/help fire telemetry (consistent with Pin 4)

### Files created
- `internal/telemetry/telemetry.go` — core telemetry package
- `internal/telemetry/telemetry_test.go` — 18 tests (all pass, no race)
- `cmd/sworn/telemetry.go` — sworn telemetry on|off|status subcommand

### Files modified
- `cmd/sworn/main.go` — extracted dispatch(), added ShowDisclosure + telemetry.Fire
- `docs/release/2026-06-19-safe-parallelism/S26-telemetry/design.md` — updated per Coach

### Test results
- `go test -race ./internal/telemetry/...` — PASS (18 tests)
- `go build ./cmd/sworn/...` — compiles
- `go test ./...` — all existing tests pass

### Open items (to be verified externally)
- AC1/AC2 (sworn init consent) will be verified when T3/S09 lands the init flow wiring
- api.sworn.sh/v1/events backend is not yet live — telemetry silently drops on connection refused
- TestFireSchema uses HTTP transport rewriting (not httptest on the real URL) which is correct for client-side verification

## Verifier verdicts received

### Round 1 — 2026-06-21: FAIL (3 violations)

- **Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant)
- **Slice**: S26-telemetry → state: **failed_verification**

**Violation 1 (Gate 2 — touchpoints mismatch + proof.md inaccurate):** The committed diff `start_commit..HEAD` includes the binary `sworn` (16 MB ELF, committed in `2da8599`). `3f496d1 chore: remove tracked sworn binary from repo` is present in the T9 branch's history, meaning the removal was already processed before implementation began and the implementer re-added it. The spec's planned touchpoints do not include the binary; `.gitignore` has `/sworn`. proof.md's "Files changed" section omits `sworn`, making the proof inconsistent with the live committed diff. Fix: `git rm --cached sworn && git commit` before re-submitting.

**Violation 2 (Gate 3 — required test absent):** Spec names `TestIsEnabled_Neither` as a required test (no env var, no sentinel → `IsEnabled()` returns false). journal.md Flag (a) notes "Renamed TestIsEnabled_Neither → TestIsEnabled_OptedIn_NoOverrides," but the renamed test covers a different path — it tests `.telemetry-enabled` present → true (case 3 in the logic). Case 4 (neither sentinel file exists → `IsEnabled()` returns false) — the "init not run → telemetry disabled" default — is covered by no test. Fix: add `TestIsEnabled_Neither` (or equivalent) that creates a clean temp home with no sentinel files and asserts `IsEnabled() == false`.

**Violation 3 (Gate 6 — claimed scope vs spec threshold):** Spec AC8 states "sworn run exits within 10ms." `TestFireNonBlocking` uses a 100ms threshold (10x more permissive). proof.md claims AC8 delivered against 100ms, not 10ms, without listing this in "Divergence from plan." The spec's `Required tests` section also says 100ms (internal contradiction), but the AC is the primary gate. Fix: either tighten the test to ≤10ms, or document the threshold divergence in proof.md "Divergence from plan" with rationale (goroutine-launch overhead in practice is ≪10ms; the test margin is deliberately conservative).

- **Next**: `/implement-slice S26-telemetry 2026-06-19-safe-parallelism` in a fresh session to address all 3 violations.

## Session 2: Re-entry — address verifier violations (2026-06-28)

### State transitions
- `failed_verification` → `in_progress` (commit e5759fa)
  - Re-entering implementation per verifier verdict; design unchanged
  - Cleared stale verification.result
  - Preserved start_commit (6593323)
  - Re-entry triggered via `/implement-slice S26-telemetry 2026-06-19-safe-parallelism`

### Violations addressed

**V1 (Gate 2 — tracked binary `sworn`):**
- Ran `git rm --cached sworn` to remove the binary from git tracking
- Verified: `git ls-files sworn` returns error (not tracked)
- `.gitignore` already has `/sworn` to prevent re-addition

**V2 (Gate 3 — missing `TestIsEnabled_Neither`):**
- Added `TestIsEnabled_Neither` before existing `TestIsEnabled_OptedIn_NoOverrides`
- Test creates a clean temp home dir with no sentinel files, asserts `IsEnabled() == false`
- This covers case 4 (no consent yet → telemetry disabled)

**V3 (Gate 6 — AC8 10ms threshold):**
- Tightened `TestFireNonBlocking` threshold from 100ms to 10ms
- Updated proof.md AC8 claim and Divergence from plan
- Goroutine launch overhead is <1ms in practice; 10ms is a generous margin

### Commit
- `f46ea72 fix(telemetry): address verifier violations — rm tracked binary, add TestIsEnabled_Neither, tighten Fire latency to 10ms`

### Test results
- `go test -race ./internal/telemetry/...` — PASS (19 tests, 1 added)
- `go build ./...` — compiles
- `go test ./...` — all existing tests pass

### Open items (unchanged from Session 1)
- AC1/AC2 remain deferred to T3/S09
- api.sworn.sh/v1/events backend not yet live