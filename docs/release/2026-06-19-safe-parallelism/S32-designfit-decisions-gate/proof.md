---
title: 'Proof bundle: S32-designfit-decisions-gate'
description: 'Design-fit gate fails closed when a slice implies Type-1 work but design_decisions is empty'
---

# Proof bundle: S32-designfit-decisions-gate

## Scope

Extend `internal/designfit.Run()` so a slice whose `planned_files` touch
architecturally-significant packages (`cmd/sworn/`, `internal/state/`,
`internal/verdict/`) but whose `design_decisions` is empty/absent records a
violation — the gate fails closed instead of silently passing. Benign empty
cases (no Type-1-implied work) still pass.

## Files changed

```
internal/designfit/designfit.go
internal/designfit/designfit_test.go
docs/release/2026-06-19-safe-parallelism/S32-designfit-decisions-gate/status.json
```

## Test results

```
$ go test ./internal/designfit/... -v -count=1
=== RUN   TestDesignfit_Type1WithoutDecision
--- PASS: TestDesignfit_Type1WithoutDecision (0.00s)
=== RUN   TestDesignfit_Type2WithNotedDefault
--- PASS: TestDesignfit_Type2WithNotedDefault (0.00s)
=== RUN   TestDesignfit_Type1WithHumanDecision
--- PASS: TestDesignfit_Type1WithHumanDecision (0.00s)
=== RUN   TestDesignfit_Type2WithoutDecision
--- PASS: TestDesignfit_Type2WithoutDecision (0.00s)
=== RUN   TestDesignfit_ArchitecturallySignificantMustBeType1
--- PASS: TestDesignfit_ArchitecturallySignificantMustBeType1 (0.00s)
=== RUN   TestDesignfit_ArchitecturallySignificantType1Passes
--- PASS: TestDesignfit_ArchitecturallySignificantType1Passes (0.00s)
=== RUN   TestDesignfit_MultipleSlices
--- PASS: TestDesignfit_MultipleSlices (0.00s)
=== RUN   TestDesignfit_Print_RoundTrip
--- PASS: TestDesignfit_Print_RoundTrip (0.00s)
=== RUN   TestDesignfit_EmptyRelease
--- PASS: TestDesignfit_EmptyRelease (0.00s)
=== RUN   TestType1ImpliedEmptyDecisionsFails
--- PASS: TestType1ImpliedEmptyDecisionsFails (0.00s)
=== RUN   TestNoType1EmptyDecisionsPasses
--- PASS: TestNoType1EmptyDecisionsPasses (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/designfit	0.007s
```

```
$ go vet ./internal/designfit/...
(clean)
```

```
$ go build ./...
(clean)
```

## Reachability artefact

`TestType1ImpliedEmptyDecisionsFails` exercises `designfit.Run()` with a fixture
slice whose `planned_files` touch `cmd/sworn/` and whose `design_decisions` is
empty — the exact S23-memory-config bypass shape. The test asserts violations
are recorded (gate fails closed). The CLI entry point (`cmd/sworn/designfit.go`)
maps `HasViolations()` → exit 1 — that wiring is unchanged.

## Delivered

- [x] `designfit.Run()` records a violation when a slice implies Type-1 work (planned_files touch `cmd/sworn/`, `internal/state/`, or `internal/verdict/`) but `design_decisions` is empty/absent — proves `TestType1ImpliedEmptyDecisionsFails`
- [x] Benign empty `design_decisions` (no Type-1-implied work) still passes — proves `TestNoType1EmptyDecisionsPasses`
- [x] Existing two checks (arch-significant-but-Type-2; Type-1-without-human-decision) unchanged — proved by all 9 pre-existing tests passing
- [x] `go build ./...` and `go vet ./internal/designfit/...` pass
- [x] Coach pins 1–2 addressed inline: (1) prefix set rationale in `impliesType1Work()` doc comment; (2) D1 rationale gap ("When design_decisions is empty, DesignDecision.ArchitecturallySignificant cannot be checked") in function comment

## Not delivered

None.

## Divergence from plan

None. The design specified path-prefix matching against architecturally-significant packages — implemented as `impliesType1Work()` checking `{cmd/sworn/, internal/state/, internal/verdict/}`. D1 recorded as a Type-2 design_decision in S32's own status.json (Coach flag a).
