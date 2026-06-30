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

### Next
- `/design-review S01-d6-record-reconciliation 2026-06-30-sworn-operational-readiness` (Captain) to
  ratify D1 (record `human_decision`), then the Coach acknowledges PROCEED before implementation.
