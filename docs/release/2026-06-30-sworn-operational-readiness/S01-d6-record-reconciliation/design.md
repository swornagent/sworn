# Design TL;DR — S01-d6-record-reconciliation

**Slice:** `S01-d6-record-reconciliation` · **Track:** `T1-operational-unblock` ·
**Release:** `2026-06-30-sworn-operational-readiness`

**User outcome (spec):** sworn reads a real coach-produced `status.json` (object-form
`open_deferrals` and `verification.violations`) without an unmarshal error and round-trips it
without dropping any field, so the autonomous loop can run a real release instead of dying
instantly on status-read.

This is the D6 #31 hard blocker from the live fired dogfood run. The fix migrates sworn's Go
carrier types **up** to the `slice-status-v1` schema (the schema is the ratified contract; the
Go structs lag it).

---

## Approach (one paragraph)

`slice-status-v1` defines `open_deferrals` and `verification.violations` as arrays of **objects**
(`additionalProperties: true`), but sworn carries them as `[]string`. So `state.Read` on real
coach data fails with `cannot unmarshal object into Go struct field ... of type string`. We
introduce two carrier structs — `state.Deferral` and `state.Violation` — that model the schema's
named keys **and preserve any extra keys** (via a custom `(Un)MarshalJSON` with an overflow map),
so the Go representation is loss-free. To keep the diff bounded to the contract surface (Rule 6
scope ceiling), display-only consumers read through **string-projection helpers**
(`Status.DeferralStrings()`, `Verification.ViolationStrings()`) rather than each call site being
rewritten. Only the type defs, the write path, the Rule-10 boundary consumer, the helpers, the
two `need_ids` writers, and the schema enum change. The migration is **atomic** — a type change
cannot compile half-applied — so it stays one slice despite sitting at the ~15-file ceiling.

---

## Key design choices + rationale

### D1 — Carrier representation: struct + overflow map with custom marshalers  *(Type-1, architecturally significant — needs Captain ratification)*

`Deferral` / `Violation` are structs with named fields for the schema's known keys **plus**
`Extra map[string]json.RawMessage` (json:"-") to capture unknown keys, with custom
`UnmarshalJSON` (decode known fields, route the rest into `Extra`) and `MarshalJSON` (merge known
fields + `Extra`, marshal as a map so key order is deterministic).

```go
type Deferral struct {
    Item            string `json:"item,omitempty"`
    Why             string `json:"why,omitempty"`
    Tracking        string `json:"tracking,omitempty"`
    Acknowledgement string `json:"acknowledgement,omitempty"`
    Extra           map[string]json.RawMessage `json:"-"`
}
type Violation struct {
    Gate              string `json:"gate,omitempty"`
    Description       string `json:"description,omitempty"`
    Evidence          string `json:"evidence,omitempty"`
    ProposedAmendment string `json:"proposed_amendment,omitempty"`
    Extra             map[string]json.RawMessage `json:"-"`
}
```

Options considered:
- **(chosen) struct + `Extra` overflow + custom marshalers** — typed access for the Rule-10
  consumer (AC-05); preserves unknown keys (AC-03); spec explicitly says "structs modelling the
  object shapes AND preserving unknown keys".
- `type Deferral map[string]any` — trivially loss-free but no typed fields; AC-03 says *structs*,
  and the boundary consumer would devolve to map lookups + type asserts.
- `[]json.RawMessage` — byte-perfect round-trip but zero typed access; rejected for the same reason.

The planner already ratified the *direction* (migrate up to schema, structs, preserve unknowns) in
`spec.json` rationale + AC-03. What the Captain ratifies here is the **representation mechanism**
(custom marshalers + overflow map) and its determinism guarantee. Recorded in `status.json`
`design_decisions` with `human_decision` left for the Captain.

### D2 — Projection helpers, not call-site rewrites *(Type-2)*

`Status.DeferralStrings() []string` and `Verification.ViolationStrings() []string` give display-only
consumers a `[]string` view. Bounds the diff and keeps the oracle's `SliceState.Violations []string`
(which the router/route.go consume) unchanged — the oracle just calls `ViolationStrings()`.

### D3 — Rule-10 boundary consumer reads structured fields *(Type-2)*

`CheckBoundaryMocks` / `isDeclared` change signature `[]string` → `[]state.Deferral`. The declaration
match runs over the **description-bearing** fields (`Item` + `Why`) — the semantic equivalent of the
old free-form string — *not* `Tracking`/`Acknowledgement` (IDs/URLs, which could spuriously contain a
boundary keyword and over-declare). This keeps enforcement **at least as strict** (AC-05). A
regression test proves an undeclared mock at a validated boundary still fails closed.

### D4 — verdict→state bridge stays string-sourced *(Type-2)*

sworn's own write path (`run/slice.go:712-718`) sets `Verification.Violations` from
`verdict.Result.Violations` (`[]string`, kept as strings by design — `verdict.go:40`). A small
`violationsFromStrings([]string) []Violation` helper wraps each into `Violation{Description: s}`.
No field-loss concern here — this is sworn-generated, not coach-read; `ViolationStrings()` reproduces
the same display string for the oracle.

---

## Files I intend to touch (AC → change traceability)

