---
title: Rule 12 — Guard Fidelity
description: A check must be mutation-proved against the form the defect actually takes, and its scope must equal the scope of the claim it backs. A check narrower than its claim is a decoration.
---

# Rule 12 — Guard Fidelity

## The rule

A **guard** is any automated check whose purpose is to prevent a class of defect from recurring: a regression test, a lint rule, a CI gate, an invariant assertion. Guards are the only durable output of a quality effort. Everything else is a convention, and conventions lose.

Before a guard may be cited as evidence in a proof bundle (Rule 6) or relied on by a verifier (Rule 7), it must satisfy **four** conditions:

1. **Mutation proof.** The guard must be demonstrated to FAIL. Break the thing it protects, observe red, restore, observe green. Record the mutation and both outcomes in the proof bundle. **A guard that has never failed is not a guard; it is a decoration that returns green.**

2. **Scope parity.** The domain the guard *checks* must equal the domain the claim *quantifies over*. If the claim is "no component does X", the guard must search every component — not every component in one directory. **A check whose scope is narrower than the claim it backs is a decoration**, and it is worse than no check, because it converts an unknown into a false assurance.

3. **Mutate the form the defect ACTUALLY takes.** This is the condition that is nearly always violated. Authors mutate the form they *imagined* — and real defects arrive in forms they did not. A guard that catches only the shape you thought of will pass its own mutation test and still miss every real instance. Derive the mutation from **how the defect has actually occurred in this codebase**, not from how you would write it.

4. **Right instrument.** If detecting the defect requires resolving scope, bindings, or structure, use a **parser**, not a pattern match. A regex over a structured language is a guess that looks like a check.

### The corollary: quantifier discipline

**A universally-quantified claim is a promise about a search you have not run.**

"No X exists." "Every Y is Z." "This is machine-checked." "It never happened."

Each of those is a claim over a domain. State it only if a check covers that whole domain. Otherwise **bound the claim to the search you actually ran** ("no X in `packages/ui`") and say so. An unbounded claim backed by a bounded check is the single most common way a green suite ships a live defect.

## Why

Rules 6 and 7 assume the *evidence* is sound and adversarially verify the *delivery*. Rule 12 closes the gap underneath them: **the evidence itself can be structurally incapable of detecting the defect it claims to prevent**, and neither a proof bundle nor a fresh-context verifier will notice, because both see the same green.

A guard fails in one of exactly two ways, and the second is the dangerous one:

- **It fails loudly** — a broken guard that goes red on correct code. Annoying, self-correcting.
- **It fails silently** — a guard that returns green over a domain it never searched. This *adds confidence while removing safety*, and it is indistinguishable from success at every layer above it: the implementer's proof cites it, the verifier runs it, CI enforces it, and the defect ships.

The economics are stark. Writing the guard costs an hour. Writing the guard *wrong* costs every verification round that follows, plus the false confidence banked in every artefact that cites it.

## Priority-order note

Rule 12 is numbered last but sits **logically upstream of Rules 6 and 7**: it governs whether the evidence those rules rest on means anything. The number is an append, not a ranking — Baton's priority order breaks *conflicts* ("higher rules win"), and Rule 12 rarely conflicts with 1–11; it strengthens their foundation. It is numbered 12 rather than renumbered into the low positions because renumbering eleven established rules would break every reference in every adopter's pasted fragment, every vendored engine copy, and every provenance citation — a cost far larger than the small conceptual awkwardness of a foundational rule wearing a high number.

## Relationship to existing rules

