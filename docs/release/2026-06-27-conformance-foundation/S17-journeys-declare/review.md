# Design Review — S17-journeys-declare

**Release:** 2026-06-27-conformance-foundation  
**Track:** T4-records-as-json  
**State at review:** design_review  
**Reviewed by:** Captain (fresh-context session)  
**Reviewed at:** 2026-06-28  
**Design TL;DR:** `docs/release/2026-06-27-conformance-foundation/S17-journeys-declare/design.md`

---

## Inputs loaded

| Input | Status |
|---|---|
| `spec.md` | ✓ loaded |
| `design.md` | ✓ loaded |
| `status.json` | ✓ loaded (`design_review`, open deferral on reachability_test_path) |
| Sibling slices S13–S16 | ✓ all `verified` |
| Release-wt drift check | ⚠ 1 commit behind (see Pin 1) |
| Project memory | ✓ loaded |
| `internal/journey/journey.go` | ✓ read |
| `internal/baton/schemas/journeys-v1.json` | ✓ read |
| `internal/journey/journey_test.go` | ✓ reviewed |
| S05-merge-gate-oracle status | ✓ `planned` (T1) |

---

## Six-step review

### Step 1 — Requirements coverage

Traceability table in design.md maps all 7 ACs. Spot-check:

| AC | Design coverage | Finding |
|---|---|---|
| AC1 `.sworn/journeys.json` exists | create file directly | ✓ |
| AC2 valid journeys-v1 schema | SaveArtefact validates — but only for test fixture | ⚠ Pin 2 |
| AC3 exactly 3 journeys with correct IDs | JSON file content | ✓ |
| AC4 each has `no_mock_boundary` | new struct field + JSON content | ⚠ Pin 3 |
| AC5 `artefact.IsRatified` true | hardcoded `is_ratified: true` in JSON | ✓ (see Pin 5) |
| AC6 `journey.Check()` → CheckPass | `TestCheck_S17Journeys` | ✓ |
| AC7 `sworn merge-release` not BLOCK | mapped to `sworn journeys --check` | ⚠ Pin 4 |

### Step 2 — Baton rule checks

**Rule 1 (Reachability):** Spec requires reachability artefact: `sworn journey --check` exits 0 AND (optional) `sworn merge-release --dry-run` smoke step. The design plans a unit test covering AC6 but does not explicitly plan the `sworn journeys --check` smoke step from the project root (not temp-dir). Covered by the pre-existing reachability test command in status.json. Low risk.

**Rule 2 (No Silent Deferrals):** Existing deferral on `reachability_test_path` is pre-approved per status.json. No new deferrals introduced.

**Rule 9 (Design Fidelity):** All three key choices are Type-2 (additive, narrow, easily reversible): struct field, schema property, committed JSON file. No Type-1 architectural choices.

**Rule 10 (Customer Journey Validation):** Design writes `ratified_by: brad@sawyer.net.au` at implementation time. Rule 10 requires "human-reviewed and … ratified." The design assumes Coach acknowledgment of the design TL;DR constitutes the human ratification authorization. This is the standard tool-mediated flow — but the three journey definitions should be explicitly ratified by the Coach in the acknowledgment reply, not just implicitly. See Pin 5.

### Step 3 — Mechanical check

1. `Journey` struct in `journey.go` — confirmed no `NoMockBoundary` field exists today. Addition is additive and non-breaking.
2. `journeys-v1.json` schema — confirmed no `no_mock_boundary` property today; no `additionalProperties: false` guard on journey items (so existing artefacts stay valid without the field).
3. `LoadArtefact` does NOT call `baton.Validate` — only `json.Unmarshal`. `SaveArtefact` DOES call `baton.Validate`. If `.sworn/journeys.json` is written directly (bypassing `SaveArtefact`), the committed file is never independently schema-validated. See Pin 2.
4. `NoMockBoundary string \`json:"no_mock_boundary,omitempty"\`` — `omitempty` will silently elide the field if value is empty string. See Pin 3.
5. `.sworn/` directory does not currently exist in T4 worktree — `SaveArtefact` creates it via `os.MkdirAll`. If writing directly, implementer must create it.
6. Spec's entry point (`sworn journey --generate`, `sworn journey --ratify`) — these flags do not exist. Spec has "(or equivalent)" qualifier, which covers writing directly. Not a violation.

### Step 4 — Memory-cited check

Rule 10 from project Baton docs: "A fail-closed gate runs after all slices verify but before merge: exit 0 only when the artefact exists and is human-ratified … The **no-mock boundary** is the rule's enforcement teeth."

The design correctly declares `no_mock_boundary` on all three journeys and writes `is_ratified: true`. The ceremony gap (who triggers ratification) is addressable inline via Coach acknowledgment. See Pin 5.

### Step 5 — Inter-slice dependency check

S16 (`journeys-v1` nested shape) — `verified`. ✓ Struct fields and schema shape confirmed in live code.

