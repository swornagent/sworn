---
title: 'S45-design-tldr — sworn run generates a design TL;DR before implementation'
description: 'Before the implement loop, sworn run has the implementer produce a design TL;DR (§1–6: user-visible change, decisions-not-in-spec, files, not-doing, reachability, open questions) written to the slice''s design.md — mirroring the coach-loop''s pre-code TL;DR and giving the captain review (S46) something to gate on.'
---

# Slice: `S45-design-tldr`

## User outcome

When `sworn run` processes a slice, it produces a **design TL;DR** (`design.md` in the slice
dir) *before* writing any code — the same six-section TL;DR the coach-loop implementer emits:
§1 user-visible change, §2 design decisions not in the spec (max 5), §3 files I'll touch by
purpose, §4 things I'm NOT doing, §5 reachability plan, §6 open questions. This makes the design
inspectable and reviewable (by S46's captain stage) instead of jumping straight from spec to
code.

## Entry point

`sworn run` → `internal/run` (a new design-TL;DR stage in `RunSlice`, ahead of the implement
loop). Verifiable by: running a slice and finding a populated `design.md` written before any
implementation commit.

## Background

The coach-loop's sequence is spec → **design TL;DR** → captain review → ack → implement. sworn
today goes spec → implement directly: `implement.Run` reads `spec.md` and launches the agent
loop, with no design artefact. Re-introducing the TL;DR is the first half of restoring the
captain/design-review parity (S46 reviews it). This slice produces the artefact; S46 gates on it.

## In scope

- A design-TL;DR generation step invoked by `RunSlice` before the implement loop: prompt the
  implementer model to emit the six sections from `spec.md` + the live repo context, and write
  the result to `<slice>/design.md`.
- A design-TL;DR prompt (`internal/prompt/`), mirroring the §1–6 contract the coach-loop uses.
- Idempotency: if `design.md` already exists for the slice (e.g. authored by the planner or a
  prior run), do not overwrite it unless `--regenerate-design` is set; note it in the run log.

## Out of scope

- The captain *review* of the TL;DR and the pin gate — that is **S46** (this slice only
  produces the artefact).
- Blocking implementation on the TL;DR's content — S45 generates; S46 gates.

## Design decisions (for the captain review to ratify)

- **Dedicated call vs first turn of implement**: proposed — a dedicated, tool-less model call
  (cheaper, deterministic artefact) rather than folding it into the agent tool loop. Confirm.
- **Model**: proposed — the same implementer model resolved for the slice. Confirm.

## Planned touchpoints

- `internal/run/slice.go` (invoke the design-TL;DR step before implement)
- `internal/design/tldr.go` (new — generate + write design.md)
- `internal/design/tldr_test.go` (new)
- `internal/prompt/design-tldr.md` (new — the §1–6 prompt) + `internal/prompt/prompt.go` (embed/accessor)

## Acceptance checks

- [ ] Running a slice with no `design.md` writes a `design.md` containing all six sections
  (§1–§6 headers present) before any implementation commit
- [ ] An existing `design.md` is not overwritten unless `--regenerate-design` is passed
- [ ] The TL;DR step uses the resolved implementer model and is a tool-less call (no file writes
  by the model itself — the step writes `design.md`)
- [ ] `go test -race ./internal/design/... ./internal/run/...` passes

## Required tests

- **Unit**: `internal/design/tldr_test.go` — `TestGenerateWritesSixSections` (fake agent returns
  a TL;DR; assert design.md written with all headers), `TestGenerateRespectsExisting`
  (pre-existing design.md untouched without the flag).
- **Reachability artefact**: paste in `proof.md` a sample generated `design.md` from a fixture
  slice run, showing the six sections, captured before the implement step.

## Risks

- The TL;DR model call must be bounded by the same per-attempt timeout (S42) — a hung TL;DR call
  should not wedge the run. Confirm the deadline wraps this step too.

## Deferrals allowed?

Yes, with Rule 2 compliance — `--regenerate-design` handling may defer to a follow-up if it
complicates the first cut, with why/tracking/ack.
