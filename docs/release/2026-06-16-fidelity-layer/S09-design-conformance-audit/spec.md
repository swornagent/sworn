---
title: 'S09-design-conformance-audit'
description: 'Design-system conformance audit: a deterministic first-pass (no hardcoded hex, spacing/borders on the token scale, reuse-over-recreate) plus a human judgement pass. The visual analog of the schema-vs-spec audit.'
---

# Slice: `S09-design-conformance-audit`

> Rule 9's anti-drift enforcement. Two layers mirroring the gate's existing shape: a
> deterministic machine-check + a human cohesion judgement — the visual analog of Baton's
> schema-vs-spec audit. T3; depends_on T1; consumes S08's declared design system.

## User outcome

When a maintainer runs `sworn designaudit <project>`, sworn scans the UI source against the
declared design system (S08) and **fails closed** on machine-detectable drift — hardcoded hex
colours, spacing/border values off the token scale, or a recreated component that duplicates a
library one — naming each violation with its file + line. A human cohesion judgement
("does it feel on-brand") is recorded alongside, human-owned.

## Entry point

- **Native:** `sworn designaudit <project>` (additive `case "designaudit"`; implementation in
  `cmd/sworn/designaudit.go`) wrapping `internal/designaudit/`. A `bin/design-audit.sh` exposes
  the deterministic pass for first-pass / CI use.
- **Protocol:** `internal/adopt/baton/rules/09-design-fidelity.md` documents the two-layer
  conformance audit.

## In scope

- **Deterministic first-pass** (`internal/designaudit/`): against the S08 design system, flag
  (a) hardcoded hex / colour literals not sourced from tokens, (b) spacing / border-width values
  off the declared token scale, (c) recreated components that duplicate a component-library
  entry (reuse-over-recreate). Each violation named with file + line.
- **Human judgement record**: a recorded human cohesion verdict ("on-brand / coheres with the
  whole") — model-surfaced considerations, human decision.
- **Fail-closed**: machine violations fail the audit; the human verdict is required to be
  present (not auto-set).

## Out of scope

- **Declaring the design system** — S08 (consumed here).
- **The medium-agnostic design-fit decision** — S07.
- **Non-UI projects** — a `ui_bearing: false` project is not audited (exempt via S08).

## Planned touchpoints

- `internal/designaudit/designaudit.go`, `internal/designaudit/designaudit_test.go` (new)
- `cmd/sworn/designaudit.go` (new command)
- `cmd/sworn/main.go` (additive `case "designaudit"`)
- `bin/design-audit.sh` (new — first-pass wrapper)
- `internal/adopt/baton/rules/09-design-fidelity.md` (conformance-audit section)

## Acceptance checks

- [ ] WHEN UI source contains a hardcoded hex colour not sourced from the declared tokens, THE
      SYSTEM SHALL exit non-zero from `sworn designaudit <project>` and name the file + line.
- [ ] WHEN a spacing or border value is off the declared token scale, THE SYSTEM SHALL flag it
      with its file + line.
- [ ] WHEN a component duplicates a component-library entry instead of reusing it, THE SYSTEM
      SHALL flag the recreation.
- [ ] WHEN the deterministic pass is clean AND a human cohesion verdict is recorded, THE SYSTEM
      SHALL exit 0.
- [ ] THE SYSTEM SHALL require the human cohesion verdict to be human-set; it SHALL NOT
      auto-pass the cohesion judgement.

## Required tests

- **Unit**: `internal/designaudit/designaudit_test.go` — hardcoded hex flagged; off-scale
  spacing flagged; recreated component flagged; clean source + human verdict passes; missing
  human verdict blocks.
- **Integration**: `sworn designaudit <fixture-project>` + `bin/design-audit.sh` against a
  fixture with seeded drift (Rule 1).
- **Reachability artefact**: smoke step — "run `sworn designaudit <fixture>` with a hardcoded
  hex; observe the named violation + non-zero exit; replace it with a token reference; record
  the human verdict; observe pass."
- **E2E gate type**: `local`.
- **playwright-screenshot** not applicable — gate type is `local`; integration tests run as
  `go test ./cmd/sworn/ -run TestDesignaudit` (no browser, no Playwright).

## Risks

- **Token-format adaptation** — the audit must resolve "is this value on the token scale" across
  token formats (S08 risk). Mitigate: drive off S08's declared source/format; start with the
  common cases (hex, px/rem spacing, border-width); report unresolved cases rather than
  silently passing.
- **False positives on intentional one-offs** — provide an explicit inline allow mechanism
  (declared exception) so the human can sanction a deliberate deviation rather than the gate
  blocking forever.

## Deferrals allowed?

No.
