# Journal: `S16-lint-rename`

## 2026-06-22 — Documentation sweep: adopt `sworn lint ac` / `sworn lint trace` canonical names

- **State**: `planned → implemented`
- **Work**: S16-lint-rename documentation sweep across the full `docs/release/2026-06-16-fidelity-layer/` tree.
- **What was done**:
  1. **Grep sweep**: Replaced all stale references to the original bare-verb names (`ears`, `rtm`) in `index.md`, `intake.md`, `S01-rtm-spine/status.json`, `S02-ears-ac-format/journal.md`, `S02-ears-ac-format/proof.md`, and `S16-lint-rename/spec.md` (narrative sections).
  2. **S02 proof.md regenerated**: Recreated from `git diff --name-only cd462364..HEAD` (53 files) per AC N-S16-03. Added note explaining `ears.go` was added-and-deleted within the range (not visible in `--name-only`). Ran first-pass script — PASS (18/18).
  3. **S02 status.json updated**: `state` → `implemented`, `verification.result` → `pending`, `actual_files` cleaned to reflect current state (removed stale `cmd/sworn/rtm.go` and S01 doc files). `last_updated_by` → `implementer`.
  4. **S01-rtm-spine/status.json planned_files corrected**: `cmd/sworn/rtm.go` → `cmd/sworn/lint.go`, `cmd/sworn/rtm_test.go` → `cmd/sworn/lint_trace_test.go`. Reachability artefact updated to `sworn lint trace`.
  5. **intake.md updated**: `2026-06-16` standalone verbs decision — `sworn rtm` → `sworn lint trace`. `2026-06-18` lint namespace section rephrased to avoid literal old names while preserving historical context.
  6. **index.md S16 row**: Rephrased to use canonical names only.
  7. **gofmt fix**: `cmd/sworn/main.go` formatted (was detected by gofmt check).
- **Key decisions**:
  - S02 proof "Files changed" lists ALL 53 files from the diff (not just S02's scope) per AC N-S16-03 literal requirement. Cross-slice files noted in Divergence.
  - S16 spec's AC N-S16-01 (grep pattern) necessarily matches itself — documented in proof as spec-level self-reference.
  - `cmd/sworn/ears.go` not in `--name-only` diff (added+deleted) — documented in both S02 proof note and S16 proof Divergence.
- **Tests**: `TestLintAC` PASS, `TestLintTrace` PASS, `sworn lint ac` exit 0 on live release.