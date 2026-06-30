# Journal — S01-d6-record-reconciliation

## 2026-07-01 — Session 1 (Implementer): design TL;DR, state planned → design_review

**State transition:** `planned` → `design_review` (Rule 9 design gate — design review happens
before any code is written).

### Done this session
- Materialised the release worktree (`release-wt/2026-06-30-sworn-operational-readiness`) and the
  T1 track worktree (first `/implement-slice` of the release) and recorded both on `board.json`;
  track `T1-operational-unblock` → `in_progress`.
- Read the spec; confirmed Definition of Ready (9 EARS-typed ACs, all naming concrete artefacts) —
  no spec gaps to surface.
- Grounded the design in the live code: current carriers (`Status.OpenDeferrals []string`,
  `Verification.Violations []string`, `Status.NeedIDs` tagged `need_ids`), the schema object shapes
  (`open_deferrals` required `[why, tracking, acknowledgement]`, `additionalProperties:true`;
  `verification.violations` gate/description/evidence; result enum lacks `inconclusive`), all
  consumers, the Rule-10 `CheckBoundaryMocks`/`isDeclared` reader, and the two `need_ids` writers.
- Wrote `design.md` (approach, type design, AC→file traceability, 4 design decisions, 4 risk pins).
- Recorded `design_decisions` in `status.json`: D1 (carrier representation) classified **Type-1 /
  architecturally significant** with `human_decision` empty — this correctly holds the `sworn
  designfit` gate closed until the Captain ratifies at `/design-review`. D2/D3/D4 = Type-2.

### Key design findings
- Schema **already** names `covers_needs` (not `need_ids`) — so the Go `need_ids` tag is the lagging
  side, and planner-written `covers_needs` is currently silently dropped on read (this *is* N-03).
  AC-06's rename is Go-only; no schema change for AC-06.
- Round-trip-fidelity trap: schema **requires** `acknowledgement`, but fired's real deferrals carry
  `acknowledged_by` and no `acknowledgement`. `state.Write` validates against the schema, so a naive
  Read→Write of such a deferral fails closed on *validation*. Resolution surfaced as Risk #1 for the
  Captain: the AC-02 round-trip fixture must carry schema-required fields **plus** the extras.

### Out-of-scope discovery (Rule 2) — RESOLVED this session by the human
- `sworn board --release 2026-06-30-... --json` returned `tracks: null`. Two stacked causes:
  (1) the planner's `board.json` had `release` as an **object** the typed `BoardRecord.Release
  string` reader couldn't parse; (2) the **installed `sworn` binary was stale** — it predated the
  board.json read path, so it never read board.json and silently fell back to the empty `index.md`
  frontmatter (why even the object form gave exit 0 / null rather than a parse error).
  The human reconciled `board.json` to the conformant `board-v1` string form (+`schema_version`) on
  `release-wt` (commit `3cfd54c`); I reinstalled the binary (`go install ./cmd/sworn`); `sworn board`
  now returns both tracks with correct states. Same class as S01 (Go carrier lagging the record) but
  one layer up and **out of S01's scope** (`oracle.go` is a touchpoint here only for
  `parseStatusJSON`). No S01 work item — context only.

### Forward-merge before re-review (track hygiene)
- The track was cut from `release-wt` at `ed7a707`, before the human's board.json oracle fix
  (`3cfd54c`) and a subsequent replan that added track `T3-consumer-repo-hygiene` /
  `S03-sworn-self-ignore` (`364765d`). Propagated by **merge, not copy**:
  `git merge --no-ff release-wt/2026-06-30-...` into the track. Disjoint file sets (release-wt owns
  board.json/index.md, track owns the slice design artefacts) → clean merges, no conflict. Drift
  gate reads 0 (`rev-list --count track..release-wt == 0`) after the merge; board.json on the track
  is the corrected string form + `schema_version` and carries all three tracks. Track pushed.

### Design review outcome (Captain → Coach) — 2026-07-01
PASS-with-pins. 6 pins + 5 flags; design anchors verified live, scope bounded, round-trip trap
surfaced. Two pins needed the Coach's call:

- **Pin 2 (D1 Type-1 ratification) — DONE.** Coach (Brad) ratified the carrier representation
  (structs + `Extra` overflow + custom marshalers). Recorded in `status.json`
  `design_decisions[0].human_decision`; `sworn designfit` now PASSES (3 slices clear).
- **Pin 1 (write-back validation gap) — RESOLVED as Option A (reconcile the schema), routed to
  replan.** Grounding: real fired data has **127 object deferrals using `acknowledged_by`, none
  with the schema-required `acknowledgement`** — so `state.Write` validation (not just read) fails
  on real data. Read-only would just relocate the fired run's death to the first write-back. Coach
  ratified Option A: relax `slice-status-v1` `open_deferrals.required` to accept **either**
  `acknowledgement` **or** `acknowledged_by` (`anyOf`), keeping Rule 2 intent. This needs a small
  **/replan-release** to add an AC ("real `acknowledged_by`-only deferral round-trips through Write
  without a validation error") + fold the schema relaxation into S01 scope, planner-ratified before
  `in_progress` (Rule 8). The schema is vendored from Baton → upstream mirror tracked as **#38**
  (PR-up follow-up; sworn-local patch lands now).

Pins 3–6 + flags (a)–(e) are implementer-owned, to address inline during implementation:
byte-stable round-trip assertion (map-based marshal), compile-thread the new types
(slice.go:712/718 via `violationsFromStrings`, tools_ops.go:601, tools_plan.go:70, verify.Input
through RunFirstPass/CheckBoundaryMocks/isDeclared), edit-corruption grep + FULL `go test ./...`
with per-package timeout, update the stale `verdict.go:42` `// Kept as []string` comment, confirm
no `switch result` defaults `inconclusive` into pass, oracle `blockedReason` via `ViolationStrings()[0]`,
and grep-confirm the not-touched report types don't alias `state.Verification.Violations`.

