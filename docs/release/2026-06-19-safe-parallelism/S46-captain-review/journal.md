# S46-captain-review — Journal

## 2026-07-21 — Implementation session

### Design decisions

- **Halt state**: Reused the existing `DesignReview` state rather than adding a new `AwaitingDesignDecision` state. `DesignReview` is already the semaphore for "design review in progress / awaiting human decision" and the state machine already permits `DesignReview → InProgress` and `DesignReview → Deferred`. Adding a separate halt state would duplicate semantics without adding value. The spec's acceptance checks use "awaiting-design-decision state" generically; `DesignReview` satisfies the intent.

- **Captain model**: Used the first escalation model (same as TL;DR generation) rather than a separate `captain.model` config. A dedicated captain model setting (`captain.model`) is deferred — tracking in the spec's open design decisions. The TL;DR already uses this model, so reusing it for the captain review is consistent.

- **Error handling**: On model error or timeout, the captain review is skipped and implementation proceeds. The captain review is advisory — a model outage should not block the pipeline. This aligns with the TL;DR generation's error handling pattern.

- **Pin parsing**: Structured pin parsing (`parsePins`) uses a lightweight line-scan approach looking for `[escalate]`, `[mechanical]`, `[memory-cited]` tags. The full captain output (including pin details) is preserved verbatim in review.md. The structured result is used for the gate decision (hasEscalatePins) and feedback injection (FormatPinsAsFeedback).

### Trade-offs

- Pin parsing is deliberately simple — it looks for tag patterns rather than parsing the full captain output format. This trades parse precision for robustness: a captain model that produces slightly different formatting still generates a usable review.md. The downside is that pin details (observations, actions) are only in review.md, not in the structured result. This is acceptable because the structured result is only used for the gate decision and feedback injection — both of which only need the tag and summary.

## Verifier verdicts received

### 2026-07-21 — Verifier verdict

**PASS** — All six verification gates satisfied.

Gate 1 (User-reachable outcome): ✓ — Captain review wired into RunSlice, called after design TL;DR, gates implement loop on escalate pins.
Gate 2 (Planned touchpoints match actual): ✓ — review.go, review_test.go, slice.go; state.go/prompt.go untouched by design (reused existing).
Gate 3 (Required tests exist): ✓ — 5 unit tests all pass with go test -race.
Gate 4 (Reachability artefact): ✓ — Integration in RunSlice exercised by TestRunSlice* suite.
Gate 5 (No silent deferrals): ✓ — No TODO/FIXME/HACK in S46 code.
Gate 6 (Claimed scope matches implemented): ✓ — All 8 delivered items verified. Not-delivered items (captain.model, interactive mode) are spec-acknowledged.

### Files changed
- `internal/captain/review.go` — new: Captain Review function, pin parsing, review.md generation
- `internal/captain/review_test.go` — new: tests for escalate halt, clean proceed, pin classification
- `internal/run/slice.go` — modified: captain review step inserted between TL;DR and implement loop