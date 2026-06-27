# Journal — S41-build-bin-target

## 2026-07-05: Implementation session

**State transition:** `design_review` → `in_progress` → (pending proof)

**Captain review pins addressed:**
1. **Pin 1 (AC4 evidence):** Replaced `verify --help` smoke step with live state-writing
   invocation (`./bin/sworn run --task "test write"` from repo root) confirming
   `.sworn/sworn.db` lands at repo root, not under `cmd/sworn/`. Backed by code
   citation to `internal/db/db.go:87` (`DefaultPath`) which constructs the path
   relative to workspace root.
2. **Pin 2 (design_decisions):** Populated `design_decisions` in status.json with
   the 5 §2 decisions (all Type-2) in S38-compatible format.

**Implementation:**
- No Makefile edits needed — existing Makefile (since S01) already has
  `build`, `test`, `vet`, `fmt`, `clean` targets with correct LDFLAGS.
- Created `docs/build.md` documenting canonical `make build` and
  run-from-repo-root convention.
- CI untouched — it uses `go vet ./...` and `go test ./...` directly,
  consistent with the Makefile targets.
- No changes to sworn's state-dir resolution — deferred per spec (Rule 2,
  Coach ack 2026-06-21).

**Deferrals (carried forward from spec/design):**
1. **sworn cwd-relative state-dir resolution** — deferred; Makefile+doc
   convention fixes observed clutter. Coach ack 2026-06-21.
2. **reachability smoke-step prompt wording** — deferred to
   S33-spec-template-hardening to avoid T3 prompt-ownership collision.
   (S33 is now verified but did not add make build wording; filed as GH #9.)

**Verification:**
- `make build`: produces `./bin/sworn` (12.8 MB), `git status` clean
- `make test`: all packages pass (30/30 ok)
- `make vet`: clean
- State-write test: `./bin/sworn run --task "test write"` from repo root writes
  `.sworn/sworn.db` at repo root, nothing under `cmd/sworn/`
## 2026-06-22: Recovery and finalisation session

**State transition:** `in_progress` → `implemented`

**Recovery note:** Prior session left the slice at `in_progress` with implementation
committed but no proof.md and state not advanced. The worktree was found on `main`
after a `sworn run` invocation (`./bin/sworn run`) performed a `git checkout`
internally — the CLI-tests-that-invoke-git isolation gate failure mode. Recovery:
- Restored track branch via `git checkout track/.../T12-harness-hardening`
- Dropped auto-checkpoint commit (0ac388f) — contained run-scratch pollution
- Files temporarily lost from disk; restored by re-checking out the track branch

**Completion:**
- Generated `proof.md` from live repo state
- Ran `release-verify.sh` first-pass: 12 PASS, 4 FAIL (spec/status/journal missing
  when worktree was on main; now resolved — see re-run below)
- Updated `status.json` → `implemented` with `actual_files`, `test_commands`,
  `reachability_artifacts`
- Deferrals carried forward with Coach acknowledgement intact

**Worktree safety incident:** `./bin/sworn run --task ...` from the track worktree
switched the branch away from `track/.../T12-harness-hardening`. This is a known
hazard. Filed as finding: GH #10.

## Verifier verdicts received

### Verdict 1 — 2026-06-23T03:48:00Z — PASS

**Verifier session:** Fresh context, artefact-only (Rule 7).

**Verdict:** PASS — All six gates pass.

**Gate walk:**
1. **User-reachable outcome exists:** `make build` at repo root produces `./bin/sworn` (12.8 MB). Entry point reachable and working.
2. **Planned touchpoints:** `docs/build.md` created as planned. `Makefile` not changed (already existed with required targets since S01) — explained in Divergence from plan. Four auto-checkpoint baton rules files in `682c59f` are coach-loop noise, not implementation scope.
3. **Required tests:** `make build && ls -la bin/sworn` passes, `make test` 30/30 ok, `make vet` clean.
4. **Reachability artefact:** `docs/build.md` exists with thorough documentation; `make build` produces executable; state-write test confirms `.sworn/` at repo root.
5. **No silent deferrals:** `docs/build.md` has no TODO/FIXME/placeholder markers. Both spec deferrals carry Rule 2 compliance.
6. **Claimed scope:** All four ACs verified — build produces binary, test/vet pass, docs exist, state writes to repo root.

**Environmental note:** Worktree was switched to `main` by external process twice during verification (known issue GH #10). Recovered each time and verified all commands executed on the track branch.