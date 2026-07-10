# Design TL;DR — S11-baton-revendor

Release: `2026-06-28-driver-contract` · Track: `T7-baton-revendor` (first of two; S12 owns the data migration) · State on write: `planned → design_review`

## User outcome (restated)

Pin sworn's vendored Baton protocol to the **v0.9.0** upstream tag (`fc497b4`) across
both embed roots, adopt the reconciled schemas (required `in_scope`/`out_of_scope`,
`verifier-verdict-v1`, `acknowledged_by`, `schema_version` retirement), flip the
quadrant enum from `chore`/`epic` to `quick`/`beast` with **transitional** acceptance
of the retired names, and stop persisting a track's worktree branch/path/state —
compute them from `(release, track-id)` + git ancestry (sworn#80). This is the code
half of sworn#48 + all of sworn#80; **S12 migrates the live records and tightens.**

## Grounding (verified live this session)

- Current pin: `internal/adopt/baton/VERSION` → `baton-protocol: v0.6.3` @ `a5aca64`. Target: **v0.9.0**, tag `fc497b4811adc6aa88afce9141b618363382071f` (annotated tag object `82c4f78…`; the commit is `fc497b4`), present in `/home/brad/projects/baton`.
- **Two independent sync pipelines**, not one:
  1. `internal/baton` Vendor pipeline (`source.go` `batonFileMappings` + `vendor.go` `Transform`) covers rules (`internal/adopt/baton/rules/01..11`), role-prompts (`internal/prompt/*.md`), protocol docs (`internal/prompt/baton/*`), `architecture.json`, and the concatenated `rules.md`. It does **NOT** map schemas and does **NOT** map `commands/`.
  2. Schemas under `internal/baton/schemas/*.json` are embedded via `embed.go`/`SchemaMap` and synced by a **separate** mechanism (`fetch.go`/`version.go`/`validate_schema.go`), independent of `batonFileMappings`.
- Quadrant: `internal/state/state.go` `Quadrant()` returns `chore`/`grind`/`puzzle`/`epic`; `EffortComplexity.Validate()` uses the derived quadrant as a checksum. Fixtures in `internal/state/effort_complexity_test.go` assert `chore`/`epic` directly.
- board-v1 @ v0.9.0: `tracks[]` items = `{id, slices, depends_on}`, `additionalProperties:false`; top-level `additionalProperties:false`, **no** `schema_version`. The **live** `board.json` for this release still carries per-track `worktree_path`/`worktree_branch`/`state` **and** top-level `schema_version` (e.g. T1 track object has all three).
- Worktree derivation nuance (spec R-05): `internal/scheduler/worker.go:698 defaultTrackWorktreePath` is **repo-local — a sibling of the release worktree** — added for eval finding 3 (a fired-repo run silently materialised on sworn's tree when the naive `$HOME/projects/<repo>-worktrees` formula was used). `internal/board` currently *parses* worktree/state from `board.json` (`board.go` `BoardTrack{WorktreePath,WorktreeBranch,State}`, `boardTracksToTrackInfos`, `oracle.go readTrackInfos`); `internal/mcp/tools_plan.go:148` already derives the branch inline as `track/<release>/<track-id>`.
- Upstream v0.9.0 `commands/implement-slice.md` + `commands/merge-track.md` are **already** write-isolation-correct ("No board write, no release-wt commit"; derived paths). **sworn does not vendor `commands/` at all** — no such file exists in-repo.
- AC-09 target (track-branch-first `ReadSliceStatus`) is **already implemented** in `oracle.go` (owner-track → release-wt → HEAD); this AC only locks it against regression.

## Approach, per acceptance criterion

- **AC-01 (pin + re-sync + doctor).** Bump `internal/adopt/baton/VERSION` (`baton-protocol: v0.9.0`, `upstream-sha`, `upstream-digest`, `vendored:` date). Re-run the `internal/baton` Vendor pipeline against the v0.9.0 checkout to re-sync rules/role-prompts/protocol docs byte-identically, and re-sync `internal/baton/schemas/*.json` from the tag via the schema pipeline. `sworn doctor` already reads the pin (`cmd/sworn/doctor.go`); confirm it reports v0.9.0 and passes the "semver not SHA" + "not PIN-STALE" checks.
- **AC-02 (quadrant enum, transitional).** `Quadrant(effort,complexity)` returns `quick` (low/low) and `beast` (high/high); `grind`/`puzzle` unchanged. `Validate()` accepts the stored quadrant when it equals **either** the new canonical name **or** the retired synonym (`chore≡quick`, `epic≡beast`) via an equivalence map, emitting a deprecation warning on the retired form. Update the struct comment (`Quadrant string // "quick"|"grind"|"puzzle"|"beast"`). `TestQuadrant` covers canonical, transitional-accepted, and rejected values.
- **AC-03 (in_scope/out_of_scope writer→reader→schema).** `internal/spec` (`spec.go`) emits both fields (planner content or empty arrays); `internal/implement/spec_record.go` parses + exposes them. Add a writer→reader round-trip test validating the record against the vendored v0.9.0 `spec-v1`.
- **AC-04 (acknowledged_by round-trip).** Write→re-read a status record carrying `acknowledged_by`, assert unchanged, validate against vendored `slice-status-v1`.
- **schema_version retirement.** The spec writer stops emitting `schema_version`; readers/validators accept records **with or without** it (transitional) — the `$schema` URL carries the version. See D1 for how this coexists with `additionalProperties:false`.
- **AC-05 (build + targeted suites green).** Sweep fixtures across `internal/state`, `internal/spec`, `internal/implement`, `internal/run`, `internal/router`, `internal/board`, `internal/scheduler`, `internal/mcp`, `cmd/sworn`. Full `go test -count=1 -timeout 300s ./...` before any state transition (project hazard).
- **AC-06 (board-v1 drops worktree/state).** Vendor board-v1 with `tracks[]` = `{id, slices, depends_on}`. Remove `WorktreePath`/`WorktreeBranch`/`State` from `BoardTrack` (writer stops emitting) so `internal/board` neither reads nor requires them.
- **AC-07 (derive branch/path/state).** New pure helpers in `internal/board` deriving: branch = `track/<release>/<track-id>`; path = the **sibling-of-release-worktree** location (the `defaultTrackWorktreePath` logic, **not** the naive `$HOME` formula); state = `planned` (no branch ref) / `in_progress` (ref exists, not ancestor of `release-wt/<release>`) / `merged` (ancestor). `Oracle.readTrackInfos`/`ReadSliceStatus`/`render.go` call the helpers instead of parsing `board.json`. Point `mcp/tools_plan.go:148` at the shared branch helper. Delete `worker.go`'s now-redundant `defaultTrackWorktreePath` fallback once `internal/board` always returns a populated path (move/share the logic first, then delete — no dead code).
- **AC-08 (command specs don't write board.json).** See D3 — sworn vendors no command specs; the in-repo enforcement surface is the **engine** (AC-06/AC-07: no writer emits track worktree/state).
- **AC-09 (slice-state skew lock).** Add/confirm a regression test asserting an unmerged-but-verified slice reports `verified`, never `planned`, via the existing track-branch-first `ReadSliceStatus`.

## Files intended to touch

Vendor/schema: `internal/adopt/baton/VERSION`, `internal/adopt/baton/rules/*` (re-sync), `internal/prompt/*` + `internal/prompt/baton/*` (re-sync), `internal/baton/schemas/{spec-v1,slice-status-v1,board-v1,journeys-v1,attestations-v1,verifier-verdict-v1}.json`, possibly `internal/baton/source.go` (only if D3 adds a mapping).
Code: `internal/state/state.go` (+ `effort_complexity_test.go`), `internal/spec/spec.go` (+ test), `internal/implement/spec_record.go` (+ test), `internal/board/{board.go,oracle.go,track.go,render.go,index.go}` (+ their tests), `internal/scheduler/worker.go` (+ test), `internal/mcp/tools_plan.go`.
Fixtures: `internal/run/parallel_test.go`, `internal/router/router_test.go`, `cmd/sworn/{doctor_test.go,regress_test.go}`, board/render testdata.

## Design decisions / pins for the Captain

- **D1 — Transitional tolerance vs `additionalProperties:false` (Type-1, escalate).** v0.9.0 board-v1 and spec-v1 set `additionalProperties:false`, drop `schema_version`, and board-v1 drops per-track worktree/state — but **S11 does not migrate the live records** (S12 owns that). The moment S11 lands, existing `board.json` (this release's own included) carries `worktree_path`/`worktree_branch`/`state` + `schema_version`, and existing `spec.json`/`status.json` carry `schema_version`; strict `additionalProperties:false` validation would reject them, and the quadrant checksum would see `epic` where it derives `beast`. Because S11+S12 merge as one track unit the **integration branch** never sees the intermediate state, but **in-track** gates (`sworn verify`, `sworn board`, `designfit`, fixtures) operating on the still-live records will. **Proposed resolution:** vendor the strict v0.9.0 schemas faithfully (end state) **but** keep sworn's readers/validators transitionally tolerant — Go struct parsing already ignores unknown fields; for JSON-Schema validation, do not enforce `additionalProperties:false`/`schema_version`-absence against un-migrated records until S12 (e.g. tolerant validation path or pre-normalise), mirroring the quadrant `chore≡quick`/`epic≡beast` equivalence. Requesting a human decision on the tolerance mechanism (tolerant-validate vs. normalise-before-validate) since it is architecturally significant and load-bearing for the in-flight release.
- **D2 — Worktree-path derivation source of truth (Type-1, memory-cited).** The `internal/board` path helper MUST reuse `worker.go`'s repo-local sibling-of-release-worktree logic (eval finding 3), NOT track-mode.md's plainly-documented `$HOME/projects/<repo>-worktrees/...` formula. Plan: lift that logic into a shared `internal/board` helper, point both `internal/board` and `worker.go` at it, then delete `worker.go`'s redundant fallback. Flagging because re-deriving the naive convention would reintroduce a real prior cross-repo collision.
- **D3 — AC-08 command specs not vendored in sworn (escalate).** AC-08/in-scope name `internal/adopt/baton/rules/` command specs `implement-slice.md`/`merge-track.md`, but sworn's `rules/` holds only the numbered rule docs and `batonFileMappings` maps no `commands/`. Upstream v0.9.0's command specs are already write-isolation-correct. **Recommendation:** treat AC-08 as satisfied in-repo by the **engine** change (AC-06/AC-07 ensure no sworn writer stamps track worktree/state to `board.json`) plus a Rule 2 note that the command-spec prose lives in the private `~/.claude` harness (out of this repo), rather than expanding scope to vendor `commands/`. Requesting the Captain's ruling on satisfied-by-engine vs. add-`commands/`-mapping.
- **D4 — Quadrant equivalence map (Type-2, mechanical).** `chore≡quick`, `epic≡beast` in `Validate()` with deprecation warning; removed by S12. Default: proceed.
- **D5 — Two sync pipelines (Type-2).** AC-01 requires driving both the `batonFileMappings` Vendor pipeline and the separate schema-embed sync; re-sync must be byte-identical to the tag for the non-schema files. Default: proceed, verifying `sworn doctor` + vendor diff are clean.

## Design-level risks

- Strict-reader/schema regressing OTHER packages' fixtures (the S05 lesson) — full `go test ./...` is the backstop; touchpoints enumerate the sweep.
- Newline-eating edit corruption on the many `.go` edits — gofmt/vet + grep for code fused onto `//` lines before transition.
- Scope creep into baton#55's remaining board-v1 conformance gaps — R-04/out_of_scope bound the change to exactly worktree/state derivation + schema_version + quadrant.

## AC → planned change traceability

AC-01→VERSION+both sync pipelines+doctor · AC-02→state.go Quadrant/Validate+test · AC-03→spec.go+spec_record.go+round-trip test · AC-04→status acknowledged_by round-trip test · AC-05→build+targeted suites+fixture sweep · AC-06→board-v1 schema+BoardTrack struct · AC-07→internal/board derivation helpers+worker.go fallback deletion+mcp/tools_plan.go · AC-08→engine (AC-06/07) + D3 ruling · AC-09→oracle track-branch-first regression test. schema_version retirement → spec writer + tolerant readers (D1).
