---
title: 'Change-sets — deferral additive-canonical (S01) + board strict-reader (S05)'
description: 'Two ratified change-sets: make the Rule-2 deferral shape additive-canonical (kept acknowledgement + added acknowledged_by/acknowledged_at), and make the board release reader strict (no string tolerance). No wild data exists, so strict + migrate beats back-compat.'
date: 2026-07-01
---

# Change-sets — deferral + board canonical

Ratified by Brad 2026-07-01. Principle: there is **no wild data yet** (all old artefacts are the
operator's own), so the compat-free window is open — define the canonical shape strictly, migrate
the operator's data once, and drop back-compat tolerance rather than carrying it forever. A strict
reader is also more fail-closed: it fails loud on a non-migrated artefact instead of silently
tolerating it.

---

## Change-set A — Deferral: additive-canonical, strict, migrate (S01)

**Decision:** the Rule-2 deferral is `why` + `tracking` + `acknowledgement` (the plain-text
evidence the decision-maker was *told* — Rule 2's actual substance) **plus** structured
attribution `acknowledged_by` (who) and `acknowledged_at` (when). Additive, not a swap: a name
alone does not carry the "told in plain text" evidence, and a bare structured field is easier to
stamp vacuously than a forced sentence — which would weaken No-Silent-Deferrals. Strict (no
`anyOf`); migrate the coach-produced data that has only `acknowledged_by`.

### A1 — schema `internal/baton/schemas/slice-status-v1.json` (open_deferrals.items)
Replace the `anyOf` relaxation with a strict additive required set:
```json
"items": {
  "type": "object",
  "additionalProperties": true,
  "required": ["why", "tracking", "acknowledgement", "acknowledged_by"],
  "properties": {
    "item": { "type": "string" },
    "why": { "type": "string", "minLength": 1 },
    "tracking": { "type": "string", "minLength": 1 },
    "acknowledgement": { "type": "string", "minLength": 1 },
    "acknowledged_by": { "type": "string", "minLength": 1 },
    "acknowledged_at": { "type": "string", "format": "date-time" }
  }
}
```
- Drop the `anyOf: [{required:[...acknowledgement]},{required:[...acknowledged_by]}]` block.
- `acknowledged_at` optional (data may lack timestamps); make it required only if you want hard
  provenance and are willing to backfill timestamps.

### A2 — Go type `internal/state/state.go` (the Deferral struct S01 introduced)
- Fields: `Item, Why, Tracking, Acknowledgement, AcknowledgedBy string` + `AcknowledgedAt string`
  (json: `acknowledged_at,omitempty`).
- Remove any either-or read-tolerance S01 added (no mapping `acknowledged_by`→`acknowledgement`).
  The two are distinct fields now; both are carried.
- Preserve round-trip fidelity exactly as D6 requires (keep the raw object / unknown keys so a
  write-back drops nothing — same rule as the board `Release.raw`).

### A3 — migrate the coach-produced deferral data (operator-owned, one-time)
The coach-produced status records carry `acknowledged_by` but no `acknowledgement`. Backfill a real
`acknowledgement` line (the plain-text record of the decision-maker being told) into each
open_deferral, and an `acknowledged_at` if available. This is operator data — write true
acknowledgements, don't auto-stamp. One-off script or manual pass across the affected `status.json`.

### A4 — push the shape up to baton (contract change)
This edits Rule 2's deferral definition + `slice-status-v1` upstream. PR to `~/projects/baton`:
Rule 2 wording = `why` + `tracking` + `acknowledgement` + `acknowledged_by` (+ optional
`acknowledged_at`); sworn's vendored schema then matches canonical (no drift). Supersedes #38's
"mirror the relaxation upstream" — we are NOT loosening baton; we are improving the field.

### A5 — process placement
S01 is **`in_progress`** (active implementer). This is a **spec amendment**, so it routes through
`/replan-release` (planner amends S01's `spec.json` to add the additive-deferral AC), then the S01
implementer applies A1–A3 and the migration. Do NOT hand-edit the active T1 worktree from another
session. The verifier then grades S01 against the amended spec.

---

## Change-set B — Board: strict reader, no string tolerance (S05/board)

**Decision:** with no wild string boards, the tolerant reader has no beneficiary. Make the reader
object-only too, so a legacy string board fails loud (surfacing any un-migrated straggler), and
migrate the operator's string boards once. Schema + validator + writer are already canonical
(S05); this closes the last back-compat path.

### B1 — `internal/board/board.go` `Release.UnmarshalJSON`
Drop the string branch; require the object:
```go
func (r *Release) UnmarshalJSON(b []byte) error {
	var o struct{ Name string `json:"name"` }
	if err := json.Unmarshal(b, &o); err != nil {
		return fmt.Errorf("board release: expected canonical object {name, ...}: %w", err)
	}
	if o.Name == "" {
		return fmt.Errorf("board release object missing required \"name\"")
	}
	r.Name = o.Name
	r.raw = append(json.RawMessage(nil), b...)
	return nil
}
```
Keep `StringRelease(name)` — it is a *constructor* that marshals to the canonical object `{name}`,
not reader-tolerance; `migrateFromIndex` still uses it.

### B2 — tests `internal/board/board_release_test.go`
- `TestRelease_StringForm` → invert: a bare string now **errors** (rename `..._Rejected`).
- Remove/replace `TestRelease_StringReadEmitsCanonicalObject` (string no longer reads).
- Keep the object-form and round-trip-fidelity tests.

### B3 — migrate the operator's string boards (one-time, SEQUENCED)
Convert `release: "X"` → `release: { "name": "X" }` in every string-form `board.json`:
`2026-06-30-sworn-operational-readiness`, `2026-07-01-release-hygiene`, `2026-06-27-conformance-foundation`,
and any others. **CRITICAL sequencing:** a strict reader + migrated boards must land together, and
only once every active session is on a canonical binary — a pre-S04 binary (string-only reader)
breaks on object boards, and a strict-reader binary breaks on un-migrated string boards. The
operational-readiness board is mid-flight (S01 active), so do B **after** that release's sessions
are on the canonical binary (post merge-track + `make build` reinstall). Until then, S05's tolerant
reader is harmless and bridges the gap.

### B4 — process placement
The reader-strict change contradicts S05's spec AC-03 ("reader SHALL remain tolerant"). Since S05 is
`implemented` (not yet verified), cleanest is to **amend S05's spec** (AC-03 → object-only reader; add
the migration AC) before its fresh verify, rather than spawn S06 — but only schedule the migration
(B3) for the post-cutover window above. If you'd rather not reopen S05's scope, B is a small S06
appended to T4 instead.

---

## Sequencing summary
1. **A (deferral)** routes through `/replan-release` → S01 implementer; independent of the board work.
2. **B reader+tests** can land whenever (code-only, behind the cutover for the migration).
3. **B3 migration** is the only sequenced piece: after the operational-readiness sessions are on a
   canonical (S04/S05) binary. Do it as the deliberate cutover step, not mid-run.