### Next
- `/replan-release 2026-06-30-sworn-operational-readiness` — add the `acknowledged_by` write-back AC
  + schema required-set relaxation to S01 (Pin 1, Option A). Then S01 returns to design-review-clear
  and proceeds to `in_progress`.

## 2026-07-01 — Session 2 (Implementer): design_review → in_progress → implemented

**State transition:** `design_review` → `in_progress` (gate satisfied: `approved-ack.md`
DECISION: PROCEED, Coach Brad; D1 Type-1 recorded in `design_decisions[0].human_decision`)
→ `implemented`. `start_commit` = `000ee08`.

### Delivered (all 10 ACs) — see proof.json for AC→evidence map
- AC-01/03: `state.Deferral`/`state.Violation` structs (named schema fields + `Extra
  map[string]json.RawMessage` overflow + custom `(Un)MarshalJSON`); `OpenDeferrals []Deferral`,
  `Verification.Violations []Violation`.
- AC-02: map-based `MarshalJSON` (sorted keys) → byte-stable write; round-trip fixture asserts
  identical bytes + all unknown keys survive.
- AC-04: `DeferralStrings()`/`ViolationStrings()` projections; repointed oracle/ledger/implement
  display consumers; router/route/validate_blocked untouched (they read the oracle's []string
  SliceState).
- AC-05: `CheckBoundaryMocks`/`isDeclared` → `[]state.Deferral`, match on `Item`+`Why` only;
  new regression proves a keyword in Tracking/Acknowledgement does NOT over-declare and an
  undeclared boundary mock still fails closed.
- AC-06: `NeedIDs`→`CoversNeeds` (`need_ids`→`covers_needs`); writers spec_record.go + task.go.
- AC-07: schema `verification.result` enum + `inconclusive`. Confirmed safe (merge.go:105 gates on
  STATE; routeImplemented default-routes to verify; no switch passes inconclusive).
- AC-08: reachability proven NON-DESTRUCTIVELY on the live fired repo (git clean before+after).
  Board oracle (same `state.Read` path as `RunSlice: read status`): real object slice S05 reads
  `unknown` on old binary → `verified` on new binary. Direct `state.Read` on S01-networth + S05:
  both OK, Extra preserves acknowledged_by/id on real data.
- AC-09: `go build ./...` + full `go test ./...` green (39 pkgs, 0 FAIL, no hang; per-package
  120s timeout). gofmt clean. Edit-corruption grep clean (Pin 6).