| Rule | What it does | How Rule 12 relates |
|---|---|---|
| Rule 6 — Proof Bundle | Requires evidence generated from live repo state | Rule 12 requires that evidence be *sound* — a guard cited in a proof bundle must be mutation-proved against the real defect form and scope-matched to its claim |
| Rule 7 — Adversarial Verification | Fresh-context verifier grades delivery against the spec | The verifier sees the same green a silent guard shows; Rule 12 is what stops a structurally-blind guard from passing verification. Enforced in the verifier role prompt: before accepting a guard as evidence, mutate the form the defect actually took and confirm the guard fails |
| Rule 8 — Requirements Fidelity | The criterion must be bounded and enumerable | Same root cause one layer up: an unbounded AC is a claim wider than any check can discharge, just as a narrow guard is a check narrower than its claim |
| Rule 2 — No Silent Deferrals | Surfaces deferrals explicitly | A guard whose scope is narrower than its claim is an *undeclared* deferral of the uncovered domain — Rule 12 makes it a named condition rather than a silent gap |

## When this rule applies

- Any guard cited as evidence in a proof bundle or relied on by a verifier — regression test, lint rule, CI gate, invariant assertion, or a documentation/prose check.
- Any claim in a spec, proof, or verdict that quantifies over a domain ("no X", "every Y", "machine-checked", "never happens").

## When this rule does NOT apply

- Exploratory or scratch checks not cited as evidence — a guard becomes subject to Rule 12 the moment a proof bundle or verifier leans on it.
- A claim already bounded to the exact domain its check covers ("no undeclared colour in `packages/ui`") — that is Rule 12 satisfied, not exempted.

## Provenance

Derived from a design-system release in the source monorepo (2026-07-11/12), where a single guard — enforcing that UI components own their own styling — failed fresh-context verification **four consecutive times**, each time in a new disguise of one error: **the check's scope was narrower than the claim it backed.**

It was defeated, in turn, by:

1. **No word boundary inside an identifier.** `/\bfieldClassName\b/` does not match `termFieldClassName`. A clone shipped, and the guard's own name claimed it caught "every incarnation".
2. **A tag scanner that stops at the first `>`.** In JSX that is routinely the arrow in `onChange={(e) => ...}` — long before `className` is reached. (The codebase already contained a brace-aware scanner whose doc comment *warned about this exact bug*. It was walked into anyway, ten lines below the warning.)
3. **Literal-only class reading.** `className={someConst}` was invisible.
4. **Template literals.** `` className={`${a} ${b}`} `` — the extractor's `[^}]*` truncated at the first interpolation.
5. **`cn()` / `clsx()` composition.**
6. **Double-quoted bindings** (the resolver handled only single quotes).
7. **A basename-anchored exemption** (`/Input\.tsx$/` exempts *any* `Input.tsx`, anywhere).
8. **An incomplete file list** — two whole applications were outside the glob.
9. **A missing element type** (`<textarea>` was never in the list, so a textarea had no owner and went back to improvising).
10. **Fill-only surfaces** — the guard required a border *and* a radius, and the style the slice itself introduced was fill-first.

**Every one of those guards passed its author's own mutation tests.** Each author dutifully broke the thing, watched it go red, restored it, and recorded the proof — because each author mutated the form they *imagined*, and every real clone used a form they did *not*. That is condition (3), and it is why conditions (1) and (2) alone are insufficient.

A sibling slice in the same release failed verification **seven** times on the same root cause in prose rather than code: a documentation guard that asserted the **absence of a known-bad string** (`not.toMatch(/tremor/i)`) rather than the **presence of the truth**, and so sat green while the document stated a falsehood. Same disease: the check's scope (one string) was narrower than the claim's scope (the document is true).

Two live WCAG failures in that codebase — a primary button at 3.29:1 against a 4.5:1 floor, and a mobile touch target at ~20px against a 24px minimum — had shipped and persisted for the same structural reason: **there was no guard for them to violate.** Neither was ever *chosen*. Both were drifted into, at call sites, because the system had authority nowhere and enforcement nowhere.

The releasing engineer's summary, which is the rule in one line:

> *A guard that has to be clever is a guard that will be outsmarted. Ask "is this a field at all?" before you ask "is this styled like a field?" — the first needs a substring search, the second needs a compiler.*
