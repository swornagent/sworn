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
