# Proof bundle — S35-mutation-guard

## Scope

Add a standing Captain design-review check (Step 7) that flags any design
touching process-global state (`os.Chdir`, raw `git` with cwd, worktree
creation/switching, global env/cwd mutation in tests) without guaranteed
restore, non-empty-dir assertion, and a reachability artefact. Codify the
same guard as Baton Rule 11 (`11-process-global-mutation.md`).

## Files changed

<!-- from `git diff 65be8da08b8a4cfeb414f89f37f5da7949bae6f9..HEAD --stat` -->

```
 cmd/sworn/doctor.go                                |   4 +-
 cmd/sworn/doctor_test.go                           |   7 +-
 .../S35-mutation-guard/status.json                 |  35 +++++--
 internal/adopt/adopt.go                            |   4 +-
 .../baton/rules/11-process-global-mutation.md      | 105 +++++++++++++++++++++
 internal/prompt/captain.md                         |  46 ++++++++-
 6 files changed, 185 insertions(+), 16 deletions(-)
```

Full list:
- `cmd/sworn/doctor.go` — add `11-process-global-mutation.md` to `batonRuleFiles`
- `cmd/sworn/doctor_test.go` — update expected rule count from 10/10 to 11/11
- `docs/release/2026-06-19-safe-parallelism/S35-mutation-guard/status.json` — add `design_decisions` (3 Type-2), set `start_commit`, transition state
- `internal/adopt/adopt.go` — register new rule file in `files` slice
- `internal/adopt/baton/rules/11-process-global-mutation.md` — **new**: Rule 11 clause codifying the guard
- `internal/prompt/captain.md` — add Step 7 "Process-global mutation guard" to the review function

## Test results

### `go build ./...`

```
BUILD: PASS
```

### Doc-content checks (AC verification)

```
=== AC1: Captain step 7 check ===
1  (Process-global mutation guard heading)
3  (os.Chdir references)
1  (expected-dir assertion reference)
1  (Reachability artefact reference)

=== AC2: (a) restore (b) non-empty-dir (c) reachability ===
1  (Guaranteed restore)
1  (Non-empty / expected-dir assertion)
1  (Reachability artefact showing the guard)

=== AC3: Baton-rule clause exists ===
-rw-rw-r-- 1 brad brad 4975 Jun 23 01:14 internal/adopt/baton/rules/11-process-global-mutation.md

=== AC4: sworn#6 reference ===
3  (references to sworn#6)

=== AC5: go build passes ===
BUILD PASS
```

## Reachability artefact

The "user-reachable artefact" for a prompt/rule change is the prose itself.

### Captain Step 7 — verbatim check block (from `internal/prompt/captain.md` lines 157-199):

```
### Step 7 — Process-global mutation guard

Process-global mutation (`os.Chdir`, a raw `git` invocation with a cwd argument,
worktree creation/switching, or a global env/cwd mutation in tests) is a
**systematic** failure class — sworn#6 (a git op with an empty dir flipped a
worktree to `main`) and its recurrence on S28 (`os.Chdir` → `t.Chdir`) are two
instances of the same root cause. This step makes the catch systematic.

For each design.md §3 file path and each sibling's `planned_files` touching the
same Go code:

1. **Scan for the four patterns.** Read the files the design plans to touch.
   Search for:
   - `os.Chdir` — any call, test or production
   - `exec.Command("git", ...)` with a `Dir` field set — a raw git invocation
     with a cwd argument
   - Worktree creation/switching — `git worktree add`, `git worktree remove`,
     `git checkout` targeting a different branch, or any operation that changes
     the repo's working directory
   - Global env/cwd mutation in tests — `os.Setenv`, `os.Setwd`, `os.Environ()`
     mutation, `os.Args` mutation

2. **For every match, verify three properties are present in the design:**
   - **(a) Guaranteed restore.** The state is restored before the owning
     function returns. Acceptable: `t.Chdir` (test-scoped), `defer <restore>()`,
     or a cleanup callback that runs irrespective of test outcome.
   - **(b) Non-empty / expected-dir assertion.** Any git operation with a cwd
     argument first asserts the directory exists, is non-empty, or matches an
     expected path. The assertion must fail closed.
   - **(c) Reachability artefact.** The slice cannot reach `verified` without
     a reachability artefact showing the guard: a test exercising the restore
     path, a test run screenshot, or an explicit smoke step proving the
     non-empty-dir check fires.

   If a match exists and any of (a), (b), or (c) is missing → pin:
   `[mechanical]` (if the fix is a one-line test-scoping change) or
   `[escalate]` (if the design needs rework). Use the exact pattern and missing
   property in the pin text.

3. **If no match exists** — none of the four patterns appear in the design's
   touchpoints — no pin. The slice is clean on this class.

The governing Baton-rule clause is `internal/adopt/baton/rules/11-process-global-mutation.md`
(Rule 11). Cite it in any pin surfaced here.
```

