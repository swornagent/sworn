---
title: Slice proof bundle
description: Rule 6 proof bundle. Populated by implementer.
---

# Proof Bundle: `S22-sworn-doctor`

## Scope

Implement `sworn doctor` — a health check command that verifies embedded prompt integrity, detects legacy Baton artifacts, checks local Baton sync, and checks dependency version freshness. Each failing check prints `[OK]`, `[WARN]`, or `[ERROR]` with a specific repair command. `--fix` auto-removes legacy artifacts; `--sync-baton` copies embedded Baton docs to `~/.claude/baton/`.

## Files changed

```
cmd/sworn/doctor.go       (new — 500+ lines)
cmd/sworn/doctor_test.go  (new — 400+ lines)
cmd/sworn/main.go         (modified — additive `case "doctor"` dispatch + usage text)
internal/adopt/adopt.go   (modified — exported AgentsFragment() accessor)
```

## Test results

```
$ go test ./cmd/sworn/... -run Doctor -v

=== RUN   TestDoctorAllOK
--- PASS: TestDoctorAllOK (0.00s)
=== RUN   TestDoctorLegacyBatonDir
--- PASS: TestDoctorLegacyBatonDir (0.00s)
=== RUN   TestDoctorLegacySpliceAgentsMD
--- PASS: TestDoctorLegacySpliceAgentsMD (0.00s)
=== RUN   TestDoctorFixRemovesBatonDir
--- PASS: TestDoctorFixRemovesBatonDir (0.00s)
=== RUN   TestDoctorFixMigratesAgentsMD
--- PASS: TestDoctorFixMigratesAgentsMD (0.01s)
=== RUN   TestDoctorSyncBaton
--- PASS: TestDoctorSyncBaton (0.00s)
=== RUN   TestDoctorNoBatonHomeNoWarn
--- PASS: TestDoctorNoBatonHomeNoWarn (0.00s)
=== RUN   TestDoctorGroup4StalePins
--- PASS: TestDoctorGroup4StalePins (0.00s)
=== RUN   TestDoctorGroup4EmptyPins
--- PASS: TestDoctorGroup4EmptyPins (0.00s)
=== RUN   TestDoctorGroup4RegistryUnreachable
--- PASS: TestDoctorGroup4RegistryUnreachable (0.00s)
=== RUN   TestDoctorGroup4VerifierHeadings
--- PASS: TestDoctorGroup4VerifierHeadings (0.00s)
=== RUN   TestDoctorCorruptPrompt
--- PASS: TestDoctorCorruptPrompt (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.025s
```

```
$ go build ./...
(clean — no errors)
```

```
$ go vet ./...
(clean — no warnings)
```

## Reachability artefact

Running `sworn doctor` on this repo (the actual binary, not a mock):

```
$ go run ./cmd/sworn doctor

== Group 1: Embedded prompt integrity ==
[OK]    planner.md
        length=29008   headings=all present
[WARN]  implementer.md
        missing headings (WARN — not yet landed): ## Dependency discipline, ## Deviation check
[WARN]  verifier.md
        missing headings (WARN — not yet landed): ## Catalog conformance check, independently query the package registry
[OK]    captain.md
        length=27664
[OK]    verify-stateless.md
        length=3267
[OK]    baton/rules/
        10/10 rule files present, README heading OK
[OK]    baton/track-mode.md
        length=13698   heading present
[OK]    baton/VERSION.txt
        version=v1.0.0

== Group 2: Repo artifact audit ==
[WARN]  docs/baton/
        legacy per-repo Baton copy. The binary is now the canonical source. Safe to remove: rm -rf docs/baton/
[WARN]  AGENTS.md
        contains legacy Baton splice content. Run 'sworn init' to replace with the current minimal MCP-pointer template (backs up old AGENTS.md to AGENTS.md.bak)

== Group 3: Local Baton sync ==
[WARN]  ~/.claude/baton/
        differs from the binary's embedded Baton (11 files differ). Slash commands use the local copy; run 'sworn doctor --sync-baton' to update. (Only affects interactive slash commands, not autonomous sworn run.)

== Group 4: Dependency version freshness ==
[WARN]  dependency catalog
        dependency file found but docs/considerations.md not found. Run 'sworn induction' to populate it — implementers need this to know which versions are already pinned.
EXIT_CODE=0
```

