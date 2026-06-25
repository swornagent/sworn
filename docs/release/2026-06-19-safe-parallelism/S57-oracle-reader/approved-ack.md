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