- AC-10: schema `open_deferrals` required → `anyOf[acknowledgement | acknowledged_by]`; write-path
  positive + schema-level negative tests.

### Decisions / divergences (also in proof.json `divergence`)
- **Backward-compat read tolerance (added, not in AC set):** the unmarshalers accept the legacy
  string form and upgrade it to object on write-back (one-way). Without it the migration would make
  sworn unable to read its own previously-written status.json — a regression for every existing
  board, contrary to the operational-readiness goal. Does not conflict with the design's
  "no flatten-to-string on WRITE" rule.
- **AC-10 write-path reading (the one judgement call):** live `state.Write` runs the legacy
  structural `baton.Validate` (ignores open_deferrals), NOT `baton.ValidateSchema`. The wholesale
  `Validate`→`ValidateSchema` rewire is explicitly the deferred keystone step-1b follow-up
  (validate_schema.go comment) and, measured here, would break out-of-AC writes (task.go
  `covers_needs:["N/A-task-mode"]` vs `^N-\d+$`; defer_slice has no tracking). So AC-10 is satisfied
  via schema anyOf + write-path positive test + schema-level negative test, and the wholesale rewire
  stays the named follow-up. Filed **#39**. Surfaced for the verifier/Coach.
- **Touchpoints subset:** 3 planned touchpoints (validate_blocked.go len-only; router.go + route.go
  consume the unchanged oracle []string) needed no change — fewer files than planned, none outside
  the track.

### Reachability artefacts
- scratchpad/ac08-reachability.txt (old-vs-new board on real object data)
- scratchpad/ac08-direct-read.txt (direct state.Read on real fired files)

## 2026-07-01 — Session 3 (Implementer): re-spec to strict additive, in_progress → implemented

**State transition:** `in_progress` → `implemented`. `start_commit` unchanged (`000ee08`) so the
verifier sees the full slice diff (sessions 2 + 3). Trigger: replan `61df7ac` (forward-merged into
T1 as `2d2a4e2`) REVERSED the first cut's anyOf — AC-10 amended to the STRICT ADDITIVE shape and
AC-11 added (migrate the data + push the shape to baton). The Coach ratified "improve the field,
migrate the data" over "loosen the schema": a name (`acknowledged_by`) is not Rule 2's plain-text
`acknowledgement`, and an anyOf in the vendored schema is permanent drift from canonical baton.

### Design-gate posture (Rule 9)
The carrier MECHANISM (decision[0]: structs + Extra overflow + custom marshalers) — the Type-1
architecturally-significant choice — is UNCHANGED by this flip. The anyOf→strict-additive change is
a required-set tightening the Coach explicitly directed (recorded in spec rationale Pin-1-REVISED +
replan commit body), and the planner returned the slice to `in_progress`, signalling the gate is
cleared. Adding one named field (`acknowledgement`, plus `acknowledged_by`/`acknowledged_at`) is what
the Extra-map design already anticipated. Recorded the strict-additive decision as a new Type-1
`design_decisions` entry with the Coach's `human_decision`. Proceeded as implementer.

### Delivered this session
- **AC-10 (strict additive):** schema `open_deferrals.required` = `[why, tracking, acknowledgement,
  acknowledged_by]`, `acknowledged_at` optional, anyOf REMOVED (`slice-status-v1.json`). Go `Deferral`
  gains named `AcknowledgedBy`/`AcknowledgedAt` (handled in Un/MarshalJSON; byte-stable map marshal
  preserved). Tests: `TestWrite_CanonicalDeferral_RoundTrips` (canonical writes + validates +
  round-trips with all named fields populated); `TestSchema_OpenDeferralStrictAdditive` (7 subtests —
  full canonical + acknowledged_at pass; acknowledged_by-alone, acknowledgement-alone, missing
  tracking/why, neither-key all FAIL closed via `baton.ValidateSchema`).
- **AC-03 test update:** `acknowledged_by` now a named field, asserted populated AND asserted NOT in
  Extra; id/description stay in Extra.
