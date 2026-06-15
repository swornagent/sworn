# Activity log — release 2026-06-15-e2e-turnkey-loop

- 2026-06-16T00:00:00Z — S01-verifier-core (T1-engine): implemented → verified. All 6 gates passed; live tests green; CLI smoke confirmed.
- 2026-06-16T06:00:00Z — S02-oai-model-client (T1-engine): implemented → failed_verification. Gate 3: spec prescribes table-driven httptest tests; implementation uses 4 separate functions instead.
- 2026-06-16T16:00:00Z — S02-oai-model-client (T1-engine): implemented → failed_verification. Gate 2: proof.md Divergence omits cmd/sworn/main.go wire touchpoint swap. Gate 4: no fresh CLI reachability artefact (Path: N/A; prior round cited).
- 2026-06-16T17:30:00Z — S02-oai-model-client (T1-engine): implemented → verified. All 6 gates passed; 22 tests green; reachability.txt confirms PASS/FAIL/BLOCKED CLI paths with correct cost_usd.
