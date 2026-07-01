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

# Project Considerations

This file drives the planner's Phase 2b consideration audit. Every dimension is
checked against its `required_for` tags before a slice is decomposed. The planner
consults this catalog before asking any design question.

## [security]

required_for: all
core_checks:
  - Authentication and authorisation model matches project norms
  - No secrets, tokens, or keys in logs, error messages, or client-visible payloads
  - Input validation gates on every externally-facing entry point
  - SQL injection / XSS / CSRF surfaces assessed for the chosen stack
  - Data exposure risk evaluated for every new response shape and query

## [api]

required_for: api, data
core_checks:
  - Error response shapes are consistent with the project's envelope convention
  - Rate limiting or throttling considered for any new endpoint
  - Versioning strategy (header, path, none) matches project norm
  - Backward compatibility of existing callers assessed
  - Idempotency considered for mutating endpoints (PUT/PATCH/DELETE)

## [data]

required_for: data
core_checks:
  - Schema migration path is explicit (additive-only? backward-compatible? downtime?)
  - Data residency requirements satisfied (region, sovereignty)
  - Encryption at rest and in transit confirmed
  - Retention and deletion policy aligned with compliance requirements
  - New indexes assessed for write-path impact

## [observability]

required_for: all
core_checks:
  - Structured logging on every new decision path (no secret leakage)
  - Metrics exposed for key operations (latency, error rate, throughput)
  - Tracing context propagated across service boundaries where applicable
  - Alert thresholds defined for new error conditions

## [ui]

required_for: ui
core_checks:
  - WCAG 2.1 AA conformance: colour contrast, keyboard navigation, screen-reader labels
  - Responsive behaviour defined for mobile, tablet, and desktop breakpoints
  - Design system consultation: new components vs. existing library
  - Loading, empty, error, and edge-case states handled
  - Focus management and tab order correct for keyboard-only users

## [performance]

required_for: api, ui
core_checks:
  - Latency SLO defined for the new path (p50/p95/p99)
  - Memory ceiling assessed — no unbounded collections or caches
  - Cold-start or first-request latency considered
  - Pagination applied to any unbounded list response
  - N+1 query risk assessed for any new data access pattern

## [compliance]

required_for: data, api
core_checks:
  - GDPR data subject rights: access, rectification, erasure, portability
  - SOC2 controls: audit log coverage, change management, access review
  - Data processing agreement scope unchanged by new data collection
  - Cookie / tracking consent updated if new client-side storage introduced

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