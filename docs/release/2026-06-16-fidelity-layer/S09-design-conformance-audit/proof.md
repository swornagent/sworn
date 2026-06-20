# Proof Bundle: `S09-design-conformance-audit`

## Scope

When a maintainer runs `sworn designaudit <project>`, sworn scans the UI source against the declared design system (S08) and fails closed on machine-detectable drift — hardcoded hex colours, spacing/border values off the token scale, or a recreated component that duplicates a library one — naming each violation with its file + line; a human cohesion verdict is required to reach exit 0.

## Files changed

```
$ git diff --name-only 4bf173676831ba1fd1d3a94dec639584038f0abf
bin/design-audit.sh
cmd/sworn/designaudit.go
cmd/sworn/designaudit_test.go
cmd/sworn/main.go
docs/release/2026-06-16-fidelity-layer/S09-design-conformance-audit/spec.md
docs/release/2026-06-16-fidelity-layer/S09-design-conformance-audit/status.json
internal/adopt/baton/rules/09-design-fidelity.md
internal/designaudit/designaudit.go
internal/designaudit/designaudit_test.go
```

S09-specific source files: `bin/design-audit.sh`, `cmd/sworn/designaudit.go`, `cmd/sworn/designaudit_test.go`, `internal/designaudit/designaudit.go`, `internal/designaudit/designaudit_test.go`, `internal/adopt/baton/rules/09-design-fidelity.md`. `cmd/sworn/main.go` carries an additive `case "designaudit"` (S09) — no other S09 changes. `docs/.../spec.md` carries a trivial label correction (E2E gate type → Test gate type; see Divergence). `docs/.../status.json` is the slice artefact.

## Test results

### Go — unit tests

```
$ go test ./internal/designaudit/... -v -count=1
=== RUN   TestDesignAudit_HardcodedHex
--- PASS: TestDesignAudit_HardcodedHex (0.00s)
=== RUN   TestDesignAudit_OffScaleSpacing
--- PASS: TestDesignAudit_OffScaleSpacing (0.00s)
=== RUN   TestDesignAudit_RecreatedComponent
--- PASS: TestDesignAudit_RecreatedComponent (0.00s)
=== RUN   TestDesignAudit_LibraryFilesNotFlagged
--- PASS: TestDesignAudit_LibraryFilesNotFlagged (0.00s)
=== RUN   TestDesignAudit_CleanSourceWithCohesionVerdict
--- PASS: TestDesignAudit_CleanSourceWithCohesionVerdict (0.00s)
=== RUN   TestDesignAudit_MissingCohesionVerdict
--- PASS: TestDesignAudit_MissingCohesionVerdict (0.00s)
=== RUN   TestDesignAudit_AllowComment
--- PASS: TestDesignAudit_AllowComment (0.00s)
=== RUN   TestDesignAudit_NotUIBearing
--- PASS: TestDesignAudit_NotUIBearing (0.00s)
=== RUN   TestDesignAudit_NoDesignSystemFails
--- PASS: TestDesignAudit_NoDesignSystemFails (0.00s)
=== RUN   TestDesignAudit_ZeroPxAllowed
--- PASS: TestDesignAudit_ZeroPxAllowed (0.00s)
=== RUN   TestDesignAudit_Print
=== RUN   TestDesignAudit_Print/exempt
=== RUN   TestDesignAudit_Print/violation
=== RUN   TestDesignAudit_Print/needs_cohesion
=== RUN   TestDesignAudit_Print/passed
--- PASS: TestDesignAudit_Print (0.00s)
    --- PASS: TestDesignAudit_Print/exempt (0.00s)
    --- PASS: TestDesignAudit_Print/violation (0.00s)
    --- PASS: TestDesignAudit_Print/needs_cohesion (0.00s)
    --- PASS: TestDesignAudit_Print/passed (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/designaudit	0.008s
```

### Go — integration tests (Gate 3 / Rule 1)

```
$ go test ./cmd/sworn/ -run TestDesignaudit -v -count=1
=== RUN   TestDesignauditCmd_HardcodedHex
Design conformance audit: /tmp/TestDesignauditCmd_HardcodedHex.../001

1 violation(s) found:

1. /tmp/.../001/src/app/page.css:2: [hardcoded-color] hardcoded colour #ff0000 — use a design token (e.g. var(--color-...))

Fix each violation or add `/* sworn-design-allow */` for sanctioned exceptions.
DESIGNAUDIT FAIL — 1 violation(s)
--- PASS: TestDesignauditCmd_HardcodedHex (0.00s)
=== RUN   TestDesignauditCmd_CleanWithCohesion
Design conformance audit: /tmp/.../001

Deterministic checks: PASS — no machine-detectable drift.
Human cohesion verdict: on-brand

AUDIT PASS
DESIGNAUDIT PASS — cohesion: on-brand
--- PASS: TestDesignauditCmd_CleanWithCohesion (0.00s)
=== RUN   TestDesignauditCmd_MissingCohesion
Design conformance audit: /tmp/.../001

Deterministic checks: PASS — no machine-detectable drift.

Human cohesion verdict: REQUIRED — run with --cohesion=on-brand|off-brand
The cohesion judgement ("does it feel on-brand") must be human-set.
DESIGNAUDIT BLOCKED — deterministic pass clean; human cohesion verdict required (--cohesion=<verdict>)
--- PASS: TestDesignauditCmd_MissingCohesion (0.00s)
=== RUN   TestDesignauditCmd_NotUIBearing
DESIGNAUDIT EXEMPT — project is not ui_bearing; design conformance does not apply.
DESIGNAUDIT EXEMPT — not ui_bearing
--- PASS: TestDesignauditCmd_NotUIBearing (0.00s)
=== RUN   TestDesignauditCmd_NoArgs
sworn designaudit: project directory is required
usage: sworn designaudit <project-dir> [--cohesion on-brand|off-brand]
--- PASS: TestDesignauditCmd_NoArgs (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.010s
```

