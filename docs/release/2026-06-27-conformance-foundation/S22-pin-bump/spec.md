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

**sworn#23 was CLOSED, not merged — do NOT wait for it.** Its source-map fix (the vendor source map's path prefix `claude/baton/` → `baton/`, in `internal/baton/source.go` — NOT a `source_map.json`; that file does not exist) is already done on branch `refactor/baton-vendor-paths` (origin `6b35304`). **Incorporate that work into THIS slice**: cherry-pick or lift the `internal/baton/source.go` (+ `vendor.go`, `diff.go`, and the test-fixture path) changes from `6b35304` rather than redoing them or waiting for a merge that will never happen. Without the source-map fix, the sync resolves the old `claude/baton/` paths against the new `baton/` layout and fails closed.

## In scope

- Update `internal/adopt/baton/VERSION`:
  - `baton-protocol:` bump from `v0.5.0` to `v0.6.1`
  - `upstream-sha:` set to `42eb48b` (git rev-list -n1 v0.6.1 = 42eb48b; confirmed 2026-06-27)
  - `vendored:` set to implementation date
  - `upstream-digest:` update to the sha256 of the new commit's content
- `internal/prompt/` embed root: also bump to the same SHA `42eb48b` — both embed roots must reference the same canonical commit (sworn#24 pattern)
- Update the vendor source map (`internal/baton/source.go`, the `batonFileMappings`/`RuleSources()` — there is no `source_map.json`): ensure all source paths reference the tool-neutral `baton/` layout (e.g. `baton/schemas/*-v1.json`, `baton/role-prompts/planner.md`). This is the #23 work — lift it from `6b35304` (see Pre-requisites), do not wait for a merge.

## Out of scope

- Copying any actual vendored files (prompt files, schemas) — that is S20 (prompts) and S13 (schemas); this slice only updates the VERSION and source map metadata
- doctor checks (S23)
- VERSION string centralisation (S23)

## Planned touchpoints

- `internal/adopt/baton/VERSION` (update pin SHA + semver + date)
- `internal/baton/source.go` (update source-map paths to baton/ layout — lift from `6b35304`)
- `internal/prompt/` VERSION (bump the second embed root to the same SHA)

## Acceptance checks

- [ ] `internal/adopt/baton/VERSION` `upstream-sha:` is `42eb48b` and `baton-protocol:` is `v0.6.1`
- [ ] `internal/prompt/` embed root references the same SHA `42eb48b` (both embed roots in sync — sworn#24 requirement)
- [ ] the vendor source map (`internal/baton/source.go`) source paths reference `baton/…` (not `claude/baton/…`)
- [ ] `sworn doctor` does not report "pin predates baton/ layout" after S23 adds the doctor check (this AC is verified in S23; the implementer only needs to set up the VERSION for this slice)
- [ ] `go build ./...` exits 0 after this change (VERSION and source_map.json are data files, not compiled code; build should be unaffected)

## Required tests

- **Reachability artefact**: `cat internal/adopt/baton/VERSION` shows new SHA; `grep -c 'claude/baton' internal/baton/source.go` returns 0 (all source paths on the `baton/` layout); `go build ./...` exits 0

## Risks

- sworn#23 is CLOSED (will not merge); its source-map fix must be lifted from branch `refactor/baton-vendor-paths` (`6b35304`) as part of this slice — do not block waiting for a merge
- The canonical tag is `v0.6.1` at SHA `42eb48b` — confirmed by Brad 2026-06-27; do not change to a different SHA without Coach approval

## Deferrals allowed?

No.
