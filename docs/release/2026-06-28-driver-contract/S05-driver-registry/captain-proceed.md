# Coach acknowledgement — S05-driver-registry

Date: 2026-07-10
Decided by: Brad (Coach) — escalate pin ratified live; mechanical pins
applied per the pre-authorised batch protocol.
Review: review.md (Captain, 2026-07-10)
Verdict: PROCEED — dispositions below

## Pin dispositions

1. **[escalate] AC-05 ViaProxy vs proxy-blind dispatch — RESOLVED: S06
   forward-note R-04, Coach-ratified.** The gap is owned by
   S06-loop-dispatch-rewire: its spec.json now carries R-04 (committed
   e2b5472 on release-wt), binding registry-dispatched in-process
   drivers to resolve their client via the FromEnv-equivalent path so
   the actual dispatch route satisfies the exact condition
   `sworn capabilities` advertises, with a test that enumeration and
   dispatch evaluate ONE shared predicate. S05 does NOT widen to
   inprocess.go and AC-05 is NOT re-cut. The track merges as one unit,
   so the S05-landed/S06-pending intermediate state never reaches the
   integration branch.

2. **Record the four AC-literalism divergences in proof.json —
   ACCEPTED.** internal/driver/registry/ subpackage vs AC-01's literal
   path (forced by TestNoWireImports + the driver<->inprocess import
   cycle, S04 precedent); registry.Default vs DefaultRegistry symbol
   name; touchpoint internal/model/registry_test.go does not exist;
   `sworn capabilities` is created, not re-pointed. Each recorded as a
   proof.json divergence with the forcing constraint named.

3. **D2 prefix breadth + D3 utility-path rename spillover — ACCEPTED.**
   Append both to status.json.design_decisions as Type-2 noted defaults
   at the in_progress transition so the Rule 9 gate sees them.

4. **Prefix-table consistency test — ACCEPTED.** Add the test iterating
   every registered in-process prefix and asserting model.NewClient
   returns the wire family the registered identity claims — the two
   hand-synced tables (registry + NewClient switch) must drift-fail a
   test, not silently contradict the enumeration.

5. Any remaining mechanical pin in review.md not restated above is
   accepted as written in the Captain's suggested acknowledgement.

Proceed to implementation.
