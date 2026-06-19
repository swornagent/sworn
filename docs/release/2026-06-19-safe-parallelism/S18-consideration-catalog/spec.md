---
title: 'S18-consideration-catalog — typed consideration catalog for requirements elicitation and solution design'
description: 'A project-level consideration catalog (docs/considerations.md) defines typed dimensions — security, api, data, observability, ui, performance, compliance — that the planner must cross-reference against every slice spec. sworn init scaffolds a starter catalog from a shipped template. The embedded planner prompt gains a mandatory Phase 2b step to consult the catalog and surface uncovered dimensions as acceptance checks or explicit N/A notes.'
---

# Slice: `S18-consideration-catalog`

## User outcome

A project maintainer runs `sworn init` and is prompted to set up a consideration catalog;
they choose yes and get a typed starter at `docs/considerations.md`. In every subsequent
`/plan-release` or `/replan-release` session, the planner automatically checks each
slice against the enabled dimensions — a UI slice gets accessibility checks, an API slice
gets rate-limiting and backward-compat checks, a data slice gets migration and residency
checks — and either adds a falsifiable acceptance check or writes an explicit one-liner
N/A justification. Nothing falls through silently.

## Entry point

`sworn init` (catalog setup) and any `/plan-release` or `/replan-release` session
(the planner reads the catalog during Phase 2b and applies it in Phase 4 spec writing).
Verifiable by: running `sworn init`, confirming `docs/considerations.md` is created;
then running `/plan-release` on a scratch release, confirming the planner's spec output
includes dimension-sourced acceptance checks or explicit N/A notes.

## In scope

### Catalog format — `docs/considerations.md`

A Markdown file with YAML frontmatter and typed dimension sections. The planner reads it
as a document; no binary parsing required.

Frontmatter:
```yaml
---
version: 1
project: <name>
enabled_dimensions: [security, api, data, observability, ui, performance, compliance]
---
```

Each dimension section follows the pattern:
```markdown
## [security]
**Required for**: all slices
**Checks to apply**:
- Auth/authz: does this slice expose or consume an authenticated surface?
- Injection: are user-supplied strings ever interpolated into commands, queries, or paths?
- Secrets: are any credentials, tokens, or keys handled? Are they masked in logs?
- Data exposure: can this slice inadvertently leak PII or credentials in error messages?
**When N/A**: pure refactor with no new I/O surface and no credential handling.
```

Built-in dimension set (shipped in the template, all enabled by default, all editable):

| Dimension | `required_for` default | Core questions |
|---|---|---|
| `[security]` | all | auth, injection, secrets, data exposure |
| `[api]` | api, data | rate limiting, error shapes, versioning, backward compat |
| `[data]` | data | schema migration, data residency, encryption at rest, retention |
| `[observability]` | all | logging (no key leakage), metrics, tracing, alerting |
| `[ui]` | ui | WCAG 2.1 AA, keyboard nav, responsive, screen reader |
| `[performance]` | api, ui | latency SLOs, memory ceiling, cold-start, pagination |
| `[compliance]` | data, api | GDPR data subject rights, SOC2 controls, audit log |

The developer adds or removes sections, edits `required_for`, and adds project-specific
notes. The planner applies the enabled sections; disabled sections are skipped.

### Planner prompt update — `internal/prompt/planner.md`

Add **Phase 2b — Consideration catalog audit** immediately after Phase 2 discovery and
before Phase 3 decomposition:

```
### Phase 2b — Consideration catalog audit

If `docs/considerations.md` exists in the project root:

1. Read it.
2. For each enabled dimension, determine whether it applies to the work being planned
   based on its `required_for` field and the nature of the slices being discussed.
3. For each applicable dimension, either:
   a. Add a falsifiable acceptance check to the relevant slice's spec (Phase 4), or
   b. Write an explicit one-liner N/A justification in the spec's "Out of scope" section.
   "N/A" without a reason is not acceptable — it is a silent deferral.
4. If `docs/considerations.md` does not exist, note it once to the human:
   "No consideration catalog found at docs/considerations.md. Run 'sworn init' to
   scaffold one, or proceed without it." Do not block planning.
```

### `sworn init` update — `cmd/sworn/init.go`

After the existing model prompts (added by S09), add:

```
Set up a consideration catalog? (y/n) [y]:
  → y: copy docs/templates/considerations.md → docs/considerations.md
        print: "Catalog created at docs/considerations.md — edit dimensions to fit your project."
  → n: skip; print: "You can run 'sworn init --considerations' later to set one up."
```

### Template file — `docs/templates/considerations.md`

