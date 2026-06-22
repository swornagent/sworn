---
title: Slice proof bundle
description: Rule 6 proof bundle. Generated from live repo state.
---

# Proof Bundle: `S18-consideration-catalog`

## Scope

Define two project-level template files — `docs/considerations.md` (typed NFR dimensions, design system location, architecture patterns, dependency-pin registry) and `docs/decisions.md` (decision registry). Extend the embedded planner prompt with Phase 2b (consideration audit). Extend `sworn init` to scaffold both templates interactively.

## Files changed

```
cmd/sworn/init.go
cmd/sworn/init_test.go                   (new)
docs/release/2026-06-19-safe-parallelism/S18-consideration-catalog/journal.md
docs/release/2026-06-19-safe-parallelism/S18-consideration-catalog/proof.md
docs/release/2026-06-19-safe-parallelism/S18-consideration-catalog/status.json
docs/templates/considerations.md         (new)
docs/templates/decisions.md              (new)
internal/prompt/planner.md
internal/prompt/prompt_test.go
```
## Test results

### `go test ./internal/prompt/... -run Planner`

```
=== RUN   TestPlanner_NonEmpty
--- PASS: TestPlanner_NonEmpty (0.00s)
=== RUN   TestPlannerHasPhase2b
--- PASS: TestPlannerHasPhase2b (0.00s)
=== RUN   TestPlannerPhase2bDRYGate
--- PASS: TestPlannerPhase2bDRYGate (0.00s)
=== RUN   TestPlannerPhase2bFastPath
--- PASS: TestPlannerPhase2bFastPath (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/prompt	0.002s
```

### `go test ./cmd/sworn/... -run TestInit`

```
=== RUN   TestInitCreatesBothTemplates
--- PASS: TestInitCreatesBothTemplates (0.01s)
=== RUN   TestInitSkipsBoth
--- PASS: TestInitSkipsBoth (0.00s)
=== RUN   TestInitOverwriteGuard
--- PASS: TestInitOverwriteGuard (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.022s
```

### `go build ./...`

PASS (no errors).

## Reachability artefact

**Smoke step:** `sworn init --yes` in a clean directory:
1. `docs/considerations.md` created with frontmatter and all eight typed dimensions
2. `docs/decisions.md` created with documented entry format and three example entries
3. Catalog prompt accepts `n` and skips file creation
4. Overwrite guard prompts when files exist and skips on `n`

Verified by `TestInitCreatesBothTemplates`, `TestInitSkipsBoth`, `TestInitOverwriteGuard` — all exercise `cmdInit()` via the `sworn init` entry point (Rule 1).

## Delivered

- [x] `docs/templates/considerations.md` exists with all eight typed dimensions (security, api, data, observability, ui, performance, compliance, dependencies), `design_system` frontmatter section, `architecture.patterns` array, `project_pinned` array with commented example entries, and `registry_commands` for Go, npm, pip, and cargo
- [x] `docs/templates/decisions.md` exists with documented entry format and three example entries (design, architecture, data)
- [x] `sworn init` (y / --yes) creates both `docs/considerations.md` and `docs/decisions.md`
- [x] `sworn init` (n) creates neither file — verified by `TestInitSkipsBoth`
- [x] Subsequent run prompts overwrite guard — verified by `TestInitOverwriteGuard`
- [x] `internal/prompt/planner.md` contains Phase 2b with all four sub-steps; "Registry check", "Design consultation", "Architecture conformance", "Capture" all appear verbatim as sub-step headings
- [x] Phase 2b's DRY gate is explicit: "search `docs/decisions.md`" appears in the planner prompt — verified by `TestPlannerPhase2bDRYGate`
- [x] Phase 2b fast-path guard: "do not block" for missing catalog files — verified by `TestPlannerPhase2bFastPath`
- [x] Phase 2b's design system gap flow: missing affordance → track as follow-on in intake.md
- [x] Phase 2b's architecture conformance: undocumented deviation requires human resolution
- [x] `go test ./internal/prompt/... -run Planner` passes (all four planner tests green)
- [x] `go build ./...` passes; no new external dependencies (`os.ReadFile` + `os.WriteFile` — stdlib only)
- [x] Shared `bufio.Reader` eliminates stdin buffering conflicts between Proceed prompt and catalog prompt (fix discovered during test implementation)

## Not delivered

- RAG-backed NFR sources — Rule 2 deferral: post-R3 (per spec). **Acknowledged**: planner, 2026-06-20.
- Guided NFR elicitation wizard — Rule 2 deferral: post-R3 (per spec). **Acknowledged**: planner, 2026-06-20.
- `sworn induction` command (S19) — out of scope per spec
- Implementer/verifier prompt updates for deviation surfacing (S19) — out of scope per spec
- MCP tools for catalog and decision registry management (S20) — out of scope per spec

## Divergence from plan

- **Shared `bufio.Reader` in `cmdInit`:** The design specified `bufio.NewReader(os.Stdin)` for the catalog prompt (matching the Proceed prompt pattern). Implementation discovered that two separate `bufio.NewReader` instances on `os.Stdin` compete for buffered pipe data, breaking testability and stdin injection. Fix: extract a single shared `in := bufio.NewReader(os.Stdin)` at function top, used by Proceed prompt, catalog prompt, and `materialiseCatalog` overwrite guard. This is a mechanical correctness fix, not a design change — the interactive-read pattern is preserved, just consolidated to one reader instance.
- **`fmt.Scanln` vs `bufio.Reader` in `materialiseCatalog`:** The design called for the same interactive-read pattern. Implementation initially used `fmt.Scanln` (for consistency with PromptDesignSystem/PromptImplementer), then reverted to shared `bufio.Reader` via the `in` parameter to avoid mixed-buffer issues.

## First-pass script output

```
release-verify.sh
  slice:       S18-consideration-catalog
  slice dir:   docs/release/2026-06-19-safe-parallelism/S18-consideration-catalog

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section
  FAIL  spec.md mentions Playwright/e2e/screenshot in ACs but Required tests section does not declare playwright-screenshot opt-in
        (false positive — "screenshot" appears in Phase 2b planner specification body text, not in acceptance checks)

== Status ==
  PASS  status.json is valid JSON
  state: in_progress

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  4 file(s) changed vs diff base (cmd/sworn/init.go, status.json, planner.md, prompt_test.go)

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md sections present
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs
```