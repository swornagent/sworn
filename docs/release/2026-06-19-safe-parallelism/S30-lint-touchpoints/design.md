# Design TL;DR — S30-lint-touchpoints

## §1. User-visible change

A planner or implementer runs `sworn lint touchpoints <slice-id> <release>` and gets a fail-closed report when the slice's design/spec references files or packages not declared in `planned_files`, when a file in the release touchpoint matrix is claimed by more than one slice/track without acknowledgement, or when two slices declare the same migration number. A clean slice exits 0. This mechanises the most common Captain-catch class — designs that touch undeclared files or collide across tracks — before any code is written.

## §2. Design decisions not in spec (max 5)

1. **Parse back-ticked file/package references from spec.md only, not design.md.** The spec is the binding contract; design.md is advisory and may name files in exploratory context. The spec's "In scope" and "Planned touchpoints" sections are the authoritative source of declared intent. Rationale: avoids false positives from design prose while still catching the dominant defect (spec references absent from planned_files).

2. **File-reference extraction uses a regex on back-ticked tokens that contain `/` or end in `.go`/`.ts`/`.tsx`/`.md`.** This mirrors how the spec names files (e.g. `` `internal/lint/touchpoints.go` ``) and avoids matching prose back-ticks like `` `sworn` `` or `` `planned_files` ``. Package references like `` `internal/lint` `` (no extension, contains `/`) are also captured and checked as prefixes against planned_files.

3. **Touchpoint matrix parsing reads the markdown table under `### Touchpoint matrix` in `index.md`.** The parser extracts the file/surface column and the `✓` marks per track column. A file with `✓` in more than one column that is not annotated "DOCUMENTED SHARED" is a collision. The parser does not need to understand `(dep)` notation — those are serialised by dependency, not parallel, and the matrix already annotates them; we flag only raw multi-`✓` rows without the DOCUMENTED SHARED annotation.

4. **Migration number detection scans planned_files for a pattern like `NNNNNN_` prefix (6-digit migration id).** If two slices in the release share the same 6-digit prefix in any planned_file path, that's a collision. This is a simple string-prefix check, not a database introspection.

5. **The function signature is `CheckTouchpoints(sliceDir, releaseDir string) error`** following the same pattern as `CheckDeps`. It returns nil on pass, an error naming violations on fail. The CLI wrapper maps error → exit 1, nil → exit 0, matching `cmdLintDeps`.

## §3. Files I'll touch grouped by purpose

- **`internal/lint/touchpoints.go`** (new) — core logic: parse spec.md for file/package references, reconcile against planned_files, parse index.md touchpoint matrix for cross-slice collisions, detect duplicate migration numbers. Exported `CheckTouchpoints` function.
- **`internal/lint/touchpoints_test.go`** (new) — table-driven tests with temp-dir fixture releases: undeclared reference fails, matrix collision fails, migration collision fails, clean passes.
- **`cmd/sworn/lint.go`** (extend) — add `touchpoints` case to the target switch, `cmdLintTouchpoints` function that parses `<slice-id> <release>` args, resolves release dir, calls `lint.CheckTouchpoints`, maps result to exit code.

## §4. Things I'm NOT doing

- Symbol/identifier resolution (functions, fields, constants) — that is S31 (`lint symbols`).
- go.mod/go.sum dependency reconciliation — that is S29 (`lint deps`), already landed.
- Auto-editing `planned_files` or the matrix to fix the gap — report only.
- Parsing design.md for file references — spec.md is the binding contract.
- Non-additive edit detection for DOCUMENTED SHARED files — the spec describes this as a "coordination signal" not a flat reject. Implementing heuristic detection of "non-additive" edits from prose is too fuzzy for a fail-closed gate. I will flag any DOCUMENTED SHARED file that appears in the slice's planned_files and report it as an informational note (not a violation), so the Coach is aware. The non-additive judgement stays human.

## §5. Reachability plan

Reachability artefact: run `sworn lint touchpoints S30-lint-touchpoints 2026-06-19-safe-parallelism` from the worktree root against the real release. This slice's own spec references `internal/lint/touchpoints.go`, `internal/lint/touchpoints_test.go`, and `cmd/sworn/lint.go` — all in planned_files — so it should exit 0. Then run against a crafted fixture (temp dir) with an undeclared reference and capture the non-zero exit. Both captured in proof.md.

## §6. Open questions for the Coach

None.