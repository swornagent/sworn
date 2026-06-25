---
title: 'Slice spec — S65-lint-trace'
description: 'Port release-trace.sh from bash to Go: `sworn lint trace` — mechanically verifies RTM chain (intake→covers_needs→AC→test), EARS conformance, sniff-test.'
---

# Slice: S65-lint-trace

## User outcome

A developer runs `sworn lint trace <release>` and receives a structured report of every traceability gap: orphaned intake needs, missing covers_needs, unclaimed coverage, free-form ACs lacking EARS conformance, and vague-scope specs without concrete artefacts. Exits 0 on fully-traced release, non-zero with violations. The release name is a **positional** argument, consistent with every sibling `sworn lint` subcommand (`ac`, `deps`, `touchpoints`, `symbols`, `status`) — there is no `--release` flag.

## Entry point

New `internal/gate/trace.go` package. CLI registration via `internal/command` registry (S51 pattern). Invoked as `sworn lint trace <release>` — the release name is the sole positional argument (`fs.Arg(0)`), matching the arg-parsing convention of the other `sworn lint` subcommands in `cmd/sworn/lint.go`. No `--release` flag.

## In scope

- Parse `intake.md` "What the human wants" section: extract N-NN need IDs
- Parse every slice's `status.json` for `covers_needs` array
- Parse every slice's `spec.md` for checkbox ACs and EARS pattern matching
- Checks: orphaned needs, invalid covers_needs refs, unclaimed coverage, free-form ACs (no `shall` keyword), "see intake" references, vague-scope ACs (no concrete terms)
- Output: structured JSON (machine-readable) + human-friendly text
- Exit 0 on PASS, 1 on FAIL with enumerated violations

## Out of scope

- Modifying any spec or status files (read-only)
- Running during agent loop (tool only at this stage; agent-loop integration is S02b/S59)

## Planned touchpoints

- `internal/gate/trace.go` (new)
- `internal/gate/trace_test.go` (new)
- `cmd/sworn/lint.go` (extend — add "trace" subcommand)

## Acceptance checks

- [ ] `sworn lint trace <release>` (release name as positional arg, no `--release` flag) exits 0 on fully-traced release
- [ ] Detects orphaned needs and reports which N-NN is missing from covers_needs
- [ ] Detects unclaimed coverage (covers_needs ID not cited in AC)
- [ ] Detects free-form ACs lacking `shall` EARS keyword
- [ ] Detects "see intake.md" references in specs
- [ ] Output matches the canonical baton release-trace.sh behaviour

## Required tests

- **Unit**: `internal/gate/trace_test.go` — fixture-based tests with known-pass and known-fail release fixtures
- **Reachability artefact**: `sworn lint trace <fixture-release>` output showing PASS and FAIL scenarios
- **E2E gate type**: local
