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
