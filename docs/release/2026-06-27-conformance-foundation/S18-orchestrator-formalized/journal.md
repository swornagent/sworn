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
## Verifier verdicts received

### 2026-06-27T23:43:10Z — PASS (fresh context)

**Verifier session**: fresh, artefact-only. No prior implementer context loaded.

**Gates walked**:
1. **Gate 1 — User-reachable outcome exists**: PASS. Both `docs/baton/roles/orchestrator.md` (107 lines) and `docs/baton/decisions/orchestrator-model.md` (89 lines) exist and are non-empty. Documentation slice; user reachability is `cat` or file-open.
2. **Gate 2 — Planned touchpoints match actual changed files**: PASS. Both planned files are in the diff. Slice artefacts (journal, proof, status) + drift-gate index.md merge are expected non-scope noise.
3. **Gate 3 — Required tests exist and exercise the integration point**: PASS. Both `cat` commands succeed; files are non-empty with substantive content.
4. **Gate 3b — LLM AC satisfaction check**: SKIPPED (LLM provider not configured; non-blocking).
5. **Gate 4 — Reachability artefact proves the user path**: PASS. Manual-smoke-step: both files exist, non-empty, contain the Orchestrator role name and Type-1 decision record.
6. **Gate 5 — No silent deferrals or placeholder logic**: PASS. The single grep hit ("later") is a future-consequence statement ("If the hosted-layer agentic orchestrator is built later..."), not a code deferral.
7. **Gate 6 — Design conformance**: PASS. No `docs/baton/design-fidelity.json` → non-UI project; gate passes automatically.
8. **Gate 7 — Claimed scope matches implemented scope**: PASS. All 5 ACs verified against live file content:
   - AC1: orchestrator.md exists, "Orchestrator" named (27 occurrences), 6 responsibilities, relationship table mapping all 5 Baton roles.
   - AC2: orchestrator-model.md exists, `**StakeClass:** Type-1` on line 9.
   - AC3: "deterministic Go engine" as decided option, "Brad Sawyer" as decision-maker, "2026-06-27" as date.
   - AC4: 5-point rationale rejecting LLM-driven orchestrator option.
   - AC5: Both files are valid Markdown with YAML frontmatter.

**Verdict**: PASS. Slice verified against commit `b6b40e7b`. Next step: `/implement-slice S19-captain-split 2026-06-27-conformance-foundation`.
