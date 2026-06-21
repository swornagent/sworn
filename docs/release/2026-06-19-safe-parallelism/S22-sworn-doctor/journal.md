---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S22-sworn-doctor`

## Session 1 — 2026-06-28

**State transition:** design_review → in_progress → implemented

### Design review resolution

Coach approved via `approved-ack.md` (2026-06-22). Three escalated pins resolved:
1. Embed structure is authoritative — 10 rule files (not single `rules.md`).
2. Protocol grew to 10 rules (canonical 7 + 08-requirements-fidelity, 09-design-fidelity, 10-customer-journey-validation).
3. `planner.md` uses `### Phase` (h3), 6 phases (not 4).

Coach add-on: S18/S19-dependent heading checks emit `[WARN]` not `[ERROR]` — a health tool must not report a clean repo as broken for unbuilt future slices.

### Implementation decisions

1. **Exported `adopt.AgentsFragment()`** — the `batonAGENTSFragment` constant is unexported in `internal/adopt`. Added an exported accessor function so `doctor --fix` can write the minimal AGENTS.md template. This is a minimal additive change to the adopt package.

2. **Injectable `checkDepFreshness` function variable** — per design §2.5, the `go list -m -u ./...` call is wrapped in a `var checkDepFreshness = defaultCheckDepFreshness` that tests can override. This enables the "registry unreachable" test without network calls.

3. **`SWORN_BATON_HOME` env override** — for testability, the baton home directory respects `SWORN_BATON_HOME` before falling back to `~/.claude/baton/`. This allows `TestDoctorSyncBaton` and `TestDoctorNoBatonHomeNoWarn` to run in isolation.

4. **Rule file name correction** — the spec listed `04-commit-messages-as-capture-layer.md` but the actual embedded file is `04-commit-messages-as-capture.md`. Fixed in the `batonRuleFiles` constant.

5. **Splice detection uses `strings.Contains`** — the actual AGENTS.md heading is `## Engineering Process — Baton (we dogfood the protocol)`, which contains `adopt.BatonSectionHeading` (`## Engineering Process — Baton`) as a substring. This correctly detects the legacy splice.

### Deferrals surfaced

1. **`sworn://baton/rules` MCP pointer check** — deferred (Rule 2).
   - Why: the `sworn://` MCP resource-URI scheme does not exist in any landed slice yet.
   - Tracking: S22 spec acceptance check (group 2).
   - **Acknowledgement**: Coach (brad), 2026-06-22, via approved-ack.md §2.4.

### Test results

All 12 doctor tests pass. Full `go test ./...` passes (no regressions). `go build ./...` and `go vet ./...` clean.

### Skeptic panel

Skipped — runtime does not support subagent dispatch in this session.

## Open questions

None.

## Deferrals surfaced

1. `sworn://baton/rules` MCP pointer check — see above.

## Verifier verdicts received

*(None yet.)*