---
title: 'Slice spec — S73-baton-v0.5.0-pin'
description: 'Update the vendored Baton protocol from v0.4.2 (commit 729f188) to v0.5.0 (commit 9ae08fb) — role prompts incl. captain.md, rules incl. rule-11 + architecture.json, transform script substitutions; extends the vendor file-map so the new files are actually covered by sworn baton diff.'
---

# Slice: `S73-baton-v0.5.0-pin`

## User outcome

The Sworn binary vendors Baton v0.5.0 — the latest protocol version with the full mechanical + LLM verification stack, 16-hat planner, architecture rules engine, RTM traceability, and updated role prompts. Running `sworn version` shows `baton-protocol v0.5.0`. All downstream gate-engine work (S65-S72) builds on the correct protocol version.

## Entry point

Vendored Baton protocol at `internal/adopt/baton/`. Updated via `sworn baton vendor --upstream --tag v0.5.0`.

## In scope

- Update `internal/adopt/baton/VERSION`: `baton-protocol: v0.5.0`; `upstream-sha:` from the prior pin `729f188f...` (v0.4.2) to the **resolved commit SHA `9ae08fb...`** for tag `v0.5.0`; `vendored: 2026-06-25`; refresh `upstream-digest:`; `rules-added` note updated for v0.5.0 (adds rule-11 + role-prompt operational gates).
  - **SHA semantics (D1):** pin the **commit** the tag resolves to (`git rev-list -n1 v0.5.0` = `9ae08fb...`), NOT the annotated **tag-object** hash (`git rev-parse v0.5.0` = `b8452dd...`). The vendor's `resolveCommitSHA` (`internal/baton/fetch.go`) and the live GitHub commits API both return the commit `9ae08fb`; `FetchUpstream` verifies the resolved SHA against `VERSION` and aborts on mismatch, so a tag-object hash in `upstream-sha` would break the S62 `--upstream` governance gate.
- **Extend the vendor file-map (D2 — mechanism gap).** The new v0.5.0 files (`captain.md`, `architecture.json`, rule-11) are not in `internal/baton/source.go` `batonFileMappings`, so before this slice `sworn baton diff` / `vendor` inspect only the previously-mapped files and report a **false zero divergence** even when the embed is missing them. In scope:
  - `internal/baton/source.go`: add mappings `claude/baton/process-global-mutation.md → internal/adopt/baton/rules/11-process-global-mutation.md`, `claude/baton/role-prompts/captain.md → internal/prompt/captain.md`, `claude/baton/architecture.json → internal/adopt/baton/architecture.json`; add `process-global-mutation.md` to `RuleSources()`.
  - `internal/adopt/adopt.go`: extend the `//go:embed` directive to include `baton/architecture.json`.
  - `internal/baton/transform.go`: add the v0.5.0 upstream-script → sworn-native substitutions (`release-trace.sh`, `release-audit-design.sh`, `release-coverage.sh`, `release-llm-check.sh`, `release-mock-check.sh`, `release-regression.sh`, `install.sh`, `server-start.sh`/`server-stop.sh`, `install-codex.sh`).
  - `internal/baton/fetch.go`: tarball prefix `v`-stripping fix for the GitHub codeload (`<repo>-<version-without-v>/`) convention.
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
- `internal/adopt/baton/rules/*.md` (incl. new `11-process-global-mutation.md`)
- `internal/adopt/baton/architecture.json` (new)
- `internal/prompt/{planner,implementer,verifier,captain}.md` (re-vendored)
- `internal/baton/source.go` (extend `batonFileMappings` + `RuleSources()`)
- `internal/baton/transform.go` (v0.5.0 script substitutions)
- `internal/baton/fetch.go` (codeload tarball prefix fix)
- `internal/adopt/adopt.go` (embed `baton/architecture.json`)
- `cmd/sworn/baton_test.go`, `internal/baton/fetch_test.go`, `internal/baton/vendor_test.go`, `internal/prompt/prompt_test.go` (v0.5.0 content assertions)

## Acceptance checks

- [ ] `internal/adopt/baton/VERSION` has `baton-protocol: v0.5.0` and `upstream-sha:` = the **resolved commit** `9ae08fb...` (NOT the tag-object `b8452dd...`), with `vendored: 2026-06-25`
- [ ] `sworn baton vendor --upstream --tag v0.5.0` succeeds and the SHA it resolves matches `VERSION` (no `FetchUpstream` mismatch abort)
- [ ] `internal/baton/source.go` `batonFileMappings` maps `captain.md`, `architecture.json`, and rule-11 (`process-global-mutation.md`); `RuleSources()` includes `process-global-mutation.md`
- [ ] `sworn baton diff` exits 0 with the **extended** file-map — i.e. it now inspects `architecture.json`, `captain.md`, and rule-11 and still reports zero divergence (not a false green from an unmapped file)
- [ ] `sworn version` shows `baton-protocol v0.5.0`
- [ ] All 4 role prompts (`planner`, `implementer`, `verifier`, `captain`) match baton v0.5.0 upstream
- [ ] All vendored rules (08–11) and `architecture.json` match baton v0.5.0 upstream
- [ ] Existing tests pass with no regression

## Required tests

- **Unit**: `internal/prompt/prompt_test.go` — `TestBatonVersion_NonEmpty` passes, `TestBatonVersion` assert updated
- **Unit**: `internal/adopt/` — verify vendored files match upstream
- **Reachability artefact**: `sworn version` output showing `baton-protocol v0.5.0`
- **E2E gate type**: local
