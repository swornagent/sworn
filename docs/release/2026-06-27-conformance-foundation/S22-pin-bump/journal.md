# Journal — S22-pin-bump

## 2026-06-28 — Implementation session

**State transition: planned → in_progress → implemented**

### Decisions

- **Source map paths**: Replaced all `claude/baton/` → `baton/` across `internal/baton/source.go`, `vendor.go`, `diff.go`, `vendor_test.go`, `fetch_test.go`, and `cmd/sworn/baton_test.go`. The spec's planned_files listed `source_map.json` but the actual file is `source.go` (noted in spec Pre-requisites).

- **Upstream digest**: Computed from the actual baton repo commit `42eb48b` via `git cat-file commit 42eb48b | sha256sum` → `7b91a0450d6d5b577e1f8638ba39919ecca86d880d2ec328467f7a031831715f`. The baton repo at `/home/user/projects/baton` was available locally.

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
## Verifier verdicts received

### 2026-07-24 — PASS

**Verdict: PASS**

All seven verification gates passed:
- Gate 1 (User-reachable outcome): `internal/adopt/baton/VERSION` exists and is consumed by `internal/baton/version.go`; prompt VERSION embedded via `go:embed` in `internal/prompt/prompt.go`.
- Gate 2 (Planned touchpoints): All changed files are either in the plan or are natural extensions of the `claude/baton/` → `baton/` path migration (diff.go, vendor.go, test files, test fixtures). Divergence explained in proof.md.
- Gate 3 (Required tests): `go build ./...`, `go test ./internal/baton/... ./cmd/sworn/...`, `go vet` all pass. Reachability smoke checks: VERSION shows `42eb48b`, zero `claude/baton` paths in source.go.
- Gate 3b (LLM AC-satisfaction): Script not available — skipped (non-blocking).
- Gate 4 (Reachability artefact): Manual smoke-step checks verified — VERSION SHA correct, zero `claude/baton` paths, build succeeds.
- Gate 5 (No silent deferrals): Zero TODO/FIXME/deferred/placeholder/hack in changed files.
- Gate 6 (Design conformance): No design-fidelity config — non-UI project, auto-pass.
- Gate 7 (Claimed scope): All 5 delivered items match the 5 acceptance checks; evidence references are valid.

**Minor note:** `upstream-digest:` and `rules-added:` are on the same line in VERSION file (no newline separator). Does not violate any acceptance check; cosmetic only.

**Next step:** `/implement-slice S23-version-centralise-doctor 2026-06-27-conformance-foundation` in a fresh session.
