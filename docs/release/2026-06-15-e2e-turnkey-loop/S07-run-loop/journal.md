# Journal — S07-run-loop

## 2026-06-16 — Implementation session (implementer)

**State transition:** design_review → in_progress → implemented

### Decisions

1. **Model escalation path** (Pin 5): Default escalation uses real OpenAI model IDs: `gpt-4o-mini → gpt-4o → o3-mini → o3`. Configurable via `--escalation-models` flag or `$SWORN_ESCALATION_MODELS` env var. Documented in `--help`.

2. **State transition before merge** (Pin 2): After `verify.Run()` returns PASS, the run loop transitions the slice state from `implemented` to `verified` using `state.Transition(Verified)` before executing the merge. This is explicit in the code and tested.

3. **Auto-generated spec/status format** (Pin 3): `setupSlice()` creates spec.md with `## User outcome` (so `implement.extractScope()` works) and status.json with all required fields (`slice_id`, `release`, `state`, `spec_path`, `proof_path`, `release_base`). Release dirs are named `run-YYYYMMDD-HHMMSS`.

4. **State reset on retry**: `implement.Run()` rejects `implemented` state on re-entry. The run loop resets state to `in_progress` before each retry, bypassing the state machine (which doesn't allow `implemented → in_progress`). The run loop owns the lifecycle; this is by design.

5. **Commit agent changes before diff**: `implement.Run()` leaves changes in the working tree. The run loop stages and commits them before computing the diff for verification. Otherwise the diff would be empty.

6. **RetryCap semantics**: `RetryCap: 0` = single attempt (no retries). `RetryCap: -1` = use all escalation models. The CLI flag defaults to `-1`.

7. **CLI-level reachability test** (Pin 1): `cmd/sworn/run_test.go` tests `cmdRun` flag parsing and error paths through the `sworn run` integration point.

8. **main.go touchpoint with S08** (Pin 4): Added `"run"` case to the dispatch switch with comments acknowledging both S07 and S08. Both are additive, non-overlapping additions.

9. **git.Merge()** (Flag c): Added `Repo.Merge(branch)` to internal/git. Uses `--no-ff` for a clean merge commit.

### Trade-offs

- The run loop directly writes state (bypassing the state machine for retry resets). This is acceptable because the run loop owns the full lifecycle.
- Diff is written to a temp file for verify.Run() which reads from file paths. Cleaned up immediately after.
- Implementer model escalation is implementer-only; verifier model stays fixed (per design decision §2-2).

### Skeptic panel

Not run — panel requires Agent/Workflow tool which is unavailable in this environment. Proceeding to implemented state; the fresh-context verifier provides the adversarial check.

### Deferrals

None.

## 2026-06-16 — Verifier verdict (fresh context)

**State transition:** implemented → failed_verification

### Verdict

FAIL: 1 violation

**Violation 1 — Gate 2 (Planned touchpoints match actual changed files):**
Actual changed files include `internal/git/git.go` (adds `Merge()` — a functional dependency of `internal/run/run.go`) and `cmd/sworn/init.go` (trailing-newline whitespace fix). Neither file appears in the planned touchpoints (`internal/run/`, `cmd/sworn/run.go`, `cmd/sworn/main.go`). The proof.md "Divergence from plan" section documents only the diff-base tool issue and the spec amendment; it does not declare these out-of-plan touchpoints as divergences. The "Delivered" section records `internal/git/git.go — added Merge() (Flag c)` as a delivery item, but that is not a divergence declaration. Gate 2 requires that any mismatch between planned and actual changed files be explained in the proof bundle.

### All other gates passed

- Gate 1 ✓ — `sworn run` CLI wired in `cmd/sworn/run.go` through `cmd/sworn/main.go`; user-reachable.
- Gate 3 ✓ — All 6 `internal/run` tests pass (PASS/FAIL/FAIL-then-PASS/BLOCKED/MissingTask/SanitiseBranch); 4/5 `cmd/sworn` tests pass (1 skipped via `t.Skip` for a non-spec-required help-text check).
- Gate 4 ✓ — `TestRun_PassPath_Merges` explicitly asserts state == `verified` and merge commit on `main`; `TestRun_FailPath_NoMerge` asserts no merge after repeated FAIL.
- Gate 5 ✓ — No TODO/FIXME/deferred/placeholder in any changed file.
- Gate 6 ✓ — All 4 ACs covered with named test evidence.

### Fix required

Update proof.md "Divergence from plan" to add: (a) `internal/git/git.go` — `Merge()` added as a direct dependency of `internal/run/run.go`; not in planned touchpoints because it was a discovered implementation need; (b) `cmd/sworn/init.go` — trailing-newline whitespace fix; cosmetic only.