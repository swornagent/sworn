# Journal — S48-baton-vendor

## 2026-06-23 — planner: cleared BLOCKED (corrupt vendor output reverted)

- **Trigger**: verifier BLOCKED — drift gate found 19 uncommitted modifications in
  `internal/adopt/baton/**` + `internal/prompt/**` (3,596 net deletions): a corrupt
  `sworn baton vendor` run that stubbed the embed (`internal/prompt/baton/rules.md`
  1112 → 29 lines). The corruption had been auto-checkpointed onto the track tip as
  `a29a33b` ("auto-checkpoint uncommitted work before replan-release").
- **Not a spec defect** — the verifier said as much; the verdict was operational
  (dirty/corrupt tree), and the proposed resolution was "revert the noise, confirm
  clean tree, re-stage as implemented".
- **Resolution**: `git reset --hard 924c07a` dropped the corrupt checkpoint `a29a33b`
  (local-only; origin was already at `924c07a`) — losslessly restoring the legitimate
  S48 vendor output (rules.md back to 1112 lines, all 10 rule docs intact). Clean tree.
- Cleared `verification.result` blocked → pending and `violations` → [] so S48 can
  re-enter verification; `state` stays `implemented`; `start_commit` untouched.
- **Root cause still open**: the `sworn baton vendor` transform stubbed rather than
  vendored — that bug needs the implementer's attention before re-verify. The recurring
  page is what motivated issue #11 (locked upstream fetch, slice S62) and the
  protocol-vs-operational reconciliation.

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

### 2026-07-08 — Verifier session 2: PASS

PASS

Slice: `S48-baton-vendor`
Verified against: `ec7c2f046e4e3ba091c2c43cee09589f1f1acd48`
Verifier session: `fresh, artefact-only`

All six gates passed:
- Gate 1: `sworn baton vendor` entry point wired via self-registration in cmd/sworn/baton.go init(); reachable from CLI.
- Gate 2: Planned touchpoints (internal/baton/*, cmd/sworn/baton.go) match actual changed files in S48 commit; vendored content updates are expected write targets.
- Gate 3: Required tests exist (transform_test.go, vendor_test.go) and exercise integration point; re-ran `go test -race ./internal/baton/...` (PASS); slice-specific tests in cmd/sworn pass when filtered.
- Gate 4: Reachability artefact (`sworn baton vendor --check` diff) proves the user path; transform replaces all script refs.
- Gate 5: No silent deferrals in implementation code (grep on Go files clean; matches in vendored docs are protocol content).
- Gate 6: All "Delivered" items have evidence references matching live state.

Open deferral (network fetch) is Rule-2 surfaced in proof.md with why + tracking + acknowledgement.

Next step: `/implement-slice S49-baton-version 2026-06-19-safe-parallelism` (next slice in T14-baton-integration track).
