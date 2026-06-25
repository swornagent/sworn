---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S29-lint-deps`

## 2026-06-21 — planned (replan)

Added during `/replan-release` to harvest fix §3a #1 (theme T-B) from the Captain
design-review trial-log analysis (`2026-06-21-captain-trial-log-harvest.md`). A
slice that adds a Go dependency without declaring `go.mod`/`go.sum` in `planned_files`
trips Gate 2 at verify. Evidence rows: `S04a-tui-foundation` (bubbletea + lipgloss →
Gate 2 FAIL risk), `S08b-mcp-ops-tools` (yaml.v3 claimed in go.sum but absent), and
`S31-newrelic-windout-backend` (private project; go.sum diff-review step absent). The fix is a
`sworn lint deps` check that diffs go.mod/go.sum against the slice's planned_files and
fails closed, paired with a planner note to auto-add those files on any dep change.

**Rationale:** mechanise the most-deferred dependency-declaration check so it runs
pre-verify rather than surfacing as a late Gate-2 diff failure.

Placed in new track `T12-harness-hardening` (depends_on `T1-concurrency-core`) with the
other harvested harness-hardening lints (S30, S31, S32, S33, S35).

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

### 2026-06-27 — PASS

All six gates passed:
- Gate 1 (User-reachable): `sworn lint deps` wired through `main.go` → `cmdLint` → `cmdLintDeps`.
- Gate 2 (Touchpoints): 4 planned touchpoints match 4 changed code files; merge noise from forward-merge is excluded.
- Gate 3 (Tests): `TestDepsUndeclaredFails`, `TestDepsDeclaredPasses`, `TestDepsNoChangePasses` all PASS; `go build ./...` and `go vet ./internal/lint/...` clean.
- Gate 4 (Reachability): manual smoke step output shows exit 1 with "undeclared dependency file(s): go.mod" matching spec.
- Gate 5 (Silent deferrals): no TODO/FIXME/placeholder/hack in any changed file.
- Gate 6 (Claimed scope): all 4 Delivered items match acceptance checks with verifiable evidence.

Forward-merged 9 commits from release-wt/2026-06-19-safe-parallelism before verification (drift gate). Slice's actual scope from two feat commits: `cmd/sworn/lint.go`, `internal/lint/deps.go`, `internal/lint/deps_test.go`, `internal/prompt/planner.md`.
## Coach note — 2026-06-21 20:09 AEST

Partial internal/lint/deps.go from a crashed dispatch was cleared (backup at /tmp/T12-S29-partial-lint-200255). Worktree
  clean. Start S29 fresh per spec + approved-ack.md: derive release from status.Release for the start_commit-null fallback, and populate
  design_decisions before in_progress. Use os.ReadFile, not deprecated io/ioutil.

## 2026-06-27 — design_review → in_progress → implemented

**Captain pins addressed (approved-ack.md):**
1. CheckDeps null-fallback: derives `"release-wt/" + st.Release` from `status.Release` via `internal/state.Read`. No extra caller parameter needed.
2. design_decisions populated with `{choice, stake_class, rationale}` shape (4 Type-2 decisions).

**Flag (a) addressed:** `cmdLint` usage strings updated to include `deps` target with `[--base <ref>] <slice-id> <release>` arg shape.

**Implementation decisions:**
- Rewrote `internal/lint/deps.go` from scratch (WIP from crashed dispatch had a local Status struct, 4-space indent, three-dot diff). Now uses `internal/state.Read` for status.json parsing, tabs, two-dot diff (`baseRef..HEAD`).
- Added `--base` flag to `cmdLintDeps` for testability (spec Risks section calls this out).
- Tests create temp git repos with real commits to exercise the diff logic end-to-end.
- Sorted undeclared file names in error message for deterministic output.

**Trade-offs:**
- Two-dot diff (`baseRef..HEAD`) vs three-dot (`baseRef...HEAD`): two-dot captures exactly the commits on the current branch since baseRef, which is what we want for "what did this slice change." Three-dot would show changes since the merge base, which could include upstream changes.
- Using `internal/state.Read` instead of a local struct: consistent with the rest of the codebase, but couples `internal/lint` to `internal/state`. This is fine — `internal/rtm` and `internal/ears` are leaf packages and `internal/state` is a core package with no dependencies on them.

**Skeptic panel:** skipped — runtime does not support subagent dispatch.

**Reachability:** verified via `bin/sworn lint deps` against three fixture temp repos (undeclared → exit 1, declared → exit 0, no-change → exit 0).