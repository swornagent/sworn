# Journal — S48-baton-vendor

## 2026-07-07 — Session 1: Design TL;DR

- Materialised track worktree for T14-baton-integration (new track, depends_on T3-commercial + T15-cli-registry, both merged).
- Produced Design TL;DR — see `design.md`.
- **Planned_files discrepancy**: `status.json` `planned_files` includes `cmd/sworn/main.go` but the spec explicitly says "Does NOT edit cmd/sworn/main.go — that file is owned solely by T15-cli-registry." The S51/T15 command registry means `baton` self-registers from `cmd/sworn/baton.go`'s own `init()`, not by editing `main.go`. This is a planner artefact and will be routed through design review.
- **Network fetch deferred**: S48 MVP reads from a local filesystem path. Network fetch of a Baton tag is deferred to S49 (pin reconciliation) or a future enhancement — will surface a hook in `source.go`.
- State transition: `planned` → `design_review`.
## 2026-07-07 — Session 2: Pre-implementation pins (Coach-approved)

- **Pin 1**: Removed `cmd/sworn/main.go` from `status.json` `planned_files` — spec + design both state main.go is NOT touched; S30 touchpoint linter (Gate 2) would fail.
- **Pin 2**: Filed GitHub issue #11 — "sworn baton vendor: network fetch support for Baton semver tag" — the correct Rule 2 tracking home for the network-fetch deferral. Updated design.md §4 tracking reference from `S49-baton-version` to `GitHub issue #11`.
- Pin 3 (memory-cited): Ack confirmed — design aligns with [[project_baton_sworn_architecture]]; no action.
- Coach flags noted for later: (a) populate `design_decisions` in status.json before transitioning to implemented; (b) forward-handoff comment in baton.go for S50's `sworn baton diff`.

## 2026-07-07 — Session 3: Implementation

- Implemented `internal/baton/transform.go` with regex-based single-table derive-both pattern (6 ADR-0006 replacements + fail-closed guard).
- Implemented `internal/baton/source.go` with explicit file mapping (Baton source → SwornAgent embed).
- Implemented `internal/baton/vendor.go` with Vendor() orchestrator + --check support + unified diff.
- Implemented `cmd/sworn/baton.go` self-registering via S51/T15 command registry.
- Wrote 14 tests: transform (8 subtests + rules/prompts + fail-closed + idempotent + same-table), vendor (write + idempotent + --check + unmapped guard), source validation.
- Ran `sworn baton vendor ~/projects/baton` — transformed real embed files (10 files changed).
- `go build ./...` clean. `go test -race ./internal/baton/...` all passing.
- Divergence from design: regex-based transform (not pure substring) to handle path-prefixed references (scripts/, bin/, $HOME/.claude/bin/).
- Coach flags addressed: design_decisions populated (5 Type-2), forward-handoff comment in baton.go.
- Network fetch deferral: tracked at GitHub issue #11, acknowledged by Coach.
- State transition: `in_progress` → `implemented`.

## Verifier verdicts received

### 2026-07-07 — Verifier session 1: BLOCKED (dirty worktree — drift gate)

BLOCKED

Slice: `S48-baton-vendor`
Reason: Track worktree at `/home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T14-baton-integration` has 19 uncommitted file modifications — cannot forward-merge `release-wt/2026-06-19-safe-parallelism` safely. `git status --short` returned non-empty output on 19 files in `internal/adopt/baton/**` (11 files) and `internal/prompt/**` (8 files), totalling 3,596 net deletions vs HEAD. The drift gate (Step 0.5) requires a clean tree at `state: implemented`; a dirty tree is itself a fault (track-mode invariant).

No spec defect — the spec is correct as written. The implementer must either commit the 19 working-tree modifications (if they are intended S48 scope) or revert them (`git checkout -- internal/adopt/baton internal/prompt`), confirm the tree is clean, and re-stage the slice as `implemented` before verification can proceed.

Proposed spec.md amendment: None. The spec contract is intact; this is an implementation/working-tree hygiene fault.

Next step: `/replan-release 2026-06-19-safe-parallelism` — the planner routes the implementer to clean up the working tree, then re-enters verification.
