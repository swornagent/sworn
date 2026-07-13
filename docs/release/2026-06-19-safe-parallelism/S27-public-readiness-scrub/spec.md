---
title: 'S27-public-readiness-scrub — make the sworn repo + binary public-safe before launch'
description: 'Final launch-gate slice. Generalise the embedded role prompts (keep Captain/Coach; strip coach-loop / --auto-ack / approved-ack / S21-stall / project-memory; re-express loop coupling via sworn run; operationally intact). Scrub dogfood provenance comments, the fired/the consumer project product-name leak, and coach-loop references across tracked source, docs, and release artefacts. Runs last, after every other track merges.'
---

# Slice: `S27-public-readiness-scrub`

## User outcome

The `sworn` repository (and the binary it ships) is **public-safe**: a developer
who clones the public repo or reads the embedded prompts finds no references to the
private orchestration tooling (`coach-loop`, `--auto-ack`, `approved-ack`/`decline`
filesystem signals), no cryptic dogfood provenance comments (`// (Captain pin N)`),
no internal project-memory citations, and no other product's name (`fired`/the consumer project).
The distinctive sport-aligned role vocabulary (**Captain / Coach / Planner /
Implementer / Verifier**) is retained, and the autonomous loop (`sworn run`) still
works — the generalisation is public-safety, not a behaviour change.

## Entry point

A public-readiness audit + scrub across the tracked tree. Verifiable by: running the
grep guards below and getting clean output; building the binary; and confirming
`sworn run`'s design-review step still drives correctly with the generalised embedded
`captain.md` (no operational regression).

## Why this runs last

The scrub touches files authored across nearly every track (Go source comments,
embedded prompts, release artefacts). Sequencing it as the final slice — a dedicated
track (`T10-public-readiness`) that depends on every other track — means it runs after
all sibling tracks have merged, so its wide touchpoints collide with nothing in
parallel. It is the launch gate: **sworn must not be flipped public until this slice is
`verified`.**

## In scope

### 1. Generalise the embedded role prompts (public-safe + operationally intact)

`internal/prompt/captain.md` (and `implementer.md` / `verifier.md` / `planner.md` where
they carry the same coupling) ship inside the binary via `go:embed`. Generalise them:

- **Keep** the role vocabulary: Captain, Coach, Planner, Implementer, Verifier.
- **Strip** the private-orchestration coupling: `coach-loop`, `--auto-ack`,
  `approved-ack.md` / `decline.md` filesystem-signal mechanics, the "S21 stall"
  incident reference, and project-specific memory citations (`[[project_*]]`,
  `[[feedback_*]]`).
- **Re-express** the loop coupling in terms of `sworn run`'s native mechanism (the Go
  successor to the bash `coach-loop`), not the bash signal files.
- **Operationally intact:** `sworn run` must still drive the Captain design-review step
  correctly. This is a public-safety rewrite, not a behaviour change — verify the loop
  still functions.

### 2. Scrub dogfood provenance comments from Go source

Replace the 8 `// (Captain pin N)` / `(Coach Pin N)` provenance comments in tracked Go
source with plain rationale comments — **keep the engineering reason, drop the
private-review citation**. Known sites (re-grep for the current set, do not trust this
list): `cmd/sworn/main.go`, `cmd/sworn/bench.go`, `internal/agent/tools.go`,
`internal/config/init.go`, `internal/config/config_test.go`,
`internal/implement/implement.go`, `internal/model/oai.go`.

### 3. Scrub the fired/the consumer project product-name leak

