---
title: LLM checks
description: The six deterministic LLM check types — the prompt bodies, the shared user-payload contract, and the structured report every check returns. Specification, not implementation.
---

# LLM checks

Baton specifies six **deterministic LLM check types**. They sit alongside the
mechanical gates: a mechanical gate answers a question a parser can settle, an LLM
check answers a question that needs reading comprehension — *does this test actually
verify the thing it claims to verify?*

These are **specification**. The prompt body IS the contract, in the same way a
schema is the contract for a record. An engine that reworded them would be running
different checks under the same names, and a second engine could not be conformant
without them. They live here, not in any one engine.

The reference implementation runs them as `sworn llm-check --check <name>`.

## The six checks

| Check | Run by | Reads | Answers |
|---|---|---|---|
| [`spec-ambiguity`](spec-ambiguity.md) | planner | spec | Is any acceptance criterion vague, incomplete, or underspecified? |
| [`design-review`](design-review.md) | captain | project memory + diff | Does this change conflict with a documented decision? |
| [`ac-satisfaction`](ac-satisfaction.md) | implementer, verifier | spec + diff | Does the code genuinely satisfy each AC? |
| [`security-review`](security-review.md) | implementer, verifier | diff | Does the change introduce a vulnerability? |
| [`semantic-coverage`](semantic-coverage.md) | verifier | spec + test diff | Do the tests genuinely verify their claimed ACs? |
| [`maintainability-review`](maintainability-review.md) | implementer, verifier | diff | Will this code be understandable in 12 months? |

## The contract every check honours

**Deterministic.** Temperature 0. The same slice and the same diff must produce the
same verdict. A check that drifts between runs cannot gate anything.

**Structured output.** Every check returns a single JSON object validating against
[`llm-check-report-v1`](https://baton.sawy3r.net/schemas/llm-check-report-v1.json).
The report is *emitted and validated*, never prose-scraped — a check whose verdict has
to be read out of an English paragraph is a check that will eventually be misread.

**Fails closed.** A check that cannot run is a FAIL, not a pass. Absence of evidence
is not evidence of absence (Rule 7).

### Grading: severity and blocking are orthogonal

Every finding carries **two independent fields**, and keeping them apart is load-bearing:

| Field | Question | Values |
|---|---|---|
| `severity` | **Impact** — how bad is this, if real? | `critical` `high` `medium` `low` `info` |
| `blocking` | **Disposition** — does this finding fail the check? | `true` `false` |

One severity scale across all six checks. Each check's prompt states which of its findings
block; that is the only place the mapping lives.

**The verdict is derived, not asserted.** `verdict` is `FAIL` **if and only if** at least
one finding has `blocking: true`. The schema enforces this in *both* directions: a `FAIL`
with no blocking finding is invalid, and — the important one — **a `PASS` carrying a
blocking finding is invalid**. An engine whose own tally disagrees with the model's stated
verdict must fail closed.

> **Why this is a contract, not a style preference.** These were originally two vocabularies
> in one field: five checks graded `FAIL`/`WARN`/`INFO`, and `security-review` graded
> `critical`/`high`/`medium`/`low`. The reference engine decided whether a check blocked by
> scanning findings for `severity == "FAIL"` — a string `security-review` never emits. The
> security check's blocking logic was therefore dead code, and the gate silently degraded to
> trusting the model's own `verdict`. A model could return `verdict: "PASS"` beside a
> `critical` remote-code-execution finding and the check went green.
>
> That is a Rule 12 failure: a guard whose *scope* was narrower than the *claim* it backed.
> Separating impact from disposition, and deriving the verdict from the findings, makes that
> state unrepresentable rather than merely discouraged.

**Advisory to the role, not a substitute for it.** A PASS from `ac-satisfaction` does
not make a slice verified. The checks are inputs to a role's judgement, and the
verifier still owns the verdict.

## The user payload

Each check file's body is the **system prompt**, verbatim. The **user payload** is
assembled by the engine and is common to all six:

```text
You are evaluating a slice in a release of {{project_context}}.

Below is the slice specification, followed by the git diff of the code change.

--- SPECIFICATION ---

{{spec}}

--- GIT DIFF ---

{{diff}}
```

`{{project_context}}` is a one-line description of the adopting project, supplied by
the engine from the repo's configuration — for example *"a Next.js and TypeScript
web application"*. It is a **required** substitution, not a default: a
check that tells the model it is reading a Go CLI while it reads TypeScript is
grading against the wrong priors, quietly and in the model's favour.

`{{spec}}` is the slice's `spec.json` rendered to readable form (ADR-0009); a
pre-migration `spec.md` may be passed verbatim.

Checks that read no spec (`security-review`, `maintainability-review`,
`design-review`) omit the specification section. `design-review` substitutes the
project's memory / decision records for it.
