---
title: 'S03-assemble-command'
description: '`sworn assemble <release>` is the machine half of the Rule-10 assembly stage: it brings up the assembled releases stack from the release worktree, runs the releases deferred end-'
---

# Slice: `S03-assemble-command`

## User outcome

`sworn assemble <release>` is the machine half of the Rule-10 assembly stage: it brings up the assembled release's stack from the release worktree, runs the release's deferred end-to-end set no-mock, serially, with verified teardown, and emits docs/release/<release>/assembly-proof.json (validated against the vendored assembly-proof-v1). It exits non-zero on any non-excepted failure. /merge-release (baton side, already landed) reads that artefact, flipping the assembly gate from advisory to enforced.

## In scope

- A new internal/assemble package + `sworn assemble <release>` command (cmd/sworn/assemble.go)
- Bring up the assembled release's stack from the release worktree (derive_ports / service startup), run the release's deferred end-to-end set NO-MOCK, serially, with verified teardown (services actually torn down, ports released)
- Emit docs/release/<release>/assembly-proof.json validated against the vendored assembly-proof-v1 schema (baton.ValidateSchema) — per-suite counts, preflight/API observations, screenshot paths, verdict
- Exit non-zero on any non-excepted failure; a declared Rule 2 mock at a validated boundary is the only permitted exception (surfaced in the proof)
- Acceptance against the fired corpus: `assemble` on the pre-S17 tree surfaces the CORS preflight failure (seam 3)

## Out of scope

- The lint contracts grader (T1)
- Inventing the assembly-proof shape — it is the vendored assembly-proof-v1 (grade/emit against it, never fork under the same $id)
- The fired#1168 derive_ports fix itself IF it lives in the fired extension (not sworn) — S03 consumes derive_ports; where the fix lands is verified at implementation start, and the board.json-era-without-index.md path is a declared Rule 2 deferral in the proof if unresolved (Coach-acknowledged 2026-07-11)
- The /merge-release assembly-gate read (baton side, already landed) — S03 only produces the artefact it reads
- Journey elicitation/ratification (Rule 10 human-owned half) — assemble is the machine walk, not the journey artefact

## Acceptance criteria

- [ ] AC-01: When `sworn assemble <release>` runs, it SHALL bring up the release's stack, run the deferred end-to-end set no-mock and serially, tear the stack down with verified teardown (services down, ports released), and emit docs/release/<release>/assembly-proof.json that validates against the vendored assembly-proof-v1 schema.
- [ ] AC-02: When any non-excepted end-to-end check fails during assembly, `sworn assemble` SHALL exit non-zero and record the failing suite/observation in the proof with a failing verdict — proven against the fired pre-S17 tree, where the CORS preflight failure (seam 3) surfaces.
- [ ] AC-03: If a mock is present at a boundary declared validated (no-mock) and it is not a declared Rule 2 deferral, `sworn assemble` SHALL fail the gate closed rather than report a passing walk over the mock.
- [ ] AC-04: `sworn assemble` on a well-formed assembled release SHALL emit a passing assembly-proof.json and exit 0, and `go build ./...` + `go test ./internal/assemble/... ./cmd/sworn/...` SHALL pass; verified teardown SHALL be proven by a reachability test asserting ports are released after the run.
