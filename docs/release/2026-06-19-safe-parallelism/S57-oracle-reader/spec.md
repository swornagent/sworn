---
title: 'S57-oracle-reader — git-ref, ownership-resolved slice-state reader'
description: 'sworn board reads every slice''s authoritative status.json straight from git refs (track branch > release-wt > working tree), ownership-resolved, so the board is honest regardless of which branch/worktree is checked out. Ports lib/release-board.mjs + captain-route.sh:156-208,403-495 into internal/board.'
---

# Slice: `S57-oracle-reader`

> Proposed by the 2026-06-23 port-fidelity audit. The keystone of the orchestration-core port (T17): the git-ref oracle reader that the router (S58), the TUI, and the end-of-run rollup all read through. Today sworn has only `state.Read` (a single working-tree `os.ReadFile`) + `internal/board` index.md parsing — neither reads authoritative committed state across track branches.

## User outcome

A developer runs `sworn board --release <name>` and sees every slice's **authoritative** state — the copy committed on the slice's **owning track branch** — regardless of which branch or worktree is currently checked out, and regardless of stale ghost copies other track branches carry. The same reader is callable in-process as `board.ReadSliceStatus(...)` for the router and TUI.

## Entry point

`sworn board [--release <name>] [--json]` — subcommand on `cmd/sworn/board.go`, self-registered via `init()` calling the S51 command registry (never edits `main.go`). Prints the reconciled board (default) or JSON (`--json`), mirroring `release-board-status.sh --json`.

## Background

The reference orchestration brain reads state from **git refs**, not the working tree: `lib/release-board.mjs` and `captain-route.sh:156-208` resolve each slice's `status.json` via `git show <ref>:<path>` in priority order — the slice's own `track/<release>/<track-id>` branch (authoritative) → `release-wt/<release>` → working tree. Ownership is resolved from the slice→track map (every track branch carries stale copies of *other* tracks' slices; the authoritative copy is the one on the owner's branch — the ghost-slice filter, `captain-route.sh:403-412,469-495`). Reading committed refs is what keeps the board honest when a worktree is dirty or on the wrong branch — the exact stale-read that misled the planner (intake.md 2026-06-19 "oracle-check mandatory").

## In scope

- `internal/board/oracle.go` (new):
  - `ReadSliceStatus(ctx, repo, release, sliceID string) (state.Status, ResolvedFrom, error)` — reads `status.json` via `git show <branch>:<docsPrefix>/release/<release>/<sliceID>/status.json`, priority track-branch → release-wt → working-tree HEAD, with the `docs/` vs `apps/docs/content/docs/` prefix probe (`captain-route.sh:139-150`).
  - **Ownership resolution**: given the `tracks:` frontmatter slice→track map (reuse `board.ParseTracks`), the authoritative copy of a slice is the one on its owner track's branch; ghost copies on non-owner branches are ignored.
  - **Transient-read retry**: empty / `state=="unknown"` read (status.json mid-commit) retries once after a short sleep before treating as missing (`captain-route.sh:162-171,219-225`).
  - `ReadBoard(ctx, repo, release) ([]TrackState, error)` — every track + every slice's authoritative state, the JSON the CLI prints.
- `cmd/sworn/board.go` (new) + self-registration via `init()`.
- Git plumbing via the existing `internal/git` package (`git show`, `cat-file -e`), not a new git dependency.

## Out of scope

- The router decision tree (S58 — consumes this reader via an interface).
- The scheduler re-layer (S59).
- Replacing the TUI's current DB-poll view (S04b) — the TUI may adopt this reader in a later slice; not here.
- Writing status.json (this is read-only; writes stay in `internal/state` / the implementer/verifier flow).

## Planned touchpoints

- `internal/board/oracle.go` (new)
- `internal/board/oracle_test.go` (new)
- `cmd/sworn/board.go` (new)
- `cmd/sworn/board_test.go` (new)

## Acceptance checks

- [ ] `ReadSliceStatus` returns the state from the slice's **owning track branch** even when invoked from a different track's worktree (test: commit divergent status.json on two branches; assert owner wins).
- [ ] Ghost copy ignored: a slice owned by T-a, with a stale `planned` copy on T-b's branch, resolves to T-a's authoritative state, not the ghost.
- [ ] Priority fallback: a slice with no track branch yet resolves from `release-wt`; with neither, from working-tree HEAD.
- [ ] `docs/` vs `apps/docs/content/docs/` prefix probe selects the right path.
- [ ] Transient-read retry: a status.json that reads empty once then non-empty resolves to the non-empty state (fake git layer with a one-shot empty).
- [ ] `sworn board --json --release <fixture>` prints every slice's authoritative state; output `.slices[].state` matches `release-board-status.sh --json` on the same fixture (parity).
- [ ] `go test -race ./internal/board/...` passes.

## Required tests

- **Unit**: `internal/board/oracle_test.go` — `TestOwnerBranchWins`, `TestGhostCopyIgnored`, `TestRefPriorityFallback`, `TestDocsPrefixProbe`, `TestTransientReadRetry` (fakeable git layer returning canned ref contents).
- **Integration / Reachability artefact (Rule 1)**: `cmd/sworn/board_test.go` runs the real `sworn board --json` subcommand against a committed multi-track fixture release and asserts authoritative resolution. Reachability is the CLI command itself.
- **Parity**: assert `.slices[].state` equals `release-board-status.sh --json` on a shared fixture (skip with a clear message if the bash oracle is not on PATH).

## Risks

- **Reading the working tree instead of committed refs** would reintroduce the stale-read trap. The contract is committed-ref reads; tests must commit divergent state to prove the reader ignores the working tree.
- **Ownership resolution wrong** → ghost slices inflate/deflate the board (the failure `captain-route.sh:403-412` guards). The ghost-filter test is mandatory.
- Spawning `git` per slice is acceptable for correctness here; if it's a perf issue at scale it's a later optimisation, not a contract change — note in proof, do not silently cap.

## Deferrals allowed?

No. This is the keystone the rest of T17 reads through; its committed-ref + ownership semantics are the whole point.
