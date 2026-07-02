# Journal — S01-driver-contract

## 2026-07-02/03 — planned → design_review → in_progress → implemented

**planned → design_review.** Wrote `design.md`: `internal/driver` package
(`Driver`/`RoleSet`/`DispatchInput`/`Result`/`AssertWorktree`), implementing
the Type-1 role-dispatch decision already recorded in `status.json`
`design_decisions[0]`. Committed `21b03cc`, pushed, halted per Rule 9.

**Design review (Captain).** `review.md` (commit `d725c82`) returned
`DECISION: PROCEED` with 3 mechanical pins, all apply-inline:

1. The `sworn#35` citation for the "advertises-but-ignores" bug class was
   wrong — issue #35 is an unrelated Anthropic tool-use feature request; no
   GitHub issue matches the incident precisely. Fixed by rephrasing without
   a specific issue number in both `design.md` and (never introduced in)
   `docs/adr/0012-driver-contract.md`.
2. `AssertWorktree`'s design said "`git rev-parse --is-inside-work-tree` (or
   equivalent stat-based check)" — the hedge was unsafe: every worktree in
   this project (including the one this session runs in) has `.git` as a
   file, not a directory, so a stat-based directory check would fail-closed
   on every worktree the project actually uses. Committed to
   `git rev-parse --is-inside-work-tree` only; added a linked-worktree test
   case (`driver_test.go` "success: linked worktree") that creates a real
   `git worktree add` checkout and asserts it passes.
3. Confirmed `DispatchInput.Role` is the named type `Role`, not raw
   `string`, consistent with `RoleSet map[Role]bool` and AC-02.

The Coach acknowledged the review in-session (endorsed the suggested
acknowledgement reply verbatim, including "Address pins 1-3 inline during
implementation, then proceed to in_progress"). Design review gate satisfied
per Rule 9; `status.json` → `in_progress`, commit `530c553`.

**Forward-sync note.** Between design_review and this session resuming,
`release-wt/2026-06-28-driver-contract` picked up a T7-baton-revendor
roll-in and an `in_scope`/`out_of_scope` backfill across all 12 specs
(commits `2aa639a`/`8213026`/`fbf0ff8`, forward-merged onto this track
branch before I resumed). Checked S01's `spec.json` in_scope/out_of_scope
against what design.md already assumed — no scope change, the backfill only
made explicit what was already implicit in the touchpoints list.

**Implementation.** Wrote `internal/driver/{driver.go,worktree.go}` plus
`driver_test.go` and `imports_test.go`. One divergence from the spec's
touchpoint list, disclosed in `proof.json`'s `divergence`: `result.go` was
not created as a separate file — `Result` lives in `driver.go` alongside
`Driver`/`RoleSet`/`DispatchInput`, per design.md's own "kept minimal, added
only if a test needs it" framing, which the reviewer passed without
objection.

`go build ./...`, `go vet ./internal/driver/...`, and `go test ./...`
(full repo, no regressions) all green. `go test ./internal/driver/... -v`
covers all 6 ACs (see `proof.json` `delivered`).

**Proof-bundle first-pass gate.** `sworn verify` requires a configured model
API key (`SWORN_ANTHROPIC_API_KEY`); none is set in this session
(no-paid-dispatch constraint, same as the planning session's
reqverify/spec-ambiguity deferral). Ran `~/.claude/bin/release-verify.sh`
instead (the deterministic first-pass per Rule 7's cheap-cost loop). Known,
pre-existing false-negatives on this release's JSON-record format
([[feedback_releaseverify_specmd_false_fail]]): the script looks for
`spec.md`/`proof.md` (this release uses `spec.json`/`proof.json`, per
ADR-0009/ADR-0010) and infers the integration branch from `index.md`
frontmatter (this release's integration branch lives in `board.json`
`release.integration_branch`, not frontmatter). Not manufacturing
placeholder `.md` files to satisfy a legacy-format checker. See `journal.md`
completion entry below for the actual first-pass output once re-run against
committed state.

**state → implemented.**
