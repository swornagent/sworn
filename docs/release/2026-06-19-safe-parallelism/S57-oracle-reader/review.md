# Captain review — S57-oracle-reader
Date: 2026-07-14
Design commit: 02065f998e6578412a165098054b5c125a587ba2

## Pins

1. [mechanical] §2.decision5 / spec AC — `verification.routing` field does not exist in `state.Verification`
   What I observed: The spec (line 41) says the blocked routing owner is "taken from a `verification.routing` field when present, else inferred." Design §2 D5 says "if `verification.routing` is absent, `blocked` → `needs_planner`." But `internal/state/state.go:78-84` defines `Verification` with only `Result`, `VerifierSessionID`, `VerifierVerdictAt`, `VerifierWasFreshContext`, `Violations` — no `Routing` field. No `routing` key appears in any status.json in the repo, and `captain-route.sh` does not write one. The field is a spec fiction: the reader will always take the inference path because the field can never be populated by the current state struct.
   What to ask the implementer: Add a `Routing string \`json:"routing,omitempty"\`` field to `state.Verification` so the "when present" path is real, OR amend the spec to say the field is a future addition and the reader only implements inference for now. If adding the field, `internal/state/state.go` must be added to `planned_files` (it is currently absent). This is mechanical because the fix is unambiguous: either add the field to the struct (and to planned_files) or document that only inference is implemented.

2. [mechanical] §3 / status.json — `internal/state/state.go` missing from `planned_files`
   What I observed: `status.json` `planned_files` lists `internal/board/oracle.go`, `internal/board/oracle_test.go`, `cmd/sworn/board.go`, `cmd/sworn/board_test.go`. But the spec requires reading `verification.result`, `verification.violations[]`, and (per pin 1) potentially `verification.routing` from status.json. The `state.Verification` struct at `internal/state/state.go:78` is the deserialisation target. If pin 1's fix is "add the Routing field," then `internal/state/state.go` must be in `planned_files` or Gate 2 (lint touchpoints) will fail. Even without pin 1, the oracle reader will likely need to reference `state.Status` / `state.Verification` types — but since it reads via `git show` (raw JSON), it may unmarshal into a local struct instead. Confirm whether the oracle unmarshals into `state.Status` (then `internal/state/` is a read-only import, not a touchpoint) or a local struct (then no dependency). Either way, if `state.Verification` gains a `Routing` field, `internal/state/state.go` is a write touchpoint and must be declared.
   What to ask the implementer: If adding `Routing` to `state.Verification`, add `internal/state/state.go` to `planned_files`. If the oracle uses its own local struct for JSON unmarshalling, no change needed — but state it explicitly in the design.

3. [mechanical] §2 / status.json — `design_decisions` absent from status.json (8th+ recurrence)
   What I observed: `status.json` has no `design_decisions` field. The `designfit` gate (`internal/designfit/designfit.go:91-104`) checks `impliesType1Work` by prefix-matching `planned_files` against `{cmd/sworn/, internal/state/, internal/verdict/}`. S57's `planned_files` includes `cmd/sworn/board.go`, which matches `cmd/sworn/`. With empty `design_decisions`, the gate records a violation: "implies Type-1 work but design_decisions is empty." This has been the single most recurring pin across the release (S19, S21, S23, S48, S49, S60, S20, S61, S50, S16 — 10+ prior occurrences).
   What to ask the implementer: Add a `design_decisions` array to `status.json` with at least one Type-2 entry (e.g., D1 "git operations via internal/git" as Type-2, not architecturally significant). The design's 5 decisions are all Type-2 (reversible, local) — record them as such. This is a mechanical status.json fix.

