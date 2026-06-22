---
title: Slice proof bundle
description: Rule 6 proof bundle. Populated by implementer.
---

# Proof Bundle: `S21-canonical-baton`

## Scope

Embed the full Baton protocol (10 rules + supporting docs) in the `sworn` binary
via `go:embed` at `internal/prompt/baton/`. Add `prompt.Baton()` and
`prompt.BatonAll()` for programmatic/MCP access. Rewrite `sworn init` to stop
creating `docs/baton/` and stop splicing `AGENTS.md`/`CLAUDE.md`; instead write
a minimal MCP-pointer `AGENTS.md` from `docs/templates/agents.md`.

## Files changed

```
cmd/sworn/init.go
cmd/sworn/init_design_system_test.go
cmd/sworn/init_test.go
docs/adr/0008-canonical-baton.md
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/status.json
docs/templates/agents.md
internal/prompt/baton/README.md
internal/prompt/baton/VERSION.txt
internal/prompt/baton/brainstorm-patterns.md
internal/prompt/baton/rules.md
internal/prompt/baton/session-discipline.md
internal/prompt/prompt.go
internal/prompt/prompt_test.go
```

## Test results

### `go test ./internal/prompt/... -run Baton`

```
=== RUN   TestBatonVersion_NonEmpty
--- PASS: TestBatonVersion_NonEmpty (0.00s)
=== RUN   TestBatonRulesNonEmpty
--- PASS: TestBatonRulesNonEmpty (0.00s)
=== RUN   TestBatonAllKeys
--- PASS: TestBatonAllKeys (0.00s)
=== RUN   TestBatonRulesHasAllTen
--- PASS: TestBatonRulesHasAllTen (0.00s)
=== RUN   TestBatonMissingFile
--- PASS: TestBatonMissingFile (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/prompt	0.003s
```

### `go test ./cmd/sworn/... -run Init`

```
=== RUN   TestCmdInit_NonInteractive
--- PASS: TestCmdInit_NonInteractive (0.00s)
=== RUN   TestCmdInit_UIBearingFlag
--- PASS: TestCmdInit_UIBearingFlag (0.00s)
=== RUN   TestCmdInit_UIBearingOutput
--- PASS: TestCmdInit_UIBearingOutput (0.00s)
=== RUN   TestCmdInit_UIBearing_ValidateFailClosed
--- PASS: TestCmdInit_UIBearing_ValidateFailClosed (0.00s)
=== RUN   TestInitCreatesBothTemplates
--- PASS: TestInitCreatesBothTemplates (0.00s)
=== RUN   TestInitSkipsBoth
--- PASS: TestInitSkipsBoth (0.00s)
=== RUN   TestInitOverwriteGuard
--- PASS: TestInitOverwriteGuard (0.00s)
=== RUN   TestInitCreatesAgentsMD
--- PASS: TestInitCreatesAgentsMD (0.00s)
=== RUN   TestInitSkipsExistingAgentsMD
--- PASS: TestInitSkipsExistingAgentsMD (0.00s)
=== RUN   TestInitWarnsLegacyBaton
--- PASS: TestInitWarnsLegacyBaton (0.00s)
=== RUN   TestInitDoesNotSpliceClaude
--- PASS: TestInitDoesNotSpliceClaude (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.013s
```

### `go build ./...`

PASS (exit 0)

## Reachability artefact

Manual smoke test — `sworn init --yes` in a temp directory:

```
$ sworn init --yes
sworn init: scanning repo...

Changes:
  +  AGENTS.md     does not exist — will be created from MCP-pointer template

  created  AGENTS.md (MCP-pointer template)
  created  docs/considerations.md
  created  docs/decisions.md

Done. Connect your AI to sworn mcp to get the Baton protocol and role prompts.
Run 'sworn doctor' to verify your setup.
```

**AGENTS.md content:**
```
# AGENTS.md

This repository uses [sworn](https://swornagent.com) for autonomous release
management under the Baton protocol.

...

| Full Baton protocol | `sworn://baton/rules` |
| Planner role prompt | `sworn://prompts/plan` |
| Implementer role prompt | `sworn://prompts/implement` |
| Verifier role prompt | `sworn://prompts/verify` |
| Current release board | `get_board` tool |
```

- `AGENTS.md` created with `sworn://baton/rules` MCP reference
- `docs/baton/` NOT created
- Final message updated to reference MCP

