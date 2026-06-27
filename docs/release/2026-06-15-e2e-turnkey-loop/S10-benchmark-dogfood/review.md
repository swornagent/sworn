# Captain review — S10-benchmark-dogfood
Date: 2026-06-16T09:11:00+10:00
Captain version: 0.1
Design TL;DR commit: 36661d97866d17bc02d218cb31e60b3785f7618a

## Pins

1. [mechanical] §2.4 — Benchmark diff strategy is undefined
   What I observed: verify.Run requires SpecPath + DiffPath (verify.go:24–25,38–41). The design says "calls the existing verify.Run per model×task" using S01–S09 slice specs as the task set. But verified specs are not diffs. The design never states what diff each benchmark task would verify against.
   What to ask the implementer: Specify the diff strategy. Options: (a) a trivial "no-op" diff each spec passes to measure first-pass correctness, (b) one or more known-good diffs per slice harvested from the verified commits, (c) a synthetic adversarial diff to measure false-positive rate. The benchmark report must document which strategy was used.

2. [mechanical] §2.5 — Dogfood first-attempt assumption is an inference
   What I observed: §2.5 says "One attempt (no retry ladder needed — the system is already proven on S01–S09)." S07's run loop was proven on unit tests and smoke tests, not on a real repo commit. Environmental failures (missing API key, git permission, auth) on the first real sworn run are possible.
   What to ask the implementer: Smoke-test sworn run against a trivial task before claiming the dogfood attempt. If it fails for environmental reasons, fix and retry; the dogfood reachability artefact is the merged commit, not the first attempt. Drop the "already proven" framing and just run it.

3. [mechanical] §2.4 — SWORN_OPENAI_MODEL env var silently corrupts benchmarks
   What I observed: model.FromEnv (config.go:52–53) reads SWORN_<PROVIDER>_MODEL and overrides the model name if set. If a user has SWORN_OPENAI_MODEL=gpt-4.1 in their environment, every benchmark model ID — gpt-4o-mini, o3-mini, etc. — would silently resolve to gpt-4.1. The pass-rate table would be identical for all rows, with only cost varying by token count.
   What to ask the implementer: The benchmark runner must either (a) clear SWORN_OPENAI_MODEL before each iteration, (b) construct OAI clients directly (bypassing FromEnv's override), or (c) document that SWORN_OPENAI_MODEL must be unset for sworn bench and fail-early if it is set. Option (b) is cleanest — the benchmark is selecting models explicitly, so env-override is not the right behaviour.

4. [mechanical] §2.3 — Safe-hosted filter not explicit in default selection algorithm
   What I observed: §2.3 says "Highest pass-rate among safe-hosted models." The design §2.2 defines safe-hosted as openai provider with the standard US base URL. But the algorithm as stated has no explicit filter — it assumes every model in the matrix is safe-hosted. If a future benchmark run adds a non-OpenAI provider, the algorithm would treat it identically and could bless a non-trusted-hosted model as default.
   What to ask the implementer: Add an explicit safe-hosted gate in the default selection algorithm: filter to models whose provider is openai AND whose base URL matches the standard OpenAI API endpoint (or, more generally, a jurisdiction-check function). The spec AC2 ("The safe-hosted default is selected from benchmark data (no non-trusted-hosted model blessed as default)") is binding.

5. [escalate] §6.1 — Model matrix selection
   Coach decides: 8 OpenAI models as proposed, or a subset. The implementer's proposed matrix: gpt-4.1, gpt-4.1-mini, gpt-4.1-nano, gpt-4o, gpt-4o-mini, o4-mini, o3, o3-mini.

6. [escalate] §6.2 — Retries per model×task
   Coach decides: single attempt (first-pass success rate) as proposed, or N retries. The implementer's rationale — first-pass is the turnkey-experience metric — is sound but Coach owns the call.

7. [escalate] §6.3 — Report commitment strategy
   Coach decides: one-time committed report (v0.1 launch proof) as proposed, or CI-regenerated file. The implementer's rationale — v0.1 is a launch proof, not a dashboard — is sound but Coach owns the call.

8. [escalate] §6.4 — Dogfood task complexity
   Coach decides: trivial change (typo/README) as proposed, or a nontrivial code change. A typo proves the loop mechanism; a nontrivial change proves the agent's code-generation quality. Both are useful signals; Coach picks based on what the launch proof must demonstrate.

## Summary

Pins: 8 total — 4 [mechanical], 0 [memory-cited], 4 [escalate]
Critical pins (would cause the slice to ship broken if unaddressed): Pin 1 (diff strategy — benchmark can't run without it), Pin 3 (SWORN_OPENAI_MODEL override — silently corrupts results). Pins 2 and 4 are important but non-fatal if missed; Pins 5‑8 are Coach-owned decisions.

## Smaller flags (not pins, worth one-line ack)

(a) The benchmark report format (columns, sort order, JSON schema) is unspecified — fine as implementation detail, but confirm stdout table + JSON at docs/benchmark/ before coding.

(b) Benchmark error handling: if one model fails (API error, timeout) while others succeed, should the runner skip that model×task cell or abort? The design doesn't address partial failure.

(c) The design doesn't state whether benchmark runs are idempotent — given non-deterministic model responses, two runs may produce different pass-rates. Worth noting in the report.

## Suggested ack reply

TL;DR clean design — narrow scope, reuses existing verify.Run, dogfood is the right proof. 8 pins + 3 flags:

1. **Diff strategy.** verify.Run needs a diff; pick from (a) no-op diff, (b) known-good diffs from verified commits, (c) adversarial diff. Document the choice.
2. **Dogfood first-attempt.** Smoke-test sworn run before the dogfood; drop "already proven" framing; the merged commit is the artefact, not the first attempt.
3. **SWORN_OPENAI_MODEL override.** Must not leak into benchmark. Construct OAI clients directly (bypass FromEnv) or fail-early if the env var is set.
4. **Safe-hosted filter.** Add explicit safe-hosted gate in default selection algorithm (provider==openai + standard base URL). Don't rely on matrix pre-filtering.
5. **Model matrix.** 8 OpenAI models: gpt-4.1, gpt-4.1-mini, gpt-4.1-nano, gpt-4o, gpt-4o-mini, o4-mini, o3, o3-mini. Coach approves / modifies.
6. **Retries.** Single attempt per model×task (first-pass success rate). Coach confirms.
7. **Report commitment.** One-time committed report, not CI-regenerated. Coach confirms.
8. **Dogfood task.** Trivial change (typo/README) or nontrivial code change. Coach picks.

Flags (not pins): (a) confirm stdout table + JSON report format; (b) decide partial-failure behaviour (skip vs abort); (c) note non-determinism in report.

§2 decisions all clean — self-dogfood task set, OpenAI baseline, cost-aware tiebreak, verify.Run reuse, single-attempt dogfood. No memory conflicts (sworn project has no memories yet). §6 questions 1–4 routed to Coach.

Address pins 1–4 inline during implementation, Coach answers 5–8, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: 4 mechanical pins are apply-inline fixes (diff strategy, env-var isolation, safe-hosted filter, smoke-test) — no design re-review needed. 4 escalate pins are Coach decisions on benchmark parameters, answerable inline.
-->