4. [mechanical] §2.decision2 / spec Risks — test strategy "real git repos in temp dirs" vs spec AC "fakeable git layer"
   What I observed: Design §2 D2 says "fakeable without a new interface. The existing `internal/git/git_test.go` pattern (real `git init`, `git commit` in `t.TempDir()`) gives us deterministic refs." But spec AC (line 66) says "Transient-read retry: a status.json that reads empty once then non-empty resolves to the non-empty state (fake git layer with a one-shot empty)." The spec explicitly calls for a "fake git layer" for the transient-read retry test. The design's "no GitReader interface needed" decision means the transient-retry test must use a real git repo with some mechanism to produce a one-shot empty read — which is non-trivial with real git (you can't make `git show` return empty once then non-empty without a race condition or a wrapper). The design should state how the transient-retry test is implemented without a fakeable layer.
   What to ask the implementer: Clarify how `TestTransientReadRetry` is implemented with real git repos. Options: (a) introduce a thin `gitContentReader` interface in `oracle.go` that `git show` satisfies in production and a fake satisfies in the transient-retry test (this is the spec's "fake git layer"), or (b) use a real repo with a commit that has an empty status.json followed by a commit with content, and test the retry by simulating the empty read at the Go level (wrapping the read function). State which approach in the design.

5. [mechanical] §3 / spec — `internal/git/git.go` missing from `planned_files`
   What I observed: Design §3 says "Git plumbing: `internal/git/git.go` + `internal/git/git_test.go` — add `Show` and `CatFileExists` methods." But `status.json` `planned_files` does not list `internal/git/git.go` or `internal/git/git_test.go`. The lint touchpoints gate (S30) will flag this as an undeclared touchpoint. `internal/git/git.go` is owned by T11-infra-safety (S28, merged), so this is a write to a merged track's file — it must be declared.
   What to ask the implementer: Add `internal/git/git.go` and `internal/git/git_test.go` to `planned_files` in `status.json`.

6. [memory-cited] §2.decision1 — git operations via `internal/git` with Dir guard
   What I observed: Design §2 D1 says "reuse the existing package with its `Dir` guard (S28) rather than spawning raw `git`; the chokepoint protects against operating in the wrong worktree." This aligns with [[project_coach_loop_oracle_architecture]] which states the oracle reader must "stage the canonical git-tracked status.json path, never an existence-probed docs/ path" and that the reference reader resolves from git refs. The Dir guard is the S28 mutation guard (Rule 11) that prevents operating in the ambient cwd.
   What to ask the implementer: Ack — the decision is sound. The `internal/git.Repo` already has the Dir guard; the new `Show`/`CatFileExists` methods inherit it automatically since they funnel through `r.run()`.
   Citation: [[project_coach_loop_oracle_architecture]]

7. [memory-cited] §2.decision3 / spec — docs prefix probe via `git cat-file -e`
   What I observed: Design §2 D3 says "try `docs/release/...` first (this project), then `apps/docs/content/docs/release/...` (Fumadocs projects); the first path that `git cat-file -e` confirms exists wins." This aligns with [[project_coach_loop_oracle_architecture]] which warns: "the bash reconcile shipped broken — it resolved the docs prefix by file-existence (`docs/` first) then `git add docs/...`, but Fumadocs repos make `docs/` a SYMLINK to `apps/docs/content/docs/` and git silently refuses to stage beyond a symlink." The design's approach of using `git cat-file -e` (checking the git-tracked path, not filesystem existence) correctly avoids the symlink trap.
   What to ask the implementer: Ack — the `git cat-file -e` approach is the correct fix for the symlink trap. Confirm the probe checks `<branch>:<path>` not just `<path>` (i.e., it checks the ref's tree, not the working tree).
   Citation: [[project_coach_loop_oracle_architecture]]

## Summary
Pins: 7 total — 5 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins: 3, 4, 5 (would cause Gate 2 FAIL or test failure if unaddressed)

## Smaller flags (not pins, worth one-line ack)
- The spec's parity AC (line 67) says `--json` output `.slices[].state` must match `release-board-status.sh --json` for non-blocked slices. The reference JSON shape (from `lib/release-board.mjs`) includes `actionable`, `dependsOnTracks`, `owner`, `lastUpdated`, `track` per slice — the design should confirm the Go `ReadBoard` output struct includes these fields for parity, not just `state`.
- The spec mentions `board.ParseTracks` for ownership resolution. The existing `board.ParseTracks` takes a frontmatter body string, not a file path — the oracle will need to read `index.md` from a git ref first, then pass the body to `ParseTracks`. Confirm this two-step read is in the implementation plan.
- The `ReadSliceStatus` return type includes `ResolvedFrom` (spec line 25) — a type indicating which ref level resolved (track-branch / release-wt / working-tree HEAD). This is a new type not in the design's §3 file plan explicitly, but it would live in `oracle.go`. Fine, just noting it's an API surface S58 will consume.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR solid design, keystone port well-scoped. 7 pins + 3 flags:

1. **`verification.routing` field doesn't exist.** `state.Verification` (internal/state/state.go:78) has no `Routing` field. Either add `Routing string \`json:"routing,omitempty"\`` to the struct (and add `internal/state/state.go` to planned_files), or amend the spec to say only inference is implemented for now. The "when present" path is dead code without the field.
2. **`internal/state/state.go` touchpoint undeclared.** If you add the Routing field (pin 1), `internal/state/state.go` must be in `planned_files`. If the oracle uses its own local struct for JSON unmarshalling instead of `state.Status`, state that explicitly in the design — then no touchpoint needed.
3. **`design_decisions` absent from status.json.** 8th+ recurrence this release. `cmd/sworn/board.go` in `planned_files` triggers `impliesType1Work()` → designfit gate fails closed. Add a `design_decisions` array with the 5 design decisions as Type-2 entries.
4. **Transient-retry test needs a fakeable layer.** Spec AC says "fake git layer with a one-shot empty" but design D2 says "no GitReader interface needed." Clarify how `TestTransientReadRetry` produces a one-shot empty read with real git. A thin `contentReader` interface in `oracle.go` (production: `git show`, test: fake) is the spec's intent.
5. **`internal/git/git.go` missing from planned_files.** Design §3 says you'll add `Show`/`CatFileExists` to `internal/git/git.go` + `git_test.go`, but neither is in `status.json` `planned_files`. Add both.
6. **D1 (git via internal/git Dir guard) ack.** Aligns with [[project_coach_loop_oracle_architecture]] — the Dir guard is the S28 mutation guard. New methods inherit it via `r.run()`.
7. **D3 (docs prefix probe via `git cat-file -e`) ack.** Aligns with [[project_coach_loop_oracle_architecture]] symlink-trap warning. Confirm the probe checks `<branch>:<path>` (ref tree), not working-tree file existence.

Flags (not pins): (a) confirm `ReadBoard` output struct includes `actionable`, `dependsOnTracks`, `owner`, `lastUpdated`, `track` for parity with `release-board-status.sh --json`; (b) `board.ParseTracks` takes a frontmatter body string — oracle must read `index.md` from git ref first, then pass body to `ParseTracks`; (c) `ResolvedFrom` return type is a new API surface for S58 — fine, noting it.

§2 decisions D1, D3 ack (memory-cited). D2, D4, D5 ack. §6 empty — ack.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 7 pins are apply-inline corrections (status.json fixes, planned_files additions, test-strategy clarification, struct field addition); no design re-thinking needed before code is safe.
-->