## Delivered

- [x] `internal/prompt/baton/rules.md` — all 10 rules concatenated from `internal/adopt/baton/rules/01-10-*.md`; `prompt.Baton("rules.md")` returns content > 100 bytes; contains "Requirements Fidelity", "Design Fidelity", "Customer Journey Validation"
- [x] `internal/prompt/baton/session-discipline.md` — from canonical `~/.claude/baton/session-discipline.md`
- [x] `internal/prompt/baton/brainstorm-patterns.md` — from canonical `~/.claude/baton/brainstorm-patterns.md`
- [x] `internal/prompt/baton/README.md` — index/overview of embedded Baton docs
- [x] `internal/prompt/baton/VERSION.txt` — from `internal/prompt/VERSION.txt`
- [x] `internal/prompt/prompt.go` — `//go:embed` extended to `baton/*`; `Baton(name)` and `BatonAll()` added; package comment updated
- [x] `internal/prompt/prompt_test.go` — 4 new tests: `TestBatonRulesNonEmpty`, `TestBatonAllKeys`, `TestBatonRulesHasAllTen`, `TestBatonMissingFile`
- [x] `cmd/sworn/init.go` — removed `adopt.Materialise`, `adopt.SpliceAgents`, `adopt.PlanSplice`, `adopt.BatonDocsExist`; added AGENTS.md creation from template with legacy detection via `adopt.BatonSectionHeading`; updated final message
- [x] `cmd/sworn/init_test.go` — 4 new tests: `TestInitCreatesAgentsMD`, `TestInitSkipsExistingAgentsMD`, `TestInitWarnsLegacyBaton`, `TestInitDoesNotSpliceClaude`
- [x] `docs/templates/agents.md` — MCP-pointer template
- [x] `docs/adr/0008-canonical-baton.md` — architecture decision record with supersedes reference to `adopt.Materialise`
- [x] `go build ./...` passes
- [x] `sworn init` on clean dir creates `AGENTS.md` with `sworn://baton/rules`; does NOT create `docs/baton/`
- [x] `sworn init` on dir with non-legacy AGENTS.md → unchanged ("already present and up-to-date")
- [x] `sworn init` on dir with legacy Baton AGENTS.md (contains `## Engineering Process — Baton`) → warns, does not overwrite
- [x] `sworn init` does not touch CLAUDE.md

## Not delivered

- **User prompt overrides / project-level Baton customisation** — Deferred post-launch per ADR-0008. Why: opening overrides before canonical protocol is stable creates N support surfaces. Tracking: post-launch feature. **Acknowledged**: 2026-06-20 planning session (S21 spec).
- **Slash-command harness migration to read via sworn MCP** — Post-launch. Tracking: post-launch feature. **Acknowledged**: 2026-06-20 planning session.
- **Removal of `adopt` package** — Retained for `sworn doctor` legacy support (doctor.go depends on `BatonDocsFS()`, `BatonSectionHeading`, `AgentsFragment()`). Design decision 2, acknowledged by Coach.

## Divergence from plan

- **ADR filename**: Spec AC8 references `docs/adr/0005-canonical-baton.md` but 0005 was already taken by T2 (`0005-tui-dep-bubbles.md`). Created as `docs/adr/0008-canonical-baton.md` per Coach Pin 1.
- **Legacy detection string**: Spec AC7 says `<!-- baton:start -->` as legacy marker. Actual marker is `## Engineering Process — Baton` (`adopt.BatonSectionHeading` constant). Code and `TestInitWarnsLegacyBaton` use the real constant per Coach Pin 4.
- **track-mode.md pre-existing**: `internal/prompt/baton/track-mode.md` already existed from S08c-mcp-plan-tools before this slice ran. Not created by S21; embed extended to `baton/*` to include it alongside new files. Coach Pin 2.

## First-pass script output

```
release-verify.sh S21-canonical-baton 2026-06-19-safe-parallelism

  slice:       S21-canonical-baton
  slice dir:   docs/release/2026-06-19-safe-parallelism/S21-canonical-baton
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: in_progress
  FAIL  state is 'in_progress' — slice not yet ready for verifier; complete implementation first

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  13 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

== First-pass verdict ==
  checks passed: 21
  checks failed: 1
FIRST-PASS FAIL
```

The single FAIL is the `in_progress` state check — this resolves when the slice transitions to `implemented` below.