---
title: 'Release Intake: 2026-07-11-loop-operability'
description: 'Make the autonomous sworn loop runnable on modern spec-v1 releases and model-portable — spec.json read conformance (sworn#97), model-response structured conformance, and a native xAI driver.'
---

# Release Intake: `2026-07-11-loop-operability`

## Release goal

The autonomous loop (`sworn run --parallel`) can run a modern (spec-v1) release end-to-end on an arbitrary capable model. The 2026-07-11 dogfood (driving the loop with Grok 4.5) proved the loop MACHINERY is sound — cold-start bootstrap, parallel worktree materialisation, S07 routing/escalation, S14 blocked-terminal, honest cost, the sworn#93 fix all worked live — but the loop could not complete a slice because sworn parses PROSE in model-specific formats instead of the machine contract: the engine's implement leg reads spec.md (not the spec.json the planner writes, sworn#97), and the design-TL;DR + reqverify gates scrape model prose (§1-§6 headers, `## RESULTS`) that a new model (Grok) doesn't reproduce. This release makes sworn read the machine contract (spec.json) and consume schema-constrained structured model outputs everywhere the loop depends on, and adds a native `xai/` driver so `xai/grok-4.5` is first-class. "Shipped" = `sworn run --parallel` drives a spec-v1 release to a real verify verdict on Grok via the native xai driver.

## Source of truth

- **Human stakeholder**: Brad (Coach)
- **Origin**: the 2026-07-11 `sworn run --parallel` dogfood on 2026-07-11-contract-edge-gates (memory: project-sworn-loop-dogfood-2026-07-11)
- **Issues**: sworn#97 (engine reads spec.md, not spec.json; + the widened prose-parsing sweep in the issue comment), sworn#98 (claude-cli arg-passing bug + KindAuth misclassification)
- **Established pattern to follow**: `internal/ears/ears.go` + `internal/reqverify/reqverify.go` are already spec.json-preferred with spec.md legacy fallback (ADR-0009); the sweep applies that pattern to the sites that never migrated.

## Needs

- **N-01 (spec.json read conformance — sworn#97 core)**: every site that reads a slice's machine contract reads `spec.json` (spec-v1) as authoritative, with spec.md only as a legacy fallback for pre-spec-v1 slices — matching the ears.go/reqverify.go pattern. Sweep sites (from the audit): the engine implement leg (`internal/run/run.go`, `internal/implement/implement.go`, `spec_record.go`, `proof_record.go`), `internal/scheduler/worker.go`, `internal/gate/coverage.go`, `internal/specquality`, `internal/rtm`, and align `internal/gate/trace.go`'s spec.md-first order. This unblocks `sworn run` on spec-v1 releases (the loop's implement leg currently hard-fails on missing spec.md).
- **N-02 (model-response structured conformance)**: the gates that parse a MODEL response as prose in a model-specific format consume schema-constrained structured output instead (the ADR-0011 keystone, extended to the sites it missed): the design-TL;DR gate (§1-§6 header scrape) and the reqverify DoR gate (`## RESULTS` scrape). A capable model that doesn't reproduce the exact prose format must not fail the gate — the schema is the contract, not the prose shape. Model-portability for the loop.
- **N-03 (native xAI driver)**: a first-class `xai/` driver prefix (xAI's API is OpenAI-compatible) so `xai/grok-4.5` resolves natively through the driver registry, not only via `openrouter/x-ai/grok-4.5`. Registers in the driver registry (S05 predecessor) + provider config (XAI_API_KEY + base URL), declaring implementer/verifier/captain roles like the other in-process prefixes.

## Constraints and non-negotiables

- Public-safe repo; single Go binary; minimal justified deps (stdlib preferred; the model client is net/http + encoding/json).
- Fail closed. spec.json is the source of truth; spec.md is legacy fallback only (never authoritative on disagreement — the ears.go rule).
- Baton owns schemas; sworn grades/consumes them. Structured outputs use the vendored schemas (verifier-verdict-v1 pattern), never a new fork under an existing $id.
- Do NOT weaken per-slice fresh-context verification, tiering, or roles (gates/reads/driver only).

## Decisions made during planning

**2026-07-11 (Brad, Coach):**
- Loop-operability release ratified as the home for the sworn#97 fix + native xAI driver (from the dogfood). Build with the rigorous subagent workflow, since the loop cannot build its own spec-reader fix (chicken-and-egg).
- Decomposition: the "conformance sweep" splits into N-01 (file-read: spec.json) and N-02 (model-response: structured) — different layers, different risk. N-03 xai driver is disjoint.

## Track plan + touchpoint matrix (Phase 3b)

| Area | T1-conformance (S01, S02) | T2-xai-driver (S03) |
|---|---|---|
| `internal/implement/`, `internal/run/` (spec.json read) | ✅ S01 | — |
| `internal/scheduler/`, `internal/gate/`, `internal/specquality/`, `internal/rtm/` (spec.json read) | ✅ S01 | — |
| `internal/design/`, `internal/reqverify/` (model-response structured) | ✅ S02 | — |
| `internal/driver/registry/`, `internal/model/` (xai provider config) | — | ✅ S03 |

T1 (conformance, internal/implement+gates+design+reqverify) and T2 (xai, internal/driver+model) are disjoint → parallel. Within T1, S01 (file-read) → S02 (model-response) serial: both touch internal/implement + the DoR path, and S02 builds on S01's spec.json reads.

## Screenshots / references

_(none)_
