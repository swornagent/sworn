# Proof bundle: S23-version-centralise-doctor

## Scope

Centralise VERSION to a single canonical source; add `sworn doctor` checks for
SHA-vs-HEAD pin drift (`baton/pin-currency`) and pre-records-as-JSON prompt
detection (`baton/prompt-currency`). Both checks fail closed.

## Files changed

```
cmd/sworn/baton_test.go
cmd/sworn/doctor.go
cmd/sworn/doctor_test.go
docs/release/2026-06-27-conformance-foundation/S23-version-centralise-doctor/status.json
internal/baton/fetch.go
internal/baton/fetch_test.go
internal/baton/version.go
internal/baton/version_test.go
internal/prompt/VERSION.txt
internal/prompt/baton/VERSION.txt
internal/prompt/prompt.go
internal/prompt/prompt_test.go
```

## Test results

```
=== RUN   TestDoctorPin
=== RUN   TestDoctorPin/pin-currency_pre-layout_FAIL
=== RUN   TestDoctorPin/pin-currency_post-layout_PASS
=== RUN   TestDoctorPin/prompt-currency_stale_FAIL
=== RUN   TestDoctorPin/prompt-currency_clean_PASS
--- PASS: TestDoctorPin (0.00s)
    --- PASS: TestDoctorPin/pin-currency_pre-layout_FAIL (0.00s)
    --- PASS: TestDoctorPin/pin-currency_post-layout_PASS (0.00s)
    --- PASS: TestDoctorPin/prompt-currency_stale_FAIL (0.00s)
    --- PASS: TestDoctorPin/prompt-currency_clean_PASS (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.015s
```

Full test suite:
```
ok  	github.com/swornagent/sworn/internal/baton	(cached)
ok  	github.com/swornagent/sworn/internal/prompt	(cached)
ok  	github.com/swornagent/sworn/cmd/sworn	9.563s
```

## Reachability artefact

`sworn doctor` output (Group 1 excerpt):

```
[OK]    baton/pin-currency
        vendored pin is from a post-baton/ layout commit
[OK]    baton/prompt-currency
        no pre-JSON markers found in embedded prompts
```

## Delivered

- [x] **AC1**: `sworn doctor` reports PIN-STALE with pre-baton/ layout pin
  - Evidence: `TestDoctorPin/pin-currency_pre-layout_FAIL` — injects pre-layout FS, asserts ERROR + "PIN-STALE" detail
- [x] **AC2**: `sworn doctor` reports PROMPT-STALE with stale markers
  - Evidence: `TestDoctorPin/prompt-currency_stale_FAIL` — injects prompt with v0.4.2 marker, asserts ERROR + "PROMPT-STALE" detail
- [x] **AC3**: `sworn doctor` reports PASS with post-S22/S20 state
  - Evidence: `TestDoctorPin/pin-currency_post-layout_PASS` + `prompt-currency_clean_PASS`; live `sworn doctor` run shows both checks OK
- [x] **AC4**: `BatonVersion()` reads from VERSION file
  - Evidence: `internal/prompt/prompt.go:79` calls `baton.Version()` which reads from `adopt.BatonDocsFS().ReadFile("baton/VERSION")` — already centralised prior to this slice
- [x] **AC5**: Zero hardcoded version strings
  - Evidence: `grep -rn '"v0.4.2"\|"v1.0.0"' internal/baton/ internal/prompt/ cmd/sworn/` returns zero results
- [x] **VERSION centralisation**: Removed dead `internal/prompt/VERSION.txt` and `internal/prompt/baton/VERSION.txt`; removed VERSION.txt from go:embed directive
- [x] **baton/pin-currency check**: New function `checkPinCurrency()` in doctor.go; reads `baton/rules/01-reachability-gate.md` from adopt embed; FAIL if absent
- [x] **baton/prompt-currency check**: New function `checkPromptCurrency()` in doctor.go; scans embedded prompts for pre-JSON markers; FAIL if found

## Not delivered

None. All acceptance checks satisfied.

## Divergence from plan

- The `fix/centralise-baton-version` branch (commit `4d17e35`) referenced in the spec for cherry-pick did not exist on origin. The VERSION centralisation was already largely complete in the current codebase (`baton.Version()` already reads from the adopt embed). Implemented the remaining cleanup directly.
- `internal/prompt/baton/VERSION.txt` deletion required removing it from the `go:embed baton/*` pattern's matched files (the baton/ directory still contains other files so the embed directive remains valid).

## First-pass script output

```
release-verify.sh: slice S23-version-centralise-doctor
  PASS  slice folder exists
  PASS  spec.md present
  PASS  status.json present
  PASS  spec.md has Required tests section
  PASS  status.json is valid JSON
  PASS  worktree branch is current
  PASS  12 file(s) changed vs diff base
  PASS  no dark-code markers
  PASS  spec.md frontmatter is strict-YAML safe
```

Note: `release-verify.sh` has an unbound `PLAYWRIGHT_OPTIN` variable issue unrelated to this slice.