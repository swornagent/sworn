---
title: Rule 9 — Design Fidelity
description: Meeting a requirement is not the same as the right solution for the whole. Design stays human-owned and AI-augmented, with the amount of human judgement calibrated to each choice's stakes (reversibility x blast-radius).
---

# Rule 9 — Design Fidelity

**Meeting a requirement is not the same as the right solution for the whole.** Solution fit is a quality the delivery verifier (Rule 7) cannot see — the verifier checks the diff against the spec, but the spec does not encode whether *this* design was the right one for the system. Rule 9 keeps design **human-owned**, AI-augmented, and calibrates how much human judgement each choice demands by its stakes.

## Classification: stakes = reversibility × blast-radius

Every design choice has a **stakes class**:

| Class | Reversibility | Blast radius | Decision requirement |
|---|---|---|---|
| Type-1 | Hard to reverse | Wide / structural | Full human decision with options + rationale recorded |
| Type-2 | Easy to reverse | Narrow / local | AI may proceed with a noted default |

**Architecturally-significant choices are always Type-1**, regardless of other factors. A choice that shapes the whole system, the data model, the deployment architecture, or an external contract is architecturally significant — and therefore Type-1 — even if it feels locally reversible.

The Type-1/Type-2 split is the well-known "one-way vs two-way door" heuristic applied per choice. Its purpose is to spend scarce human attention where it matters: forcing a human decision on every trivial reversible call drowns the genuinely consequential ones.

## Option surfacing

When the planner reaches a design choice during planning:

1. The planner drafts **at least two options** with trade-offs and prior art.
2. For Type-1 choices, the human selects one and records the decision + rationale in the slice's `status.json`.
3. For Type-2 choices, the planner records a noted default and proceeds.

The model may propose options, classify stakes, and surface trade-offs — but for a Type-1 choice the model **may not record the human decision itself**. (This is the design-time analogue of Rule 7: the agent that proposes is not the authority that decides.)

## Record format

Each design decision is an entry in `status.json`:

```json
{
  "design_decisions": [
    {
      "choice": "database-engine",
      "stake_class": "Type-1",
      "options": ["PostgreSQL", "SQLite"],
      "human_decision": "PostgreSQL",
      "rationale": "migrations matter and we already have the infra",
      "architecturally_significant": true
    }
  ]
}
```

## Enforcement

A deterministic, fail-closed gate reads each slice's `design_decisions` and checks:

1. Every Type-1 choice has a non-empty `human_decision` field — otherwise it violates, naming the slice + choice.
2. Every `architecturally_significant` choice is classified Type-1 — otherwise it violates, naming the slice + choice.

This is the design-time counterpart to the delivery first-pass: cheap, deterministic, and run before model or human review time is spent.

## Design-system input (UI-bearing projects)

Design fidelity for a UI requires a declared source of truth. Every UI-bearing project declares its design system before design conformance can be audited. The design system is a three-tier concept:

| Tier | Name | Role |
|---|---|---|
| Umbrella | **Design system** | The whole declared input — token source + component library |
| Atoms | **Design tokens** | The named-value source of truth (colours, spacing, typography) |
| Reusables | **Component library** | The coded, reusable UI components |

A project config carries an optional declaration:

```json
{
  "ui_bearing": true,
  "design_system": {
    "token_source": "tokens.json",
    "component_library": "packages/ui"
  }
}
```

- `ui_bearing: true` with no design-system declaration = fail closed (conformance cannot proceed without a source of truth).
- `ui_bearing: false` or absent = not applicable. CLI projects and non-UI tools are exempt.

## Design-system conformance audit

A two-layer conformance audit guards UI-bearing projects against design drift.

### Layer 1 — Deterministic first-pass (machine-check)

Scans UI source files for three categories of drift:

| Category | Pattern | Example violation |
|---|---|---|
| **Hardcoded colour** | `color: #ff0000` | Hex literal in a CSS property — use `var(--color-primary)` |
| **Off-scale spacing** | `margin: 17px` | Hard-coded value off the spacing scale — use `var(--spacing-4)` |
| **Recreated component** | `function Button()` in app code | Component re-defined outside the library when a library `Button` exists |

Each violation reports `file:line: [kind] message`. A sanctioned exception marker (an inline allow-comment) suppresses a single line's violation and records a deliberate, human-approved deviation.

### Layer 2 — Human cohesion verdict (human-owned)

The deterministic pass cannot assess whether the overall design *feels on-brand* — typography consistency, visual rhythm, spacing coherence. That judgement is human-owned. The audit will **not** auto-pass cohesion; it requires a human-set `on-brand` / `off-brand` verdict to reach exit 0. A clean machine pass with no cohesion verdict stays blocked.

## Relationship to existing rules

| Rule | What it does | How Rule 9 complements it |
|---|---|---|
| Rule 7 — Adversarial Verification | Verifies the diff against the spec | Rule 9 governs the choice the spec doesn't encode — *was this the right design* |
| Rule 8 — Requirements Fidelity | Verifies the requirement is right | Rule 9 assumes the requirement is already validated and governs the solution's fit |
| Rule 2 — No Silent Deferrals | Surfaces deferrals explicitly | Rule 9 makes an unrecorded Type-1 decision a hard, detectable gate failure |

## When this rule applies

- Any slice that makes a design choice with structural reach or hard-to-reverse consequences.
- Any UI-bearing project, for the design-system conformance audit.

## When this rule does NOT apply

- Purely local, easily-reversed implementation choices (Type-2) — a noted default is sufficient.
- Non-UI projects, for the conformance-audit half (the stakes-classification half still applies).

## Provenance

Rule 9 was introduced in the `2026-06-16-fidelity-layer` cycle alongside Rule 8. It closes the design half of the fidelity gap: Rule 8 ensures the requirement is right; Rule 9 ensures the solution chosen to meet it is right for the whole — a quality the delivery verifier structurally cannot assess from the diff.
