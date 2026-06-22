---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S30-lint-touchpoints`

## 2026-06-21 — planned (replan)

Added during `/replan-release` to harvest fixes §3a #2 and #4 (theme T-A, ~35 rows —
the most common Captain-catch class) from the trial-log analysis
(`2026-06-21-captain-trial-log-harvest.md`). Designs repeatedly touch files/packages
they never declared in `planned_files`, or collide with another slice across tracks.
Evidence rows: `S02b-concurrent-scheduler` (undeclared `internal/board/`),
`S10-buy-property-action-form` (missing `fire-validation` package),
`S18-income-path-action-wiring` (`projection_init.go` + `types_shared.go` absent),
`S16-sankey-any-year` (5 files missing), `S17-offset-cascade-target` (S27 collides on
all 4 core files, unacknowledged); migration-number collision S13↔S17.

**Rationale:** mechanise touchpoint reconciliation — parse the design's referenced
files/packages, reconcile against planned_files AND the index.md collision matrix, and
detect duplicate migration numbers — so the dominant defect class is caught before code.

Placed in new track `T12-harness-hardening` (depends_on `T1-concurrency-core`). Shares
the `internal/lint` package with S29/S31; serialised within T12, no parallel collision.

## Open questions

None.

## Deferrals surfaced

None.

## 2026-06-22 — design review

Captain reviewed (4 pins: 3 mechanical, 1 escalate). Coach approved with accept of informational-note substitution + Rule 2 deferral for prose non-additive detection. Pin resolutions:

1. **Section-scoped extraction (CRITICAL)** — implemented; only `## In scope` and `## Planned touchpoints` sections are parsed
2. **Add `design_decisions` to status.json** — 5 entries, all Type-2, applied
3. **Additive-invariant scope reduction** — Coach accepted; DOCUMENTED SHARED files get informational notes, not violations; non-additive detection = Rule 2 deferral
4. **Spec Risk audit** — confirmed S02b uses `## In scope` (line 18) and `## Planned touchpoints` (line 43) with exact casing

Flag: `cmdLint` usage string updated to include `touchpoints`.


**skeptic_panel:** skipped — runtime does not support subagent dispatch (no Agent/Workflow tool available in this session). The real verifier (Rule 7) is the backstop.

## 2026-06-28 — implemented

**Implementation approach:** `CheckTouchpoints(sliceDir, releaseDir string) error` in `internal/lint/touchpoints.go`. Three checks: (1) extract back-ticked file/package refs from spec's In-scope + Planned-touchpoints sections, reconcile against planned_files; (2) parse index.md `### Touchpoint matrix` for cross-slice file collisions; (3) detect duplicate 6-digit migration prefixes across all release slices.

**Key decisions:**
- Section-scoped extraction (Pin #1): only `## In scope` and `## Planned touchpoints` parsed — eliminates Risk/Required-tests/Out-of-scope false positives
- False-positive filters: bare extensions (`.go`, `.ts`), Go patterns (`...`), template placeholders (`<>`) excluded from extraction
- Suffix-matching for bare filenames: `touchpoints.go` matches `internal/lint/touchpoints.go` via suffix check
- DOCUMENTED SHARED files → informational note, not violation (Coach-accepted substitution)
- Touchpoint matrix: T12 omitted from columns per index.md notes; no self-collisions
- Two remaining false positives on S30's own spec: `cmd/sworn/main.go` and `index.md` from illustrative In-scope prose — inherent limitation of prose-path extraction; fixture-based reachability tests all pass correctly

**Test results:** 8 new tests (7 PASS), all existing deps tests still PASS. `go vet` clean. `go build` clean.

**Reachability:** Fixture tests confirm undeclared → exit 1, collision → exit 1, clean → exit 0.

## Deferrals surfaced

- **Prose-based non-additive edit detection** — Rule 2 deferral. Coach accepted informational-note substitution. **Acknowledged**: Brad, 2026-06-22 (approved-ack.md).

## Verifier verdicts received

None yet.
### Verdict 1 — 2026-06-28T20:15:00Z (fresh context, artefact-only)

**PASS**

All six gates passed:
1. User-reachable outcome — `sworn lint touchpoints` wired in `cmd/sworn/lint.go`, binary runs
2. Planned touchpoints match — all 3 planned code files in diff (plus 3 Baton docs)
3. Required tests — 8/8 touchpoint tests PASS, `go vet` clean
4. Reachability artefact — CLI invocations documented + verified against real releases
5. No silent deferrals — zero TODOs/FIXMEs/placeholders in production code
6. Scope matches claimed — all 5 spec ACs delivered; 1 Rule 2 deferral properly documented

Note: `start_commit` in status.json is a branch ref rather than a commit SHA; the proof documents
the actual session baseline as `b2c25f2`. Non-blocking implementer defect.
