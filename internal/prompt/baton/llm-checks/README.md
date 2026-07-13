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

{{project_stakes}}

Below is the slice specification, followed by the git diff of the code change.

--- SPECIFICATION ---

{{spec}}

--- GIT DIFF ---

{{diff}}
```

### `{{project_context}}` and `{{project_stakes}}` — declared, not guessed

Both are filled from the project's **declared** context record
([`project-context-v1`](https://baton.sawy3r.net/schemas/project-context-v1.json)) — a
hand-authored, version-controlled file in the repo. They are **required** substitutions,
not defaults.

`{{project_context}}` completes the sentence *"You are evaluating a slice in a release
of ___"* — for example *"a Next.js and TypeScript frontend with a Go backend on
Postgres"*. A check that tells the model it is reading a Go CLI while it reads
TypeScript grades against the wrong priors, quietly and in the model's favour.

`{{project_stakes}}` renders the record's `stakes` — production, real users, sensitive
data, regulatory regime. **The security-review check acts on it mechanically**: at high
stakes a `medium` finding is blocking, not advisory. An information leak is a different
severity in a prototype than in a live system holding customer financial data, and the
check must be told which it is looking at.

> **Why declared and not detected.** An engine can infer a project's *languages* from its
> files. It cannot infer whether the system serves real customers or holds money — and
> that is exactly what should move a finding from advisory to blocking. Detection is a
> guess; a guess handed to the model as a fact is graded against as a fact. An engine that
> falls back to detection **must say so** (surface it as inferred, not declared), and must
> treat unstated stakes as **high**: an undeclared system is not a safe one, it is an
> unexamined one.

### How the record gets written: elicited → ratified → durable

The same three-step Rule 10 applies to journeys, for the same reason — and nobody
hand-writes a good one from a blank file.

1. **Elicited.** At project setup, the engine has the adopter's model already configured
   (it needs one to run the checks at all). It uses **that** model to *draft* the record by
   reading the repo: the stack, the frameworks, the data layer, and a **proposal** for the
   stakes — a model can see the auth code, the payment integration, the schema holding
   customer records.

2. **Ratified.** A human reviews and edits the draft, then ratifies it. The model can read
   the code; it cannot know whether *real people depend on this today*. That is a business
   fact, and it is the one that decides whether a `medium` finding blocks. **An unratified
   record is a proposal, not a declaration: its stakes are treated as HIGH until a human
   confirms otherwise.** A proposal may raise the bar; it may never lower it.

3. **Durable.** The record is committed. Every session, every teammate, and CI all read the
   same context — instead of each re-guessing it from directory names.

> **The elicitation call is the adopter's, not the protocol's.** It runs through the
> adopter's own configured model and credentials, against their own provider. Baton
> specifies no hosted service and no phone-home, and an engine must not introduce one here:
> drafting this record means sending repository content to a model, and where that content
> goes is the adopter's decision — a data-residency and privacy question, not a convenience
> one. An engine with no model configured must fall back to detection and label it inferred,
> never silently reach out to a third party to fill the gap.

`{{spec}}` is the slice's `spec.json` rendered to readable form (ADR-0009); a
pre-migration `spec.md` may be passed verbatim.

Checks that read no spec (`security-review`, `maintainability-review`,
`design-review`) omit the specification section. `design-review` substitutes the
project's memory / decision records for it.
