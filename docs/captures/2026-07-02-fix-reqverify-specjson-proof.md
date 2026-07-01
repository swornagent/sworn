# Proof bundle — fix reqverify-specjson (2026-07-02)

Finding: `reqverify-vacuous-pass-specv1` (critical, adversarially CONFIRMED).
Refs swornagent/sworn#51.

## Scope

`sworn reqverify` (and the DoR gate composing `reqverify.Run`) extracted ACs
from spec.md only, so a spec.json-only (spec-v1, current-format) release found
0 ACs and exited 0 without ever dispatching the model — a vacuous PASS on the
release format Sworn itself now produces. Fix: read `acceptance_criteria` from
spec.json (preferred) via a shared reader, fall back to spec.md for legacy
releases, and fail closed (non-zero, "no evaluable acceptance criteria") when
a release yields zero ACs.

## Files changed

`git diff --name-only 632d4f3` (plus untracked new package):

```
cmd/sworn/reqverify_test.go
internal/reqverify/reqverify.go
internal/reqverify/reqverify_test.go
internal/spec/spec.go        (new — shared spec-v1 read-side reader)
internal/spec/spec_test.go   (new)
```

Design note: the spec.json reader lives in a new `internal/spec` package
(read side only; the writer stays in `internal/implement/spec_record.go`)
rather than as a sixth bespoke scanner inside reqverify — per the finding's
guidance and sworn#22. `internal/gate` (fix 2 in this chain) consumes the same
reader. `internal/implement` cannot host it: it imports reqverify (ready.go),
which would create an import cycle.

## Test results

RED first (before the fix), through the CLI integration point (Rule 1):

```
--- FAIL: TestReqverifyCmdWithVerifier_SpecJSONViolation (0.00s)
    reqverify_test.go:79: expected exit 1 for spec.json AC violation, got 0
--- FAIL: TestReqverifyCmdWithVerifier_NoACsFailsClosed (0.00s)
    reqverify_test.go:97: expected exit 2 for release with no evaluable ACs, got 0
```

GREEN after (`go test ./internal/spec/... ./internal/reqverify/... ./cmd/sworn/... ./internal/implement/... -timeout 120s`):

```
ok  	github.com/swornagent/sworn/internal/spec	0.003s
ok  	github.com/swornagent/sworn/internal/reqverify	0.009s
ok  	github.com/swornagent/sworn/cmd/sworn	37.247s
ok  	github.com/swornagent/sworn/internal/implement	0.418s
```

(`internal/implement` run because its DoR gate composes `reqverify.Run` and
inherits the new zero-AC error contract.) `go vet` clean on all four packages.

## Reachability artefact

Live command from the finding, re-run against the rebuilt binary
(`go build -buildvcs=false -o bin/sworn ./cmd/sworn`):

```
$ SWORN_ANTHROPIC_API_KEY=bogus-key-audit ./bin/sworn reqverify 2026-06-30-sworn-operational-readiness
sworn reqverify: reqverify: model dispatch: HTTP 401 from anthropic
exit=2
```

Before the fix the same command printed "No acceptance criteria to verify."
and exited 0 without dispatching. Now the 33 spec.json ACs are extracted and
the model is actually dispatched (the bogus key surfaces as HTTP 401, exit 2).

## Delivered

- `internal/spec` shared spec-v1 reader: `ReadRecord` returns (nil, nil) on
  missing spec.json (legacy fallback allowed), error on malformed JSON
  (fail closed). Evidence: internal/spec/spec_test.go (3 tests).
- `reqverify.extractACs` prefers spec.json `acceptance_criteria`, falls back
  to spec.md scrape. Evidence: `TestExtractACs_PrefersSpecJSON`,
  `TestExtractACs_MalformedSpecJSONFailsClosed`, `TestRun_SpecJSONDispatchesModel`.
- Zero-AC releases fail closed: `reqverify.Run` returns
  "no evaluable acceptance criteria" (CLI exit 2; DoR gate blocks via the
  returned error). Evidence: `TestRun_NoACsFailsClosed`,
  `TestReqverifyCmdWithVerifier_NoACsFailsClosed`.
- CLI-boundary proof that spec.json ACs reach model grading:
  `TestReqverifyCmdWithVerifier_SpecJSONViolation` (graded FAIL → exit 1).

## Not delivered

- `Print`/`PrintCompact` retain their zero-AC copy ("No acceptance criteria to
  verify.") even though `Run` can no longer return a zero-AC report; the
  branches are now dead via `Run` but harmless. Why: cosmetic, out of the
  finding's scope. Tracking: none needed beyond this note — the fail-closed
  behaviour is owned by `Run`. Acknowledged here in plain text.
- Existing bespoke spec-v1 scanners elsewhere (e.g. `parseCoversNeeds` in
  internal/gate/trace.go) are not migrated onto `internal/spec` in this
  commit. Why: unrelated cleanup would sprawl the fix. Tracking: sworn#22
  (consolidate bespoke scanners); fix 2 of this chain consumes the shared
  reader for the AC leg.

## Divergence from plan

- The finding suggested reusing the trace.go reader; that reader parses only
  `covers_needs` and is unexported in a package reqverify shouldn't depend on
  for spec parsing, so the shared reader was placed in a new minimal
  `internal/spec` package instead (the finding's own "internal/spec package if
  one exists" shape — it now exists).
- `TestRun_NoACsPasses` was renamed/inverted to `TestRun_NoACsFailsClosed`:
  the old test asserted exactly the vacuous-PASS behaviour this fix removes.
