---
title: 'S18-consideration-catalog — typed consideration catalog, decision registry, and planner consultation pattern'
description: 'Defines two project-level files — docs/considerations.md (typed NFR dimensions, design system location, architecture patterns) and docs/decisions.md (cross-release DRY decision registry). The embedded planner prompt gains Phase 2b: check the registry before asking anything, run design/flow/architecture consultation, capture every resolution back to the registry. sworn init scaffolds both templates.'
---

# Slice: `S18-consideration-catalog`

## User outcome

A project maintainer runs `sworn init`, opts into the catalog, and gets two files:
`docs/considerations.md` (typed dimensions + design system + architecture patterns) and
`docs/decisions.md` (empty decision registry). In every subsequent planning session the
planner checks the registry before asking any design question — if a prior answer exists
it surfaces it for confirmation rather than starting from scratch. For new UI, data, or
flow decisions, the planner presents three options with a recommendation and captures the
chosen answer to the registry so it is never asked again.

## Entry point

`sworn init` (scaffold) and every `/plan-release` or `/replan-release` session (Phase 2b
reads both files and applies them). Verifiable by: running `sworn init`; confirming both
files created; running a planning session on a fixture release; confirming the planner
surfaces prior decisions from the registry rather than re-asking.

## In scope

### `docs/considerations.md` — catalog format

YAML frontmatter plus typed dimension sections. Shipped template at
`docs/templates/considerations.md`.

Frontmatter:
```yaml
---
version: 1
project: <name>
design_system:
  location: ''          # URL, Figma link, npm package, or path — filled by sworn induction
  framework: ''         # shadcn | storybook | figma | tailwind | custom | none
  component_library: '' # e.g. @repo/ui, @radix-ui, etc.
architecture:
  language: ''
  patterns: []          # list of {pattern, location, intent} — filled by sworn induction
enabled_dimensions: [security, api, data, observability, ui, performance, compliance]
---
```

Built-in typed dimensions (each section has `required_for` and `core_checks`):

| Dimension | Default `required_for` | Focus |
|---|---|---|
| `[security]` | all | auth, injection, secrets, data exposure |
| `[api]` | api, data | rate limiting, error shapes, versioning, backward compat |
| `[data]` | data | schema migration, residency, encryption, retention |
| `[observability]` | all | structured logging (no secret leakage), metrics, tracing |
| `[ui]` | ui | WCAG 2.1 AA, keyboard nav, responsive, design system consultation |
| `[performance]` | api, ui | latency SLOs, memory ceiling, cold-start, pagination |
| `[compliance]` | data, api | GDPR data subject rights, SOC2 controls, audit log |
| `[dependencies]` | all | registry-first version selection; project pin respect; no training-data version inference |

The `[dependencies]` dimension is structurally different from the others: it is both a
**check** (are the right version selection rules being followed?) and a **record**
(what are the currently-pinned versions for this project?). Its catalog section:

```yaml
## [dependencies]
required_for: all
source_of_truth: go.mod | package.json | requirements.txt | Cargo.toml
core_checks:
  - If a library is already in the project dependency file, use that exact version — no upgrade or downgrade without explicit instruction
  - If a library is NEW to the project, query the package registry at implementation time to get the current latest stable version — never infer a version from training data
  - Record every new dep version choice in docs/decisions.md
registry_commands:
  go:     "go get <module>@latest  (then read the resolved version from go.mod)"
  npm:    "npm view <package> version"
  pip:    "pip index versions <package> 2>/dev/null | head -1"
  cargo:  "cargo search <crate> --limit 1"
project_pinned:
  # Populated automatically by sworn induction (Phase 0) from the project's dependency
  # files. Updated by sworn induction --update after each release.
  # Example entries (implementer reads this before touching any dependency file):
  # - module: github.com/anthropics/anthropic-sdk-go
  #   version: v1.2.0
  #   pinned_by: go.mod
  # - module: "@radix-ui/react-dialog"
  #   version: "^1.0.5"
  #   pinned_by: package.json
```

### `docs/decisions.md` — decision registry

Append-only project-level log of every design, architecture, and data decision made
across all releases. Shipped template at `docs/templates/decisions.md`.

Format per entry:
```markdown
## <YYYY-MM-DD> — <Short decision title>
- **Type**: design | architecture | data | flow | deviation
- **Release**: <release-name> (slice <slice-id>)
- **Decision**: <one sentence — what was chosen>
- **Rationale**: <why this option over the alternatives>
- **Applies to**: <free text — when future slices should re-use this decision>
- **Overrides**: <link to prior decision if this supersedes one>
```

### Planner Phase 2b — consideration audit

Inserted into `internal/prompt/planner.md` immediately after Phase 2 discovery,
before Phase 3 decomposition. The step has four sub-steps executed in order:

**2b-i. Registry check (DRY gate).** For each design question the planner is about to
ask: search `docs/decisions.md` for a prior answer. If found, surface it:
"In <release>, we decided <decision> because <rationale> — apply the same here?
[yes / no / yes but note a deviation]." If the user says yes, skip the question and
note the reuse. If no, generate fresh options and capture the new decision. Never ask
a question that has an existing registry answer without surfacing it first.

**2b-ii. Design consultation** (for slices introducing a new UI surface, data model
change, or user-facing flow). For each:

- *UI component*: check `design_system.location` in the catalog.
  - Affordance exists → recommend it; done.
  - Close but not quite (existing component that needs extension) → three options:
    (a) reuse as-is with workaround, (b) extend with a new variant,
    (c) build new following design system conventions. Recommend one.
  - Missing → three design options + recommendation. Additionally: flag the gap as
    a design system addition to track. Record the proposed addition as a follow-on
    item in `intake.md` (not absorbed into this slice's scope). The implementing
    slice's acceptance check states "component is built to the proposed addition spec."
  - If `design_system.location` is blank: block and ask the human to run
    `sworn induction` before proceeding.
- *Data model*: three schema options (extend existing, new entity with relation,
  denormalise for reads). Options framed as migration risk / query complexity / schema
  alignment tradeoff. Recommendation explicit.
- *User-facing flow*: three options described as user journeys ("user does X then Y")
  — not implementation topology. Recommendation explicit.

In all cases: chosen option must have a verification method (screenshot, assertion, or
smoke step) before the consultation closes.

**2b-iii. Architecture conformance.** For every slice: check `architecture.patterns`
in the catalog and the referenced source files.
- Does the proposed approach conform? If yes, note the pattern in the spec.
- If no: surface the tension — name the norm, state its intent, describe the deviation,
  and ask the human to make a conscious resolution before the spec is written.
- Is the norm itself best practice? If not, say so and propose the better approach for
  the new code. Do not retouch existing code; state the divergence explicitly as a
  conscious acceptance.

**2b-iv. Capture.** Every resolution from steps i–iii is written to `docs/decisions.md`
before the session ends. Partial sessions write partial entries; entries are never left
in conversation context only.

### `sworn init` update

After the model prompts added by S09, add:

```
Set up consideration catalog? (y/n) [y]:
  y → copy docs/templates/considerations.md → docs/considerations.md
      copy docs/templates/decisions.md      → docs/decisions.md
      print: "Templates created. Run 'sworn induction' to populate them fully."
  n → skip; print hint to run 'sworn induction' later
Overwrite guard: if either file already exists, prompt before overwriting.
```

## Out of scope

- `sworn induction` command that actively discovers and populates the catalog (S19)
- Implementer and verifier prompt updates for deviation surfacing (S19)
- MCP tools for catalog and decision registry management (S20)

## Planned touchpoints

- `docs/templates/considerations.md` (new — catalog template)
- `docs/templates/decisions.md` (new — decision registry template)
- `internal/prompt/planner.md` (modify — add Phase 2b)
- `cmd/sworn/init.go` (modify — scaffold both templates; S09 lands first)

## Acceptance checks

- [ ] `docs/templates/considerations.md` exists with all eight typed dimensions
  (including `[dependencies]`), the `design_system` frontmatter section, the
  `architecture.patterns` array, and the `project_pinned` array with commented
  example entries and `registry_commands` for Go, npm, pip, and cargo
- [ ] `docs/templates/decisions.md` exists with the documented entry format and
  three example entries (one per type: design, architecture, data)
- [ ] `sworn init` (y) creates both `docs/considerations.md` and `docs/decisions.md`;
  subsequent run prompts overwrite guard; (n) creates neither
- [ ] `internal/prompt/planner.md` contains Phase 2b with all four sub-steps;
  the phrase "Registry check" and "Design consultation" and "Architecture conformance"
  and "Capture" all appear verbatim as sub-step headings
- [ ] Phase 2b's DRY gate is explicit: "search docs/decisions.md before asking any
  design question" appears in the planner prompt
- [ ] Phase 2b's design system gap flow is explicit: missing affordance → track as
  follow-on in intake.md, not absorbed into current slice scope
- [ ] Phase 2b's architecture conformance is explicit: undocumented deviation requires
  human resolution before spec is written
- [ ] `go test ./internal/prompt/... -run Planner` passes; test asserts Phase 2b
  headings present in planner.md content
- [ ] `go build ./...` passes; no new external deps

## Required tests

- **Unit** `internal/prompt/prompt_test.go` (extend):
  - `TestPlannerHasPhase2b`: assert all four sub-step headings in `Planner()` return value
  - `TestPlannerPhase2bDRYGate`: assert phrase "docs/decisions.md" in planner prompt
- **Unit** `cmd/sworn/init_test.go` (extend):
  - `TestInitCreatesBothTemplates`: y → both files created
  - `TestInitSkipsBoth`: n → neither created
  - `TestInitOverwriteGuard`: both files exist; prompt overwrite; answer n; files unchanged
- **Reachability artefact**: `sworn init` smoke step — run with y; confirm both files
  created; cat first 5 lines of each; confirm frontmatter present.

## Risks

- Phase 2b in planner.md must keep the "file not found → one note, don't block" branch
  as the fast path for projects without a catalog. Heavy blocking on missing files would
  make sworn unusable without a fully configured catalog.
- The three-option consultation adds session length. The acceptance check for "chosen
  option must have a verification method" must not create an infinite loop where the
  planner refuses to proceed; it should warn once and continue if the human accepts the
  gap.

## Deferrals allowed?

No. S19 (induction command) depends on the catalog format defined here. S20 (MCP tools)
depends on the decisions.md format defined here.