- **AC-11 (tracked, NOT sworn code):** #40 created (cutover: migrate 127 coach deferrals to add the
  plain-text `acknowledgement`); #38 re-purposed from the superseded anyOf framing to "push the
  canonical strict-additive shape up to baton". Sequencing recorded on both: do not run the fired
  LOOP on a strict binary pre-migration.
- **AC-08 re-proven on the STRICT binary** (not session 2's anyOf binary), non-destructive on
  `~/projects/fired` (clean before+after — Rule 11): board oracle renders the full
  2026-06-16-critical-journey-resolutions board, 0 "cannot unmarshal object"; direct `state.Read` of
  real S05 (4 deferrals) populates `acknowledged_by`/`acknowledged_at` into the new named fields,
  `acknowledgement=""` (real pre-migration data), id/field preserved in Extra. Direct-read artefact
  came from a throwaway test (absolute fired path) that was run then DELETED; its output is retained.
- **AC-09:** `go build ./...` + full `go test ./...` green (39 pkgs, 0 FAIL, no hang; per-package
  120s timeout). gofmt clean. Edit-corruption grep clean.

### Decisions / divergences (also in proof.json)
- **AC-10 fail-closed locus (carried from #39):** `state.Write` still uses structural `baton.Validate`
  (ignores open_deferrals item required-sets), not `ValidateSchema`. So "missing acknowledgement OR
  acknowledged_by fails closed" is enforced at the CANONICAL SCHEMA layer (schema-level negative test),
  not yet at `state.Write` runtime. The wholesale rewire is #39 — out of this slice's AC set (AC-11
  scopes the extra work as data-migration + baton-upstream). The AC-10 sentence structure (fail-closed
  clause subject is "a deferral", not "state.Write") supports the schema-layer reading.
- **Legacy string-form READ tolerance RETAINED** despite the replan's "no back-compat" theme: the
  concrete S01 instruction was the anyOf→strict required-set change + acknowledgement migration; no AC
  requires/forbids the string→object read upgrade. Removing it would regress sworn reading its OWN
  prior string-form boards — a regression NOT covered by AC-11's migration (which adds acknowledgement
  to object-form deferrals, not string→object conversion). One-way read upgrade; write is always
  object; does not weaken the strict schema. Surfaced for verifier/Coach.
- **defer_slice** writes a non-canonical deferral (Why+Acknowledgement, no tracking/acknowledged_by);
  harmless today (state.Write doesn't schema-validate), folds under #39.

## Verifier verdicts received

### 2026-07-01 — BLOCKED (fresh-context verifier, artefact-only)

**Verdict: BLOCKED** — spec defect in AC-08 (not an implementation fault).

All code acceptance criteria are delivered and verified green in a fresh window: `go build ./...`
clean, full `go test ./...` 39 packages ok / 0 FAIL, the AC-cited state tests pass (incl. the AC-10
strict-additive negative cases — `acknowledged_by`-alone and `acknowledgement`-alone both fail closed),
schema carries the strict required-set `[why, tracking, acknowledgement, acknowledged_by]` with NO
anyOf and the `inconclusive` result enum, `covers_needs` rename done, projection helpers present, the
real-data read-path artefacts exist on disk. The `verdict.go` change is comment+whitespace only.

**The single blocker is AC-08, which is internally contradictory with AC-11** (both authored in the
Coach-ratified strict-reversal replan 61df7ac): AC-08 requires the `sworn run` LOOP to run on fired and
"proceed past the D6 failure point", but AC-11 forbids running the fired loop on the strict-schema
binary (which this slice delivers) until the #40 deferral migration lands — so AC-08's loop-run is
un-runnable in-slice. Compounding this, the AC-08-named slice `S01-networth-hierarchy-remap` now has
`open_deferrals=[]` / `state=planned`, so it cannot exercise the object-form unmarshal even for a read.
The implementer correctly followed the later AC-11 (proved the read-path on the substitute object-form
slice 2026-06-16 S05 and deferred the loop to #40) — but ratifying a deviation from a binding AC is the
planner's authority, not the verifier's. Routed to `/replan-release` with a concrete AC-08 amendment
(see status.json verification.violations[0].proposed_amendment). The amendment is satisfied by the
already-committed artefacts, so re-verification should PASS without new implementation.
