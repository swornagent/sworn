---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S20-mcp-catalog-tools`

## Session 1 — 2026-07-07 (implementer)

### State transitions

- `design_review` → `in_progress` (Coach approved via `approved-ack.md`, PROCEED with 5 pins)
- `in_progress` → `implemented`

### Coach pins addressed

1. **Pin 1 — `design_decisions` added to status.json**: 5 Type-2 decisions from design.md §2.
   `sworn designfit` blocked by pre-existing S04b `open_deferrals` schema mismatch (structured
   objects vs `[]string`). S20's array is correctly formatted.
2. **Pin 2 — Decision 1 framing fixed**: "stdlib is sufficient" per [[project_dep_policy]].
3. **Pin 3 — "request-time" dropped from §4**: `internal/prompt/prompt.go` loads at `init()`.
4. **Pin 4 — `create_release` → `plan_release` in intake.md**: Both references updated.
5. **Pin 5 — AC2 fixture guard**: `TestPlanReleaseExisting` asserts against fixture count.
   Noted in proof.md that real release exceeds 24 slices.

### Flag resolutions

- **Flag (a) — full return shape**: `plan_release` returns `{exists, created_paths?, state_summary?}`.
  `state_summary` includes `slice_count` derived from the slices table row count.
- **Flag (b) — screenshots directory**: Created `docs/release/2026-06-19-safe-parallelism/screenshots/`.

### Design decisions made during implementation

1. `docs/` parent directory not auto-created by tools — tests create it via `os.MkdirAll`.
   Consistent with other tools (e.g., `CreateRelease` creates its own directory structure).
2. `extractSectionField` named to avoid collision with existing `extractField` in `context.go`.
3. `appeaseToSection` appends before the next `##` heading if section exists; creates section
   at end if absent.
4. `searchDecisions` splits on `### ` boundaries for entry isolation.
5. `releaseStateSummary` counts slice table rows by scanning for `| S` in table rows.

### Verifier guidance

- AC2 `slice_count: 24` is fixture-based in the test. The real release has ~59 slices.
- `manual-smoke-step` reachability — no automated E2E test.
- `sworn designfit` cannot run due to S04b pre-existing issue (not S20's defect).

## Open questions

None.

## Deferrals surfaced

- Semantic/vector search on decisions.md — post-R3. **Acknowledged**: Coach, 2026-06-20.
- `sworn designfit` blocked by S04b — tracked in release board; not S20 scope.

## Verifier verdicts received

*(None yet.)*
### Skeptic panel

- **skipped** — runtime does not support subagent dispatch (no Agent/Workflow tool available).
