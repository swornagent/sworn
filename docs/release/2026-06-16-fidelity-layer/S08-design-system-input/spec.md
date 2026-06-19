---
title: 'S08-design-system-input'
description: 'Declare a design system (design tokens + component library) as a first-class project input, so design fidelity has a source of truth to conform to. UI-bearing projects only; CLI projects declare none.'
---

# Slice: `S08-design-system-input`

> Rule 9's anti-drift foundation. With no documented design system, AI-generated UI drifts —
> button colours diverge, stray borders reappear, each component an unapproved extension to the
> house. This slice makes the design system a declared, first-class project input. T3;
> depends_on T1. Extends the `09-design-fidelity.md` rule doc created by S07.

## User outcome

When a maintainer of a UI-bearing project declares a design system in project config (the
design-token source + the component-library location), `sworn` reads it as the source of truth
for design conformance (S09). `sworn` **fails closed** if a project marked UI-bearing declares
no design system; a CLI project explicitly declares none and is exempt.

## Entry point

- **Native:** project config (`internal/config/`) gains a `design_system` declaration
  (token source + component-library location + a `ui_bearing` flag); surfaced via `sworn init`
  and read by `sworn designaudit` (S09).
- **Protocol:** `internal/adopt/baton/rules/09-design-fidelity.md` documents the design-system
  input (umbrella = design system; atoms = design tokens; reusables = component library).

## In scope

- **Config schema** (`internal/config/config.go`): `design_system` with token source (a
  W3C-DTCG-style tokens file/source), component-library location, and a `ui_bearing` flag.
- **Presence rule**: a `ui_bearing: true` project with no `design_system` fails closed; a
  `ui_bearing: false` project (e.g. sworn itself) is exempt.
- **`sworn init` prompt**: when initialising a UI-bearing project, prompt for the design-system
  declaration.

## Out of scope

- **The conformance audit** (no hardcoded hex, token-scale spacing) — S09 (consumes this).
- **The design-fit decision gate** — S07 (medium-agnostic; this is the visual sub-dimension).
- **Authoring the tokens / building the component library** — project work, not sworn's.

## Planned touchpoints

- `internal/config/config.go`, `internal/config/config_test.go` (the `design_system` schema)
- `internal/config/init.go` (init prompt for the declaration)
- `internal/adopt/baton/rules/09-design-fidelity.md` (design-system-input section)

## Acceptance checks

- [ ] WHEN a project declares `ui_bearing: true` with no `design_system`, THE SYSTEM SHALL fail
      closed and state that a design system is required for design conformance.
- [ ] WHEN a project declares `ui_bearing: false`, THE SYSTEM SHALL treat the design system as
      not applicable and not require it (CLI projects exempt).
- [ ] WHEN a UI-bearing project declares a `design_system` (token source + component library),
      THE SYSTEM SHALL parse it and expose it for the conformance audit (S09).
- [ ] THE SYSTEM SHALL distinguish the three concepts in the schema: design system (umbrella),
      design tokens (the named-value source of truth), component library (the coded reusables).

## Required tests

- **Unit**: `internal/config/config_test.go` — ui_bearing-without-design-system fails;
  ui_bearing:false exempt; a valid declaration parses + exposes token source + component library.
- **Integration**: `sworn init` on a fixture UI-bearing project prompts for + records the
  declaration (Rule 1 via the init entry point).
- **Reachability artefact**: smoke step — "init a fixture UI-bearing project without a design
  system; observe the failure; declare one; observe it parsed + exposed."
- **E2E gate type**: `local`.
- **playwright-screenshot**: N/A — sworn is a CLI tool; no browser interaction.

## Risks

- **Token-format diversity** — projects use different token formats (DTCG JSON, CSS vars, JS
  themes). Mitigate: declare a *source location* + format hint; S09's audit adapts; do not
  mandate one token format.
- **Scope into S09** — S08 only *declares + parses* the design system; *auditing source against
  it* is S09. Keep the seam clean.

## Deferrals allowed?

No.
