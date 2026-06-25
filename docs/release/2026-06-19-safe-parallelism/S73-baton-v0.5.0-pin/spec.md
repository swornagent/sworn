---
title: 'Slice spec — S73-baton-v0.5.0-pin'
description: 'Update the vendored Baton protocol from cf15842 (pre-v0.5.0) to v0.5.0 (b8452dd) — role prompts, rules, gate scripts, schemas.'
---

# Slice: `S73-baton-v0.5.0-pin`

## User outcome

The Sworn binary vendors Baton v0.5.0 — the latest protocol version with the full mechanical + LLM verification stack, 16-hat planner, architecture rules engine, RTM traceability, and updated role prompts. Running `sworn version` shows `baton-protocol v0.5.0`. All downstream gate-engine work (S65-S72) builds on the correct protocol version.

## Entry point

Vendored Baton protocol at `internal/adopt/baton/`. Updated via `sworn baton vendor --upstream --tag v0.5.0`.

## In scope

- Update `internal/adopt/baton/VERSION`: commit SHA `cf158423f...` → `b8452dd19...`, vendored date → `2026-06-25`, rules-added note updated for v0.5.0
- Re-vendor role prompts from baton v0.5.0 (`claude/baton/role-prompts/{planner,implementer,verifier,captain}.md`):
  - Planner: 16-hat consultant table, six-layer discovery, proactive expertise, canonical architecture, fresh-context handoff
  - Implementer: spec-completeness sniff test, "don't fill gaps from intake" rule, LLM self-checks (ac-satisfaction, security-review, maintainability-review) before `implemented`
  - Verifier: Gate 3b (ac-satisfaction LLM), Gate 4b (semantic-coverage LLM), Gate 6 (design conformance), Gate 7 (architecture), verifier scope clarification
  - Captain: spec-completeness gate before six-step review, design-review LLM check after PROCEED, fresh-context boundary
- Re-vendor rules from baton v0.5.0:
  - `requirements-fidelity.md`: updated with covers_needs, structural completeness sniff-test, release-trace.sh reference
  - `design-fidelity.md`: updated with canonical architecture concept, release-audit-design.sh reference, architecture rules
  - `architecture.json`: agnostic template with 8 universal rules + canonical_docs declaration
- Verify `sworn baton vendor --upstream --tag v0.5.0` resolves and verifies the correct commit SHA
- Verify `sworn version` shows `baton-protocol v0.5.0`
- Verify `sworn baton diff` (S50) reports zero divergence after re-vendor
- Run existing tests that assert on baton version/content (`TestBatonVersion_NonEmpty`, etc.)

## Out of scope

- Implementing the new gate scripts in Go (S65-S72 — depends on this slice)
- Adding Sworn-specific architecture rules (separate slice — S65+)
- Updating MCP tools for new baton capabilities (S71)

## Planned touchpoints

- `internal/adopt/baton/VERSION`
- `internal/adopt/baton/rules/*.md`
- `internal/adopt/baton/role-prompts/*.md`
- `internal/prompt/VERSION.txt` (if prompt version needs bumping)

## Acceptance checks

- [ ] `internal/adopt/baton/VERSION` references commit `b8452dd` with `vendored: 2026-06-25`
- [ ] `sworn baton vendor --upstream --tag v0.5.0` succeeds and resolves correct SHA
- [ ] `sworn baton diff` exits 0 (no divergence from upstream)
- [ ] `sworn version` shows `baton-protocol v0.5.0`
- [ ] All 4 role prompts match baton v0.5.0 upstream
- [ ] All vendored rules match baton v0.5.0 upstream
- [ ] Existing tests pass with no regression

## Required tests

- **Unit**: `internal/prompt/prompt_test.go` — `TestBatonVersion_NonEmpty` passes, `TestBatonVersion` assert updated
- **Unit**: `internal/adopt/` — verify vendored files match upstream
- **Unit**: `internal/baton/` — vendor, diff, transform, fetch tests pass
- **Reachability artefact**: `sworn version` output showing `baton-protocol v0.5.0`
