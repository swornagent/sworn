# Journal — S22-pin-bump

## 2026-06-28 — Implementation session

**State transition: planned → in_progress → implemented**

### Decisions

- **Source map paths**: Replaced all `claude/baton/` → `baton/` across `internal/baton/source.go`, `vendor.go`, `diff.go`, `vendor_test.go`, `fetch_test.go`, and `cmd/sworn/baton_test.go`. The spec's planned_files listed `source_map.json` but the actual file is `source.go` (noted in spec Pre-requisites).

- **Upstream digest**: Computed from the actual baton repo commit `42eb48b` via `git cat-file commit 42eb48b | sha256sum` → `7b91a0450d6d5b577e1f8638ba39919ecca86d880d2ec328467f7a031831715f`. The baton repo at `/home/brad/projects/baton` was available locally.

- **Test fixture directory**: Renamed `internal/baton/testdata/fixture/claude/baton/` → `internal/baton/testdata/fixture/baton/` to match the source-path prefix change. git detected these as renames (100% similarity).

- **Prompt embed root**: `internal/prompt/VERSION.txt` updated from `v0.4.2` to `v0.6.1` with a comment annotating the canonical commit SHA `42eb48b`. This brings both embed roots (adopt/baton + prompt) into sync per sworn#24.

- **Full SHA vs short SHA**: The spec explicitly says to use short form `42eb48b` for `upstream-sha`. The baton repo confirms `git rev-list -n1 v0.6.1` = `42eb48b` (full: `42eb48b8b73f8a75294696d40dbbc6780b9864da`).

### Trade-offs

- The `refactor/baton-vendor-paths` branch was not available (no remote, no local); the path-prefix change was applied directly as described in the spec rather than cherry-picked.
- Remaining `claude/baton` references in `internal/prompt/*.md`, `internal/prompt/prompt.go`, and `cmd/sworn/doctor*.go` are user-facing `~/.claude/baton/` paths (the runtime install directory) — deliberately unchanged.

### Out-of-scope noted

- Doctor checks for pin staleness (S23 scope)
- VERSION string centralisation (S23 scope)
- Actual vendored file copying (S13/S20 scope)