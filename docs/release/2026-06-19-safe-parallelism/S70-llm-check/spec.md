---
title: 'Slice spec — S70-llm-check'
description: 'Port release-llm-check.sh from bash to Go: `sworn llm-check` — six deterministic LLM-based quality checks with structured prompts and structured JSON output.'
---

# Slice: S70-llm-check

## User outcome

A developer or agent runs `sworn llm-check --type <check> --slice <id> --release <name>` and receives a structured PASS/FAIL verdict from a focused LLM call. Six check types available: ac-satisfaction, spec-ambiguity, design-review, security-review, semantic-coverage, maintainability-review. Each is deterministic (temp=0), fail-closed, with structured JSON output.

## Entry point

New `internal/gate/llmcheck.go`. CLI via `internal/command` registry. Invoked as `sworn llm-check`. Uses the existing model provider infrastructure (T5-providers) to make API calls.

## In scope

- Six check types, each with a structured prompt template:
  - `ac-satisfaction`: does the code actually satisfy each AC?
  - `spec-ambiguity`: are any ACs vague, incomplete, or underspecified?
  - `design-review`: does the design conflict with project memory?
  - `security-review`: does the change introduce vulnerabilities?
  - `semantic-coverage`: do tests genuinely verify their ACs?
  - `maintainability-review`: is the code understandable 12 months from now?
- Read spec.md + diff content to build prompts
- Call model via provider infrastructure (S10-S16)
- Parse structured JSON response
- Output verdict: PASS or FAIL with findings
- Exit 0 on PASS, 1 on FAIL, 2 on configuration error
- Separate from `sworn lint` (costs credits, not default lint)

## Out of scope

- Modifying the model provider infrastructure (uses existing T5 providers)
- Auto-fixing findings (reporting only)

## Planned touchpoints

- `internal/gate/llmcheck.go` (new)
- `internal/gate/llmcheck_test.go` (new)
- `cmd/sworn/llmcheck.go` (new)

## Acceptance checks

- [ ] All six check types produce valid structured prompts
- [ ] `ac-satisfaction` reports which ACs are satisfied/partial/not-satisfied
- [ ] `spec-ambiguity` reports which ACs are ambiguous/incomplete/underscoped
- [ ] `security-review` reports vulns with severity (critical/high/medium/low)
- [ ] `maintainability-review` reports naming, separation, god objects, etc.
- [ ] Model calls use temperature 0 (deterministic)
- [ ] Exits 0 on PASS, 1 on FAIL

## Required tests

- **Unit**: `internal/gate/llmcheck_test.go` — prompt generation + response parsing with mock model responses
- **Reachability artefact**: `sworn llm-check --type spec-ambiguity` output on a fixture spec
- **E2E gate type**: local (can use a cheap/free model for testing)
