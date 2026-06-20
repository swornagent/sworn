---
title: 'S22-sworn-doctor — prompt integrity checks and legacy Baton artifact detection'
description: 'sworn doctor verifies the structural integrity of all embedded role prompts and Baton protocol docs, warns on legacy per-repo Baton artifacts (docs/baton/, old-style AGENTS.md splice), and optionally checks the developer''s ~/.claude/baton/ for sync with the embedded canonical version. Surfaces actionable repair steps, not just errors.'
---

# Slice: `S22-sworn-doctor`

## User outcome

A developer runs `sworn doctor` and gets a clear health report: which embedded prompts
are structurally sound, whether the repo has legacy Baton artifacts that should be
cleaned up, and whether their local `~/.claude/baton/` is in sync with the binary.
Each failing check comes with a specific repair command. No more silent corruption —
a malformed embed or a stale Baton copy is surfaced before it causes a mid-run failure.

## Entry point

`sworn doctor` CLI command (no args for default report; `--fix` flag for auto-repair
of safe items). Verifiable by running `sworn doctor` on this repo and confirming all
embedded prompt checks pass; and on a repo with `docs/baton/` still present, confirming
the legacy artifact warning appears.

## In scope

### `cmd/sworn/doctor.go` — health check command

Runs a series of checks in order, prints a structured report, exits non-zero if any
`ERROR` check fails, zero if only `WARN` or `OK`.

**Check group 1 — Embedded prompt integrity**

For each embedded prompt (`planner.md`, `implementer.md`, `verifier.md`, `captain.md`,
`verify-stateless.md`) and each Baton protocol doc (`baton/rules.md`, `baton/track-mode.md`):

- Length check: content must be > 500 bytes. Below this implies truncation or corruption.
- Required headings check: each file has a known set of required `##` headings
  (defined as a constant in doctor.go):
  - `planner.md`: must contain `## Phase 1`, `## Phase 2`, `## Phase 3`, `## Phase 4`,
    `## Re-planning a release in flight`
  - `implementer.md`: must contain `## Deviation check` (added by S19)
  - `verifier.md`: must contain `## Catalog conformance check` (added by S19)
  - `baton/rules.md`: must contain all 7 rule headings
  - `baton/track-mode.md`: must contain `## The safety invariants`
- Version check: `internal/prompt/baton/VERSION.txt` must exist and be parseable as
  a semver string.

Output per check:
```
[OK]    planner.md           length=8421   headings=all present
[OK]    implementer.md       length=6204   headings=all present
[ERROR] verifier.md          length=12     BELOW MINIMUM (expected >500) — embed may be corrupted
[OK]    baton/rules.md       length=14321  headings=7/7 present
[WARN]  baton/VERSION.txt    version=0.0.0 — not yet set; run 'sworn doctor --set-version <v>'
```

**Check group 2 — Repo artifact audit**

Checks the current working directory (must be a git repo root):

- `docs/baton/` existence: if present → `[WARN] docs/baton/ exists — legacy per-repo
  Baton copy. The binary is now the canonical source. Safe to remove: rm -rf docs/baton/`
- AGENTS.md splice detection: if `AGENTS.md` contains `<!-- baton:start -->` →
  `[WARN] AGENTS.md contains legacy Baton splice content. Run 'sworn init' to replace
  with the current minimal MCP-pointer template (backs up old AGENTS.md to AGENTS.md.bak)`
- AGENTS.md MCP pointer: if `AGENTS.md` exists but does NOT contain `sworn://baton/rules` →
  `[WARN] AGENTS.md may be outdated — missing sworn MCP resource reference. Run
  'sworn init' to update.`
- AGENTS.md absent: `[WARN] AGENTS.md not found. Run 'sworn init' to create it.`

**Check group 3 — Local Baton sync (optional)**

Runs only if `~/.claude/baton/` exists on the developer's machine:
- Compares the embedded `baton/rules.md` content against `~/.claude/baton/` content
  (byte-level). If they differ:
  `[WARN] ~/.claude/baton/ differs from the binary's embedded Baton (N bytes differ in
  rules.md). Slash commands use the local copy; run 'sworn doctor --sync-baton' to
  update ~/.claude/baton/ from the binary. (Only affects interactive slash commands,
  not autonomous sworn run.)`
- If `~/.claude/baton/` is absent: no warning (it's optional; only developers who use
  slash commands need it).

