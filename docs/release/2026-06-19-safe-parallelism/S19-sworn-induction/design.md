---
title: 'S19-sworn-induction — Design TL;DR'
description: 'Implementation design for sworn induction command and prompt deviation/catalog checks'
---

# Design TL;DR: S19-sworn-induction

## §1. User-visible change

A developer runs `sworn induction` in a new repo. The command silently reads `go.mod` (Phase 0), prints the dependency count, then walks them through three interactive phases: design system setup (framework, location, component library), architecture pattern discovery (inspects the codebase, proposes inferred patterns the user can accept/edit/skip), and NFR stance customization (project-specific notes on security, API, data, etc.). After completion, `docs/considerations.md` is fully populated. `sworn induction --update` re-runs dependency discovery and surfaces only newly-inferred patterns after a release.

## §2. Design decisions not in spec (max 5)

1. **Phase 0 go.mod parsing: line-by-line, no module resolution** — Parse `go.mod` as a text file line-by-line (split on whitespace, extract module@version from `require` blocks). Do not shell out to `go mod download` or resolve transitive deps. Rationale: Phase 0 is about project-pinned direct deps, and `go.mod` is trivially parseable as text. This also avoids a registry network call during induction.

2. **Pattern inference: read one file from `cmd/`, `internal/`, and test dirs** — For Go repos, infer patterns by scanning `cmd/sworn/<file>.go`, `internal/<pkg>/<file>.go`, and `<pkg>_test.go` for structural signals (package naming, interface-first design, table-driven tests). This is a heuristic, not AST analysis; it surfaces "I found these patterns" with the option to edit/skip.

3. **considerations.md format: YAML frontmatter + markdown body** — S18 already defines the format; we parse the `[dependencies].project_pinned` and `architecture.patterns` sections by reading the file as text and locating marker sections. We don't use a YAML library for the frontmatter — stdlib string manipulation is sufficient for the simple list structures.

4. **Idempotent mode detection: check `design_system.location`** — If the considerations file exists and has a non-empty `design_system.location`, treat as `--update` mode (skip Phase 1, deduplicate patterns). The signal is the populated field, not file existence alone.

5. **Prompt modifications: inject new sections at specific positions** — For implementer.md, insert both "Dependency discipline" and "Deviation check" sections after "## Workflow" heading and before the existing "1. Update `status.json`" step. For verifier.md, insert the "Catalog conformance check" after the existing gates. These are structural inserts in the markdown source, detected by heading anchors.

## §3. Files I'll touch grouped by purpose

- **New induction command** — `cmd/sworn/induction.go`, `cmd/sworn/induction_test.go`: the CLI command and its tests. Self-registers via `init()` → `command.Register(...)`.
- **Prompt updates** — `internal/prompt/implementer.md`, `internal/prompt/verifier.md`: add deviation-surfacing and catalog-conformance sections to the embedded prompts.
- **Prompt tests** — `internal/prompt/prompt_test.go`: extend with `TestImplementerHasDeviationCheck`, `TestImplementerHasDependencyDiscipline`, `TestVerifierHasCatalogConformance` assertions.

## §4. Things I'm NOT doing

- **Not editing `cmd/sworn/main.go`** — the induction command self-registers via `init()` per the S51 registry pattern. This file is T15-owned.
- **Not implementing multi-language dependency parsing** — Go only per spec deferral.
- **Not implementing MCP catalog management tools** — S20 scope.
- **Not adding CI lint for catalog conformance** — post-R3 per spec.

## §5. Reachability plan

Manual smoke step: run `sworn induction` in a test repo with piped stdin (all defaults accepted); `cat docs/considerations.md` to confirm `design_system`, `architecture.patterns`, and `project_pinned` sections are non-empty. Document commands verbatim in `proof.md`.

## §6. Open questions for the Coach

*(None — the spec is clear on all scope boundaries and design decisions.)*