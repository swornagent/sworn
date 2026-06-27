---
title: S03-run-loop-verify-reachability
description: Prove the verify gate works through the user-facing command — sworn run's verify step lands a parseable verdict end-to-end, not just the leaf verify package.
---

# Slice: `S03-run-loop-verify-reachability`

## User outcome

A developer runs `sworn run` (the headline turnkey journey) and the loop's
**verify gate returns a parseable verdict** — reaching `verified` → gated-merge on
a passing change — instead of stalling on `BLOCKED / unparseable_verdict` from
format variance. This is the Rule-1 reachability slice: the fix is proven through
the integration point that owns the user affordance (`sworn run`), not only the
leaf `verify` package.

## Entry point

CLI: `sworn run` (`cmd/sworn/run.go` → `internal/run/run.go`); the verify gate is
`internal/run/run.go:232` (`verify.Run(ctx, verify.Input{…})`).

## In scope

- An integration test that drives `internal/run`'s loop (or the closest harness
  that exercises `run.go`'s verify step) with a **fake/stubbed** `model.Verifier`
  whose canned reply uses a non-byte-perfect-but-valid shape (e.g. markdown-
  emphasised `PASS`), and asserts the run loop:
  - feeds the **stateless** prompt (from S01) to the verifier, and
  - resolves a `PASS` verdict and transitions the slice `implemented → verified`
    (the existing transition at `run.go:247-254`).
- A companion case asserting a tool-call-leak reply still BLOCKS the loop
  (fail-closed end-to-end), so the run gate inherits S02's safety.

## Out of scope

- The prompt (`S01`) and parser (`S02`) themselves — this slice proves them
  *through* `sworn run`; it does not re-implement them.
- Live network / real-provider calls (covered by the manual reachability step
  below, not by committed tests).
- Any change to the run loop's *implement* step (`prompt.Implementer()` + tool
  loop) — it is not implicated by this defect.

## Planned touchpoints

- `internal/run/run_test.go` (new integration cases)
- (If `run.Run` is not already testable with an injected verifier: a minimal,
  non-behaviour-changing seam to inject a fake `model.Verifier` — surfaced as a
  Rule 2 note in the proof if added.)

## Acceptance checks

- [ ] An integration test drives `internal/run` with a fake verifier returning a
      markdown-emphasised `PASS`; the loop resolves `PASS` and the slice status
      transitions `implemented → verified`.
- [ ] The fake verifier receives the **stateless** system prompt (asserts the
      "SPEC+DIFF only / verdict-leading" marker is present, the agentic
      worktree/tool instructions are absent) — proving S01 is wired on the run
      path too.
- [ ] An integration test with a fake verifier returning a `<tool_call …>`-leading
      reply leaves the loop **not merged** (verdict BLOCKED, no transition to
      `verified`) — fail-closed end-to-end.
- [ ] Manual reachability (recorded in proof, not a committed test): on a
      known-good synthetic spec+diff, `sworn verify` returns a parseable
      `PASS`/`FAIL`/`BLOCKED`/`INCONCLUSIVE` (no `unparseable_verdict` from format
      variance) across ≥3 providers (deepseek, groq, gemini). Exit codes:
      PASS→0, FAIL→1, BLOCKED→2, INCONCLUSIVE→3.

## Required tests

- **Unit / Integration**: `internal/run/run_test.go` — the two integration cases
  above, driven through `run.Run` with an injected fake `model.Verifier`. Per
  Rule 1 this is the integration point that owns the `sworn run` affordance.
- **Public-safe fixtures (mandatory)**: synthetic spec+diff only; no private
  dogfood artefacts committed.
- **Reachability artefact**: `go test ./internal/run/...` green, PLUS the manual
  multi-provider smoke step recorded in `proof.md` with the per-provider verdict +
  exit code observed (synthetic inputs, keys via env — no keys or payloads in the
  committed proof).
- **E2E gate type**: N/A (no Playwright); the multi-provider step is a manual
  smoke recorded in the proof bundle.

## Risks

- **`run.Run` not unit-testable without a seam.** If an injection seam is needed,
  keep it behaviour-preserving and surface it as a Rule 2 note in the proof; do
  not refactor the loop under cover of a test slice.
- **Manual multi-provider step skipped and silently claimed.** Mitigated: the
  proof bundle must record each provider's observed verdict + exit code; absence
  is a Rule 2 deferral, not an implied pass.

## Deferrals allowed?

No (the live multi-provider step is a recorded manual artefact, not a deferral).
