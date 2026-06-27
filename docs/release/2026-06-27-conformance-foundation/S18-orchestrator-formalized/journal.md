---
title: Implementation journal — S18-orchestrator-formalized
description: Session decisions, trade-offs, and state transitions for S18
---

# Journal: `S18-orchestrator-formalized`

## 2026-07-25: Implementation session

### State transitions

- `planned` → `in_progress` (commit `993a433`): start implementation
- `in_progress` → `implemented`: both artefacts created, all ACs verified

### Decisions and trade-offs

1. **No production code changes.** The spec is explicitly documentation-only. The two files (`orchestrator.md`, `orchestrator-model.md`) land under `docs/baton/` alongside the vendored Baton rule docs. No `internal/` or `cmd/` changes.

2. **Role relationship table.** Chose a markdown table in `orchestrator.md` mapping each Baton contract role to the Orchestrator rather than prose-only, because the spec calls for "relationship to the existing Baton roles" and a table makes the distinction instantly scannable.

3. **Captain split articulation.** The spec calls for distinguishing the Orchestrator (Sworn) from the Captain (Baton). Added an explicit "The split" paragraph explaining that captain.md historically described both design-review and release-orchestration, and the formalised ontology absorbs the workflow coordination into the Orchestrator while the Captain retains only design-review.

4. **Design decision record format.** Followed a structure close to a lightweight ADR: Decision ID, StakeClass, Decided by/on, Status, Context, Options, Decision, Rationale, Consequences. This matches the spec's requirement for "options considered" and "rationale for rejecting option (b)." No prior ADR format exists in this repo to conform to.

5. **Five rationale points in orchestrator-model.md.** The spec says the decision record must "contain the rationale for rejecting option (b)." Provided five concrete points: auditable, cheaper, reliable, spec-compliant, and the LLM orchestrator's long-term hosted-layer placement. Each point addresses a distinct concern rather than restating the same argument.

### Dispatches

No subagent dispatches needed — slice is two markdown files with no code changes.

### Out-of-scope discoveries

None. The `docs/baton/` directory already existed (with README.md, rules/, VERSION); the new `roles/` and `decisions/` directories are net-new and don't overwrite existing files (per spec risk mitigation).