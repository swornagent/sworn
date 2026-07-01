---
title: Slice proof bundle — S02-tui-oracle-migration
description: Rule 6 proof bundle, scoped to one slice. Generated from live repo state, not recollection. Verifier reads this; do not paraphrase.
---

# Proof Bundle: `S02-tui-oracle-migration`

Rendered from `proof.json` (proof-v1). First implementation pass.

## Scope

The sworn TUI board actually renders tracks and slices for any current-format
release (the originally reported bug); the blocked-slice view resolves the
real per-track worktree path and reads violations from proof.json instead of
a stale, silently-wrong fallback.

## Files changed

```
$ git diff --name-only 622f118ef3fda5581d332fdd76a80f39432de763
docs/release/2026-07-01-render-drift-reconciliation/S02-tui-oracle-migration/status.json
internal/tui/blocked.go
internal/tui/board.go
internal/tui/tui_test.go
```

(`proof.json`, this `proof.md`, and the final `status.json` implemented-state
commit land in the bundle commit, after this diff was captured against
`start_commit`.)

## Test results

### Go

```
$ go build ./...
(no output, exit 0)

$ go vet ./internal/tui/...
(no output, exit 0)

$ gofmt -l internal/tui/board.go internal/tui/blocked.go internal/tui/tui_test.go
(no output — all three touched files already gofmt-clean)

$ go test ./internal/tui/... -count=1 -v
...
=== RUN   TestBoardViewShowsSlices
--- PASS: TestBoardViewShowsSlices (0.02s)
=== RUN   TestBoardViewLegacyIndexFallback
--- PASS: TestBoardViewLegacyIndexFallback (0.00s)
=== RUN   TestAutoTransitionNoTracks
--- PASS: TestAutoTransitionNoTracks (0.01s)
=== RUN   TestBlockedPanelExtractsViolations
--- PASS: TestBlockedPanelExtractsViolations (0.00s)
=== RUN   TestBlockedPanelViolationsFromProofJSONNotProofMD
--- PASS: TestBlockedPanelViolationsFromProofJSONNotProofMD (0.00s)
=== RUN   TestDeferWritesRuleTwo
--- PASS: TestDeferWritesRuleTwo (0.00s)
=== RUN   TestBoardEnterTransitionsToBlocked
--- PASS: TestBoardEnterTransitionsToBlocked (0.01s)
=== RUN   TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict
--- PASS: TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict (0.01s)
=== RUN   TestBlockedPanelViewProof
--- PASS: TestBlockedPanelViewProof (0.00s)
=== RUN   TestBoardViewShowsMergeBadge
--- PASS: TestBoardViewShowsMergeBadge (0.06s)
=== RUN   TestBoardViewNoMergeBadge
--- PASS: TestBoardViewNoMergeBadge (0.01s)
=== RUN   TestBoardViewLoadsRealOperationalReadinessRelease
--- PASS: TestBoardViewLoadsRealOperationalReadinessRelease (0.34s)
PASS
ok  	github.com/swornagent/sworn/internal/tui	0.987s

$ go test ./... -count=1 -timeout 250s
ok  	github.com/swornagent/sworn/cmd/sworn	39.493s
ok  	github.com/swornagent/sworn/internal/account	10.128s
ok  	github.com/swornagent/sworn/internal/adopt	0.032s
ok  	github.com/swornagent/sworn/internal/agent	0.031s
ok  	github.com/swornagent/sworn/internal/baton	1.063s
?   	github.com/swornagent/sworn/internal/baton/schemas	[no test files]
ok  	github.com/swornagent/sworn/internal/bench	1.375s
ok  	github.com/swornagent/sworn/internal/board	0.139s
ok  	github.com/swornagent/sworn/internal/captain	0.033s
ok  	github.com/swornagent/sworn/internal/command	0.007s
ok  	github.com/swornagent/sworn/internal/config	0.031s
ok  	github.com/swornagent/sworn/internal/db	1.226s
ok  	github.com/swornagent/sworn/internal/design	0.028s
ok  	github.com/swornagent/sworn/internal/designaudit	0.025s
ok  	github.com/swornagent/sworn/internal/designfit	0.024s
ok  	github.com/swornagent/sworn/internal/ears	0.009s
ok  	github.com/swornagent/sworn/internal/gate	0.102s
ok  	github.com/swornagent/sworn/internal/git	0.329s
ok  	github.com/swornagent/sworn/internal/implement	0.500s
ok  	github.com/swornagent/sworn/internal/journey	0.066s
ok  	github.com/swornagent/sworn/internal/ledger	0.027s
ok  	github.com/swornagent/sworn/internal/lint	0.148s
ok  	github.com/swornagent/sworn/internal/mcp	0.139s
ok  	github.com/swornagent/sworn/internal/memory	1.259s
ok  	github.com/swornagent/sworn/internal/model	2.035s
ok  	github.com/swornagent/sworn/internal/orchestrator	0.005s
ok  	github.com/swornagent/sworn/internal/prompt	0.014s
ok  	github.com/swornagent/sworn/internal/reqvalidate	0.018s
ok  	github.com/swornagent/sworn/internal/reqverify	0.011s
ok  	github.com/swornagent/sworn/internal/router	0.072s
ok  	github.com/swornagent/sworn/internal/rtm	0.010s
ok  	github.com/swornagent/sworn/internal/run	4.916s
ok  	github.com/swornagent/sworn/internal/scheduler	0.176s
ok  	github.com/swornagent/sworn/internal/spec	0.008s
ok  	github.com/swornagent/sworn/internal/specquality	0.024s
ok  	github.com/swornagent/sworn/internal/state	0.027s
ok  	github.com/swornagent/sworn/internal/style	0.007s
ok  	github.com/swornagent/sworn/internal/supervisor	0.972s
ok  	github.com/swornagent/sworn/internal/telemetry	0.319s
ok  	github.com/swornagent/sworn/internal/templates	0.006s
ok  	github.com/swornagent/sworn/internal/tui	1.277s
?   	github.com/swornagent/sworn/internal/verdict	[no test files]
ok  	github.com/swornagent/sworn/internal/verify	0.022s
```

