# Rule 9 — Design Fidelity

**Meeting a requirement is not the same as the right solution for the whole.**
Solution fit is a quality the delivery verifier (Rule 7) cannot see. This rule
keeps design **human-owned**, AI-augmented, and calibrates how much human
judgement each choice demands by its stakes.

## Classification: stakes = reversibility x blast-radius

Every design choice has a **stakes class**:

| Class | Reversibility | Blast radius | Decision requirement |
|-------|--------------|--------------|---------------------|
| Type-1 | Hard to reverse | Wide / structural | Full human decision with options + rationale recorded |
| Type-2 | Easy to reverse | Narrow / local | AI may proceed with noted default |

**Architecturally-significant choices are always Type-1**, regardless of other
factors. A choice that shapes the whole system, the data model, the deployment
architecture, or an external contract is architecturally significant.

## Enforcement

`sworn designfit <release>` is a deterministic, fail-closed gate that reads each
slice's `design_decisions` from `status.json`. It checks:

1. Every Type-1 choice has a non-empty `human_decision` field — otherwise
   violates, naming the slice + choice.
2. Every `architecturally_significant` choice is classified Type-1 — otherwise
   violates, naming the slice + choice.

## Option surfacing

When the planner reaches a design choice during planning:

1. The planner drafts **at least two options** with trade-offs and prior art.
2. For Type-1 choices, the human selects one and records the decision +
   rationale in the slice's `status.json` `design_decisions` field.
3. For Type-2 choices, the planner records a noted default and proceeds.

The model may propose options, classify stakes, and surface trade-offs — but
for a Type-1 choice, the model **may not record the human decision itself**.

## Record format

Each design decision is recorded as an entry in `status.json`:

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

## Design-system input

Design fidelity requires a declared source of truth. Every UI-bearing project
must declare its design system in its sworn project config before design
conformance can be audited (S09).

The design system is a **three-tier concept**:

| Tier | Name | Role | Example |
|------|------|------|---------|
| Umbrella | **Design system** | The whole declared input — token source + component library | `design_system` in `config.json` |
| Atoms | **Design tokens** | The named-value source of truth (colours, spacing, typography) | `tokens.json` (W3C DTCG), CSS custom properties, JS theme object |
| Reusables | **Component library** | The coded, reusable UI components | `packages/ui/`, `src/components/` |

### Schema

A project's `config.json` carries an optional `design_system` block:

```json
{
  "ui_bearing": true,
  "design_system": {
    "token_source": "tokens.json",
    "component_library": "packages/ui"
  }
}
```

### Enforcement

- `ui_bearing: true` with no `design_system` block = fail closed (design
  conformance cannot proceed without a declared source of truth).
- `ui_bearing: false` or absent = design system not applicable (CLI projects
  and non-UI tools are exempt).
- The format hint for tokens is not mandated here — S09's audit adapts to
  the project's token format (DTCG JSON, CSS vars, JS themes).

### Discovery

`sworn init` prompts for the design system declaration when initialising a
UI-bearing project. The `--ui-bearing` flag marks the project explicitly.

## Design-system conformance audit

`sworn designaudit <project-dir>` runs a two-layer conformance audit:

### Layer 1 — Deterministic first-pass (machine-check)

Scans UI source files (`.css`, `.scss`, `.ts`, `.tsx`, `.js`, `.jsx`, `.vue`, `.svelte`)
for three categories of design drift:

| Category | Pattern | Example violation |
|----------|---------|------------------|
| **Hardcoded colour** | `color: #ff0000` | Hex literal in CSS property — use `var(--color-primary)` |
| **Off-scale spacing** | `margin: 17px` | Hard-coded `px`/`rem` value — use `var(--spacing-4)` |
| **Recreated component** | `function Button()` in app code | Component defined outside the library when a library `Button` exists |

Each violation is reported with `file:line: [kind] message`.

**Sanctioned exceptions:** append `/* sworn-design-allow */` to a line to suppress
its violation and record a deliberate, human-approved deviation.

### Layer 2 — Human cohesion verdict (human-owned)

The deterministic pass cannot assess whether the overall design **feels on-brand** —
typography consistency, visual rhythm, spacing coherence. This judgement is human-owned.

Supply it with `--cohesion=on-brand|off-brand`. The system will NOT auto-pass the
cohesion check; it requires a human-set value to reach exit 0.

### Exit codes

| Condition | Exit code |
|-----------|-----------|
| Machine violations found | 1 |
| Clean pass, no cohesion verdict | 1 (blocked until human sets verdict) |
| Clean pass + cohesion verdict recorded | 0 |
| Project is not `ui_bearing` | 0 (exempt) |
| Config error (no design system declared for UI-bearing project) | 2 |

### CI usage

`bin/design-audit.sh <project-dir>` wraps `sworn designaudit` for first-pass CI use.

## Out of scope (sibling rules)

- Design-system declaration (tokens + component library) — S08.
- Requirements validation (Rule 8) — design fit assumes the requirement is
  already validated.