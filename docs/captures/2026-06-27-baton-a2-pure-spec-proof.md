# Proof bundle — Baton A2 residual old-model migration (2026-06-27)

Rule 6 proof bundle for the A2 slice of the records-as-JSON + Baton pure-spec
work. Generated from live repo state on `sawy3r/baton` branch
`feat/role-prompts-records` (→ PR #52). Continues the work captured in
`docs/captures/2026-06-26-records-as-json-handoff.md`.

## Scope

Migrate the 15+ residual files in the Baton repo that still carried the old
artefact model (bash/`.mjs` gates, `index.md`/`spec.md`/`proof.md`, the `awk`
materialise hack, "16 hats") to the records-as-JSON + pure-spec model
established by the already-merged role prompts (ADR-0009, ADR-0010). A1
(external README/install/templates) was already done; A2 is the remainder.

## Files changed

`git diff --name-only c17be08..HEAD` (17 files):

```
ROADMAP.md
claude/baton/AGENTS-fragment.md
claude/baton/INSTALL.md
claude/baton/README.md
claude/baton/RULES-HISTORY.md
claude/baton/adversarial-verification.md
claude/baton/architecture.json
claude/baton/release-mode-template/status.json
claude/baton/requirements-fidelity.md
claude/commands/design-review.md
claude/commands/implement-slice.md
claude/commands/mark-shipped.md
claude/commands/merge-release.md
claude/commands/merge-track.md
claude/commands/plan-release.md
claude/commands/replan-release.md
claude/commands/verify-slice.md
```

Five commits (one per file-group): `dd5e7f7` (8 slash commands), `9b73e89`
(embedded README), `d3e07be` (rule docs), `a337c86` (supporting docs),
`d4d62d0` (RULES-HISTORY transition entry). All pushed to
`origin/feat/role-prompts-records`.

## Test results

This is a documentation migration; the slice-relevant verification is a
deterministic detector for old-model tokens plus structural integrity checks.

- **Detector — bash gate scripts still called from commands**:
  `grep -rnE "release-board-status\.sh --json|\.claude/bin/release|bin/release-(verify|trace|coverage|audit|mock|regression|board)" claude/commands/` → **0 hits**.
- **Full-tree A2 detector** (`release-*|bin/release|/(spec|proof|index).md|16 hats` over `claude/` + `ROADMAP.md`) → 3 files flagged, each confirmed intentional:
  - `RULES-HISTORY.md` — the new 0.5.0 entry's own Removed/Changed lists naming the deleted scripts, plus deliberately-preserved past entries.
  - `ROADMAP.md` — the historical note that the `.mjs` oracle shipped here and moved to `sworn` (ADR-0010).
  - `mark-shipped.md` — the rendered-view `index.md` staged alongside `board.json` in the commit.
- **JSON validity** (`python3 json.load`): `architecture.json` OK, `release-mode-template/status.json` OK.
- **doctor.go invariant**: `grep -c "^## The eleven rules" claude/baton/README.md` → **1** (heading preserved verbatim; `cmd/sworn/doctor.go:93` hard-checks this string).
- **Reference-impl pointers present**: 6 of 8 commands carry `sworn board` / `sworn verify` pointers (the 2 without — `plan-release`, `mark-shipped` — legitimately do not invoke the oracle: the planner creates the board; mark-shipped reads `board.json` directly because `release-wt` may already be deleted).

## Reachability artefact

The migration is reachable through the user-facing affordances it owns: the
slash commands a human invokes (`/implement-slice`, `/verify-slice`, etc.) now
instruct the agent to read/write the JSON records and invoke the gates by their
`sworn` reference pointers. Smoke step: open any of the 8 migrated
`claude/commands/*.md`, confirm Step 0 reads `board.json` via the board oracle
(`sworn board --json`) and no step shells out to `~/.claude/bin/release-*.sh`;
confirm the embedded `claude/baton/README.md` still carries `## The eleven
rules` (doctor's check). The detector output above is the mechanical evidence.

## Delivered

- **8 slash commands** migrated — `index.md`→`board.json`, `spec.md`→`spec.json`,
  `proof.md`→`proof.json`; board-oracle script → `sworn board`; `release-verify.sh`
  → proof-bundle verification gate (`sworn verify`); frontmatter fields →
  board-v1 objects (`worktree.path`, `release.worktree`, `release.integration_branch`);
  awk materialise hack + abort-on-corruption guards deleted; markdown-board
  conflict framing reframed to JSON-record terms. (commit `dd5e7f7`)
- **Git-worktree-list `awk` primary-discovery retained** in `merge-release` /
  `mark-shipped` (Rule 11 fail-closed target assertion — a distinct idiom from
  the deleted index.md line-editor). (evidence: detector kept those lines)
- **Embedded README** harness section de-bashed; `## The eleven rules` heading
  preserved. (commit `9b73e89`, TEST 3)
- **Rule docs** `adversarial-verification.md` + `requirements-fidelity.md` —
  gates named by role with `sworn` pointers; RTM + artefact lists → JSON records.
  (commit `d3e07be`)
- **Supporting docs** `AGENTS-fragment.md`, `INSTALL.md`, `architecture.json`,
  `release-mode-template/status.json`, `ROADMAP.md` — de-bashed; `16-hat` →
  six considerations; broken `"$schema": "https://"` stub fixed to the canonical
  `architecture-rules-v1` URL. (commit `a337c86`, TEST 2)
- **RULES-HISTORY.md** 0.5.0 transition entry appended; past entries intact.
  (commit `d4d62d0`)
- **PR #52 body** updated to mark A1 + A2 done (via REST API — `gh pr edit`
  aborts on the deprecated Projects-classic GraphQL query).

## Not delivered

- **Phase B (Sworn repo, separate release)** — not in this slice's scope, tracked
  in the handoff "Remaining work B": oracle/gates reading `board.json`/`spec.json`/
  `proof.json` in Go, the `example.com` `$schema` placeholders in
  `run.go:293` / `mcp/tools_plan.go:55`, the live-board migration, scanner
  replacement (sworn #22), drift-guard CI, and the `slice-status-v1` drift
  reconciliation. Acknowledged: this is the next phase, surfaced in the handoff,
  not a silent deferral.
- **Bumping Sworn's Baton vendor pin** to pick up these commits — Phase B. Sworn
  vendors canonical Baton **from the git repo at a pinned SHA** (ADR-0006
  vendor-down; *not* from a local `~/.claude/baton` install). `internal/adopt/baton/VERSION`
  currently pins `9ae08fb` (`baton-protocol: v0.5.0`, vendored 2026-06-25) —
  which predates even the role-prompt migration, so the vendored copy is stale.
  Once PR #52 + the A1 commits merge to baton `main` and a new SHA is cut, bump
  the pin and re-run the vendor-down sync. Only the **embedded subset** updates —
  `baton/README.md`, `baton/VERSION`, `baton/rules/*`, `baton/architecture.json`
  (the slash commands, role prompts, INSTALL/ROADMAP/RULES-HISTORY are not
  embedded in the binary — they are consumed from the Baton repo / a local
  install). The doctor README-compare (`cmd/sworn/doctor.go`) re-aligns
  automatically once the canonical README is re-synced.

## Divergence from plan

- The handoff estimated "15 files"; the detector found **17** (it counted
  `design-review.md` as a command and the `status.json` template separately).
  All 17 migrated.
- Fixed `architecture.json`'s broken `"$schema": "https://"` stub to the
  canonical URL — a small in-scope correctness fix beyond the literal de-bash
  task, justified because schema-pointer correctness is the goal of the
  records-as-JSON convergence.
- `RULES-HISTORY.md` handled by **appending** a 0.5.0 entry rather than rewriting
  past entries (human-ratified decision this session): a history doc must record
  the bash/Markdown era faithfully; rewriting it would falsify the record.