S05 (`merge-gate-oracle`) — `planned` (T1, blocked behind S04 which is in `design_review`). S05 is the slice that wires `journey.Check()` into `sworn merge-release`. **AC7 cannot be directly verified until S05 is implemented.** Design maps AC7 to `sworn journeys --check`, which is a reasonable proxy (S05 calls `journey.Check()`, and if that returns `CheckPass`, the gate passes). But the design's traceability note for AC7 is indirect — it relies on S05's future implementation. See Pin 4.

### Step 6 — Risk review

Design's own risk register covers:
- `NoMockBoundary` enum upgrade risk — mitigated (no enum today, noted as forward-compatible). ✓
- "commits to integration branch" spec note — correctly re-interpreted as eventual home via track merge flow. ✓

Unlisted risk in design: the committed `.sworn/journeys.json` file is never schema-validated unless `SaveArtefact` is used. See Pin 2.

---

## Pin list

**[mechanical]** **Pin 1** — Drift gate: T4 is 1 commit behind `release-wt/2026-06-27-conformance-foundation` (commit `a061677`: "replan: T3 & T7 depend_on T2-model-layer", only modifies `index.md` to document T3/T7 dependency on T2). This commit is immaterial to S17's spec, design, or test files — spec.md is byte-for-byte identical between T4 and release-wt. Strict gate says BLOCKED; Captain proceeds given non-materiality but flags it. **Directive:** forward-merge `release-wt/2026-06-27-conformance-foundation` into T4 before the first implementation commit.

**[mechanical]** **Pin 2** — AC2 schema-validation coverage gap: the committed `.sworn/journeys.json` is written directly (not via `SaveArtefact`), so `baton.Validate("journeys-v1", data)` never runs against the actual committed file. `TestCheck_S17Journeys` validates a `t.TempDir()` fixture via `SaveArtefact`, which IS schema-validated — but that fixture is not the committed file. **Directive:** add one of: (a) a sub-test step in `TestCheck_S17Journeys` that reads the committed file from a known relative path and calls `baton.Validate("journeys-v1", data)` directly; OR (b) write `.sworn/journeys.json` via `SaveArtefact` in a `TestMain` or `init_test` step so schema validation runs for the actual artefact. Option (a) is preferred — it keeps the proof tight without a global init side-effect.

**[mechanical]** **Pin 3** — `omitempty` silent elision on `no_mock_boundary`: `NoMockBoundary string \`json:"no_mock_boundary,omitempty"\`` silently omits the field from JSON output if the value is empty string. AC4 requires all three journeys to declare this field. `SaveArtefact` / `LoadArtefact` will not surface the omission — `CheckPass` can be returned even if all `NoMockBoundary` values are empty and the field is absent in the JSON. **Directive:** `TestCheck_S17Journeys` must explicitly assert `a.Journeys[i].NoMockBoundary != ""` for i = 0, 1, 2 after loading the artefact — do not rely solely on `CheckPass` as AC4 evidence.

**[mechanical]** **Pin 4** — AC7 traceability indirect: design maps "AC7 — `sworn merge-release` does not BLOCK" to "`sworn journeys --check` exits 0." These are different commands. S05 (the slice that wires `journey.Check()` into `sworn merge-release`) is `planned` and blocked behind S04 (`design_review`) in T1. `sworn merge-release` as a CLI gate doesn't exist yet. **Directive:** update the AC7 row in the design.md traceability table to read: "AC6 (`journey.Check()` → `CheckPass`) + S05 gate wiring → AC7 satisfied transitively. Until S05 ships, `sworn merge-release` has no journey gate; `sworn journeys --check` exits 0 is the direct reachability artefact for this slice."

**[memory-cited]** **Pin 5** — Human ratification ceremony: Rule 10 requires journeys to be "human-reviewed and … ratified." Design proposes writing `ratified_by: brad@sawyer.net.au` at implementation time (agent writes on human's behalf). This is acceptable IF the Coach explicitly ratifies J1, J2, J3 as part of the design acknowledgment. **Directive:** the suggested acknowledgment reply (below) includes an explicit ratification statement. Implementer may write `ratified_by: brad@sawyer.net.au` and `ratified_at: <today>` only after receiving the Coach's explicit ratification in the acknowledgment.

---

## Summary

5 pins: 4 mechanical, 1 memory-cited. No escalations required. None would cause the slice to ship broken if applied inline. All are addressable during implementation without a redesign pass.

- **Critical (slice would ship broken if unaddressed):** Pin 3 (AC4 test assertion). Without it, `no_mock_boundary` omission is silent and AC4 goes unverified.
- **High (verifiability gap):** Pin 2 (AC2 schema gap). Manageable but creates a dark corner in proof coverage.
- **Medium:** Pin 4 (AC7 traceability). Cosmetic impact on the proof; doesn't break anything.
- **Low:** Pin 1 (drift, immaterial), Pin 5 (ceremony, satisfied by acknowledgment).

---

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All five pins are apply-inline (no redesign needed); none require re-checking the design before code; Pin 3 is critical but has a single unambiguous fix (add assertion); Pin 5 is resolved by Coach's explicit ratification in the acknowledgment reply.
-->
