---
title: 'S46-captain-review — captain design-review stage gates implementation in sworn run'
description: 'After the design TL;DR (S45), a captain agent reviews it against the spec and live code, emits pins classified mechanical / memory-cited / escalate, writes review.md, and gates the implement step: proceed autonomously when there are no escalate pins; halt and surface otherwise (the autonomous analogue of the Coach ack). Restores the coach-loop /design-review role inside the product.'
---

# Slice: `S46-captain-review`

## User outcome

`sworn run` reviews a slice's design **before** implementation: a captain agent reads the
design TL;DR (`design.md` from S45), the spec, and the live code surfaces it references, then
emits **pins** — classified `mechanical` (apply-inline fix), `memory-cited` (violates a recorded
rule/memory), or `escalate` (needs a human decision) — and writes `review.md`. Implementation
proceeds autonomously when there are **no escalate pins**; an escalate pin **halts** the run and
surfaces the review for a human (the autonomous analogue of the Coach ack/decline).

## Entry point

`sworn run` → a captain-review stage in `RunSlice`, after S45's design-TL;DR step and before the
implement loop. Verifiable by: a slice whose design has an escalate-class issue halts with the
review surfaced; a clean design proceeds to implement.

## Background

Restores the coach-loop's `/design-review` (the Captain role) inside the product. sworn already
embeds `captain.md` (`prompt.Captain()`) and mechanizes the Captain's *known* catch classes as
deterministic gates (S29–S33, S35). This slice adds the Captain's **judgment** in the loop — the
piece that catches novel/contextual design issues the deterministic gates can't.

## In scope

- A captain-review step invoked after S45: prompt the captain agent (`prompt.Captain()`) with the
  TL;DR + spec + referenced code, producing pins classified mechanical / memory-cited / escalate,
  written to `<slice>/review.md` (mirroring the coach-loop review.md contract).
- A gate on the result:
  - **no escalate pins** → proceed to implement; mechanical / memory-cited pins are passed to the
    implementer as guidance (compose with S44's prompt-injection mechanism).
  - **≥1 escalate pin** → halt the run for this slice and surface `review.md`; record the slice
    as awaiting a human design decision (a distinct, non-failure state — not `failed_verification`).
- Deterministic-gate composition: run the existing `sworn designfit` / lint checks as part of, or
  ahead of, the captain review so mechanized catches still fire.

## Out of scope

- Generating the TL;DR — that's **S45**.
- An interactive human ack/decline TUI — autonomous halt + surfaced `review.md` is the parity
  target; an interactive `--review` mode is optional future scope.

## Design decisions (for the design-review gate to ratify — yes, recursively)

- **Halt representation**: proposed — a new `awaiting_design_decision` status (or reuse
  `blocked` with a design-review reason) rather than `failed_verification`. Confirm the state model.
- **Escalate-pin autonomy**: proposed — escalate pins always halt (never auto-proceed); mechanical
  pins never halt. Confirm the thresholds.
- **Verifier model vs captain model**: proposed — a dedicated captain model setting
  (`captain.model`, analogous to verifier.model), defaulting to the verifier model. Confirm.

## Planned touchpoints

- `internal/run/slice.go` (invoke review; gate implement on the verdict)
- `internal/captain/review.go` (new — run the captain, parse pins, write review.md)
- `internal/captain/review_test.go` (new)
- `internal/state/state.go` (new `awaiting_design_decision` state, if chosen)
- `internal/prompt/prompt.go` (captain review prompt accessor if a distinct prompt is needed)

## Acceptance checks

- [ ] A fixture slice whose design references a non-existent file (escalate-class) **halts** the
  run with `review.md` written and the slice in the awaiting-design-decision state (not implemented)
- [ ] A clean fixture design produces `review.md` with zero escalate pins and the run **proceeds**
  to the implement step
- [ ] Mechanical / memory-cited pins are surfaced to the implementer (assert they reach the
  implement prompt via the S44 mechanism)
- [ ] `review.md` contains the pin classification (mechanical / memory-cited / escalate) for each pin
- [ ] `go test -race ./internal/captain/... ./internal/run/...` passes

## Required tests

- **Unit**: `internal/captain/review_test.go` — `TestEscalatePinHalts`, `TestCleanDesignProceeds`,
  `TestPinsClassified`, using a fake captain agent returning scripted pin sets.
- **Reachability artefact**: paste in `proof.md` a sample `review.md` from a fixture run showing
  classified pins, and a run log showing the halt-on-escalate vs proceed-on-clean paths.

## Risks

- Without care this becomes a second verifier; keep it **design-only** (judges the TL;DR + spec +
  surfaces, not a diff — there is no diff yet).
- The captain call must be bounded by S42's per-attempt timeout.

## Deferrals allowed?

Yes, with Rule 2 — the interactive ack mode and the precise state-model choice may carry forward
with why/tracking/ack if the first cut lands the autonomous halt + surfaced review.md.
