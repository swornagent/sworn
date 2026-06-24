# Journal — S58-slice-router

## 2026-07-15 — Re-implementation session (Gate 2 touchpoint alignment)

### State transition: failed_verification → in_progress → implemented

### Root cause of Gate 2 FAIL

The re-implementation session set `start_commit` to `ec63795` (the re-impl start), but the `planned_files` listed the full first-pass scope. From `ec63795`, only `router.go` and `router_test.go` were modified (the other files were already committed from the first pass). The verifier correctly flagged the mismatch.

### Fix applied

- Reset `start_commit` to `a82b950` (original first-pass start) — captures the full implementation scope
- Removed `internal/git/git_test.go` from `planned_files` — it was created by S57 (commit `eb1127b`) and never modified by S58
- Verified diff from `a82b950..HEAD` matches `planned_files` exactly (7 code files + docs)

### No code changes

The router code (`router.go`, `router_test.go`) is identical to the prior re-implementation that already addressed the Gate 6 (`design.md` check for planned siblings) fix. This session was a docs-only alignment.

### Deferrals

*(none)*

---

## 2026-07-15 — Re-implementation session (verifier FAIL recovery)
### State transition: failed_verification → in_progress → implemented

### Verifier violations addressed

Two violations from the 2026-07-15 verifier session:

1. **Gate 2 — Touchpoint mismatch** (FIXED): Added `internal/board/oracle.go`, `internal/git/git.go`, `internal/git/git_test.go` to spec.md "Planned touchpoints" and `internal/board/oracle.go` to status.json `planned_files`. Both files are additive-only on existing packages (no method signatures altered on existing types). The spec now accurately reflects what the code touches.

2. **Gate 6 — design.md check for planned siblings** (FIXED): `routeNextSlice` now accepts `ctx` and `content ContentReader` parameters. In the `planned` case, it checks `content.CatFileExists(trackRef, designPath)` for the sibling slice. If `design.md` exists, it routes `review` (not `implement`), matching `captain-route.sh:474-478`. New test: `TestVerifiedWalksTrackThenMerges/next_planned_sibling_with_design.md_→_review`.

### Design decisions

- The `CatFileExists` check for planned siblings uses the track branch ref (committed-ref), same pattern as the `approved-ack.md` check in `routeDesignReview`. An uncommitted `design.md` is invisible — the implementer must commit before the router can see it.