| AC | Change | Files |
|----|--------|-------|
| AC-01, AC-03 | Define `Deferral`/`Violation` structs + custom `(Un)MarshalJSON`; retype `Status.OpenDeferrals []Deferral`, `Verification.Violations []Violation` | `internal/state/state.go` |
| AC-02 | Round-trip fidelity is a property of D1's marshalers (no extra code); proven by fixture test | `internal/state/state.go` (+ test) |
| AC-04 | Add `DeferralStrings()` / `ViolationStrings()`; repoint display consumers | `internal/state/state.go`, `internal/implement/implement.go:187`, `internal/implement/proof_record.go:74`, `internal/board/oracle.go:236,259` |
| AC-05 | `CheckBoundaryMocks`/`isDeclared` → `[]state.Deferral`, match on `Item`+`Why`; thread the type through `run/slice.go:547` first-pass path | `internal/verify/verify.go:65,386,487`, `internal/run/slice.go:547`, `internal/verify/validate_blocked.go` (len-only, likely no change) |
| AC-04/D4 | verdict→state conversion helper | `internal/run/slice.go:712-718` |
| write path | construct `Deferral{}` instead of appending a string | `internal/mcp/tools_ops.go:601` (and `tools_plan.go` if it writes deferrals) |
| AC-06 | `NeedIDs`→`CoversNeeds`, tag `need_ids`→`covers_needs`; update writers | `internal/state/state.go:222`, `internal/implement/spec_record.go:58`, `cmd/sworn/task.go:145` |
| AC-07 | add `"inconclusive"` to `verification.result` enum | `internal/baton/schemas/slice-status-v1.json` |
| AC-08 | reachability: `sworn run` against live fired release | (no code — smoke run) |
| AC-09 | `go build ./...` + `go test ./...` green | (whole repo) |

**Schema note (AC-06):** the schema **already** names `covers_needs` (line 25). The Go `need_ids`
tag is the lagging side, which is why planner-written `covers_needs` is currently dropped on read
(N-03). The rename is Go-only; no schema change for AC-06.

**Explicitly NOT touched (AC-04 guard):** report types named `Violations` in `reqverify`, `ears`,
`gate`, `designaudit`, `specquality`, `lint`, `rtm`, and the **scrape-layer** `sv.Violations`
projection at `verify.go:225-235` (already a distinct structured type feeding `verdict.Result`,
which stays `[]string`). These are different types from `state.Verification.Violations`.

---

## Design-level risks / pins for the reviewer

1. **`acknowledgement` (schema-required) vs `acknowledged_by` (fired's real key).**  The schema
   requires `acknowledgement`; fired's real deferrals carry `acknowledged_by` and (per spec)
   `{id, description, why, tracking, acknowledged_by}` — **no `acknowledgement`**. `state.Write`
   validates against the schema, so a naive Read→Write of a deferral lacking `acknowledgement`
   would fail closed on *validation*, not field-loss. **Resolution:** the AC-02 round-trip fixture
   carries the schema-required fields **plus** the extras (`acknowledgement` present → schema-valid;
   `id`/`description`/`acknowledged_by` are the overflow keys whose survival we assert). The real
   fired data (AC-08) only needs to **read** (unmarshal into struct+Extra succeeds regardless of
   `required`); whether the loop *writes that deferral back* and trips validation is a separate
   question — **reviewer please confirm AC-08's fired run does not depend on writing back a
   non-schema-compliant deferral**, else we surface a follow-up deferral.
2. **Marshal determinism.** `status.json` is rewritten every transition; non-deterministic key
   order would produce phantom diffs and break the drift gate. Marshalling via a `map` (encoding/json
   sorts map keys) gives stable output. Pin: a byte-stable round-trip assertion in the fixture test.
3. **Type threading through the first-pass path.** `run/slice.go:547` holds a `[]string` local fed
   to `RunFirstPass`→`CheckBoundaryMocks`. The `[]state.Deferral` type must thread through
   `RunFirstPass`'s signature too — small ripple, called out so it isn't missed.
4. **RESOLVED (not in S01's scope) — board.json `release`-object vs oracle `string` (sibling D6 bug).**
   During Step 0 `sworn board` returned `tracks: null` for this release. Two stacked causes: (a) the
   planner's `board.json` had `release` as an **object** the typed `BoardRecord.Release string` reader
   couldn't parse, and (b) the **installed `sworn` binary was stale** — it predated the board.json
   read path entirely, so it silently fell back to the empty `index.md` frontmatter (which is why
   even the object form produced exit 0 / null instead of a parse error). The human reconciled
   `board.json` to the conformant `board-v1` string form (+`schema_version`) on `release-wt`, and the
   binary was reinstalled from current source; `sworn board` now returns both tracks with correct
   states. **Confirmed same class as S01 (a Go carrier lagging the record), but one layer up and out
   of S01's scope** — `oracle.go` is a touchpoint here only for `parseStatusJSON`. No S01 work item;
   recorded for context only.

---

## Definition of Ready (Rule 8) — confirmed

9 acceptance criteria, all EARS-typed (event-driven / ubiquitous / unwanted), each naming concrete
artefacts (file paths, line numbers, exact JSON shapes, the live fired release path). Passes the
spec-completeness sniff test. No spec gaps to fill from intake.

## Reachability artefact (Rule 1)

`manual-smoke-step` (AC-08): `sworn run` against the live fired release
`2026-06-28-yearSnapshot-schema-cleanup` (`~/projects/fired`, `--docs-prefix
apps/docs/content/docs`) — the run that previously died at the `open_deferrals` unmarshal — must
read `S01-networth-hierarchy-remap`'s status and proceed past the D6 failure point. Plus the AC-02
round-trip fixture test as the in-repo integration proof.