`docs/release/2026-06-19-safe-parallelism/S03-verify-under-concurrency/spec.md`
references `fired`/the consumer project. S03 is in the already-merged T1 track — this is a
public-readiness **doc fix** (not a re-scope of S03's contract). Re-grep the whole tree
for `fired` / `the consumer project` / `consumer` and genericise every tracked hit.

### 4. Audit & genericise coach-loop references in release artefacts + ADR

`coach-loop` appears across tracked `docs/release/*/{journal,proof,review,approved-ack}.md`,
several `status.json`, and `docs/adr/0001-*.md`. Genericise references that name the
private tool to neutral terms ("the autonomous loop" / `sworn run`). Historical dogfood
mentions that are generic and harmless may stay; the **tool/script names** (`coach-loop`,
`captain-route`, `captain-dispatch`, `baton-server-*`) must not.

## Out of scope

- Renaming the Captain/Coach role vocabulary — explicitly retained (decision 2026-06-21).
- Any production behaviour change. This slice changes comments, docs, and prompt *text*,
  never logic.
- The `internal/prompt/baton/` rule-doc embed — owned by `S21-canonical-baton`.

## Planned touchpoints

- `internal/prompt/captain.md`, `internal/prompt/implementer.md`,
  `internal/prompt/verifier.md`, `internal/prompt/planner.md` (generalise)
- `cmd/sworn/main.go`, `cmd/sworn/bench.go`, `internal/agent/tools.go`,
  `internal/config/init.go`, `internal/config/config_test.go`,
  `internal/implement/implement.go`, `internal/model/oai.go` (provenance comments)
- `docs/release/2026-06-19-safe-parallelism/S03-verify-under-concurrency/spec.md` (fired)
- `docs/adr/0001-one-binary-embedded-protocol-distribution.md` (coach-loop ref)
- Various `docs/release/**/{journal,proof,review,approved-ack}.md` + `status.json`
  (coach-loop refs — re-grep for the live set)
- `internal/prompt/prompt_test.go` (add the no-proprietary-markers guard test)

## Acceptance checks

- [ ] `git grep -nE 'coach-loop|--auto-ack|approved-ack\.md|captain-route|captain-dispatch'`
  returns no hits in tracked **source or embedded prompts** (`internal/`, `cmd/`); any
  remaining hit in `docs/release/**` historical artefacts is explicitly listed in
  `proof.md` with a why-it-is-harmless note
- [ ] `git grep -nE '\(Captain [Pp]in|\(Coach [Pp]in'` returns zero hits
- [ ] `git grep -niE 'the consumer repo|consumer|[^a-z]fired[^a-z]'` returns zero hits in tracked files
- [ ] `git grep -nE '\[\[project_|\[\[feedback_'` returns zero hits in `internal/prompt/`
- [ ] embedded `captain.md` retains the words "Captain" and "Coach" (role vocab kept)
- [ ] `go build ./...` passes
- [ ] `sworn run` design-review step still drives correctly (smoke: a dry-run / unit
  exercise of the Captain prompt path — no operational regression)

## Required tests

- **Unit** `internal/prompt/prompt_test.go` (extend):
  - `TestEmbeddedPromptsPublicSafe`: for each embedded role prompt, assert the text
    contains none of `coach-loop`, `--auto-ack`, `approved-ack`, `captain-route`,
    `[[project_`, `[[feedback_`, "S21 stall"
  - `TestCaptainKeepsRoleVocab`: `Captain()` still contains "Captain" and "Coach"
- **Guard script** (re-runnable in CI): the four grep checks above, exit non-zero on any
  source/prompt hit.
- **Reachability artefact**: run the grep guard and capture clean output; build the
  binary; exercise the `sworn run` Captain design-review path and confirm no regression.
  Document in `proof.md`.

## Risks

- **Operational regression from over-stripping `captain.md`.** The Captain prompt drives
  real loop behaviour; removing coupling text without re-expressing it via `sworn run`
  could break design-review routing. Verify the loop still works — do not strip blindly.
- **The grep for `fired` must avoid false positives** (`configured`, `transferred`,
  `required`, `deferred` all contain the substring). Use word-boundary / case-aware
  patterns and review each hit.
- Re-grep for the live set of provenance comments and coach-loop references at
  implementation time — the lists in this spec are a 2026-06-21 snapshot and other
  slices may add or remove sites before this one runs.

## Deferrals allowed?

None. This is the launch gate; the tree must be clean before sworn goes public.