This exercises the full `cmdDoctor` function through the `dispatch` integration point (via `go run ./cmd/sworn doctor`). All group 1 embedded prompt checks pass (OK or WARN for S19-dependent headings). The WARNs for groups 2-4 are expected for this repo (docs/baton/ exists, AGENTS.md has legacy splice, no considerations.md). Exit code 0 confirms no ERRORs.

## Delivered

- **`cmd/sworn/doctor.go`** (new) — full `sworn doctor` subcommand with four check groups:
  - Group 1: Embedded prompt integrity (length > 500 bytes, required headings per file, Baton rules 10-file check, track-mode.md heading, VERSION.txt parse). S19-dependent headings emit `[WARN]` not `[ERROR]` per Coach add-on.
  - Group 2: Repo artifact audit (docs/baton/ existence, AGENTS.md legacy splice detection via `adopt.BatonSectionHeading`).
  - Group 3: Local Baton sync (compares embedded vs `~/.claude/baton/`, skipped entirely when absent).
  - Group 4: Dependency version freshness (project pins vs catalog, sworn's own deps via `go list -m -u`, empty pins warning, registry unreachable warning).
  - `--fix` flag: removes docs/baton/ (with file listing), migrates legacy AGENTS.md (backup + rewrite).
  - `--sync-baton` flag: copies embedded Baton docs to `~/.claude/baton/` (or `SWORN_BATON_HOME`).
  - Exit codes: 0 = clean/WARN-only, 1 = any ERROR, 2 = `--fix` applied changes.
  - Evidence: `TestDoctorAllOK`, `TestDoctorLegacyBatonDir`, `TestDoctorLegacySpliceAgentsMD`, `TestDoctorFixRemovesBatonDir`, `TestDoctorFixMigratesAgentsMD`, `TestDoctorSyncBaton`, `TestDoctorNoBatonHomeNoWarn`, `TestDoctorGroup4StalePins`, `TestDoctorGroup4EmptyPins`, `TestDoctorGroup4RegistryUnreachable`, `TestDoctorGroup4VerifierHeadings`, `TestDoctorCorruptPrompt`.

- **`cmd/sworn/doctor_test.go`** (new) — 12 tests covering every acceptance check in the spec.

- **`cmd/sworn/main.go`** (modified) — additive `case "doctor"` dispatch + usage text. Evidence: `go build ./...` passes.

- **`internal/adopt/adopt.go`** (modified) — exported `AgentsFragment()` accessor so `doctor --fix` can write the minimal AGENTS.md template. Evidence: `go test ./internal/adopt/...` passes.

## Not delivered

- **`sworn://baton/rules` MCP pointer check** — deferred (Rule 2).
  - **Why**: the `sworn://` MCP resource-URI scheme does not exist in any landed slice yet; the check would WARN on every repo.
  - **Tracking**: S22 spec acceptance check (group 2, AGENTS.md MCP pointer check).
  - **Acknowledgement**: Coach (brad), 2026-06-22, via approved-ack.md §2.4.

## Divergence from plan

- The spec mentions `sworn doctor --set-version <v>` in the VERSION.txt WARN output, but no `--set-version` flag is implemented (it's only referenced in the warning text, not in the acceptance checks or required tests). This is cosmetic — the warning text suggests a command that doesn't exist yet. No functional impact.

## First-pass script output

```
$ release-verify.sh S22-sworn-doctor 2026-06-19-safe-parallelism

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  4 file(s) changed vs diff base
  (first 20)
    cmd/sworn/doctor.go
    cmd/sworn/doctor_test.go
    cmd/sworn/main.go
    internal/adopt/adopt.go

== Dark-code markers in changed files ==
  PASS  no dark-code markers found

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
  checks passed: 22
  checks failed: 0

FIRST-PASS PASS
```