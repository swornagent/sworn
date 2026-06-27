---
title: 'S22 — Bump vendor pin to canonical HEAD containing baton/ layout'
description: 'Update internal/adopt/baton/VERSION to a canonical Baton commit that contains the baton/ tool-neutral directory layout and records-as-JSON artefacts; update the source map so it references paths in the baton/ layout. The new pin must be ≥ the records-as-JSON canonical commit.'
---

# Slice: `S22-pin-bump`

## User outcome

`internal/adopt/baton/VERSION` references a canonical Baton commit SHA that (a) contains the `baton/` directory layout, (b) contains records-as-JSON schema definitions, and (c) results in a coherent source map — i.e., running `sworn baton vendor` with the new pin would succeed (not fail because the pin predates `baton/`).

## Entry point

`internal/adopt/baton/VERSION` (current SHA: `9ae08fbb1ef28ba5a4918a51018b01ba31b4797b` — predates `baton/` layout per audit).

## Pre-requisites (CRITICAL — do not skip)

**sworn#23 must merge before implementing this slice.** sworn#23 updates the vendor source map from the old `claude/baton/` path prefix to the new tool-neutral `baton/` prefix. Running this slice before sworn#23 merges will cause the sync to resolve paths against the old layout and fail closed.

## In scope

- Update `internal/adopt/baton/VERSION`:
  - `baton-protocol:` bump from `v0.5.0` to `v0.6.1`
  - `upstream-sha:` set to `42eb48b` (git rev-list -n1 v0.6.1 = 42eb48b; confirmed 2026-06-27)
  - `vendored:` set to implementation date
  - `upstream-digest:` update to the sha256 of the new commit's content
- `internal/prompt/` embed root: also bump to the same SHA `42eb48b` — both embed roots must reference the same canonical commit (sworn#24 pattern)
- Update `internal/adopt/baton/source_map.json`: ensure all source paths reference the tool-neutral `baton/` layout (e.g. `baton/schemas/*-v1.json`, `baton/role-prompts/planner.md`); sworn#23 must have already updated the source map's path prefix from `claude/baton/` → `baton/`

## Out of scope

- Copying any actual vendored files (prompt files, schemas) — that is S20 (prompts) and S13 (schemas); this slice only updates the VERSION and source map metadata
- doctor checks (S23)
- VERSION string centralisation (S23)

## Planned touchpoints

- `internal/adopt/baton/VERSION` (update pin SHA + semver + date)
- `internal/adopt/baton/source_map.json` (update paths to baton/ layout)

## Acceptance checks

- [ ] `internal/adopt/baton/VERSION` `upstream-sha:` is `42eb48b` and `baton-protocol:` is `v0.6.1`
- [ ] `internal/prompt/` embed root references the same SHA `42eb48b` (both embed roots in sync — sworn#24 requirement)
- [ ] `internal/adopt/baton/source_map.json` `paths` reference `baton/schemas/` (not `schemas/` without the `baton/` prefix)
- [ ] `sworn doctor` does not report "pin predates baton/ layout" after S23 adds the doctor check (this AC is verified in S23; the implementer only needs to set up the VERSION for this slice)
- [ ] `go build ./...` exits 0 after this change (VERSION and source_map.json are data files, not compiled code; build should be unaffected)

## Required tests

- **Reachability artefact**: `cat internal/adopt/baton/VERSION` shows new SHA; `cat internal/adopt/baton/source_map.json` shows `baton/` paths; `go build ./...` exits 0

## Risks

- sworn#23 is a hard pre-requisite; if it has not merged, stop and page the Coach
- The canonical tag is `v0.6.1` at SHA `42eb48b` — confirmed by Brad 2026-06-27; do not change to a different SHA without Coach approval

## Deferrals allowed?

No.
