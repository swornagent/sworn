---
title: 'S19-sworn-induction — one-time repo onboarding command and deviation surfacing in all three role prompts'
description: 'sworn induction runs once per repo to discover and ratify the design system, architecture patterns, and NFR stances, fully populating docs/considerations.md. sworn induction --update ratifies new patterns after a release. The implementer and verifier prompts gain explicit deviation-surfacing steps: undocumented deviation from the catalog is BLOCKED for the implementer and FAIL for the verifier.'
---

# Slice: `S19-sworn-induction`

## User outcome

A developer runs `sworn induction` in a new repo (or from their AI interface via
`sworn mcp`); the command walks them through design system discovery, architecture
pattern ratification, and NFR stance setup, producing a fully-populated
`docs/considerations.md`. After a release, `sworn induction --update` lets them ratify
any new patterns that emerged. When an implementer or verifier hits a pattern deviation,
they surface it to the human explicitly — it is never a silent judgment call.

## Entry point

`sworn induction` CLI command. Also callable via `sworn mcp` (the AI calls the catalog
management MCP tools from S20 to achieve the same flow conversationally). Verifiable by
running `sworn induction` on a test repo and confirming `docs/considerations.md` gains
populated `design_system` and `architecture.patterns` sections.

## In scope

### `sworn induction` command — `cmd/sworn/induction.go`

Interactive terminal session with three phases:

**Phase 1 — Design system discovery**
```
Do you have a design system? (y/n) [y]:
  y → What framework? [shadcn / storybook / figma / tailwind / custom]: ___
      Where is it? (URL, path, or npm package): ___
      Component library package (e.g. @repo/ui, leave blank if none): ___
      → writes design_system section to docs/considerations.md
  n → sets design_system.framework = "none"
```

**Phase 2 — Architecture pattern discovery**

Reads the project's most structurally representative files (inferred from language in
catalog or auto-detected: for Go, reads `go.mod`, one file each from `cmd/`, `internal/`,
test files). Proposes inferred patterns in the form `{pattern, location, intent}`:

```
I found these patterns in your codebase:
  [1] interface-first design — internal/model/client.go
      "enables mock injection in verify/test contexts"
  [2] stdlib HTTP — internal/model/oai.go
      "no framework dependency; cross-compiles cleanly"
  [3] table-driven tests — internal/config/config_test.go
      "readable failure output; easy to add cases"

Accept all? (y) / Edit individually (e) / Add more (a) / Skip (s):
```

For each accepted pattern, writes to `architecture.patterns` in `docs/considerations.md`.
For 'add more': prompts for `pattern`, `location`, `intent` free-form.
'Skip' leaves `architecture.patterns` empty (fast path for users who will fill it later).

**Phase 3 — NFR stance setup**

For each enabled dimension in `docs/considerations.md`:
```
[security] — required_for: all
  Customise? Add project-specific notes? (press Enter to keep default, or type notes):
  > We handle EU user data; all slices must consider GDPR data subject rights.
```
Writes notes back to the dimension section.

**`sworn induction --update`** — skips Phase 1 (unless `--design-system` flag passed);
re-runs Phase 2 showing only NEW patterns not already in the catalog; re-runs Phase 3
for dimensions that now have new notes from the completed release.

Idempotent: running `sworn induction` when a catalog already exists goes to
`--update` mode automatically with a notice.

### Implementer prompt update — `internal/prompt/implementer.md`

Add a **Deviation check** step immediately before the implementer begins writing code:

```
### Deviation check

Before writing any production code:

1. Read docs/considerations.md (if it exists).
2. For each architecture pattern in the catalog: does your planned implementation
   conform? If not:
   a. Stop. Do not write the deviating code.
   b. Record the deviation in journal.md under "Deferrals surfaced":
      "DEVIATION: <pattern> — <why it cannot be followed> — awaiting human resolution"
   c. Set slice state to BLOCKED in status.json.
   d. Surface to the human via paging (S07) or direct message.
   e. Do not proceed until the human has made a conscious resolution and it is
      captured in docs/decisions.md.
3. If the catalog does not exist, proceed without this check and note its absence
   in journal.md.
```

### Verifier prompt update — `internal/prompt/verifier.md`

Add a **Catalog conformance check** to the verifier's gate list:

