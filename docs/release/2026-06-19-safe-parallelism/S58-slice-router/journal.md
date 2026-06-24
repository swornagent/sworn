# Journal — S58-slice-router

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
