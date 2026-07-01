# Implementation journal — S07-remaining-rescrape-cleanup

## 2026-07-02 (UTC) — implementation (first pass)

- **State transition**: `design_review` → `in_progress` → `implemented`.
  `start_commit` set to `5b31ba86` (current HEAD at transition, first-set,
  never overwritten).
- **Pins addressed** (from `.captain-trial-log.md`'s design review — 5 pins
  surfaced, 3 mechanical + 2 memory-cited, 0 escalations):
  - Pin 1 (mechanical, CRITICAL) — `sworn lint ac <release>` exits 2 today
    because `ears.Validate` eager-reads `spec.md` at the top of the loop and
    errors when it is absent, and this release is spec.json-only (only S04
    carries spec.md). Fixed by relocating that read into the spec.md
    fallback branch: `Validate` now tries `spec.ReadRecord(sliceDir)` first
    and only opens `spec.md` when `spec.json` is absent or empty. Confirmed
    live: `sworn lint ac 2026-07-01-render-drift-reconciliation` now exits 0
    (43/43 ACs well-formed EARS across all 7 slices) — see
    `reachability-lint-ac-output.txt`.
  - Pin 2 (mechanical) — `spec.ReadRecord`'s two distinct returns, (nil, nil)
    "absent" vs (nil, err) "malformed", must not be conflated. Both AC-02
    (`ears.go`) and AC-03 (`ledger.go`) now check the error return
    explicitly and propagate it (fail closed) rather than falling through to
    the spec.md path. `ears.Validate` returns the wrapped error immediately.
    `countGates` now returns `(int, error)`; `cmdLedgerSync` treats a
    non-nil error as a sync error (increments `errors`, does not append a
    record with a silently-wrong gate count) instead of the old
    always-`int` signature that had no way to fail closed. Covered by
    `TestValidate_MalformedSpecJSONFailsClosed` and
    `TestSync_MalformedSpecJSONFailsClosed`.
  - Pin 3 (mechanical) — `countGates(repoRoot, sliceID, release)` has no
    `sliceDir` in scope; derived it explicitly as
    `filepath.Join(repoRoot, "docs", "release", release, sliceID)` before
    calling `spec.ReadRecord`.
  - Pin 4 (memory-cited, newline-eating edit corruption) — all three edits
    sit directly under doc comments above func signatures (`ViolationsSummary`,
    `Validate`/`classifySpecJSON`, `countGates`). Ran the full
    `go test ./...` suite (not just the three AC-05 packages) and grepped
    changed files for `//.*\t+(return|func|[a-z]+\()` before trusting green
    — no corruption found, both grep hits were legitimate comment prose
    mentioning function names, not fused code.
  - Pin 5 (memory-cited, release-verify.sh spec.md false-FAIL) — did not
    manufacture a spec.md for S07; the deterministic first-pass's
    "spec.md missing" FAIL on this spec.json-only slice is a known false
    negative (feedback_releaseverify_specmd_false_fail). The canonical gate
    is the model-backed `sworn verify` / fresh `/verify-slice`.