### Baton Rule 11 — verbatim (from `internal/adopt/baton/rules/11-process-global-mutation.md`):

See the full rule at `internal/adopt/baton/rules/11-process-global-mutation.md` (105 lines). The rule codifies:
- (a) **Guaranteed restore** — `t.Chdir`, `defer`, cleanup callback
- (b) **Non-empty / expected-dir assertion** — fail-closed git ops
- (c) **Reachability artefact showing the guard** — before `verified`
- Cites **sworn#6** as motivating bug (github.com/swornagent/sworn#6)
- References **S28-git-dir-guard** and **trial-log harvest §5 (theme T-F)** as provenance

## Delivered

1. ✅ `internal/prompt/captain.md` contains Step 7 standing check — fires on `os.Chdir`, `git` with cwd, worktree creation/switching, global env/cwd mutation (AC1)
2. ✅ Step 7 requires (a) guaranteed restore, (b) non-empty/expected-dir assertion, (c) reachability artefact (AC2)
3. ✅ Baton-rule clause `internal/adopt/baton/rules/11-process-global-mutation.md` codifies the same guard (AC3)
4. ✅ Rule clause references sworn#6 class as motivating bug (AC4)
5. ✅ `go build ./...` passes — no Go breakage (AC5)
6. ✅ `design_decisions` populated in status.json (3 Type-2 entries matching design.md §2) — Captain pin 1 addressed
7. ✅ Captain flag (a): S36 handoff noted — S36 touches `captain.md` sequentially after S35 in T12; no collision
8. ✅ Captain flag (b): stale planned_files reference to `02-no-silent-deferrals.md` in §4 NOT-doing — the NOT-doing item itself is correct; no action required

## Not delivered

None. All acceptance checks satisfied.

## Divergence from plan

**Mechanical registration files changed beyond spec's two planned_files.**
The spec listed `internal/prompt/captain.md` and `internal/adopt/baton/rules/11-process-global-mutation.md` as planned touchpoints. Three additional files required mechanical changes to register the new rule:

- `internal/adopt/adopt.go` — the `files` slice that extracts embedded rules at `sworn init` time must list the new file
- `cmd/sworn/doctor.go` — the `batonRuleFiles` list that `sworn doctor` checks must include the new file
- `cmd/sworn/doctor_test.go` — the expected rule count assertion must change from 10/10 to 11/11

These are registration-only changes (no logic change); the embed directive (`//go:embed baton/rules/*`) already covers new files automatically. This is not a scope expansion — it's the minimum mechanical registration required for any new rule in the vendored baton directory.

**S36 captain-resolve-dirty-worktree handoff.** S36 (T12, planned, after S35) also touches `internal/prompt/captain.md`. S35 inserted Step 7 between Step 6 and `## Output`; S36's future edit should land cleanly after Step 7 without touching its content. Named here as proof-bundle commitment per Captain flag (a).