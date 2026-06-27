# Activity log — release 2026-06-15-e2e-turnkey-loop

- 2026-06-16T00:00:00Z — S01-verifier-core (T1-engine): implemented → verified. All 6 gates passed; live tests green; CLI smoke confirmed.
- 2026-06-16T06:00:00Z — S02-oai-model-client (T1-engine): implemented → failed_verification. Gate 3: spec prescribes table-driven httptest tests; implementation uses 4 separate functions instead.
- 2026-06-16T16:00:00Z — S02-oai-model-client (T1-engine): implemented → failed_verification. Gate 2: proof.md Divergence omits cmd/sworn/main.go wire touchpoint swap. Gate 4: no fresh CLI reachability artefact (Path: N/A; prior round cited).
- 2026-06-16T17:30:00Z — S02-oai-model-client (T1-engine): implemented → verified. All 6 gates passed; 22 tests green; reachability.txt confirms PASS/FAIL/BLOCKED CLI paths with correct cost_usd.
- 2026-06-16T00:00:00Z — S03-agentic-tool-loop (T1-engine): implemented → failed_verification. Gate 3: build error — computeCost return statement inside comment; no tests can run.
- 2026-06-16T08:30:00Z — S03-agentic-tool-loop (T1-engine): implemented → verified. All 6 gates passed; 5/5 agent tests green; reachability artefact confirmed at agent.Run() boundary.
- 2026-06-16T18:30:00Z — S04-embed-baton-prompts (T1-engine): implemented → verified. All 6 gates passed; 13/13 tests green (8 prompt + 5 verify); binary smoke confirms baton-protocol v1.0.0 embedded.
- 2026-06-16T18:45:00Z — track `T1-engine` merged to release-wt/ (commit f10649d). 4 verified slice(s): S01-verifier-core, S02-oai-model-client, S03-agentic-tool-loop, S04-embed-baton-prompts. Track state → merged.
- 2026-06-16T19:00:00Z — S05-state-and-git (T2-orchestration): implemented → verified. All 6 gates passed; 16/16 tests green; go vet + go build clean.
- 2026-06-16T21:00:00Z — S06-implementer (T2-orchestration): implemented → verified. All 6 gates passed; 6/6 tests green (implement package); go vet clean.
- 2026-06-16T10:00:00Z — S08-init-config (T3-turnkey-ux): implemented → verified. All 6 gates passed; 14/14 tests green; live sworn init smoke confirms config + docs/baton/ + AGENTS.md splice with idempotency.
- 2026-06-16T21:30:00Z — S09-distribution (T3-turnkey-ux): implemented → verified. All 6 gates passed; go test ./... green; make build + Docker build + both Docker smoke tests independently reproduced.
- 2026-06-16T22:00:00Z — track `T3-turnkey-ux` merged to release-wt/ (commit f37b730). 2 verified slice(s): S08-init-config, S09-distribution. Track state → merged.
- 2026-06-16T23:15:00Z — S07-run-loop (T2-orchestration): implemented → failed_verification. Gate 2: proof.md Divergence section omits internal/git/git.go and cmd/sworn/init.go as out-of-plan touchpoints.
- 2026-06-16T23:55:00Z — S07-run-loop (T2-orchestration): implemented → verified. All 6 gates passed; 6/6 internal/run tests green; 4/5 cmd/sworn tests pass (1 skip); merge invariant confirmed.
- 2026-06-17T00:15:00Z — track `T2-orchestration` merged to release-wt/ (commit 16db025). 3 verified slice(s): S05-state-and-git, S06-implementer, S07-run-loop. Track state → merged.
- 2026-06-16T23:59:00Z — S10-benchmark-dogfood (T4-proof): implemented → failed_verification. Gate 2: main.go unplanned touchpoint not in Divergence; Gate 4: AC3 reachability artefact absent (future-tense placeholder); Gate 5: AC3 deferred without tracking+ack, spec prohibits deferrals.
- 2026-06-17T00:45:00Z — S10-benchmark-dogfood (T4-proof): implemented → failed_verification. Gate 4/6 FAIL: AC3 dogfood merge not found in repo (no branch, README.md unchanged on main).
- 2026-06-17T02:30:00Z — S10-benchmark-dogfood (T4-proof): implemented → verified. All 6 gates passed; 10/10 bench tests green; dogfood commit 52ae89e confirmed on main; benchmark report table complete.
- 2026-06-17T02:45:00Z — track `T4-proof` merged to release-wt/ (commit 55848d2). 1 verified slice(s): S10-benchmark-dogfood. Track state → merged.

### 2026-06-16 — release merged to release/v0.1.0 (commit c1794bd)

- **Actor**: release integrator (/merge-release)
- **Note**: 10 verified slices merged (S01–S10), 0 deferred. Integration branch is `release/v0.1.0` (the v0.1 version branch cut from `main`); index.md "Release summary" already records this — the stale `main` checkout still showed the old value, corrected target confirmed by the release owner. A forward-merge of `main` into `release-wt/` was retained at the owner's request (commit 2fe3064), carrying main's docs-only drift (README phrasing + the unrelated `run-20260616` journal) into this snapshot. Slices remain in `verified` state until `release/v0.1.0` ships to production (i.e. merges to `main` = prod); at that point each slice's `status.json` flips to `shipped`. Branch `release-wt/2026-06-15-e2e-turnkey-loop` retained; remove with `git branch -D release-wt/2026-06-15-e2e-turnkey-loop` once you're sure no more work belongs to this release.
