# Coach acknowledgement — S02-model-response-structured

Date: 2026-07-12
Decided by: Brad (Coach) — "Go with all four." All three escalate pins ratified
plus the D3 taxonomy confirm; mechanical/memory pins carried into implementation.
Verdict: PROCEED

## Pin dispositions
1. **[escalate / Type-1] Pin 1 — D1 driver-contract change: RATIFIED (rename).**
   Rename `DispatchInput.VerdictSchema` → `StructuredSchema` (role-agnostic) and
   give `dispatchCaptain` a structured-output path when a schema is present.
   Chosen over the parallel `CaptainSchema` field: the driver enforces an output
   schema, it does not care which role asked — two fields for one concept invites
   drift, and the field's own doc already anticipated generalisation. This is an
   **ADR-0012** amendment to the driver contract; record it as a Type-1,
   architecturally-significant `design_decision` carrying this Coach call. The
   rename is compile-checked across all call sites (driver.go, claude.go,
   codex.go, verify.go, inprocess*.go, conformance.go, tests).
2. **[escalate] Pin 2/3 — D2 schema home + reqverify validate: CONFIRMED.**
   Sworn-local inline emit schemas (spec-blessed; do NOT fork canonical Baton
   `*-v1.json` under an existing `$id`) — these are sworn-local gate
   transports, not a cross-tool contract. Validate where it gates: **design
   TL;DR = inline emit-only** (an artefact, not a gate); **reqverify DoR-results
   = inline emit + a lightweight sworn-local validate** (a fail-closed gate, so
   validate the structured output before trusting it — matches the
   verifier-verdict-v1 precedent and "exit 0 only on PASS").
3. **[escalate / Type-1-adjacent] Pin 4 — D3 new ErrKind: RATIFIED.**
   Add a dedicated `ErrKindUnsupported = "unsupported"` to the binding
   cross-driver ErrKind taxonomy (subprocess.go), so capability-absent
   ("this model can't emit structured output") is a **declared Rule 2 deferral**
   distinct from `ErrKindProtocol` (a real failure) — this is what makes AC-03's
   deferral declared rather than a silent fold. Confirm the subprocess-family
   drivers can produce/map it too, so capability-absent stays distinguishable on
   every driver path. Cite [[project_driver_contract_recut]] (ErrKind vocab is a
   contract "for all future drivers").
4. **[escalate] Pin 7 — keep one slice: CONFIRMED (no split).**
   D1 is the shared spine both gate migrations depend on; splitting would
   duplicate or serialise the same contract edit. One coherent slice: port the
   two prose scrapers onto the proven verifier-verdict-v1 structured pattern. If
   the file count balloons past the ceiling during implementation, the fallback
   is split-with-D1-first — but start as one.

## Mechanical / memory pins carried into implementation
- **Pin 2 (Rule 9 gate):** populate status.json `design_decisions` before
  in_progress (done in this commit) — D1 (Type-1), D3 (Type-1-adjacent), D2/D4/D5
  at their stake class, plus the no-split confirmation.
- **Pin 5 (cross-track):** T2-xai-driver (S03) is verified but UNMERGED to
  release-wt and touches internal/model + internal/driver/registry; it does NOT
  modify the files S02 renames. Resolves at merge — whoever lands second
  forward-merges the first and re-runs the full suite. Do not trust the board's
  `verified` as `merged`.
- **Pin 6 (newline-eating corruption):** after every `.go` edit run
  `grep -nE '//.*\t+(return|[a-z]+\()'` on the changed files, `gofmt -l`,
  `go vet`, and a full `go test -count=1 -timeout 300s ./...` before any state
  transition — this slice edits shared driver-contract files.
- **Pin 8 (stale touchpoint):** build against `internal/design/tldr_test.go`
  (design.md is right); the spec's `design_test.go` touchpoint is cosmetic.
- **Flag §4 "later audit":** if the interpreter/orchestrator scraper sweep is
  real follow-on work, file/cite an issue — do not leave it as prose (Rule 2).

Proceed to implementation.
