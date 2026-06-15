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
