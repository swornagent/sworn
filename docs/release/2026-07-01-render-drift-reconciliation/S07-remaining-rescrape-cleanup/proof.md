---
title: Slice proof bundle — S07-remaining-rescrape-cleanup
description: Rule 6 proof bundle, scoped to one slice. Generated from live repo state, not recollection. Verifier reads this; do not paraphrase.
---

# Proof Bundle: `S07-remaining-rescrape-cleanup`

Rendered from `proof.json` (proof-v1). First implementation pass.

## Scope

Paging notifications report violations from `proof.json.not_delivered` (the
same source of truth as S02/S04's fixes) instead of regex-scraping
`proof.md`; `internal/ears`'s lint-time EARS classification and
`cmd/sworn/ledger.go`'s acceptance-check counts read `spec.json` instead of
independently re-deriving from `spec.md` text.

## Files changed

```
$ git diff --name-only 5b31ba86c166dc55a29fef49d4ce02ff976c4e11
cmd/sworn/ledger.go
cmd/sworn/ledger_test.go
docs/release/2026-07-01-render-drift-reconciliation/S07-remaining-rescrape-cleanup/journal.md
docs/release/2026-07-01-render-drift-reconciliation/S07-remaining-rescrape-cleanup/proof.json
docs/release/2026-07-01-render-drift-reconciliation/S07-remaining-rescrape-cleanup/proof.md
docs/release/2026-07-01-render-drift-reconciliation/S07-remaining-rescrape-cleanup/reachability-lint-ac-output.txt
docs/release/2026-07-01-render-drift-reconciliation/S07-remaining-rescrape-cleanup/status.json
docs/release/2026-07-01-render-drift-reconciliation/index.md
internal/account/notify.go
internal/account/notify_test.go
internal/ears/ears.go
internal/ears/ears_test.go
internal/run/slice.go
```

`start_commit` is `5b31ba86` (current HEAD when the slice moved to
`in_progress`, first-set, never overwritten). All production-code changes
are inside T5's six declared touchpoints plus the one call-site edit in
`internal/run/slice.go`, flagged in design.md as required for AC-01's only
caller to compile/behave correctly, not a hidden scope expansion.

## Test results

### Go

```
$ go build ./...
(no output, exit 0)

$ go test ./internal/account/... ./internal/ears/... ./cmd/sworn/... -count=1 -timeout 300s
ok  	github.com/swornagent/sworn/internal/account	10.133s
ok  	github.com/swornagent/sworn/internal/ears	0.009s
ok  	github.com/swornagent/sworn/cmd/sworn	44.683s

$ go test ./internal/run/... -count=1 -timeout 300s
ok  	github.com/swornagent/sworn/internal/run	5.098s
(includes TestRunSlice_FailNotifiesOnce, the run_test.go call site design.md
flagged as a risk — still passes through the ViolationsSummary(absSliceDir, ...)
call site)

$ go test ./... -count=1 -timeout 600s
ok — all 39 test packages PASS, 0 failures
(only internal/baton/schemas and internal/verdict have no test files)

$ go vet ./internal/account/... ./internal/ears/... ./cmd/sworn/... ./internal/run/...
(no output, exit 0)

$ gofmt -l internal/account/notify.go internal/account/notify_test.go internal/ears/ears.go internal/ears/ears_test.go cmd/sworn/ledger.go cmd/sworn/ledger_test.go internal/run/slice.go
(empty — all formatted)
```

(Full-suite verification is the merge gate's responsibility; this bundle runs
the slice-relevant packages only, per Rule 6. `go test ./...` was additionally
run here per the design-review's memory-cited pin on newline-eating edit
corruption — three prior incidents in this project fused code onto a
preceding `//` comment line, invisible to a narrower test scope. A grep for
`//.*\t+(return|func|[a-z]+\()` across every changed `.go` file found no
corruption — the two hits were legitimate comment prose mentioning function
names.)

## Reachability artefact

- **Type**: live command run captured to file
- **Path**: `docs/release/2026-07-01-render-drift-reconciliation/S07-remaining-rescrape-cleanup/reachability-lint-ac-output.txt`
- **What it proves**: `sworn lint ac 2026-07-01-render-drift-reconciliation`
  exits **0** with **43/43** acceptance checks well-formed EARS across all 7
  slices of this release (a spec.json-only release — only S04 carries
  `spec.md`). Before this slice, the same command exited **2** with
  `ears: read .../S01-render-drift-guard/spec.md: no such file or directory`
  — the exact defect the Captain review's pin 1 named as this slice's
  required reachability artefact. This is AC-02's own literal acceptance
  text turned into a live command run against the real release, not a
  substitute.

## Delivered

- **AC-01** — `ViolationsSummary(sliceDir string, violationCount int) string`
  (`internal/account/notify.go`) reads `sliceDir/proof.json`'s
  `not_delivered` array directly — no `proof.md` read remains in the
  function. Mirrors S04's `internal/mcp/context.go readProofViolations`
  pattern: a missing, unreadable, or unparseable `proof.json`, or an empty
  `not_delivered`, all fall through to the same generic fallback
  (`"%d violation(s) found"` / `"verification failed"`). Evidence:
  `TestViolationsSummary_FromFile` (rewritten to build `proof.json`
  fixtures, plus a case proving a decoy `proof.md` alongside `proof.json` is
  ignored entirely), `TestViolationsSummary_Truncation` (rewritten),
  `TestViolationsSummary_MalformedProofJSONFallsBack` (new). The sole call
  site, `internal/run/slice.go:865` (the `failed_verification` transition),
  is updated from `account.ViolationsSummary(proofPath, ...)` to
  `account.ViolationsSummary(absSliceDir, ...)` — `absSliceDir` was already
  in scope. `proofPath` is untouched everywhere else in `slice.go`
  (`checkProofAbsent`, the first-pass gate's `ProofPath`, the agentic
  verifier's prose payload — none of which are JSON consumers).
- **AC-02** — `internal/ears.Validate` now calls `spec.ReadRecord(sliceDir)`
  per slice and prefers `spec.json` whenever it exists and carries ≥1 AC —
  even when `spec.md` also exists (spec.json wins on disagreement). A new
  `classifySpecJSON`/`patternFromKeyword` pair maps the already-computed
  `ears_keyword` field directly to an EARS `Pattern`, case-insensitively,
  defaulting unrecognized/empty values to `PatternUbiquitous` (mirroring the
  writer's own default). The eager `spec.md` read that made `sworn lint ac`
  exit 2 on this spec.json-only release now sits only in the legacy fallback
  branch, reached only when `spec.json` is absent or has zero ACs. Evidence:
  `internal/ears/ears.go`; `TestValidate_ReadsEARSKeywordFromSpecJSON` (a
  spec.json-only slice covering all 5 non-complex EARS classes + a NOTE:,
  classified purely from the stored keyword — no "THE SYSTEM SHALL" text
  regex involved); the **Reachability artefact** above.
- **AC-03** — `cmd/sworn/ledger.go`'s `countGates` now derives `sliceDir`
  explicitly (`filepath.Join(repoRoot, "docs", "release", release,
  sliceID)`, the design-review pin 3 gap) and returns
  `len(spec.json.acceptance_criteria)` via `spec.ReadRecord`, falling back
  to the existing `- [ ]`-line `spec.md` scan only when `spec.json` is
  absent. Evidence: `cmd/sworn/ledger.go`; `TestSync_GateCountFromSpecJSON`
  (spec.json-only, 4 ACs, driven through the `cmdLedgerSync` integration
  point — the same one `TestSync_GateCountFromSpec` exercises for the
  legacy `spec.md` path, which is re-run unchanged and still passes).
- **AC-04** — JSON is authoritative on disagreement, and no historical
  artefact is touched (only which source future reads prefer). Satisfied by
  construction for both AC-01 and AC-03 (no dual-source consumer exists to
  disagree — `ViolationsSummary`/`countGates` each read exactly one JSON
  file), and additionally directly proven for AC-02 by
  `TestValidate_SpecJSONWinsOverDisagreeingSpecMd`: a fixture slice whose
  `spec.md` text ("Make sure the form is saved.", no SHALL clause) would
  classify `PatternNone` (a violation) under the legacy path, while its
  `spec.json` says `ears_keyword: "When"` — the test asserts zero violations
  and one event-driven classification, proving spec.json wins.
- **AC-05** — `go build ./...` succeeds and
  `go test ./internal/account/... ./internal/ears/... ./cmd/sworn/...`
  passes. Evidence: **Test results** above.

## Not delivered

None. Every acceptance check is delivered.

## Divergence from plan

- **countGates signature change (Captain pin 2).** design.md's literal AC-03
  wording ("non-nil record with len>0, else fall back") would have folded
  `spec.ReadRecord`'s malformed-JSON error into the same path as
  spec.json-absent. The Captain review's pin 2 required handling the error
  explicitly, which meant changing `countGates`'s signature from `int` to
  `(int, error)` and updating its sole caller `cmdLedgerSync` (same file, an
  already-declared touchpoint) to treat a non-nil error as a sync error
  rather than silently zeroing the gate count. `internal/ears.Validate`
  needed no signature change — it already returns `(*Report, error)`.
  Covered by `TestSync_MalformedSpecJSONFailsClosed` and
  `TestValidate_MalformedSpecJSONFailsClosed`.
- **llm-check (Rule 2 deferral).** `sworn llm-check --type ac-satisfaction`
  was not run in the implementer session — no `SWORN_ANTHROPIC_API_KEY`
  credential is available here. Tracking: the fresh-context `/verify-slice`
  pass is the model-backed check for this slice, consistent with the
  sibling S04/S06 slices. Acknowledgement: surfaced in the implementer's
  session-end output.
- **release-verify.sh false-FAIL (not manufactured around).** The
  deterministic first-pass script FAILs "spec.md missing" on this slice — a
  known false negative for spec.json-only (spec-v1) slices on this release
  (`feedback_releaseverify_specmd_false_fail`). No `spec.md` was
  manufactured to satisfy it, per the design-review pin. The canonical gate
  is the model-backed `sworn verify` / fresh `/verify-slice`.

## First-pass script output (informational — see false-FAIL note above)

```
$ $HOME/.claude/bin/release-verify.sh S07-remaining-rescrape-cleanup 2026-07-01-render-drift-reconciliation
release-verify.sh
  slice:       S07-remaining-rescrape-cleanup
  slice dir:   docs/release/2026-07-01-render-drift-reconciliation/S07-remaining-rescrape-cleanup
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  FAIL  spec.md missing            <- known false negative, spec.json-only slice
  FAIL  proof.md missing           <- ran before this proof bundle existed
  PASS  status.json present
  FAIL  journal.md missing         <- ran before journal.md existed

== Status ==
  PASS  status.json is valid JSON
  state: in_progress               <- ran before state -> implemented
  FAIL  state is 'in_progress' — slice not yet ready for verifier; complete implementation first

== Diff vs start_commit (verifier base) ==
  diff base: start_commit 5b31ba86c166dc55a29fef49d4ce02ff976c4e11
  PASS  9 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files
```

Run mid-implementation (before `journal.md`/`proof.json`/`proof.md` existed
and before the final `state: implemented` transition) to confirm the diff
base and dark-code checks were clean early; the "spec.md missing" FAIL is
the documented, accepted false negative for this spec.json-only release —
see Divergence above.
