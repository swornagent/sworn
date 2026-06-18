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

## Out of scope (sibling rules)

- Design-system declaration (tokens + component library) — S08.
- Design-system conformance audit (no hardcoded hex, token-scale spacing) — S09.
- Requirements validation (Rule 8) — design fit assumes the requirement is
  already validated.