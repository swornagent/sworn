---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S21-canonical-baton`

## 2026-06-21 — re-scoped (replan)

Spec corrected during `/replan-release`. Original spec said "**7 rules**, copied
**verbatim** from `~/.claude/baton/`." Two problems: (1) the canonical set is now **10
rules** (Rules 8 Requirements Fidelity, 9 Design Fidelity, 10 Customer Journey
Validation were added in the fidelity-layer cycle), and (2) `~/.claude/baton/` is the
stale local install — copying it verbatim would drop 8/9/10 *and* risk embedding
internal content. Re-scope: `rules.md` is now built from the **repo's in-repo canonical
rule docs** at `internal/adopt/baton/rules/` (`01`–`10`), which already carry all ten
rules plus the no-mock→Rule-10 reconciliation (release-wt synced from `release/v0.1.0`
commit `5139882` during this replan). The role-prompt generalisation that the verbatim
copy would have leaked is split out to the new final slice `S27-public-readiness-scrub`.

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

### 2026-07-06 — PASS (verifier)

All six gates passed:

1. **Gate 1 — User-reachable outcome exists**: `sworn init` wired to `cmdInit` via `commands.go:14`; `sworn mcp` serves `sworn://baton/rules` from embed.
2. **Gate 2 — Planned touchpoints match actual changed files**: All 6 planned touchpoints present in diff. `init_design_system_test.go` minor adaptation (+`setupTemplates(dir)` calls) related to init rewrite. ADR 0005→0008 documented.
3. **Gate 3 — Required tests exist and exercise the integration point**: All 16 required tests pass. `go test ./internal/prompt/... -run Baton` — 5/5 PASS. `go test ./cmd/sworn/... -run Init` — 11/11 PASS. `go build ./...` — PASS.
4. **Gate 4 — Reachability artefact proves the user path**: Manual smoke test documented in proof.md — `sworn init --yes` creates AGENTS.md with `sworn://baton/rules`, no `docs/baton/` directory.
5. **Gate 5 — No silent deferrals or placeholder logic**: Zero TODOs/FIXMEs/deferred/placeholder in changed source files. Deferrals documented with all 3 Rule 2 elements.
6. **Gate 6 — Claimed scope matches implemented scope**: All 14 delivered items have verifiable evidence. `rules.md` carries all 10 rules confirmed.

Track T3-commercial is now complete — all 7 slices verified.
## 2026-07-06 — implemented (S21-canonical-baton)

**State transition**: design_review → in_progress → implemented.

**Coach pins addressed**:
1. ADR filename: `0005`→`0008` (0005 already taken by T2's `0005-tui-dep-bubbles.md`). Updated planned_files.
2. `track-mode.md` pre-exists from S08c — not recreated. Embed extended to `baton/*`.
3. `design_decisions` populated in status.json (5 Type-2 decisions).
4. Legacy detection in `TestInitWarnsLegacyBaton` uses `adopt.BatonSectionHeading` (`## Engineering Process — Baton`), not `<!-- baton:start -->`.
5. T14 boundary respected — all content is verbatim from sources; no bash→sworn transforms.

**Key trade-offs**:
- `docs/templates/agents.md` is read from the filesystem (not embedded), matching the pre-existing `materialiseCatalog` pattern. This means `sworn init` requires the repo templates directory to exist. A future slice could embed templates if needed.
- `adopt` package retained for `sworn doctor` — not deleted.

**Skeptic panel**: skipped — runtime does not support subagent dispatch.

**Reachability**: manual smoke test — `sworn init --yes` in temp dir creates AGENTS.md with `sworn://baton/rules`, does NOT create `docs/baton/`.

**Test commands**:
- `go test ./internal/prompt/... -run Baton` — 5/5 PASS
- `go test ./cmd/sworn/... -run Init` — 11/11 PASS
- `go build ./...` — PASS

**First-pass script**: 21 PASS, 1 FAIL (expected — `in_progress` state; resolved by transitioning to `implemented`).