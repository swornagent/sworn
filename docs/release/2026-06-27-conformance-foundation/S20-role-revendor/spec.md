---
title: 'S20 — Re-vendor planner, implementer, captain from canonical post-records-as-JSON'
description: 'Copy the current canonical versions of planner.md, implementer.md, and captain.md from the canonical Baton source (post-records-as-JSON pin) into internal/prompt/; bump VERSION.txt to record the re-vendor commit reference. Requires T6 to have merged (pin bump from S22).'
---

# Slice: `S20-role-revendor`

## User outcome

`internal/prompt/planner.md`, `internal/prompt/implementer.md`, and `internal/prompt/captain.md` match the canonical Baton role prompts from the post-records-as-JSON commit (the same commit bumped as the vendor pin in S22). Stale references to `proof.md`/`spec.md`-only workflows are updated to reflect the records-as-JSON reality. `internal/prompt/VERSION.txt` is bumped to the new pin commit SHA.

## Entry point

`internal/prompt/` — re-vendor the three role prompt files from canonical source.

## In scope

- Copy `planner.md`, `implementer.md`, `captain.md` from `$HOME/.claude/baton/role-prompts/` (canonical source) to `internal/prompt/`; these must be the versions at or after the records-as-JSON commit (post-S22 pin)
- `internal/prompt/VERSION.txt`: update to reflect the canonical commit SHA being vendored (must match S22's bumped pin)
- `verifier.md` is NOT re-vendored here (already done in S12, T3)
- Verify each copied file does NOT contain stale `proof.md`-only references, `scripts/release-verify.sh` references, or `v0.4.2` version strings

## Out of scope

- Any changes to how the prompts are embedded or served (that is T6 S22/S23)
- Changes to the design-reviewer split (S19 handles that)
- Re-vendoring verifier.md (S12)
- Any production Go code changes

## Planned touchpoints

- `internal/prompt/planner.md` (re-vendor)
- `internal/prompt/implementer.md` (re-vendor)
- `internal/prompt/captain.md` (re-vendor)
- `internal/prompt/VERSION.txt` (bump to new pin SHA)

## Acceptance checks

- [ ] `diff <(cat internal/prompt/planner.md) <(cat ~/.claude/baton/role-prompts/planner.md)` exits 0 (files are identical)
- [ ] `diff <(cat internal/prompt/implementer.md) <(cat ~/.claude/baton/role-prompts/implementer.md)` exits 0
- [ ] `diff <(cat internal/prompt/captain.md) <(cat ~/.claude/baton/role-prompts/captain.md)` exits 0
- [ ] `internal/prompt/VERSION.txt` content matches the pin SHA from S22 (T6)
- [ ] `grep -rn "v0.4.2\|proof.md-primary\|PROOF-optional\|scripts/release-verify.sh" internal/prompt/planner.md internal/prompt/implementer.md internal/prompt/captain.md` returns zero results
- [ ] `go build ./...` exits 0 after the re-vendor (no compilation errors from changed prompts)

## Required tests

- **Reachability artefact**: manual smoke step: diff commands above exit 0; `go build ./...` exits 0

## Risks

- The re-vendor requires the T6 pin bump to have landed (T5 depends_on T6); if T6 has not merged, the implementer must wait before implementing S20
- The canonical source path `$HOME/.claude/baton/role-prompts/` may not exist on all machines; the implementer should verify the path at implementation time

## Deferrals allowed?

No.
