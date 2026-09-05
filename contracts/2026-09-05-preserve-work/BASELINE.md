# Prepared source validation

Prepared source: `109de8a0a30af19dace62d5ec68dfa10742f6eee`, containing
R7 and PR #290. The approved plan and contract files are the only added
product-tree content; no Go source was changed during preparation.

The sequential host validation command completed with exit 0 in the release
preparation worktree (execution session 89674). Results:

- Product tests: all packages passed; runtime 340.949s.
- End-to-end tests: passed, `-parallel=1 -timeout=35m`, 2071.200s.
- Product race tests: all packages passed; runtime 525.740s.
- `GOFLAGS=-buildvcs=false go vet ./...`: passed.
- Asserted `gofmt -l ./cmd ./internal ./tools`: empty.
- `go mod tidy -diff`: passed with no changes.
- `git diff --check`: passed.
- Darwin arm64 non-CGO cross-build: passed.

The plan's SHA-256 remains
`8d67b13bbee0744acfcbe4b7a506c5b50537d7ea165258109ce0173404961aa4`.
Native pinning/scope lint passed all four contracts against this preparation
base. These checks establish the baseline and pre-commit preparation only;
they are not evidence for implementation that has not occurred.
