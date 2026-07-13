---
title: "Session handoff — loop-operability shipped; next = two self-driven dogfood runs (CLI, then MCP)"
date: 2026-07-12
author: Brad (Coach) + Claude (orchestrator)
---

# Session handoff — 2026-07-12

## 1. What landed this session

**Release `2026-07-11-loop-operability` — verified, merged, pushed.**
`origin/release/v0.1.0 @ f7e2c08`. Three slices, each fresh-context Rule-7
verified; both tracks merged; full `go test ./...` green on the integration
branch. Built via subagent workflows (design → Captain review → Coach ratify →
implement → fresh verify).

- **S01-spec-json-read-conformance** — the engine now reads `spec.json` as the
  machine contract (fixes **sworn#97**, the dogfood blocker: `sworn run
  --parallel` can now drive a spec.json release end-to-end). A whole-tree
  completeness grep beat the design's hand-enumerated audit (found
  `llmcheck`/`lint`/`task` sites the 9-site list missed).
- **S02-model-response-structured** — the two prose-scraping gates (design
  §1–§6 header scrape + reqverify `## RESULTS` scrape) now emit
  schema-constrained JSON. Coach Type-1 calls (captain-proceed.md, "go with all
  four"): D1 rename `DispatchInput.VerdictSchema` → `StructuredSchema` (ADR-0012)
  + structured `dispatchCaptain`; D3 new `ErrKindUnsupported` (declared
  capability-absent deferral, distinct from `ErrKindProtocol`); D2 sworn-local
  inline schemas, reqverify emit+validate; one slice (no split). This retires
  the model-fragility class that broke the loop on Grok.
- **S03-xai-driver** — native `xai/` grok-4.5 provider, OpenAI-compatible, no
  SDK (ADR-0007), priced $3/$15. (Exact grok-4.5 pricing → **sworn#99**.)

**Local binary reinstalled → `sworn 0.1.0`** (`make install`). This fixed the
"board not working" complaint — the root cause was a stale installed binary
(v0.6.3) misreading the pure-plan board, not the board data.

**Marketing sites** (details in project memory; public URLs only here):
`sworn.sh` now serves a new Astro + shadcn build with a sword logo, copy widgets
on install blocks, loop-engineering positioning + canon citations, and a locked
mobile viewport. `baton.sawy3r.net` got a shadcn foundation (+ a token-collision
fix for its warm palette). House style sweep: em dashes → en dashes on both.

## 2. Open releases — SPECCED but NOT yet run

| Release | Slices | State | Notes |
|---|---|---|---|
| **2026-07-11-contract-edge-gates** | 3 | all `planned` | Fully specced: intake + specs + track plan + Coach decisions all recorded. 2 parallel tracks (disjoint touchpoints). Target v0.1.0. **Prime next target.** |
| **2026-07-01-loop-cli-ux** | 3 | all `planned` | `sworn use` (active-release store) + bare `sworn loop` (drop the mandatory `--parallel`). Fully specced. |
| **2026-07-01-release-hygiene** | 2 | S01 `design_review`, S02 `planned` | Embedded version + version-bump CI gate. Half-started; small. |

(All other release folders are `merged`/verified or are ephemeral `run-*`
single-slice task runs.)

## 3. Is assembly sliced? YES

**`sworn assemble` = `S03-assemble-command`** in `2026-07-11-contract-edge-gates`
(track `T2-assemble`), currently `planned`. It is the machine half of the Rule 10
assembly stage: bring up the assembled release from the release worktree, run the
deferred end-to-end set **no-mock, serially, with verified teardown**, and emit
`assembly-proof.json` (validated against the vendored `assembly-proof-v1`); exit
non-zero on any non-excepted failure. `/merge-release` already reads that
artefact — so **once S03 ships, the assembly gate flips advisory → enforced**,
closing the exact "advisory-absent" gap this session's merge-release ran into.

Sibling gate in the same release: **`sworn lint contracts`** = `S01-registry` +
`S02-mock-parity` (track `T1-lint-contracts`, both `planned`). Together the two
graders catch the cross-slice wire-seam failure class that per-slice
verification structurally cannot see.

`S03-assemble` has **no `depends_on`**; its one start-of-implementation
verification item (an upstream `derive_ports` fix for board.json-era releases
without `index.md`, tracked as R-01 in the spec) has a declared-deferral
fallback — the common case (index.md present) ships regardless.

## 4. Next steps — two self-driven dogfood runs (Coach directive)

The goal: exercise **both** driving surfaces of sworn, on the two ready releases.

- **Run 1 — CLI orchestration → `contract-edge-gates`.**
  The orchestrator (me) drives `sworn loop --release 2026-07-11-contract-edge-gates
  --parallel` via the CLI, tracks T1 ∥ T2 in parallel. Why this release first:
  (a) the loop can now run a spec.json release (S01/S02 just landed — this is the
  first real proof of that on a fresh release); (b) it delivers `sworn assemble`
  + `sworn lint contracts`, making the assembly gate real; (c) it is the release
  the earlier dogfood was attempted on, so the validation corpus + acceptance
  shapes are already worked out; (d) it is fully planned with Coach decisions
  recorded — ready for `/implement-slice` per track.

- **Run 2 — MCP driving → `loop-cli-ux`.**
  The orchestrator drives sworn through its **MCP server** (`internal/mcp`)
  rather than the CLI, on the next planned release. Dogfoods the "one engine,
  three surfaces (CLI/TUI/MCP)" claim from the other side. `loop-cli-ux` is the
  natural target (3 clean planned slices); `release-hygiene` is a smaller
  alternative.

## 5. Prereqs / caveats for the runs

- **Keys**: `~/.config/coach/env` (OpenRouter/XAI/etc.; `ANTHROPIC` commented out
  — claude-cli uses the CLI subscription). The dogfood ran grok-4.5 via
  OpenRouter successfully; native `xai/` now also available (S03).
- **sworn#98 still OPEN**: the claude-cli subprocess driver parses a leading
  `---` frontmatter as a CLI flag → exits 1 → misclassified as auth. Use
  grok/OpenRouter or an in-process driver for the runs until #98 lands.
- **Remove the HACK `spec.md`** files on `contract-edge-gates` before/as Run 1 —
  they were committed as a sworn#97 workaround so the loop could read a spec;
  now that the engine reads `spec.json` (S01), they are obsolete. spec.json is
  the source of truth.
- **Verify the R-01 dependency** at the start of the S03-assemble implementation
  (see §3); declared-deferral fallback exists if unresolved.

## 6. Open threads (all tracked in project memory)

- `release/v0.1.0` pushed to origin; slices stay `verified` until it deploys to
  prod, then flip to `shipped`.
- sworn#98 (claude-cli), sworn#99 (grok-4.5 exact pricing) open.
- HACK spec.md removal on contract-edge-gates (see §5).
- `sworn assemble` unbuilt — Run 1 builds it.

## 7. Pointers

- Merged release: `docs/release/2026-07-11-loop-operability/` (S01/S02/S03).
- Next release specs: `docs/release/2026-07-11-contract-edge-gates/`
  (intake.md + S01/S02/S03 spec.json + track plan) and
  `docs/release/2026-07-01-loop-cli-ux/`.
- Origin handoffs for contract-edge-gates:
  `docs/captures/2026-07-11-contract-edge-step3-handoff.md`,
  `docs/captures/2026-07-11-replan-driver-contract-contract-edges.md`.
- Project memory: the loop-dogfood note (release complete + follow-ups) and the
  site-rebuild note.
