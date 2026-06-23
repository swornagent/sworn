# Design TL;DR: S18-consideration-catalog

## §1. User-visible change

Running `sworn init` on a fresh repo (or a repo without catalog files) now asks
"Set up consideration catalog? (y/n) [y]:" after the existing agent-config and
model-prompts steps. Answering `y` copies two template files from
`docs/templates/` into `docs/` — `considerations.md` (typed NFR dimensions,
design system location, architecture patterns, dependency-pin registry) and
`decisions.md` (empty decision registry). Answering `n` skips both. If either
file already exists, the prompt surfaces an overwrite guard rather than silently
clobbering.

The embedded planner prompt inside the `sworn` binary gains a new Phase 2b
(consideration audit) that runs after discovery and before decomposition. In
every subsequent planning session the planner checks `docs/decisions.md` before
asking any design question, runs design/architecture/flow consultations with
three-option recommendations, and captures every chosen answer to the registry
so it is never asked again.

## §2. Design decisions not in spec (max 5)

1. **Catalog prompt placement in init.go.** The catalog prompt is inserted after
   the implementer-model prompt (S09's contribution) and before the final "Done."
   line. It uses the same `bufio.NewReader(os.Stdin)` interactive-read pattern as
   the design-system and implementer-model prompts — a separate interactive
   sub-prompt, not integrated into the scan/confirm/apply change-list pattern.
   Rationale: the catalog is optional (y/n), not a structured change-list item;
   treating it like the other interactive sub-prompts keeps the UX consistent.

2. **Planner Phase 2b insertion point.** Phase 2b is inserted into
   `internal/prompt/planner.md` after the last paragraph of Phase 2 (the
   schema-vs-spec audit note) and before Phase 3's heading. This is the natural
   seam — discovery is complete, decomposition hasn't started, and the planner
   has all the information it needs to consult the catalog and registry.
   Rationale: inserting mid-phase would break the planner's cognitive flow;
   inserting after Phase 3 would make registry-check a post-hoc exercise.

3. **Templates are raw markdown, not Go templates.** `docs/templates/considerations.md`
   and `docs/templates/decisions.md` are plain markdown files copied verbatim by
   `sworn init` (via `os.ReadFile` + `os.WriteFile`). They are not processed
   through `text/template` or `html/template`. Rationale: the spec does not
   require interpolation; the templates ship as static reference files that
   `sworn induction` (S19) will later populate. Adding a template engine now
   would be speculative complexity.

4. **Overwrite guard uses the same interactive-read pattern.** When either target
   file exists, the prompt becomes "File exists — overwrite? [y/N]:" (defaulting
   no). This matches the existing `--force`/overwrite guard conventions in
   init.go rather than the scan-phase change-list pattern. Rationale: the catalog
   files are not tracked in the config or agent-splice systems, so they don't
   fit the existing `planned`/`informational` change-list model.

5. **Phase 2b sub-step headings use the exact strings from the acceptance checks.**
   The four sub-steps are headed with literal markdown headings: "2b-i. Registry
   check (DRY gate)", "2b-ii. Design consultation", "2b-iii. Architecture
   conformance", "2b-iv. Capture". Rationale: the acceptance checks grep for
   exact phrases; a heading that paraphrases ("Check the Registry" vs. "Registry
   check") breaks the grep-based test.

## §3. Files I'll touch grouped by purpose

**New template files (static reference docs):**
- `docs/templates/considerations.md` — full catalog template with YAML
  frontmatter, all eight typed dimensions (security, api, data, observability,
  ui, performance, compliance, dependencies), design_system section,
  architecture.patterns array, and project_pinned array with commented examples
- `docs/templates/decisions.md` — empty decision registry with documented entry
  format and three example entries (design, architecture, data)

**Planner prompt update (planner behaviour):**
- `internal/prompt/planner.md` — insert Phase 2b after the schema-vs-spec audit
  paragraph and before Phase 3. The insert is a clean block with no edits to
  existing content.

**CLI entry point (user-facing affordance):**
- `cmd/sworn/init.go` — add catalog prompt after the implementer-model prompt
  block (S09's territory). Include: prompt for y/n, copy logic for both template
  files, overwrite guard when files exist, skip path for `n`.

**Test files (Rule 1 reachability):**
- `internal/prompt/prompt_test.go` — add `TestPlannerHasPhase2b` and
  `TestPlannerPhase2bDRYGate`; both read `Planner()` and assert on
  grep-able substrings
- `cmd/sworn/init_test.go` (new) — add `TestInitCreatesBothTemplates`,
  `TestInitSkipsBoth`, `TestInitOverwriteGuard`; each runs `cmdInit([]string{})`
  with controlled stdin and checks filesystem side effects

## §4. Things I'm NOT doing

- `sworn induction` command — that's S19
- Implementer/verifier prompt updates for deviation surfacing — S19
- MCP tools for catalog and decision registry management — S20
- Any RAG-backed NFR source integration — declared deferral in status.json
- Any guided NFR elicitation wizard — declared deferral in status.json
- Auto-populating the catalog from the project's dependency files — that's
  `sworn induction` (S19)

## §5. Reachability plan

Integration point: `sworn init` CLI command.
- **Primary artefact:** `sworn init` smoke step — run `cmdInit([]string{"-yes"})`
  with a temp directory; assert `docs/considerations.md` and `docs/decisions.md`
  both created; cat first 5 lines of each to confirm frontmatter present.
- **Unit tests (prompt):** `go test ./internal/prompt/... -run Planner` —
  `TestPlannerHasPhase2b` asserts all four sub-step headings in `Planner()`;
  `TestPlannerPhase2bDRYGate` asserts "docs/decisions.md" appears.
- **Unit tests (cmd):** `go test ./cmd/sworn/... -run Catalog` — three test
  cases covering y-creates-both, n-creates-neither, overwrite-guard-works.
- **Build:** `go build ./...` passes with no new external dependencies.

## §6. Open questions for the Coach

*(none)*