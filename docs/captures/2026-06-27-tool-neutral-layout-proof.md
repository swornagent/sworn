# Tool-neutral Baton layout (2b) — proof bundle (2026-06-27)

Rule 6 proof bundle for step 2b: de-Claude the Baton repo structure and track it
in Sworn's vendor map. Spans two repos and two branches.

## Scope

Move tool-agnostic Baton content out of the Claude-namespaced `claude/`
directory into a tool-neutral top-level layout, rename the implicit-default
installer, and update Sworn's vendor source map in lockstep so the next sync
resolves the new paths.

## Type-1 decision (Rule 9, human-selected)

- **Layout:** top-level siblings — `claude/baton/` → `baton/`, `claude/commands/`
  → `commands/`, `claude/` removed. (Brad chose this over nesting under `baton/`.)
- **Installer rename:** `install.sh` → `install-claude.sh` (Brad's call) — a bare
  `install.sh` implicitly privileged Claude; named per-tool installers are symmetric.
- Decision-maker: Brad, via AskUserQuestion + the install-rename instruction,
  2026-06-27. Recorded in baton `RULES-HISTORY.md` 0.5.1.

## Files changed

**baton — `refactor/tool-neutral-layout` (`1af521a`), `git diff --stat main`:**
45 files, 81 insertions / 44 deletions. 41 renames (the `git mv` of every file
under `claude/baton/` and `claude/commands/`, plus `install.sh` →
`install-claude.sh`), and 4 content edits: `README.md`, `ROADMAP.md`,
`RELEASING.md`, `install-codex.sh`. `RULES-HISTORY.md` and `INSTALL.md` are
renames-with-edits.

**sworn — `refactor/baton-vendor-paths` (`1821934`), `git diff --stat release/v0.1.0`:**
24 files, 47 / 47. `internal/baton/source.go` (35 Source paths
`claude/baton/` → `baton/`), `vendor.go` + `diff.go` (the synthesized-rules.md
literal), `vendor_test.go` + `fetch_test.go` (in-test source-tree builders), and
the `testdata/fixture/claude/baton/` → `baton/` fixture move.

## Test results

- **baton:** both installers `--dry-run` resolve the new source paths and exit 0
  (`projects/baton/commands/*.md`, `projects/baton/baton/.`,
  `projects/baton/schemas/*.json`). `bash -n install-claude.sh` and
  `bash -n install-codex.sh` pass.
- **sworn:** `go build ./...` exit 0; `gofmt -l` clean;
  `go test ./internal/baton/... ./internal/prompt/... ./cmd/sworn/` all pass
  (the vendor tests that initially failed on the layout mismatch now pass after
  the fixture + literal updates). `grep -rc claude/baton internal/baton/` → zero.

## Reachability artefact

The installers are the user-facing affordance, and they are proven against the
new layout: `./install-claude.sh --dry-run` and `./install-codex.sh --dry-run`
both enumerate `would: cp` lines reading from `baton/` and `commands/` and exit
0. The Sworn vendor path is proven by the vendor tests, which build a fake Baton
source tree at the new `baton/` layout and run `Vendor(check)` green. Smoke for a
regression: rename any `Source:` back to `claude/baton/...` in `source.go` and
the vendor tests fail with "source file missing".

## Delivered

- `claude/` removed; `baton/` and `commands/` at top-level; `schemas/` unchanged.
  Content byte-identical (git renames) — the eleven rules, role prompts, and
  templates are unchanged; this is packaging (RULES-HISTORY 0.5.1).
- `install.sh` → `install-claude.sh`; source paths updated; install-destination
  paths (`~/.claude/`, `~/.codex/`) and the `.claude/`→`.codex/` runtime-ref
  rewrite preserved.
- Repo-relative refs updated: README, ROADMAP, RELEASING, INSTALL,
  release-mode-slice-ref, both install scripts' self/cross-references.
- Sworn vendor source map + special-case literals + test fixtures realigned to
  `baton/`.
- Open-core reconciliation: the seam decision doc and project memory updated to
  reflect ADR-0010 (oracle/gates Baton-bash → Sworn-Go) and coach-loop parity
  in Go (S57–S59).

## Not delivered (Rule 2 deferrals — flagged, acknowledged)

- **The `$HOME/.claude/baton/…` runtime references inside the role prompts and
  commands.** These are install-*location* paths (tool-specific by design; the
  install scripts rewrite `~/.claude/`→`~/.codex/`), a separate concern from the
  source layout. Untouched deliberately.
- **ROADMAP's "Next — cross-tool adapters" section** still describes a future
  unified tool-aware `install.sh` (`--tools=`, auto-detect). The per-tool
  installer rename puts that vision in tension. Left for a strategy decision, not
  silently rewritten. Flagged to Brad in-session.
- **The actual re-vendor** (pulling migrated content at a new pinned baton SHA)
  is the later Phase-B `/plan-release` step; this only makes the source map track
  the new paths.

## Divergence from plan

- The plan estimated the repo-ref ripple; the installer-rename (`install.sh` →
  `install-claude.sh`) was added mid-flight at Brad's instruction and folded in,
  including the scripts' own self-references and the ROADMAP current-script refs
  (but not the future-vision section).
- RULES-HISTORY past entries' `claude/baton` path mentions left intact
  (historical record), consistent with the append-don't-rewrite principle used
  for the 0.5.0 entry.