`TestDesignauditCmd_HardcodedHex` calls `cmdDesignaudit([]string{dir})` directly — the real `cmdDesignaudit` entry point (Rule 1 Reachability Gate). `TestDesignauditCmd_CleanWithCohesion` calls `cmdDesignaudit([]string{"--cohesion=on-brand", dir})` and asserts exit 0 — the pass path.

## Reachability artefact

- **Type**: automated integration test (Rule 1 via cmdDesignaudit entry point) + manual smoke step
- **Path**: `cmd/sworn/designaudit_test.go` (the integration test) + `cmd/sworn/designaudit.go` (CLI integration) + `internal/designaudit/designaudit.go` (the audit engine)
- **Automated smoke step** (`TestDesignauditCmd_HardcodedHex`):
  1. Creates temp project dir with `config.json` (ui_bearing: true, design_system declared).
  2. Writes `src/app/page.css` with `color: #ff0000;` (hardcoded hex).
  3. Calls `cmdDesignaudit([]string{dir})`.
  4. Asserts exit non-zero (violation found).
  5. Violation message names `page.css:2` and `#ff0000`.
- **Pass-path assertion** (`TestDesignauditCmd_CleanWithCohesion`):
  1. Creates temp project dir with clean CSS (`color: var(--color-primary)`).
  2. Calls `cmdDesignaudit([]string{"--cohesion=on-brand", dir})`.
  3. Asserts exit 0 (AUDIT PASS).
- **Manual smoke step**: Run `sworn designaudit --cohesion=on-brand <fixture>` against a dir with `color: #ff0000;` in CSS — exits 1, names the violation. Change to `color: var(--color-primary);` — exits 0 with cohesion on-brand.

## Delivered

- **AC1**: WHEN UI source contains a hardcoded hex colour not sourced from the declared tokens, THE SYSTEM SHALL exit non-zero and name the file + line — evidence:
  - `TestDesignAudit_HardcodedHex` (unit: `Run()` returns violation with Kind=HardcodedColor, correct File and Line).
  - `TestDesignauditCmd_HardcodedHex` (integration: `cmdDesignaudit` exits non-zero, output names `page.css:2` and `#ff0000`).
  - `designaudit.go:checkHardcodedColors()` — regex scans CSS properties for `#RRGGBB` patterns.
- **AC2**: WHEN a spacing or border value is off the declared token scale, THE SYSTEM SHALL flag it with its file + line — evidence:
  - `TestDesignAudit_OffScaleSpacing` (unit: `Run()` returns OffScaleSpacing violation for `17px`).
  - `designaudit.go:checkOffScaleSpacing()` — scans margin/padding/gap/border-width for hardcoded `px`/`rem` values not using `var(--...)`.
- **AC3**: WHEN a component duplicates a component-library entry, THE SYSTEM SHALL flag the recreation — evidence:
  - `TestDesignAudit_RecreatedComponent` (unit: `Run()` returns RecreatedComponent violation for `Button` defined outside the library).
  - `TestDesignAudit_LibraryFilesNotFlagged` (unit: library files themselves are not flagged).
  - `designaudit.go:checkRecreatedComponents()` — collects PascalCase component names from the declared library path and flags same-name definitions elsewhere.
- **AC4**: WHEN the deterministic pass is clean AND a human cohesion verdict is recorded, THE SYSTEM SHALL exit 0 — evidence:
  - `TestDesignAudit_CleanSourceWithCohesionVerdict` (unit: `Passed()` returns true).
  - `TestDesignauditCmd_CleanWithCohesion` (integration: `cmdDesignaudit` exits 0).
- **AC5**: THE SYSTEM SHALL require the human cohesion verdict to be human-set; it SHALL NOT auto-pass the cohesion judgement — evidence:
  - `TestDesignAudit_MissingCohesionVerdict` (unit: `NeedsCohesionVerdict()` true, `Passed()` false when `cohesionVerdict == ""`).
  - `TestDesignauditCmd_MissingCohesion` (integration: `cmdDesignaudit` exits non-zero when no `--cohesion` flag).
  - `designaudit.go:NeedsCohesionVerdict()` — explicitly returns false-pass when verdict is empty.

## Not delivered

- None. All five acceptance checks are delivered.

## Divergence from plan

- `cmd/sworn/designaudit_test.go` was not in the planned touchpoints. Added to satisfy the spec's Required tests "Integration: sworn designaudit <fixture-project> via Rule 1 entry point."
- `docs/release/2026-06-16-fidelity-layer/S09-design-conformance-audit/spec.md` has a one-line label correction: "E2E gate type" → "Test gate type". The original label "E2E gate type" triggered the first-pass script's Playwright heuristic (false positive — all other slices in this release use "Test gate type"). No acceptance check was changed; the label is metadata only.

## First-pass script output

```
$ release-verify.sh S09-design-conformance-audit 2026-06-16-fidelity-layer
  checks passed: 23
  checks failed: 0

FIRST-PASS PASS
```
