---
version: 1
project: <name>
design_system:
  location: ''
  framework: 'shadcn'
  component_library: ''
architecture:
  language: ''
  patterns:
  - pattern: interface-first design
    location: internal/model/client.go
    intent: enables mock injection in verify/test contexts
  - pattern: stdlib HTTP
    location: internal/model/oai_test.go
    intent: no framework dependency; cross-compiles cleanly
  - pattern: table-driven tests
    location: internal/model/oai_test.go
    intent: readable failure output; easy to add cases

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
  - module: github.com/charmbracelet/bubbles
    version: v1.0.0
    pinned_by: go.mod

  - module: github.com/charmbracelet/bubbletea
    version: v1.3.10
    pinned_by: go.mod

  - module: github.com/charmbracelet/lipgloss
    version: v1.1.0
    pinned_by: go.mod

  - module: gopkg.in/yaml.v3
    version: v3.0.1
    pinned_by: go.mod

  - module: modernc.org/sqlite
    version: v1.52.0
    pinned_by: go.mod

  - module: github.com/atotto/clipboard
    version: v0.1.4
    pinned_by: go.mod

  - module: github.com/aymanbagabas/go-osc52/v2
    version: v2.0.1
    pinned_by: go.mod

  - module: github.com/charmbracelet/colorprofile
    version: v0.4.1
    pinned_by: go.mod

  - module: github.com/charmbracelet/x/ansi
    version: v0.11.6
    pinned_by: go.mod

  - module: github.com/charmbracelet/x/cellbuf
    version: v0.0.15
    pinned_by: go.mod

  - module: github.com/charmbracelet/x/term
    version: v0.2.2
    pinned_by: go.mod

  - module: github.com/clipperhouse/displaywidth
    version: v0.9.0
    pinned_by: go.mod

  - module: github.com/clipperhouse/stringish
    version: v0.1.1
    pinned_by: go.mod

  - module: github.com/clipperhouse/uax29/v2
    version: v2.5.0
    pinned_by: go.mod

  - module: github.com/dustin/go-humanize
    version: v1.0.1
    pinned_by: go.mod

  - module: github.com/erikgeiser/coninput
    version: v0.0.0-20211004153227-1c3628e74d0f
    pinned_by: go.mod

  - module: github.com/google/uuid
    version: v1.6.0
    pinned_by: go.mod

  - module: github.com/lucasb-eyer/go-colorful
    version: v1.3.0
    pinned_by: go.mod

  - module: github.com/mattn/go-isatty
    version: v0.0.20
    pinned_by: go.mod

  - module: github.com/mattn/go-localereader
    version: v0.0.1
    pinned_by: go.mod

  - module: github.com/mattn/go-runewidth
    version: v0.0.19
    pinned_by: go.mod

  - module: github.com/muesli/ansi
    version: v0.0.0-20230316100256-276c6243b2f6
    pinned_by: go.mod

  - module: github.com/muesli/cancelreader
    version: v0.2.2
    pinned_by: go.mod

  - module: github.com/muesli/termenv
    version: v0.16.0
    pinned_by: go.mod

  - module: github.com/ncruces/go-strftime
    version: v1.0.0
    pinned_by: go.mod

  - module: github.com/remyoudompheng/bigfft
    version: v0.0.0-20230129092748-24d4a6f8daec
    pinned_by: go.mod

  - module: github.com/rivo/uniseg
    version: v0.4.7
    pinned_by: go.mod

  - module: github.com/xo/terminfo
    version: v0.0.0-20220910002029-abceb7e1c41e
    pinned_by: go.mod

  - module: golang.org/x/sys
    version: v0.42.0
    pinned_by: go.mod

  - module: golang.org/x/text
    version: v0.3.8
    pinned_by: go.mod

  - module: modernc.org/libc
    version: v1.72.3
    pinned_by: go.mod

  - module: modernc.org/mathutil
    version: v1.7.1
    pinned_by: go.mod

  - module: modernc.org/memory
    version: v1.11.0
    pinned_by: go.mod

  