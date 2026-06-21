---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S21-canonical-baton`

## 2026-06-21 — re-scoped (replan)

Spec corrected during `/replan-release`. Original spec said "**7 rules**, copied
**verbatim** from `~/.claude/baton/`." Two problems: (1) the canonical set is now **10
rules** (Rules 8 Requirements Fidelity, 9 Design Fidelity, 10 Customer Journey
Validation were added in the fidelity-layer cycle), and (2) `~/.claude/baton/` is the
stale local install — copying it verbatim would drop 8/9/10 *and* risk embedding
internal content. Re-scope: `rules.md` is now built from the **repo's in-repo canonical
rule docs** at `internal/adopt/baton/rules/` (`01`–`10`), which already carry all ten
rules plus the no-mock→Rule-10 reconciliation (release-wt synced from `release/v0.1.0`
commit `5139882` during this replan). The role-prompt generalisation that the verbatim
copy would have leaked is split out to the new final slice `S27-public-readiness-scrub`.

## Open questions

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

*(None yet.)*
