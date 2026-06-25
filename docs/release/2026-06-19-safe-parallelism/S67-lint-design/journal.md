# S67-lint-design — Implementation journal

## 2026-07-15 — Implementation session

### Decisions

- **Colour detection operates on the diff, not the full project.** Unlike the existing `designaudit` package (which scans the full project), `sworn lint design` detects hardcoded colours only in lines added in the slice's diff. This is the correct design for a CI/lint gate — it checks what the slice introduces, not pre-existing code.
- **Architecture rules engine is a separate package-level component (`archrules.go`).** It supports four check types: `grep` (regex in changed files), `touchpoints` (files vs planned_files), `diff-size` (growth/absolute limits), and `external` (tool invocation with exit-code parsing). Each check type is independently tested.
- **Design-fidelity config is optional.** If `docs/baton/design-fidelity.json` is absent or has `ui_bearing: false`, colour detection is skipped (project is exempt). Architecture rules still run regardless — grep rules like `no-hardcoded-secrets` apply to all projects.
- **Architecture.json is also optional.** If `docs/baton/architecture.json` is missing, zero rules are checked and the report passes — this is graceful degradation, not a hard failure.
- **Allowlist is per-slice.** `design-allowlist.json` in the slice directory can suppress specific rules for specific files. Fully honoured across all check types.
- **Test files are always skipped.** Both colour detection and architecture rule checks skip test files (matching the same `isTestFilePath` pattern used in coverage.go with extensions `.test.ts`, `.spec.ts`, `_test.go`, `test_*.py`, etc.).
- **Glob matching supports brace expansion.** The `compileGlobToRegex` function handles `{ts,tsx}` style brace alternatives via sentinel substitution — necessary because `architecture.json` patterns use `**/*.{ts,tsx,js,jsx,...}` globs.

### Trade-offs

- **Git diff dependency.** The grep, touchpoints, and diff-size checks all read from git diff output. Unit tests test the core logic (regex matching, allowlist logic, config parsing) directly; full git-diff integration is tested indirectly via the `sworn lint design` CLI reachability artefact.
- **`readPlannedFiles` uses regex not JSON.** Parsing `status.json`'s `planned_files` with regex rather than a full JSON unmarshal is a pragmatic choice to avoid extra type definitions for the status schema. This is a known trade-off — if the status.json schema changes to nest `planned_files`, this parser will silently miss entries.
- **No caching of architecture rules.** Rules are loaded fresh from disk on every invocation. For a lint gate that runs once per CI invocation, this is negligible overhead and avoids staleness bugs.

### Out-of-scope discoveries

- The `designaudit` package (`internal/designaudit/`) already has colour and spacing checks against the full project. There is some conceptual overlap but the scopes differ (full-project audit vs diff-only lint). Future work could unify these under a single design-check surface.
- Architecture.json at `docs/baton/` is not yet materialised for this project — the canonical file lives at `internal/adopt/baton/architecture.json`. The `sworn lint design` gate reads from `docs/baton/architecture.json` which currently doesn't exist, so 0 rules are checked. This is correct behaviour — a project without a declared architecture config gets a pass, not a failure.

### Subagent dispatches

None.