# Design TL;DR — S11-baton-revendor

**Release:** 2026-06-28-driver-contract · **Track:** T7-baton-revendor · **Slice order:** S11 → S15 → S12
**Retarget:** v0.9.0 → **v0.10.0** (baton tagged v0.10.0 mid-build, 2026-07-11). Prior v0.9.0 design/review/ack removed; status reset to `planned`.
**Governing pin:** baton-protocol **v0.10.0**, upstream commit **a5ab2aa** (annotated-tag object `fa591989…`), superseding the live pin **v0.6.3 @ a5aca64** (`internal/adopt/baton/VERSION`).

## User outcome (from spec.json)
sworn's vendored Baton protocol is pinned to v0.10.0 in both embed roots; the graded schema set carries the reconciled shapes (required `in_scope`/`out_of_scope`, published `verifier-verdict-v1`, `board-v1` with `worktree_path`/`worktree_branch`/`state` removed from `tracks[]`); the two NEW v0.10.0 schemas (`contracts-v1`, `assembly-proof-v1`) are vendored **advisory-only** (stored, byte-matched to the published `$id`, doctor-declarable — **not graded here**); the quadrant enum moves `chore→quick` / `epic→beast` with a strict `Validate` fed by a read-path `normalise()` shim; `acknowledged_by` round-trips; and sworn#80 lands — a track's worktree branch/path/state are **computed** from `(release, track-id)` + git ancestry, never persisted to `release-wt`'s board.json.

## Approach (per acceptance criterion)

### AC-01 — v0.10.0 pin + byte-identical re-sync of both embed roots
- Rewrite `internal/adopt/baton/VERSION`: `baton-protocol: v0.10.0`, `upstream-sha: a5ab2aa…`, refreshed `upstream-digest`, `vendored:` date. Keep the `rules-added` / `rules-hardened` provenance lines, append the v0.10.0 line.
- Re-sync **embed root A** (`internal/adopt/baton/` — `rules/*`, `README.md`, `architecture.json`) byte-identical from the v0.10.0 tag (RELEASING.md: vendor from the **tag**, never `main`).
- Re-sync **embed root B** (`internal/baton/schemas/`) byte-identical from the tag: `spec-v1` (required `in_scope`/`out_of_scope`, `additionalProperties:false`, no `schema_version`), `slice-status-v1`, `board-v1` (tracks[] shorn of worktree/state), `journeys-v1`, `attestations-v1`, `proof-v1`, `verifier-verdict-v1`.
- `sworn doctor` reports the new pin via the existing `baton.Version()` semver-tag check (`cmd/sworn/doctor.go:432`) — no doctor code change; the assertion is that it prints `on Baton v0.10.0`.

