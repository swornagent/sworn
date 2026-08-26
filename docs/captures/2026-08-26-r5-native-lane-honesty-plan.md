# R5 native-lane honesty + learning foundations - release plan (2026-08-26)

Slate ratified by Brad 2026-08-26 (in-session, post-R4 close sweep).
Successor to the R5 line in `2026-08-24-r4-contract-identity-plan.md`
("deliberately out" ladder) reshaped by the contract-identity run's
findings (`2026-08-26-contract-identity-run-report.md`) and the
in-engine learning spec (`2026-08-25-in-engine-learning-spec.md`,
phases 1-2). Contracts authored against post-R4 main (9e81cfac).

This is the first release authored on R4's own surface: `sworn plan
pin` / `lint` / `record` with digest-addressed contracts. plantool and
sync_plan.py are retired; R4's acceptance test in anger is authoring
R5 with them gone.

Root cause the release owns: the native lane works only when a human
operator carries uncommitted engine patches, and when it fails it lies
about why. Every claude-lane run since R3 has required a hand-patched
binary (pin bump, proxy tolerance, stream cap - the 3-patch archive in
the contract-identity ops home), and the r4-r10 diagnostic spiral cost
six burned tries plus three run generations because sanitization
scrubbed the real failure site, preflight had nothing to say, and an
economy condition wore a surface-integrity code. The release lands the
patches as product and makes the failure surfaces tell the truth -
which is also the training-signal prerequisite (learning-spec phase 2)
for the phase 3-4 graph.

## Core slices (ratified scope)

- **S1 CLI-pin admission policy** (#220; `internal/driver` native.go):
  ADR-0013 shape - receipts stay exact (digest of the binary that ran,
  always), admission becomes policy: pin mode `exact` (today, default)
  or `minor` (accept a self-reported version satisfying the pinned
  major.minor range, hash what runs into receipts, carry certification
  evidence as a soft prior across the range). Lands the 2.1.241 pin
  bump (main still compiles the server-side-dead 2.1.234 at
  native.go:13-14; operator patch #220 is the seed). Includes the
  dispatch-time pin-liveness probe from learning-spec phase 1 (a
  certified pin can be dead server-side - that is this class's
  trigger), distinct from cert-time certification.
- **S2 capture-proxy tool-less tolerance** (#237 tolerance half;
  `internal/driver` native_capture_linux.go:244-277): a tool-less
  request cannot reach the workspace, so it is admitted without the
  model/tool exact-match checks; every tool-bearing request still pins
  both exactly. Operator patch #6 is the seed, r19-validated and
  live through the whole contract-identity endgame. The observability
  half of #237 (name the failed check, capture a bounded request head)
  rides S5 - one taxonomy seam, not two spellings of it.
- **S3 output-stream economy** (#241; `internal/driver`
  native_linux.go scanNativeEvents + `internal/runtime` economy
  guards): the 1MB cumulative stdout cap
  (total > MaxProviderResponseBytes, provider.go:24) is an economy
  condition wearing NATIVE_SURFACE_INVALID. It becomes
  manifest-governed budget (the R3-S4 `limits.*` omitempty pattern,
  contract.go:171-181 precedent) failing with
  ECONOMY_OUTPUT_BUDGET_EXCEEDED (already in the valid set). The
  per-line 1MB cap stays surface-integrity and keeps its code.
  Operator patch #7 (16x) is the interim seed; the r10 instrumentation
  (trip at total=1048664 on a 963-byte assistant event) is the
  evidence. Retroactive suspect for R3's opus ~20min deaths.
- **S4 preflight probes** (#243; learning-spec phase 1): a preflight
  registry seam at dispatch admission - cheap, side-effect-free,
  bounded-timeout, journaled probes that refuse with named codes
  before a dispatch burns. In scope from phase 1: (a) CLI-native
  liveness/identity probe (the r4-r6 class: cert said
  native_preflight_not_required while six tries burned;
  CREDENTIAL_STALE), (b) provider balance/quota probe
  (PROVIDER_EXHAUSTED_HARD; the deepseek -$0.02 class), (c) honest
  classification of instant native exits - duration 0 must be
  auth-vs-transport-vs-surface distinguishable on a durable surface
  (depends on S5's taxonomy). Phase 1's pin-liveness probe rides S1
  (same seam); the toolchain probe (#238) and window-admission split
  (#236) stay on the ladder - not in the ratified slate, and #238
  wants the closure fix decided first.
- **S5 refusal taxonomy + captured evidence** (learning-spec phase 2;
  `internal/driver`, the backlogged provider-error-taxonomy item):
  every driver-boundary refusal carries a typed Kind distinguishing
  cause, and retry/park policy consumes the Kind (hard exhaustion
  never retries into a window; a dead pin never paces). Each refusal
  persists the named check that failed plus a bounded, secret-free
  request head (R2-S6 precedent; #237 observability half). The
  provider's own message ("Insufficient Balance", "input token count
  exceeds ...") reaches the journal at least once per dispatch instead
  of being cleared. This is the training signal for phases 3-4;
  without it the graph learns from mush.
- **S6 sanitization keeps bounded diagnostics** (#242;
  `internal/driver` invoke.go sanitizeFailedObservation :383-421):
  sanitizing a non-admitted diagnostic code stops discarding the
  evidence - it preserves a bounded secret-free diagnostic (raise site
  or stable code, the r10 instrumentation shape) instead of flattening
  to adapter_failed/duration 0/stderr 0. The r4-r10 spiral (two wrong
  theories pursued on the scrubbed record) is the evidence. Pulled
  forward from the R6 worker-observability line. Depends on S5's Kind
  vocabulary so preserved diagnostics and typed refusals are one
  story.
- **S7 park lane-scoping + lineage freshness** (#239 as RULED, comment
  5423645369; `internal/runtime` projection/admission): (1) a park
  pins the affected work immediately (cause/code/paths stamped at
  crossing time), other lanes keep dispatching, and the run
  transitions to parked only when no admissible non-parked work
  remains - park is evidence about one work item, not a run verdict;
  (2) park-guard facts derive from current truth - a
  consecutive-failure streak breaks on any later success in the same
  slice-stage lineage, not just the same work id. Evidence: the R3 CI
  flake it was filed on, plus the production instance (contract-identity
  r1 parked run-wide on S1's stale t1/t2 facts after the slice had
  recovered to verified PASS). Board grows a parked-work row (the
  fields R4-S9 delivered) beside running lanes. Exactly-once machinery
  untouched.

## Riders pending verification

- **#227** degradation-budget calibration for continuation-less
  adapters: verify the remainder against post-R4 main first - asks
  1+2 were delivered by R2-S5, and R3-S4/S6 may have mooted more. If
  the miscount stands (fresh_rehydrate.fallback counted as degradation
  on adapters that never carry continuation state), a small
  calibration slice: count against declared continuation capability,
  or scope the budget per-dispatch. S5's "surface the provider
  message" already covers its observability ask.

## Deliberately out (stays on the ladder)

#219 continuation-economics slots directly behind this release (Brad,
2026-08-26). Learning-spec phases 3-4 (the graph + soft-prior routing)
wait for S5's signal and the literature-informed design. #238
toolchain closure, #236 runtime input-growth guard, #202 navigation,
#208/#151 test-economics, #235 operator tails, R6 worker-observability
remainder (#195/#176).

## Sequencing and touchpoint notes (for contract authoring)

S1/S2/S3 all edit `internal/driver` native files and S4/S5/S6 share
the driver invoke/preflight seam - overlapping touchpoints force
serialization (the learning-spec DAG principle); S7 is disjoint
(`internal/runtime` projection) and can ride a parallel track if the
authoring surface derives that. S5 before S4's classification half and
S6 (both consume the Kind vocabulary). S1-S3 are the "stop
hand-patching the binary" set: until they land, any main-built ops
binary needs patches #220+#6+#7 (archive:
`~/.local/share/sworn/sworn/2026-08-25-contract-identity/ops/operator-engine-patches.diff`).

## Hygiene beside the release (operator tasks, no slices)

1. Fired dogfood is sequenced AFTER R5 (Brad ratified) - fired never
   sees the hand-patched binary class. Checklist:
   `project_fired_dogfood_readiness` memory /
   `2026-08-22` capture.
2. GA version ritual still pending (swornVersion 1.0.0-rc.2-dev).
3. Post-R5 close-with-evidence candidates: #220 #237 #241 #242 #243
   #239 (+#227 if the rider rides).
