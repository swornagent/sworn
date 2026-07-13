# Proof bundle — fix trace-ac-leg-specjson (2026-07-02)

Finding: `trace-ac-leg-vacuous-specv1` (high, adversarially CONFIRMED).
Refs swornagent/sworn#51.

## Scope

`sworn lint trace` read `covers_needs` from spec.json but every AC-level check
(Check 4 EARS conformance, Check 5a see-intake, Check 5b vague-AC, and Check 3
"every covers_needs ID cited by an AC") silently `continue`d when spec.md was
absent — 2 of 3 RTM legs inert on spec-v1 (spec.json-only) releases, which
vacuously PASSed with "ACs checked: 0". Fix: when spec.md is absent, drive the
AC-level checks from spec.json `acceptance_criteria` (via the shared
`internal/spec` reader landed in the previous commit of this chain).

## Files changed

`git diff --name-only 632d4f3`, restricted to this fix (the remainder belongs
to the previous commit, fix-reqverify-specjson):

```
cmd/sworn/lint_trace_test.go
internal/gate/trace.go
internal/gate/trace_test.go
```

Constraint honoured: `internal/ears/ears.go` and `internal/rtm/rtm.go`
(owned by in-flight T5) are untouched.

## Test results

RED first (before the fix), through the CLI integration point (Rule 1):

```
--- FAIL: TestLintTraceCmd_SpecJSONACsEvaluated (0.00s)
    lint_trace_test.go:165: expected exit 1 for free-form spec.json AC, got 0
--- FAIL: TestRunTrace_SpecJSONACsEvaluated ... expected 2 ACs checked from spec.json, got 0
--- FAIL: TestRunTrace_SpecJSONFreeFormAC ... expected FAIL for free-form spec.json AC, got PASS
--- FAIL: TestRunTrace_SpecJSONUnclaimedCoverage ... expected unclaimed-coverage violation for N-02, got []
--- FAIL: TestRunTrace_SpecJSONSeeIntakeAC ... expected see-intake violation, got []
--- FAIL: TestRunTrace_MalformedSpecJSONFailsClosed ... expected error for malformed spec.json, got nil
```

GREEN after (`go test ./internal/gate/... ./cmd/sworn/... ./internal/tui/... ./internal/mcp/... ./internal/rtm/... -timeout 180s`):

```
ok  	github.com/swornagent/sworn/internal/gate	0.115s
ok  	github.com/swornagent/sworn/cmd/sworn	39.001s
ok  	github.com/swornagent/sworn/internal/tui	0.579s
ok  	github.com/swornagent/sworn/internal/mcp	0.095s
ok  	github.com/swornagent/sworn/internal/rtm	0.007s
```

(tui and mcp run because they also call `gate.RunTrace`; rtm run to prove the
T5-owned package is unaffected.) `go vet ./internal/gate/...` clean.

## Reachability artefact

Live command from the finding, re-run against the rebuilt binary.

Before (baseline, reproduced this session at 632d4f3):

```
$ ./bin/sworn lint trace 2026-06-30-sworn-operational-readiness
needs: 9  slices: 6  ACs checked: 0
EARS:  free-form=0
PASS — all 9 needs traced, 0 ACs conformant
exit=0
```

After:

```
$ ./bin/sworn lint trace 2026-06-30-sworn-operational-readiness
needs: 9  slices: 6  ACs checked: 41
EARS: Ubiquitous=25 Complex=1 When=13 Where=1 If=1 free-form=0
FAIL — 9 violation(s)   [all unclaimed-coverage: covers_needs IDs never cited in AC text]
exit=1
```

AC check count 0 → 41; the AC leg now evaluates and the gate fails closed on a
real gap (the release's spec-v1 AC texts never cite their claimed N-IDs).

Legacy regression check — spec.md-era release identical to the finding's
baseline:

```
$ ./bin/sworn lint trace 2026-06-27-conformance-foundation
needs: 28  slices: 27  ACs checked: 150
EARS: Ubiquitous=8 Complex=2 When=51 free-form=89
exit=1
```

## Delivered

- Check 4 (EARS conformance + classification stats), Check 5a (see-intake) and
  Check 5b (vague-AC) now run over spec.json `acceptance_criteria` texts when
  spec.md is absent. Evidence: `TestRunTrace_SpecJSONACsEvaluated`,
  `TestRunTrace_SpecJSONFreeFormAC`, `TestRunTrace_SpecJSONSeeIntakeAC`.
- Check 3 (unclaimed coverage) evaluates spec.json slices: covers_needs IDs
  must be cited by an AC text. Evidence: `TestRunTrace_SpecJSONUnclaimedCoverage`.
- Malformed spec.json fails closed (RunTrace returns an error → CLI exit 2)
  instead of silently reading as "no spec". Evidence:
  `TestRunTrace_MalformedSpecJSONFailsClosed`.
- CLI-boundary proof: `TestLintTraceCmd_SpecJSONACsEvaluated` (free-form
  spec.json AC → exit 1 through `cmdLintTrace`).

## Not delivered

- Check 5c (vague in-scope items) has no spec.json equivalent: spec-v1 records
  carry no "In scope" section, so the check is inert on spec.json-only slices
  by data absence, not by code skip. Why: nothing to evaluate. Tracking: if a
  future spec-v1 revision adds scope fields, wire them then (rides sworn#22 /
  the spec-v1 schema evolution). Acknowledged here in plain text.
- `parseCoversNeeds`'s bespoke spec.json/status.json scanning is not migrated
  onto `internal/spec`. Why: behaviour-preserving refactor outside this
  finding's scope. Tracking: sworn#22. Acknowledged here in plain text.
- The 9 unclaimed-coverage violations the gate now surfaces on
  2026-06-30-sworn-operational-readiness (merged release) are real spec-v1
  authoring gaps, not fixed here — this fix restores the gate, it does not
  backfill AC citations. Tracking: surfaced in the audit punch list under
  swornagent/sworn#51. Acknowledged here in plain text.

## Divergence from plan

- Check 3 no longer re-reads spec files from disk; it reuses the spec text
  captured in the discovery loop (spec.md body, or joined spec.json AC texts).
  For spec.json slices this is slightly stricter than the spec.md path (which
  searched the whole markdown body, not just ACs) — searching the raw
  spec.json would be vacuous since covers_needs itself contains the IDs, so AC
  texts are the only honest search surface.
- Violation message for unclaimed coverage now says "no AC in its spec cites"
  (was "no AC in spec.md cites") since the spec artefact may be spec.json.