The canonical template shipped in the sworn repo; `sworn init` copies it verbatim to
the project's `docs/considerations.md`. Contains all seven built-in dimensions with
their default `required_for` settings, example `required_for` overrides, and
instructional comments explaining how to add project-specific dimensions.

## Out of scope

- RAG-backed NFR discovery: connecting to external knowledge sources (internal wikis,
  regulatory document stores, third-party NFR databases) to auto-suggest considerations.
  (Deferred — see Rule 2 card below.)
- Guided NFR elicitation flow when no catalog exists: interactive wizard that asks about
  the project's domain, scale, and compliance profile to derive a starter catalog.
  (Deferred — see Rule 2 card below.)
- Per-slice consideration overrides (overriding `required_for` at the individual slice
  level): post-R3.
- Automated CI enforcement of catalog coverage (a script that checks every spec for
  dimension coverage): post-R3.
- MCP tool for catalog management (`sworn.update_considerations`): post-R3.

### Rule 2 deferral — RAG-backed NFR sources and guided elicitation

- **What**: (1) A guided wizard that, when no catalog exists, asks the developer about
  their project's domain, compliance environment, and scale to synthesise a tailored
  starter catalog. (2) RAG sources — the ability to point sworn at internal wikis,
  regulatory document stores, or standard NFR libraries (ISO 25010, OWASP ASVS) so the
  planner can auto-suggest dimension content and checks grounded in authoritative sources.
- **Why deferred**: The guided wizard needs a conversational design session to get right
  (asking the wrong questions produces a worse catalog than the template). RAG integration
  requires a retrieval pipeline and connectivity design (file system, HTTP, MCP resource
  server?) that is its own release scope.
- **Tracking**: post-R3 issue — "Guided NFR elicitation + RAG-backed consideration
  catalog". Acknowledged: 2026-06-20 planning session.

## Planned touchpoints

- `docs/templates/considerations.md` (new — shipped template)
- `internal/prompt/planner.md` (modify — add Phase 2b consideration audit step)
- `cmd/sworn/init.go` (modify — add catalog setup prompt; serialised by S09 in T3)

## Acceptance checks

- [ ] `docs/templates/considerations.md` exists in the repo with all seven built-in
  dimensions, each with `required_for`, at least three core questions, and an example
  N/A condition
- [ ] `sworn init` (with "y" to catalog prompt) creates `docs/considerations.md` as a
  verbatim copy of the template; subsequent `sworn init` with file already present
  prompts "catalog already exists, overwrite? (y/n)" rather than silently overwriting
- [ ] `sworn init` (with "n" to catalog prompt) does not create `docs/considerations.md`
  and prints the "run later" hint
- [ ] `internal/prompt/planner.md` contains the Phase 2b section describing the
  catalog audit step; the phrase "docs/considerations.md" appears verbatim
- [ ] A `/plan-release` session on a project with a catalog containing `[security]`
  (required_for: all) produces specs that either include a security-sourced acceptance
  check or an explicit one-liner N/A justification — verified by running a test
  planning session on a fixture release (documented in proof.md)
- [ ] `go build ./...` passes with no new external deps

## Required tests

- **Unit** `cmd/sworn/init_test.go` (extend existing or new):
  - `TestInitCreatesCatalog`: run init with catalog prompt answered "y" against a temp
    dir; assert `docs/considerations.md` created and is non-empty
  - `TestInitSkipsCatalog`: run init with "n"; assert file not created
  - `TestInitCatalogOverwritePrompt`: run init twice with "y" both times; second run
    prompts overwrite; answer "n"; file unchanged
- **Planner prompt test** `internal/prompt/prompt_test.go` (extend):
  - `TestPlannerPromptContainsCatalogStep`: `Planner()` return value contains the string
    "Phase 2b" and "docs/considerations.md"
- **Reachability artefact**: smoke step — run `sworn init` in a temp dir; confirm
  `docs/considerations.md` created; cat the first 10 lines; confirm `[security]` present.
  Document in proof.md as explicit smoke step with exact commands.

## Risks

- `internal/prompt/planner.md` is embedded in the binary. Editing it changes the
  planner's behaviour globally for all users of the binary. The Phase 2b addition must
  be written carefully — it must not make planning sessions significantly slower for
  projects without a catalog (the "file not found → skip with one note" branch must
  be the fast path).
- The template shipped with the binary is a point-in-time default. Projects customise
  their own `docs/considerations.md`; sworn updates do not overwrite existing catalogs.
  `sworn init --considerations` is the explicit refresh path.

## Deferrals allowed?

The RAG and guided wizard deferral is explicitly Rule 2 above. No other deferrals.
