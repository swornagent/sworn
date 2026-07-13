# Proof bundle — fix doctor-fix-agents-splice (2026-07-02)

Finding: `doctor --fix` AGENTS.md migration was destructive and non-convergent
(finding id `doctor-fix-agents-destructive-nonconvergent`, CONFIRMED; matches
open issue swornagent/sworn#43, plus two unreported aspects: backup clobber on
run 2 and never-converging re-migration).

## Scope

Make `sworn doctor --fix` migrate a legacy AGENTS.md by splicing out only the
Baton section (preserving all user content), converge (second run = no-op),
never clobber an existing `AGENTS.md.bak`, write replacement content that
references neither the legacy trigger heading nor the `docs/baton/` directory
the same run deletes, and fix the circular doctor→init→doctor advice.

## Files changed

`git diff --name-only 632d4f3`:

```
cmd/sworn/doctor.go
cmd/sworn/doctor_test.go
internal/adopt/adopt.go
```

## Test results

RED first (new tests against pre-fix code, all failing through the `cmdDoctor`
integration point):

```
--- FAIL: TestDoctorFixMigratesAgentsMD (0.01s)      # user content lost, trigger re-written, docs/baton/ pointer
--- FAIL: TestDoctorFixMigrationConverges (0.00s)    # run 2 re-migrated, exit 2, backup clobbered
--- FAIL: TestDoctorFixNeverClobbersExistingBackup   # existing .bak overwritten
--- FAIL: TestDoctorAdviceNotCircular                # advice points at 'sworn init' which refuses legacy
```

GREEN after fix:

```
$ go test -timeout 120s ./cmd/sworn/ ./internal/adopt/...
ok  	github.com/swornagent/sworn/cmd/sworn	43.189s
ok  	github.com/swornagent/sworn/internal/adopt	0.009s
```

`go vet ./cmd/sworn/ ./internal/adopt/` clean; touched files `gofmt`'d.

## Reachability artefact

Live run of the built binary (`go build -buildvcs=false -o bin/sworn
./cmd/sworn`) in a scratch git repo containing an AGENTS.md with user content
before AND after the legacy Baton section, plus `docs/baton/rule-1.md` —
the exact repro from the finding:

```
=== RUN 1 === (bin/sworn doctor --fix, exit 2)
== --fix: removing legacy docs/baton/ ==
  rm: rule-1.md
  removed docs/baton/
== --fix: migrating legacy AGENTS.md ==
  backed up old AGENTS.md to AGENTS.md.bak
  replaced legacy Baton section with MCP pointer (rest of file preserved)

AGENTS.md after run 1:
  # My Project                              <- preserved
  Custom onboarding: run kubectl apply ...  <- preserved
  ## Engineering Process                    <- replacement (no legacy trigger,
     ... served by the sworn MCP server ...    no docs/baton/ pointer)
  ## Deployment / Ship with make deploy.    <- preserved

grep -c 'My Project|kubectl|Deployment|make deploy' AGENTS.md = 4 (was 0 pre-fix)
grep -c 'kubectl' AGENTS.md.bak = 1 (original backed up)

=== RUN 2 === grep -c 'migrating legacy AGENTS.md' = 0 (was 1 pre-fix; converged)
grep -c 'kubectl' AGENTS.md.bak = 1 (backup NOT clobbered; was 0 pre-fix)
=== RUN 3 === exit 0, only AGENTS.md + AGENTS.md.bak present (no backup churn)
```

Re-run: `cd <scratch repo with legacy AGENTS.md + docs/baton/> && sworn doctor --fix && sworn doctor --fix`
— expect run 1 exit 2 with user content preserved, run 2 exit 0 with no
"migrating" line and the original still in AGENTS.md.bak.

## Delivered

- Splice migration: `migrateLegacyAgents` in `cmd/sworn/doctor.go` replaces
  only the Baton section(s) (heading → next `## ` heading or EOF), preserving
  surrounding content (test: `TestDoctorFixMigratesAgentsMD`).
- Convergence: replacement section `agentsMCPPointerSection` contains neither
  `adopt.BatonSectionHeading` nor `docs/baton/`; loop guarantees the trigger is
  gone (tests: `TestDoctorFixMigratesAgentsMD`, `TestDoctorFixMigrationConverges`).
- Backup safety: `AGENTS.md.bak` written only if absent; timestamped fallback
  `AGENTS.md.bak.<UTC>` otherwise (test: `TestDoctorFixNeverClobbersExistingBackup`).
- Circular advice fixed: doctor's legacy-AGENTS.md WARN now points at
  `sworn doctor --fix` instead of `sworn init` (test: `TestDoctorAdviceNotCircular`);
  init's existing "run 'sworn doctor' to migrate" advice is now truthful.
- Dark-code removal: `adopt.AgentsFragment()` deleted — its only consumer was
  the destructive overwrite this fix removes (`grep -rn AgentsFragment
  --include='*.go'` = 0 call sites).

## Not delivered

- No change to `sworn init`'s legacy handling (it correctly refuses and now
  points at a working migration path) — out of the finding's scope.
- The stale seven-rule `batonAGENTSFragment` in `internal/adopt` (still
  references `docs/baton/`, still used by `SpliceAgents`/`spliceOne` for the
  legacy splice path) is NOT reworked here. Why: reconciling the splice
  machinery with the MCP-pointer template is a design question beyond this fix.
  Tracking: swornagent/sworn#43 comment territory / audit punch list
  (Refs swornagent/sworn#51). Acknowledged here in plain text per Rule 2.

## Divergence from plan

- Added removal of the now-unused `adopt.AgentsFragment()` export (guidance said
  keep diff to applyFixes/advice + tests; leaving a dark exported helper whose
  only consumer was the removed destructive path would violate the repo's
  dark-code rule).
- Overlap note: `cmd/sworn/doctor.go` is a planned touchpoint of in-flight
  T1-drift-guard (render-drift release) — that slice touches the drift check,
  not `applyFixes`/`checkRepoArtifacts`; no functional collision expected.
