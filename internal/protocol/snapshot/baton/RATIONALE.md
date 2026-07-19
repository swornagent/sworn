# Why Baton became smaller

Baton 0.x grew from real failures. Each addition was locally reasonable, but
the combined protocol made every model repeatedly learn incident-specific
procedures. Baton 1.0 preserves the trust properties and retires the universal
ceremony.

| Baton 0.x concern | Baton 1.0 home |
|---|---|
| Reachability gate | B3 Real Evidence; evidence declares its exercised boundary |
| No silent deferrals | B1 Bounded Authority + B2 Durable Truth |
| Session discipline | B2; only durable records carry authority or completion |
| Capture discipline | B2; records and addressable evidence, not prescribed prose |
| Commit messages as capture | Optional implementation choice, not protocol |
| Proof bundle | Submission record under B2/B3 |
| Adversarial verification | B4 Independent Verification |
| Requirements fidelity | B1 bounded acceptance; deeper analysis is an assurance pack |
| Design fidelity | B1 authority; irreversible choices use `design-decision` |
| Customer journeys | B3; critical journeys use `system-journey` |
| Process-global mutation | B1 allowed effects + `production` pack + B5 |
| Guard fidelity | B3 evidence scope must match the claim |
| Track/worktree mechanics | Engine implementation of B5, not agent instructions |
| Capability/model policy | Engine configuration outside Baton; assurance policy binds delivery checks and packs only |

The old rules, prompts, commands, templates, and schemas remain available at the
immutable `v0.16.0` tag. They are historical rationale, not a compatibility
surface for Baton 1.x.

The compression is deliberate:

- roles define authority and outputs, not personalities or handbooks;
- schemas carry facts, not a second workflow engine;
- deterministic mechanics live in Sworn;
- fresh verification remains universal;
- heavy review is selected by risk; and
- failure routes to repair, re-authorization, or re-verification instead of
  accumulating more prompt clauses.