### AC-10 / AC-11 — two NEW advisory schemas (contracts-v1, assembly-proof-v1)
- Copy `schemas/contracts-v1.json` and `schemas/assembly-proof-v1.json` from the v0.10.0 tag into `internal/baton/schemas/`, byte-identical to the published `$id`s (both confirmed present upstream: `https://baton.sawy3r.net/schemas/contracts-v1.json`, `…/assembly-proof-v1.json`). **Do not fork the shape under the same `$id`** (the baton#55 divergence class).
- Add two `//go:embed` vars + two `SchemaMap` entries in `internal/baton/schemas/embed.go` (`ContractsV1`, `AssemblyProofV1`).
- **Advisory-only:** no grader is wired. `sworn lint contracts` / `sworn assemble` are the follow-on contract-edge-gates release (out of scope). A byte-match test asserts each vendored file equals the tag's bytes.
- **⚠ Pin P1 (see below): placement across "both embed roots" is ambiguous — needs Captain ratification.**

### AC-02 + D1 — quadrant `quick`/`beast` with strict Validate + normalise shim
- `internal/state/state.go`: `Quadrant(effort, complexity)` returns `quick` (low/low, was `chore`) and `beast` (high/high, was `epic`); `grind`/`puzzle` unchanged. Update the `Quadrant`/`EffortComplexity.Quadrant` doc comments (state.go:97, :415, :424–425) that still say `chore/epic`.
- `EffortComplexity.Validate` (state.go:446) stays **strictly strict**: it compares against `Quadrant()`, so it now accepts only `{quick,grind,puzzle,beast}` and rejects the retired `chore`/`epic`. **It never weakens** — no tolerance branch inside Validate.
- **D1 normalise() shim (read-path):** a single enumerated `normalise(record)` maps retired `chore→quick` / `epic→beast` **before** Validate/checksum. `TestQuadrant` covers: canonical-accepted, retired-rejected-by-Validate, normalise-maps-then-validates. Shim is deleted wholesale by S12.

### AC-03 — spec writer emits / strict reader accepts in_scope + out_of_scope
- `internal/spec/spec.go` `Spec` struct (currently ID/Text/Type/EARSKeyword + SliceID/Release/UserOutcome/CoversNeeds/AcceptanceCriteria — **no scope fields**): add `InScope []string \`json:"in_scope"\`` + `OutOfScope []string \`json:"out_of_scope"\``, emitted always (planner content or `[]`).
- `internal/implement/spec_record.go` `specRecord`: add matching fields, parse + expose. Round-trip test validates the written record against the vendored v0.10.0 `spec-v1`.

### schema_version retirement (same D1 shim)
- Writer stops emitting `schema_version`; `spec_record.go:18` `SchemaVersion int \`json:"schema_version"\`` is dropped from what the writer produces. `$schema` carries the version.
- **Same normalise shim** strips `schema_version` and reshapes legacy `board-v1` `tracks[]` (drops `worktree_path`/`worktree_branch`/`state`) into canonical v0.10.0 form **before** validating against the strict schema. `additionalProperties:false` is **never relaxed** — genuine drift is still caught during the S11→S12 window. S12 strips on-disk data and removes the shim.

### AC-04 — acknowledged_by round-trip
- Smoke test: write a status record carrying `acknowledged_by`, re-read, assert unchanged + validates against the vendored `slice-status-v1` (sworn#48 item-4). `state.Status` already carries the surrounding fields (state.go:457+); confirm `AcknowledgedBy` maps in the `Deferral` shape.

### AC-06 / AC-07 + D2 — sworn#80: derive worktree branch/path/state, stop persisting
- **Vendored `board-v1` (root B) already drops** `worktree_path`/`worktree_branch`/`state` from `tracks[]` (comes with the AC-01 re-sync; today's vendored schema still declares all three).
- **Derivation helpers** (new, pure): branch = `track/<release>/<track-id>`; path = the **sibling-of-release-worktree** logic `internal/scheduler/worker.go:698 defaultTrackWorktreePath` already carries for eval-finding-3 (repo-local, NOT the naive `$HOME/projects/<repo>-worktrees/…` convention); state = `IsAncestor` check — no branch ref → `planned`, branch exists & not ancestor of `release-wt/<release>` → `in_progress`, ancestor → `merged`.
- **D2 (ratified):** the path helper **reuses** worker.go's derivation (extract to a shared exported helper, e.g. `board`/a small shared pkg), not a re-derived naive formula. The state helper **reuses** the existing `internal/git/git.go:153 Repo.IsAncestor` (`git merge-base --is-ancestor`) — the same primitive `cmd/sworn/merge.go:247` and `router.go:458` already use.
- **Rewire `internal/board`:** `boardTracksToTrackInfos` (board.go:242) and the `track.go` index.md regex fallbacks (`reTrackWorktreePath`/`reTrackWorktreeBranch`/state, track.go:37–170) stop sourcing these from persisted data; `Oracle.readTrackInfos` / `ReadSliceStatus` / `render.go` compute them. `BoardTrack` (board.go:87–89) drops the three JSON fields.
- **Delete** worker.go's now-redundant `defaultTrackWorktreePath` fallback once `internal/board` always returns a populated path (AC-07 final clause).
- **Point `internal/mcp/tools_plan.go:148`** (already inline `fmt.Sprintf("track/%s/%s", …)`) at the shared branch helper — single source of truth (R-05).

### AC-08 — write-isolation SATISFIED-BY-ENGINE (D5, ratified)
- No engine code path writes a track's worktree identity/state to `release-wt`'s board.json — owned by the AC-06/AC-07 removal. `/merge-track`'s merge commit stays the only release-wt write attributable to a track.
- sworn vendors **no `commands/` dir** (adopt.go embeds only `rules/*` + README + VERSION + architecture.json — confirmed). The implement-slice.md/merge-track.md prose lives in the private `~/.claude` harness + upstream baton (ADR-0010 boundary). Command-spec prose edit tracked as **baton#61**, out of this binary's scope.
- **Evidence test:** assert sworn's vendored `internal/adopt/baton/rules/` contains no `implement-slice.md`/`merge-track.md`.

### AC-05 / AC-09 — build, targeted suites, fixture sweep, board-skew regression lock
- Fixture sweep for the quadrant rename across `internal/state`, `internal/run/parallel_test.go`, `internal/router/router_test.go`, `cmd/sworn/doctor_test.go`, `cmd/sworn/regress_test.go` (chore→quick / epic→beast).
- Board fixtures dropping worktree/state from `tracks[]` across `internal/board/*_test.go`.
- **AC-09** is already implemented by `oracle.go`'s track-branch-first `ReadSliceStatus` (owner branch → release-wt → HEAD); add a **regression test** asserting an unmerged-but-verified slice reports `verified`, never `planned` (locks fired-brief Defect 2). No production change.

## Files to touch (⊆ spec touchpoints)
`internal/adopt/baton/VERSION`, `internal/adopt/baton/rules/*`, `internal/adopt/baton/README.md`, `internal/adopt/baton/architecture.json` · `internal/baton/schemas/{spec-v1,slice-status-v1,board-v1,journeys-v1,attestations-v1,proof-v1,verifier-verdict-v1,contracts-v1,assembly-proof-v1}.json` + `embed.go` · `internal/state/state.go` (+ effort_complexity/normalise tests) · `internal/spec/spec.go` (+test) · `internal/implement/spec_record.go` (+test) · `internal/board/{board.go,track.go,oracle.go,render.go,index.go}` (+ their `_test.go` + a shared derivation helper) · `internal/scheduler/worker.go` (delete fallback) (+test) · `internal/mcp/tools_plan.go` · fixtures in `internal/run`, `internal/router`, `cmd/sworn`.

## Decision list

| # | Decision | Type (Rule 9) | Status |
|---|----------|---------------|--------|
| **D1** | **Normalise-before-validate**, not tolerant-validate: one read-path `normalise()` shim maps retired `chore→quick`/`epic→beast`, strips `schema_version`, reshapes legacy board `tracks[]`; `Validate` + `additionalProperties:false` stay strictly strict; vendored schemas byte-identical to the tag; S12 deletes the shim. | Type-1 (transition mechanism, wide) | **Ratified** — Coach 2026-07-11 (in spec rationale) |
| **D2** | Track-path derivation **reuses** worker.go's sibling-of-release-worktree logic (eval finding 3), NOT the naive `$HOME` convention; state derivation reuses `git.Repo.IsAncestor`. | Type-1 (correctness-critical) | **Ratified** — in spec R-05 / AC-07 |
| **D3** | sworn#80: `board-v1` `tracks[]` **loses** `worktree_path`/`worktree_branch`/`state` as persisted fields (upstream-first, v0.10.0); `internal/board` computes them; scope bounded to exactly these 3 fields (baton#55 owns the rest of board-v1 conformance). | Type-1 (schema contract, wide blast radius) | **Ratified** — AC-06/AC-07 + baton v0.10.0 |
| **D4** | The two new schemas are **VENDORED-ADVISORY**: stored + byte-matched + doctor-declarable, **not graded**. No `lint contracts` / `assemble` grader wired here. | Type-1 (contract surface) | **Ratified** — AC-10/AC-11 + task brief |
| **D5** | AC-08 **satisfied-by-engine**: sworn vendors no `commands/`; command-spec prose is baton#61. sworn's obligation is Go behaviour only (no board.json worktree/state write). | Type-2 (boundary already set by ADR-0010) | **Ratified** — Coach 2026-07-11 |
| **D6** | Do **not** write records in `quick`/`beast` on disk yet (binary accepts `chore/grind/puzzle/epic` until this slice lands; sworn#90). This slice's own status/spec quadrant fields stay legacy-valid until merge; the enum flip + normalise land atomically in-track. | Type-2 (sequencing) | Proposed default |

## Design-level pins for the reviewer

- **P1 (ESCALATE — needs Captain decision). "Both embed roots" placement of the two advisory schemas.** AC-10/AC-11 say vendor `contracts-v1`/`assembly-proof-v1` "in **both embed roots**", but the two roots hold **different content**: root A (`internal/adopt/baton/`) carries `rules/*` + README + VERSION + architecture.json and **no schemas dir at all**; root B (`internal/baton/schemas/`) is the sole home of every existing `spec-v1`/`board-v1` schema and the graded set doctor declares. **Recommendation:** vendor the two new schemas into **root B only** (where "the existing spec-v1/board-v1 schemas live", AC-10's operative clause), and re-sync root A's rules/docs/architecture from the tag without creating an `internal/adopt/baton/schemas/` mirror — unless the intent is to newly scaffold schemas into consumer repos via `sworn adopt`, which is a larger, separate change. Requesting explicit ratification of "root B only" vs "create an adopt/baton schemas mirror". This is Type-1 (contract-surface / adoption-bundle shape).
- **P2 (mechanical). Shared derivation helper home.** worker.go's path logic + the branch/state helpers need a single home that both `internal/board` and `internal/scheduler`/`internal/mcp` import without an import cycle. Proposed: a small exported helper in `internal/board` (consumers already depend on it) or a leaf `internal/track` pkg. Flagging so the reviewer can veto the package choice before code.
- **P3 (memory-cited). Full-suite backstop is mandatory.** A tightened reader/schema regressing fixtures in other packages is the S05-strict-reader scar (spec R-03) and the newline-eating-edit scar both apply; the plan runs `gofmt -l` + `go vet` + full `go test -count=1 -timeout 300s ./...` before any state transition, not just the targeted AC-05 subset.
- **P4 (advisory). Effort/complexity self-classification.** spec sets `quadrant: epic` with `confirmed_by_implementer:false`; folding sworn#80 keeps this at high/high. I concur it is `beast`-class (post-rename) — a vendor bump + enum migration + a schema-contract removal with a ~12-file consumer sweep and a shared-helper extraction. Will set `confirmed_by_implementer:true` at implementation start (recorded in legacy `epic` on disk per D6 until the enum flips in-track).

## AC → change traceability
AC-01→VERSION+both-root re-sync+doctor · AC-02→state.go Quadrant/Validate+normalise · AC-03→spec.go+spec_record.go scope fields+round-trip · AC-04→acknowledged_by smoke · AC-05→build+targeted suites+fixture sweep · AC-06→board-v1 tracks[] fields removed + board readers · AC-07→derivation helpers (path=worker.go logic, state=IsAncestor)+worker.go fallback delete+mcp helper · AC-08→no-commands evidence test+AC-06/07 write-isolation · AC-09→ReadSliceStatus regression lock · AC-10→contracts-v1 vendor+embed+byte-match · AC-11→assembly-proof-v1 vendor+embed+byte-match.

## Divergence from plan
None. D1–D5 are the spec's ratified decisions restated; D6 is a sequencing default; P1 surfaces a spec-wording ambiguity for ratification rather than resolving it silently (Rule 2 / Rule 9).