## Reachability artefact

- **Type**: recorded terminal session (`tmux capture-pane` against a real, live-run `sworn` binary — not a synthetic fixture, not just a unit test)
- **Path**: `docs/release/2026-07-01-render-drift-reconciliation/S02-tui-oracle-migration/reachability-tui-capture.txt`
- **User gesture**: built `./bin/sworn` from this worktree's HEAD, ran it live inside a 220x50 tmux pane rooted at this worktree's repo root, navigated the release list down to the real, committed `2026-06-30-sworn-operational-readiness` release (5 tracks, `board.json`-backed), pressed Enter, and captured the resulting screen. The board pane shows all 5 real tracks (`T1-operational-unblock` .. `T5-model-pricing-registry`) and all 6 real slices with their live `status.json` states (`verified`) and last-updated timestamps. Before this slice, the identical gesture would have silently rendered zero tracks with no error — `sworn render` no longer emits the `tracks:` frontmatter `LoadBoard` used to hand-parse.
- **Also covered by an integration test**: `TestBoardViewLoadsRealOperationalReadinessRelease` (`internal/tui/tui_test.go`) drives the same integration point (`Model.handleReleasesKey`'s `enter` case → `BoardView.LoadBoard`) programmatically against this same real repo root and release, asserting all 5 track IDs and non-`unknown` slice states — kept alongside the manual capture per Rule 1 ("leaf-level unit tests are fine in addition; they cannot be the sole proof of life").

## Delivered

- **AC-01** — `BoardView.LoadBoard` (`internal/tui/board.go`) reads `board.json` via `internal/board.ReadBoard` instead of hand-parsing `index.md`'s `tracks:` YAML frontmatter. Evidence: `TestBoardViewShowsSlices` (the exact bug-reproduction fixture, now green) and `TestBoardViewLoadsRealOperationalReadinessRelease` (live-repo reachability test).
- **AC-02** — `LoadBlockedView` (`internal/tui/blocked.go`) resolves `worktree_path` via `board.ReadBoard`, matching the track whose ID equals the slice's `status.json.track`, instead of a frontmatter parse that silently fell back to `repoRoot`. Evidence: `TestDeferWritesRuleTwo`, which now asserts `bv.worktreePath` equals the `board.json`-sourced path explicitly.
- **AC-03** — `ExtractViolations` reads a slice's `proof.json.not_delivered` array directly; the `proof.md` regex scraper is deleted (no fallback). Evidence: `TestBlockedPanelExtractsViolations` (new signature) and `TestBlockedPanelViolationsFromProofJSONNotProofMD` (a decoy `proof.md` with `## Violations` bullets and a `LEGACY-SCRAPE-MARKER` is present in the same slice dir and never surfaces — proving the scrape path is fully retired).
- **AC-04** — every test in `tui_test.go` that previously hand-authored `tracks:` YAML frontmatter and drove `LoadBoard`/`LoadBlockedView` through it now builds its fixture via the new `writeBoardFixture` helper (calls the real `board.WriteBoard` path, validated against `board-v1`): `TestBoardViewShowsSlices`, `TestAutoTransitionNoTracks`, `TestDeferWritesRuleTwo`, `TestBoardEnterTransitionsToBlocked`, `TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict`, `TestBlockedPanelViewProof`, `TestBoardViewShowsMergeBadge`, `TestBoardViewNoMergeBadge`. Exactly one dedicated test, `TestBoardViewLegacyIndexFallback`, is kept on the legacy frontmatter-only shape (AC-06).
- **AC-05** — `TestBoardViewLoadsRealOperationalReadinessRelease` plus the live `tmux`-recorded manual smoke described above. See "Reachability artefact".
- **AC-06** — `TestBoardViewLegacyIndexFallback` writes ONLY a legacy `tracks:` frontmatter `index.md` (no `board.json`) and asserts `LoadBoard` still populates tracks via `board.ReadBoard`'s `migrateFromIndex` lazy-migration fallback — unmodified, pre-existing behaviour.
- **AC-07** — `go build ./...` exits 0; `go test ./internal/tui/...` passes (0 failures); `go vet ./internal/tui/...` clean; `gofmt -l` empty on all three touched files; full `go test ./...` (39 packages) passes with no cross-package regression.

## Not delivered

None. Every acceptance check is delivered.

## Divergence from plan

- Design review pin 1 (proof-visibility theme scope check): `DC-1`'s local `struct{ NotDelivered []string }` read of `proof.json` in `blocked.go` is confirmed as a narrow, spec-scoped fix for AC-03 only, not a stand-in for the future proof-panel theme (`project_proof_visibility_theme` memory) — recorded explicitly here so that later release isn't surprised by this ad hoc reader pattern needing to be unified or replaced.
- Design review pin 2 (board-v1 shape cross-check): already independently verified by the Captain against live code before implementation (`StringRelease` emits the canonical object form per `board_release_test.go:TestStringRelease_EmitsCanonicalObject`) — no implementation action was needed; citing the confirmation here per the review's suggested acknowledgement.
- Design review pin 3 (`tui_test.go` touchpoint overlap with sibling S03-tui-chrome-rework): no design change made. S03 is still `planned`; single serial implementer per track worktree means S02 lands first and S03's implementer inherits the regenerated `writeBoardFixture` helper and converted fixtures without surprise.
- AC-04 was applied per its literal text ("any sibling test using a hand-written index.md fixture with the legacy tracks: YAML shape SHALL be regenerated") rather than design.md's narrower named list of 5 tests: all 8 tests that hand-authored `tracks:` frontmatter and drove `LoadBoard`/`LoadBlockedView` were converted, and one NEW dedicated test (`TestBoardViewLegacyIndexFallback`) was added rather than repurposing an existing test — matching design.md's "keep one dedicated test" intent for AC-06.
- Manually driving the live TUI via `tmux` to capture the AC-05 reachability artefact triggered `board.ReadBoard`'s designed lazy-migration side effect (DC-2) for two other, unrelated releases (`2026-06-15-e2e-turnkey-loop`, `2026-06-16-fidelity-layer`) as I navigated past them during manual testing. Both stray `board.json` files were deleted before committing — confirmed via a clean `git status --short` except the intended reachability artefact — since they are outside this slice's declared scope.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S02-tui-oracle-migration 2026-07-01-render-drift-reconciliation
release-verify.sh
  slice:       S02-tui-oracle-migration
  slice dir:   docs/release/2026-07-01-render-drift-reconciliation/S02-tui-oracle-migration
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  FAIL  spec.md missing
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  could not determine integration branch from docs/release/2026-07-01-render-drift-reconciliation/index.md; skipping drift check

== Diff vs start_commit (verifier base) ==
  diff base: start_commit 622f118ef3fda5581d332fdd76a80f39432de763
  PASS  4 file(s) changed vs diff base
  (first 20)
    docs/release/2026-07-01-render-drift-reconciliation/S02-tui-oracle-migration/status.json
    internal/tui/blocked.go
    internal/tui/board.go
    internal/tui/tui_test.go

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
  PASS  deferrals (proof 'Not delivered' + spec 'Out of scope') carry concrete tracking refs
  PASS  proof.md 'Files changed' count (~4) consistent with diff vs start_commit (4)

== Frontmatter YAML safety ==

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 19
  checks failed: 1

FIRST-PASS FAIL
Address the failures above before invoking the LLM verifier session.
See /home/brad/.claude/baton/adversarial-verification.md for the verifier protocol.
```

The single FAIL (`spec.md missing`) is a known false negative for `spec-v1`
(`spec.json`) slices — this repo's canonical spec format for this release —
not a real gap. Consistent with S01/S04's precedent (`feedback_releaseverify_specmd_false_fail`
memory): no verified sibling slice in this repo's current-schema releases
carries a `spec.md` either. No `spec.md` was manufactured to silence this
check. Every other applicable check PASSED, including all 7 required
`proof.md` structural sections and the file-count cross-check against the
diff. Treating first-pass as green given the single FAIL is the documented,
pre-existing spec.md/spec.json false negative and every substantive,
applicable check passed — the same posture S01 and S04 took for this exact
gap.