- `ctx context.Context` added to `routeNextSlice` signature for consistency with other router functions that accept it; currently unused inside the function (the `ContentReader` interface methods don't take `ctx`).

### Deferrals

*(none)*## Verifier verdicts received

### 2026-07-15 — verifier verdict — FAIL

FAIL

Slice: S58-slice-router

Violations:

1. Gate 2 — Planned touchpoints do not match actual changed files.
   Evidence: spec.md "Planned touchpoints" lists only internal/router/* and cmd/sworn/route*; `git diff --name-only <start_commit>` shows internal/board/oracle.go and internal/git/git.go changed; proof.md "Divergence from plan" does not explain these (only mentions docs prefix and design.md check gap).

2. Gate 6 — Claimed scope matches implemented scope.
   Evidence: spec AC "verified with a later `planned` sibling routes to it (`review` if it has `design.md`, else `implement`)" is not implemented; routeNextSlice always routes planned to implement (no design.md check); TestVerifiedWalksTrackThenMerges only tests planned case, no design.md test; proof.md lists it as delivered but acknowledges "No `design.md` presence check in `routeNextSlice` for planned siblings" as "minor fidelity gap".

Required to address:
1. Align planned touchpoints in spec.md with actual (add board/oracle.go, git/git.go) or remove the extra changes; update proof.md Divergence section.
2. Implement design.md check for planned siblings in routeNextSlice (using ContentReader.CatFileExists on track ref), add test coverage, update proof.md to reflect delivery or defer per Rule 2.

STATE: blocked_needs_human
SLICE: S58-slice-router
NEXT: NONE
REASON: Gates 2 and 6 failed: touchpoint mismatch and verified sibling routing AC not met.

### 2026-07-15 — verifier verdict — FAIL (re-verification after re-impl)
FAIL
Slice: S58-slice-router
Violations:
1. Gate 2 — Planned touchpoints do not match actual changed files.
   Evidence: spec.md "Planned touchpoints" lists internal/board/oracle.go, internal/git/git.go, internal/git/git_test.go, cmd/sworn/route.go, cmd/sworn/route_test.go, internal/router/parity_test.go; `git diff --name-only ec63795caf94eec6c5c124027542ae38cffb1a65..HEAD` only shows internal/router/router.go and internal/router/router_test.go (plus docs). proof.md "Divergence from plan" claims "These were always in the actual diff" but that refers to prior commit ff14848 (before re-impl start_commit); the slice scope per start_commit does not include them.
Required to address:
1. Align planned touchpoints in spec.md and status.json `planned_files` with files actually changed since start_commit (only router.go + router_test.go + docs), or re-set start_commit to original implementation start if full scope intended. Update proof.md "Divergence from plan" and "Files changed" to match the verified scope.
STATE: blocked_needs_human
SLICE: S58-slice-router
NEXT: NONE
REASON: Gate 2 failed: touchpoint mismatch between spec planned and git diff vs start_commit.

### 2026-07-15 — verifier verdict — PASS
PASS

Slice: S58-slice-router

All six gates passed.

- Gate 1: User-reachable outcome exists — `sworn route <slice> <release>` entry point wired via `command.Register` in `cmd/sworn/route.go` (init), dispatched through `main.dispatch` → `cmdRoute` → `router.Route`. Exercised by `TestRouteIntegration` which builds and runs the real binary.
- Gate 2: Planned touchpoints match actual changed files — `git diff --name-only a82b950..HEAD` matches planned_files in status.json (core: router.go, router_test.go, parity_test.go, route.go, route_test.go, oracle.go, git.go); docs/* and S64/* are forward-merge artifacts from release-wt (documented in proof.md "Divergence from plan").
- Gate 3: Required tests exist and exercise the integration point — `internal/router/router_test.go` (table-driven per state, including `TestBlockedPrecedesState`, `TestDesignReviewCommitTimeNewest`, `TestFailedVerificationGateClassification`, `TestVerifiedWalksTrackThenMerges`, `TestGhostSliceFiltered`), `cmd/sworn/route_test.go` (Rule 1 reachability), `internal/router/parity_test.go` (golden against captain-route.sh). Re-ran `go test -race ./internal/router/...`, `./internal/git/...`, `go build ./...`, `TestRouteIntegration`, `TestCaptainRouteParity` — all PASS.
- Gate 4: Reachability artefact proves the user path — `TestRouteIntegration` in `cmd/sworn/route_test.go` creates temp git fixture with committed status.json for every state, builds `sworn`, runs `sworn route <slice> <release>` and asserts `.next.type` and JSON shape.
- Gate 5: No silent deferrals or placeholder logic — grep for TODO/FIXME/deferred/placeholder in *.go yields only legitimate "deferred" state name (terminal state like "shipped"/"verified", used in decision tree and tests; documented in proof.md first-pass output and Divergence).
- Gate 6: Claimed scope matches implemented scope — every AC in spec.md has evidence:
  - planned → implement: TestPlannedRoutesImplement
  - implemented/pending/stale → verify: TestImplementedRoutesVerify
  - blocked → replan-release: TestBlockedPrecedesState
  - failed_verification Gate 1/2/6 → redesign, else implement: TestFailedVerificationGateClassification
  - design_review by commit-time-newest (approved-ack, review, decline, design): TestDesignReviewCommitTimeNewest
  - verified walks track (next planned/review if design.md, merge-track/release): TestVerifiedWalksTrackThenMerges (includes design.md sibling case)
  - ghost-slice filter: TestGhostSliceFiltered
  - shipped/unrecognised → none: TestShippedRoutesNone, TestUnrecognisedStateRoutesNone
  - deferred → none + skipped in track walk: TestDeferredRoutesNone, TestDeferredSkippedInTrackWalk
  - parity: TestCaptainRouteParity (all 8 states match captain-route.sh)
  - Previous FAILs addressed: Gate 2 (start_commit reset to a82b950, git_test.go removed), Gate 6 (design.md check added to routeNextSlice + test).

Drift gate: clean (0 commits). Verified against track HEAD after clean forward-merge check.

**Gates passed**: 1–6.

STATE: verified_implement_next
SLICE: S58-slice-router
NEXT: S59-scheduler-relayer
REASON: All six gates passed. S59-scheduler-relayer is the next slice in track T17-orchestration-core.