**`--fix` flag** — applies safe auto-repairs without confirmation:
- Removes `docs/baton/` if present (after printing what it's removing)
- Backs up `AGENTS.md` to `AGENTS.md.bak` and rewrites with minimal template if legacy
  splice detected
- Does NOT auto-apply `--sync-baton` (modifying ~/.claude/ requires explicit opt-in)

**`--sync-baton` flag** — copies embedded Baton docs to `~/.claude/baton/`, creating
the directory if needed. Prints each file written. Useful for developers who use slash
commands and want their local Baton to match the binary.

### Exit codes
- `0`: all checks OK or WARN only (warnings are advisory; nothing is broken)
- `1`: any ERROR check (embed corrupted, parse failure, git not found, etc.)
- `2`: `--fix` applied changes (distinction useful in CI: 0=clean, 2=fixed, 1=error)

## Out of scope

- Automatic version bumping of Baton
- Checking consistency between `docs/considerations.md` and the embedded catalog
  format (that is `sworn lint` scope)
- Network-based update checks (no phone-home; purely local)
- Fixing corrupted embedded prompts (if the embed is corrupt, a `sworn` reinstall is
  the fix; doctor cannot re-embed)

## Planned touchpoints

- `cmd/sworn/doctor.go` (new)
- `cmd/sworn/doctor_test.go` (new)
- `cmd/sworn/main.go` (DOCUMENTED SHARED — additive `case "doctor"` dispatch)

## Acceptance checks

- [ ] `sworn doctor` on a clean repo (S21-initialized, no legacy artifacts) prints all
  group 1 checks as `[OK]` and exits 0
- [ ] `sworn doctor` on a repo with `docs/baton/` present prints `[WARN] docs/baton/
  exists — legacy` for group 2; exits 0 (WARN does not trigger non-zero exit)
- [ ] `sworn doctor` with a corrupt embedded prompt (simulated in test by length check)
  prints `[ERROR]` and exits 1
- [ ] `sworn doctor --fix` removes `docs/baton/` if present and prints what it removed;
  exits 2
- [ ] `sworn doctor --fix` on a legacy-splice AGENTS.md backs it up to AGENTS.md.bak
  and writes the minimal template; exits 2
- [ ] `sworn doctor --sync-baton` writes embedded Baton files to a temp `~/.claude/baton/`
  path (use `SWORN_BATON_HOME` env override in tests); prints each file written; exits 0
- [ ] When `~/.claude/baton/` does not exist, group 3 checks are skipped entirely
  (no error, no warning about its absence)
- [ ] `go test ./cmd/sworn/... -run Doctor` passes with zero failures
- [ ] `go build ./...` passes

## Required tests

- **Unit** `cmd/sworn/doctor_test.go`:
  - `TestDoctorAllOK`: run against this repo post-S21 (embedded prompts intact,
    no docs/baton/, AGENTS.md is minimal template); assert all OK, exit 0
  - `TestDoctorLegacyBatonDir`: temp dir with `docs/baton/` present; assert WARN
    for legacy artifact; exit 0
  - `TestDoctorLegacySpliceAgentsMD`: AGENTS.md with `<!-- baton:start -->` marker;
    assert WARN; exit 0
  - `TestDoctorFixRemovesBatonDir`: `--fix` on dir with `docs/baton/`; assert dir
    removed; exit 2
  - `TestDoctorFixMigratesAgentsMD`: `--fix` on legacy-splice AGENTS.md; assert
    AGENTS.md.bak created with old content; AGENTS.md replaced with minimal template;
    exit 2
  - `TestDoctorSyncBaton`: `--sync-baton` with `SWORN_BATON_HOME` set to temp dir;
    assert embedded files written there; exit 0
  - `TestDoctorNoBatonHomeNoWarn`: no `~/.claude/baton/`; assert group 3 section
    absent from output entirely
- **Reachability artefact**: run `sworn doctor` in this repo after S21 lands;
  capture full output; all checks OK. Document in proof.md.

## Risks

- The required heading lists for each embedded prompt are hardcoded in doctor.go.
  They must be updated whenever S18/S19/future slices add new mandatory sections.
  A test that runs doctor against the actual embedded files guards against this.
- `--fix` auto-removes `docs/baton/`. Since `docs/baton/` may contain user-edited
  files in repos that predated this release, print each file being removed (with
  `rm: <path>`) before deleting, so the user can see exactly what was touched.
- The `<!-- baton:start -->` legacy marker: confirm against the actual string in
  `internal/adopt/` before finalising. If the actual marker differs, the doctor
  check silently misses legacy repos.

## Deferrals allowed?

No blocking deferrals. The `--sync-baton` flag is strictly additive and can be
omitted from the first implementation pass if time-constrained, with a one-liner
stub that prints "not yet implemented" — but this must be surfaced as a Rule 2
deferral in proof.md, not silently dropped.