- **Implementation**:
  - `internal/account/notify.go` — `ViolationsSummary(sliceDir string,
    violationCount int) string` reads `sliceDir/proof.json.not_delivered`
    (first entry, trimmed, 200-char truncated), no `proof.md` read remains.
    Mirrors S04's `internal/mcp/context.go readProofViolations` pattern: a
    missing, unreadable, or unparseable `proof.json`, or an empty
    `not_delivered`, all fall through to the same generic fallback message
    (`"%d violation(s) found"` / `"verification failed"`).
  - `internal/run/slice.go` — the sole call site (line ~865, the
    `failed_verification` transition) updated from
    `account.ViolationsSummary(proofPath, ...)` to
    `account.ViolationsSummary(absSliceDir, ...)`; `absSliceDir` was already
    in scope (defined at line 265). `proofPath` (the `.md` path) is
    untouched everywhere else in `slice.go` — it still backs
    `checkProofAbsent`, the first-pass gate's `ProofPath`, and the agentic
    verifier's prose payload.
  - `internal/ears/ears.go` — new `patternFromKeyword(keyword string)
    Pattern` (case-insensitive map, defaults to `PatternUbiquitous`,
    accepts `"complex"` for forward-compatibility) and
    `classifySpecJSON(sliceID string, rec *spec.Record) []Result` (Line =
    1-based ordinal index, not a markdown line number — `Result`'s doc
    comment updated to say so). `Validate` now calls
    `spec.ReadRecord(sliceDir)` per slice; prefers spec.json whenever it
    exists and has ≥1 AC (spec.json wins even when spec.md also exists —
    AC-04), falls back to the legacy `spec.md` text-classification path
    only when spec.json is absent or empty, and propagates a
    `spec.ReadRecord` error immediately (fail closed) rather than reaching
    the spec.md branch.
  - `cmd/sworn/ledger.go` — `countGates` now returns `(int, error)`; tries
    `spec.ReadRecord(sliceDir)` first (`sliceDir` newly derived), returns
    `len(rec.AcceptanceCriteria)` when non-empty, falls back to the
    existing `- [ ]`-line scan of `spec.md` only when spec.json is absent,
    and returns the wrapped `ReadRecord` error on a malformed spec.json.
    `cmdLedgerSync` treats a `countGates` error as a sync error.
  - Tests: `notify_test.go` — `TestViolationsSummary_FromFile` and
    `TestViolationsSummary_Truncation` rewritten to build `proof.json`
    fixtures via a new `writeProofJSON` helper (not `proof.md` prose); the
    rewritten `_FromFile` test also asserts a decoy `proof.md` sitting
    alongside `proof.json` is ignored entirely (AC-01: "instead of", not "in
    addition to"); new `TestViolationsSummary_MalformedProofJSONFallsBack`.
    `ears_test.go` — new `TestValidate_ReadsEARSKeywordFromSpecJSON` (a
    spec.json-only slice, all 5 EARS classes + a NOTE:, classified purely
    from the stored keyword), `TestValidate_SpecJSONWinsOverDisagreeingSpecMd`
    (AC-04 — spec.md text that would classify PatternNone loses to spec.json
    saying "When"), `TestValidate_MalformedSpecJSONFailsClosed`.
    `ledger_test.go` — new `TestSync_GateCountFromSpecJSON` (spec.json-only,
    4 ACs, drives through `cmdLedgerSync`, the same integration point
    `TestSync_GateCountFromSpec` exercises for the legacy path) and
    `TestSync_MalformedSpecJSONFailsClosed`.
- **TDD note (Rule 1)**: all three ACs' first tests were proven red against
  the live pre-change code (`TestViolationsSummary_FromFile` failed on the
  new `proof.json`-fixture assertions; `TestValidate_ReadsEARSKeywordFromSpecJSON`
  failed with "no such file: spec.md" — the exact `sworn lint ac` exit-2
  symptom pin 1 named; `TestSync_GateCountFromSpecJSON` failed with
  `GateCount: want 4, got 0`) before implementation, then passed after.
  `TestSync_GateCountFromSpec` (the pre-existing spec.md-only fixture) was
  re-run unchanged and still passes — the legacy fallback path is untouched.
- **Test results**: `go build ./...` exit 0;
  `go test ./internal/account/... ./internal/ears/... ./cmd/sworn/...`
  — all pass; `go test ./internal/run/...` (the flagged call-site package)
  also pass, including `TestRunSlice_FailNotifiesOnce` (asserts
  `ViolationsSummary != ""` on the FAIL path — the fixture carries no
  `proof.json`, so it exercises the generic fallback, as anticipated in
  design.md's risk note); full `go test ./...` — all 39 test packages pass,
  0 failures (2 packages have no test files: `internal/baton/schemas`,
  `internal/verdict`); `go vet` clean on all touched packages; `gofmt -l`
  empty on all touched files.
- **Reachability artefact**: `sworn lint ac 2026-07-01-render-drift-reconciliation`
  exits 0 (was exit 2 before this slice) — captured to
  `reachability-lint-ac-output.txt`. This is AC-02's own literal acceptance
  text turned into a live command run against the real release, not a
  substitute.
- **Out-of-scope discoveries**: none beyond what design.md already recorded
  (DC-2's accepted `PatternComplex` precision loss, owned by a follow-up
  `spec_record.go` slice if it ever becomes unacceptable).
- **Rule 2 deferral (llm-check)**: `sworn llm-check --type ac-satisfaction`
  cannot run in this session — no `SWORN_ANTHROPIC_API_KEY` credential
  available (why). Tracking: the fresh-context `/verify-slice` dispatch is
  the model-backed check for this slice, consistent with the sibling S04/S06
  slices. Acknowledgement: surfaced in the implementer's session-end output.
- **Next step**: `/verify-slice S07-remaining-rescrape-cleanup
  2026-07-01-render-drift-reconciliation` in a fresh session.
