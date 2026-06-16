# SwornAgent Benchmark Report

**Generated:** 2026-06-16T23:00:00Z

## Summary

- **Models tested:** 8
- **Tasks:** 9 (S01–S09 slice specs with known-good diffs)
- **Cells:** 72
- **Total time:** 15m30s
- **Safe-hosted default:** `openai/o4-mini`

## Notes

- **Diff strategy:** known-good diffs (trivial comment addition to each spec). A PASS means the model correctly identified the change as non-violating.
- **Single attempt** per model × task (first-pass success rate).
- **Non-determinism:** model responses are inherently non-deterministic; re-running the benchmark may produce different pass-rates.
- **Partial failure:** if a model errors (API failure, timeout), the cell is marked ERR and excluded from pass-rate calculation.
- **Safe-hosted filter:** only models with provider `openai` + standard base URL are eligible for default selection (AC2).

## Results Table

```
model_id                 jurisdiction   S01-verifier-core        S02-model-client         S03-agentic-loop         S04-embed-prompts        S05-state-git            S06-implementer          S07-run-loop             S08-init-config          S09-distribution         pass-rate  total_cost
-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
openai/o4-mini           US (trusted)   PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                       100%     $0.0180
openai/o3-mini           US (trusted)   PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                       100%     $0.0214
openai/gpt-4.1           US (trusted)   PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                       100%     $0.0359
openai/gpt-4o            US (trusted)   PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                       100%     $0.0449
openai/o3                US (trusted)   PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                       100%     $0.2140
openai/gpt-4o-mini       US (trusted)   PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     PASS                     FAIL                        89%     $0.0076
openai/gpt-4.1-mini      US (trusted)   PASS                     PASS                     FAIL                     PASS                     PASS                     FAIL                     PASS                     PASS                     PASS                        78%     $0.0066
openai/gpt-4.1-nano      US (trusted)   FAIL                     PASS                     FAIL                     FAIL                     FAIL                     FAIL                     FAIL                     PASS                     FAIL                        22%     $0.0028
```

## Default Model Selection

**openai/o4-mini** — selected by the tie-break algorithm:
1. Filter to safe-hosted (provider == openai)
2. Highest pass-rate (100%)
3. Tie → lowest cost ($0.0180)
4. Tie → fewest non-PASS cells (0)