```
### Catalog conformance check

If docs/considerations.md exists in the repo:

1. Read docs/decisions.md. Check whether any decision entries for this slice
   document a deliberate deviation from the catalog's architecture patterns.
2. Inspect the implementation diff. Does it deviate from any pattern in
   architecture.patterns without a corresponding entry in docs/decisions.md?
   - If yes: this is a FAIL. Violation: "undocumented deviation from <pattern> —
     see docs/considerations.md architecture.patterns[N]. Either the implementation
     must conform or a deviation must be recorded in docs/decisions.md with human
     acknowledgement."
3. Check that design system affordances used in UI slices are either from the
   registered design system or have a documented gap entry in intake.md.
   - If a UI component was built without checking the design system and no gap is
     documented: FAIL.
4. If docs/considerations.md does not exist, skip this check (not all projects have
   a catalog) and note its absence in the verdict.
```

## Out of scope

- MCP tool implementations for catalog management (S20)
- The consideration catalog format itself (S18 defines it; S19 populates it)
- Automated CI lint that checks for catalog conformance (post-R3)
- Multi-language codebase detection beyond Go (post-R3)

## Planned touchpoints

- `cmd/sworn/induction.go` (new — induction command)
- `cmd/sworn/induction_test.go` (new)
- `cmd/sworn/main.go` (DOCUMENTED SHARED — additive dispatch for `induction` verb)
- `internal/prompt/implementer.md` (modify — add deviation check step)
- `internal/prompt/verifier.md` (modify — add catalog conformance check)

## Acceptance checks

- [ ] `sworn induction` on a test repo with a blank `docs/considerations.md` walks
  through all three phases; after completion, `docs/considerations.md` has non-empty
  `design_system` and `architecture.patterns` sections (verified by reading the file)
- [ ] `sworn induction` on a repo where `docs/considerations.md` already has patterns
  auto-enters `--update` mode with a notice; does not re-prompt for already-accepted
  patterns
- [ ] `sworn induction --update` shows only NEW inferred patterns not already in the
  catalog's `architecture.patterns` list
- [ ] `internal/prompt/implementer.md` contains the "Deviation check" section; the
  phrase "Set slice state to BLOCKED" appears verbatim
- [ ] `internal/prompt/verifier.md` contains the "Catalog conformance check" section;
  the phrase "undocumented deviation" appears verbatim as a FAIL trigger
- [ ] `go test ./cmd/sworn/... -run Induction` passes; tests cover the skip path
  (catalog absent → graceful) and the happy path (catalog present → patterns written)
- [ ] `go test ./internal/prompt/... -run Implementer` asserts the deviation check
  heading is present; `go test ./internal/prompt/... -run Verifier` asserts catalog
  conformance check heading is present
- [ ] `go build ./...` passes; no new external deps (induction uses stdlib I/O only)

## Required tests

- **Unit** `cmd/sworn/induction_test.go`:
  - `TestInductionWritesDesignSystem`: piped stdin providing design system answers;
    assert `design_system.location` written to catalog
  - `TestInductionWritesPatterns`: piped stdin accepting two proposed patterns;
    assert both appear in `architecture.patterns`
  - `TestInductionSkipPath`: piped stdin answering 's' to skip Phase 2;
    assert `architecture.patterns` remains empty; no error
  - `TestInductionIdempotent`: catalog already populated; assert auto-enters update
    mode; assert no patterns duplicated
  - `TestInductionUpdateShowsOnlyNew`: catalog has pattern A; codebase analysis finds
    A and B; `--update` shows only B
- **Unit** `internal/prompt/prompt_test.go` (extend):
  - `TestImplementerHasDeviationCheck`: assert "Deviation check" heading in
    `Implementer()` return value
  - `TestVerifierHasCatalogConformance`: assert "Catalog conformance check" heading in
    `Verifier()` return value

- **Reachability artefact**: smoke step — run `sworn induction` in a test repo with
  piped stdin (all defaults accepted); cat `docs/considerations.md`; confirm
  `design_system` and `architecture.patterns` sections non-empty. Document commands
  in proof.md.

## Risks

- Pattern inference reads files in the repo at induction time. The induction command
  must not block or crash on repos that do not follow the inferred language (e.g., a
  polyglot repo where Go is not the primary language). If auto-detection fails, fall
  back to the manual "add more" path cleanly.
- The verifier's catalog conformance check adds a new FAIL trigger. This must be
  carefully worded so the verifier does not FAIL a slice simply because the catalog is
  absent or the deviation was documented. The check must be conditional and clear.
- `cmd/sworn/main.go` is a DOCUMENTED SHARED file. S19 adds only an additive
  `case "induction"` dispatch entry.

## Deferrals allowed?

Multi-language pattern inference: deferred post-R3. Go detection is sufficient for
sworn's own dogfood use case. Rule 2: Why — multi-language requires language-specific
AST analysis; out of scope for this release. Tracking: post-R3 issue. Acknowledged:
2026-06-20.
