---
title: Rule 1 — Reachability Gate
description: TDD's failing test must render through the integration point that owns the user-facing affordance, not the leaf component in isolation
---

# Rule 1 — Reachability Gate

## The rule

For any feature that has a user-facing affordance (UI control, route, form field, API endpoint), the **first failing test in a TDD cycle must render through the integration point that owns the affordance** — not the leaf component in isolation.

## Why

The most common AI-assist failure mode is "dark code":

1. Plan calls for feature X with affordance Y.
2. AI agent (or human) writes a leaf component for X.
3. Agent writes a leaf-level unit test: `render(<X prop="...">)` with the prop set to what the affordance would produce.
4. Test passes green.
5. Agent marks task done.
6. Nobody ever wires X into a parent that *can produce* that prop value through Y.
7. Feature ships unreachable. Tests stay green. CI stays green. Reviewers don't catch it because the diff looks complete.

The trap is that the test was *technically valid* — the leaf does render correctly under that prop. But the test never asked "can the user reach this?" That question must be at the *top* of the TDD cycle, not the bottom.

## How to apply

- **The first failing test must render at or above the integration point that owns the affordance.**
  - For a UI toggle: render the parent panel/container that owns the toggle UI, simulate the click, assert the leaf's state changes.
  - For a form field: render the section/page that owns the field, fill it, assert downstream behaviour (validation, projection update, persistence).
  - For an API endpoint: assert via an integration test that hits the route, not by importing the handler function directly.

- **If the integration point can't render the feature yet** (because the toggle/state/route doesn't exist), THAT failure is the correct TDD red. Build the integration glue first; the leaf falls out.

- **Leaf-level unit tests are fine** *in addition* for edge cases (error states, boundary values, prop combinations). They cannot be the **sole** proof of life.

- **"Pass 1 / Pass 2" splits** — building a primitive now and wiring it later — are acceptable ONLY when:
  - The Pass 2 task is created in your tracker the moment Pass 1 lands.
  - Pass 2 has a named owner.
  - Pass 2 has a deadline or a clear unblocking condition.
  - All three are visible to the decision-maker, not just inferred from a code comment.

## Red flags

A new component, hook, or module is suspect if, after a phase merges, it:

- Is imported only by its own test file.
- Has no `grep` hits outside its own module's directory.
- Has a unit test that hardcodes a state value the user has no UI affordance to produce.
- Has a sibling component that would naturally consume it but doesn't.

A `grep` heuristic that surfaces these: list new files added during a phase; for each, run `grep -rln "<FileBaseName>" .` excluding test files. If zero hits outside the module's own directory, investigate before declaring the phase done.

## Phase completion artefact

Before marking any phase complete, produce a **reachability artefact**:

- A screenshot of the rendered affordance, OR
- A Playwright (or equivalent end-to-end test) run that clicks through to it, OR
- An explicit "open browser, do X, observe Y" smoke step that names the *user gesture* — not just "the test passes"

A green typecheck plus green unit-test suite is **not** a reachability artefact. End-to-end coverage is.

For release-mode slices whose artefact is a screenshot, the canonical path, per-track spec layout, and bit-stable capture pattern live in [`role-prompts/implementer.md`](role-prompts/implementer.md) → "Reachability screenshot convention". This rule defines *what counts*; the implementer prompt defines *where it goes* and *how to capture it reproducibly*.

## When this rule applies

- Any feature with a user-facing affordance.
- Any code with a contract surface (a public type, a schema, an API endpoint, a CLI flag) — even if not user-facing, the contract has a "consumer" that plays the role of the integration point.

## When this rule does NOT apply

- Pure utility functions with no consumer yet (rare — usually a smell that the utility is premature).
- Internal helpers exercised exclusively by their parent module (the parent module's test IS the integration test).
- Deliberate scaffolding clearly marked as such with tracking — see Pass 1 / Pass 2 conditions above.

## Provenance

The v0.5.0 audit on the source project's monorepo (May 2026) found five primitives shipped as dark code, all with passing TDD-written unit tests: per-section Summary/Detail mode toggle (component prop existed, parent hardcoded the literal), `SectionStatusBadge` (built + tested, zero render sites), `FieldErrorIndicator` (built, no consumers), `useCheckoutFlow` (496 lines, 26 tests, no consumer), per-line `taxRate` Pro gate inside `InvoiceSection.Detailed` branch (Detail mode unreachable from UI). Each had been "done" at the leaf-component test level. None were reachable.
