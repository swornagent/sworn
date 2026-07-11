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

## 2026-07-11 — implementation complete (state: implemented)

All 11 acceptance criteria delivered. Full suite: **47 packages ok, 0 FAIL**
(`go test -count=1 -timeout 300s ./...`). gofmt -l clean on all 33 changed .go
files; `go vet ./...` clean; newline-eating scan (fused `//`+code) clean.

### Self-check (sworn#90 resolution — PROVEN)
- `sworn board --release 2026-06-28-driver-contract` → EXIT 0. Reads the
  un-migrated board.json (legacy schema_version + release_worktree_* + tracks[]
  worktree/state) and derives track state from git ancestry: T1-T6/T8 = merged,
  T7 = planned.
- `sworn designfit 2026-06-28-driver-contract` → DESIGNFIT PASS, EXIT 0 over 15
  slices, reading the un-migrated status.json records (quadrant=epic) through the
  D1 normalise() shim (epic→beast) so state.Read's strict EffortComplexity.Validate
  passes. This is the sworn#90 proof: un-migrated records still load after the
  enum flip; S12 owns the data migration + shim deletion.
- Pin-4: `TestNormaliseRealRecordsValidateStrict` normalises the LIVE board.json
  + S11 spec.json (which FAIL strict v0.10.0 ValidateSchema as authored) and both
  then PASS — the strip-set is derived from the real delta, not hand-listed.

### Necessary divergences from plan (all in proof.json `divergence`)
1. **git.Repo.IsAncestor bug fix** — it returned `(false, err)` not `(false, nil)`
   for the documented exit-1 not-ancestor case (run() drops the exit code into an
   empty-stderr string). AC-07/D2 mandate reusing IsAncestor for state derivation,
   so it had to be correct; the fix also corrects merge-release's ancestor gate
   (was exit 2 instead of the intended exit 1). Added RefExists + PrimaryWorktreeRoot.
2. **Consumer sweep beyond touchpoints** — spec R-05 assumed all consumers read
   worktree/state via the internal/board TrackInfo struct, but internal/run/parallel.go,
   internal/mcp/{context,tools_ops}.go, internal/tui/{board,blocked}.go, cmd/sworn/regress.go
   read BoardTrack/BoardRecord DIRECTLY. The BoardTrack struct change (a T7 touchpoint)
   necessarily breaks them; all other tracks (T1-T6,T8) are merged so there is no live
   collision. Fixed to use the derivation helpers. Surfaced here (Rule 2).
3. **render.go** derives the tracks-table State via git (AC-07 lists it as a
   state-resolution site); **tui/board.go resolvedRefIsLiveCheckout** now treats
   releaseRef=="HEAD" as a live checkout (solo single-worktree run shape).
4. **Extensive test-fixture sweep** across internal/{run,mcp,tui,bench} + cmd/sworn:
   derivation changed WHERE worktrees live (sibling-of-repo, not a configurable
   path), so orchestration tests pre-create the derived worktree dirs and the
   merge/regress fixtures use a real primary+release-worktree layout.

### Proof-bundle gate (`sworn verify`) — environmental note
The model-backed `sworn verify` gate cannot run in this environment
(`SWORN_ANTHROPIC_API_KEY not set`); it exits at verifier-model resolution
before the deterministic first-pass. This is an environment limitation (keyless
env = model-judge unavailable — see memory `project_model_layer_service_refactor`),
not a proof-bundle failure. The deterministic evidence above stands; the canonical
Rule-7 verification is the separate fresh-context `/verify-slice` session.
`release-verify.sh` would also false-FAIL "spec.md missing" on this spec-v1 slice
(Captain Pin 7) — a spec.md was deliberately NOT manufactured.
