# Journal — S11-baton-revendor

## 2026-07-11 — implementation start (design-review gate satisfied)

Design review PROCEED is committed (captain-proceed.md, Coach 2026-07-11). Critical
escalate Pin 1 ratified: EXPAND board pure-plan to the release level. Recorded D1-D6 +
the Pin-1 release-level decision in status.json.design_decisions (D1-D4 + Pin-1 Type-1,
D5/D6 Type-2). Flipped to in_progress; quadrant stays legacy `epic` on disk per D6.

### Investigation findings (durable capture, Rule 3)

- **Pin target.** baton tag `v0.10.0` peels to commit `a5ab2aaab23429961cb17871c20254ecac4e1436`
  (annotated-tag object `fa591989…`). codeload tarball digest =
  `sha256:68cae0347d77ee2082acadbf010b56bff498b90990018285460d6fa6bb7a7ec0`
  (`https://codeload.github.com/sawy3r/baton/tar.gz/refs/tags/v0.10.0`, 197810 bytes).
  Live pin was `v0.6.3 @ a5aca64`.

- **Vendoring mechanism (root A + prompts).** `internal/baton` Vendor() pipeline driven by
  `sworn baton vendor <src>` maps `batonFileMappings` (source.go): `baton/<rule>.md` ->
  `internal/adopt/baton/rules/NN-<rule>.md` (renamed, content byte-identical after Transform),
  `baton/README.md` -> `internal/adopt/baton/README.md`, `baton/architecture.json` ->
  `internal/adopt/baton/architecture.json`, AND role-prompts/docs -> `internal/prompt/*`.
  Transform() replaces bash/node script refs with sworn-native commands (fail-closed).
  VERSION (`internal/adopt/baton/VERSION`) is NOT in the mapping — maintained separately
  (`baton-protocol:`/`vendored:` manual; `upstream-sha`/`upstream-digest` via WriteUpstreamPin).

- **Schemas (root B) are NOT in the Vendor mapping.** `internal/baton/schemas/*.json` are
  byte-identical manual copies from the tag's `schemas/`. embed.go lists each + SchemaMap.
  6 of 7 vendored schemas differ from v0.10.0 (only verifier-verdict-v1 identical).

- **AC-01 scope boundary (DECISION).** AC-01 names exactly two embed roots:
  `internal/adopt/baton` (rules/docs) + `internal/baton/schemas`. `internal/prompt/*` (role
  prompts) is a SEPARATE embed root, NOT named by AC-01, NOT in T7's touchpoint matrix, and
  owned by no track (role-revendor was historically its own slice, S20). So I re-sync root A +
  root B only and leave internal/prompt untouched. Mechanism: run the vendor tool then revert
  internal/prompt (no gate checks it — `sworn baton diff` is not in the automated suite).

- **Normalise strip-set (Pin 4, derived from the real live-record-vs-strict-schema delta):**
  - slice-status-v1: top-level additionalProperties stays TRUE; effort_complexity tightens to
    additionalProperties:false + quadrant enum {quick,grind,puzzle,beast}. => map quadrant
    chore->quick / epic->beast. Load-bearing site: `state.Read` calls EffortComplexity.Validate
    after unmarshal, so a legacy `epic` record FAILS on read once Quadrant() returns beast.
  - spec-v1: additionalProperties:false; drops schema_version; requires in_scope/out_of_scope;
    AC items additionalProperties:false ({id,text,ears_pattern,test_refs} only) — the current
    on-disk spec.json AC items carry `type`/`ears_keyword` and top-level `schema_version`.
    => strip schema_version + AC type/ears_keyword; map effort_complexity.quadrant.
  - board-v1: additionalProperties:false; top-level = $schema/release/tracks only; tracks[] =
    id/slices/depends_on only. => strip top-level schema_version/release_worktree_path/
    release_worktree_branch; strip tracks[].worktree_path/worktree_branch/state.

- **Board derivation (AC-06/07).** BoardRecord emits schema_version + release_worktree_path/
  branch; BoardTrack emits worktree_path/worktree_branch/state; WriteBoard validates on write
  (board.go:171). ReadBoard does NOT validate on read. oracle.readTrackInfos (376) +
  boardTracksToTrackInfos copy worktree/state from the board; ReadReleaseWorktreePath (438)
  reads release_worktree_path. worker.go defaultTrackWorktreePath is the eval-finding-3
  sibling-of-release-worktree derivation to reuse: `filepath.Join(filepath.Dir(releaseWTPath), "release-<release>-<track>")`.

### Out-of-scope surfaced (Rule 2)

- `internal/prompt/*` role-prompt re-sync to v0.10.0: NOT AC-01 scope (see decision above);
  a coherent role-prompt revendor is a separate role-revendor concern. Tracked: this is the
  same class as the historical S20-role-revendor slice; if a future release wants role prompts
  at v0.10.0 it re-runs the full `sworn baton vendor`. No gate depends on it.
