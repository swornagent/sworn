# Design TL;DR — S57-oracle-reader

## §1. User-visible change

A developer runs `sworn board --release <name>` and sees every slice's **authoritative** state — the copy committed on the slice's **owning track branch** — regardless of which branch or worktree is currently checked out. `--json` emits the same structure as `release-board-status.sh --json`. Blocked slices (`verification.result == "blocked"`) render distinctly with their reason and routing owner, never collapsing into a healthy-looking state.

## §2. Design decisions not in spec (max 5)

1. **Git operations via `internal/git` with new `Show`/`CatFileExists` methods** — reuse the existing package with its `Dir` guard (S28) rather than spawning raw `git`; the chokepoint protects against operating in the wrong worktree.
2. **Test strategy: real git repos in temp dirs** — fakeable without a new interface. The existing `internal/git/git_test.go` pattern (real `git init`, `git commit` in `t.TempDir()`) gives us deterministic refs for ownership-resolution and ghost-copy tests. No `GitReader` interface needed.
3. **Docs prefix probe is a simple ordered trial** — try `docs/release/...` first (this project), then `apps/docs/content/docs/release/...` (Fumadocs projects); the first path that `git cat-file -e` confirms exists wins. Inline the two paths rather than a configurable list — spec says one fallback.
4. **`ReadBoard` returns the full board struct; the CLI formats it** — separation of concerns. `internal/board/oracle.go` returns typed Go structs; `cmd/sworn/board.go` handles JSON marshal and table/text formatting. This lets the router (S58) call `ReadBoard` later without pulling in CLI formatting.
5. **Blocked owner inference**: if `verification.routing` is absent, `"blocked"` → `needs_planner`, `"failed_verification"` → `needs_implementer`. Simple default, still overridable.

## §3. Files I'll touch grouped by purpose

- **Git plumbing**: `internal/git/git.go` + `internal/git/git_test.go` — add `Show` and `CatFileExists` methods. The existing package wraps `os/exec` with a `Dir` guard; these new methods extend it for the read path the oracle needs.
- **Oracle reader**: `internal/board/oracle.go` (new) + `internal/board/oracle_test.go` (new) — core logic: ref-priority resolution, ownership filtering, transient retry, blocked visibility.
- **CLI command**: `cmd/sworn/board.go` (new) + `cmd/sworn/board_test.go` (new) — `sworn board` subcommand with self-registration via `init()`.

## §4. Things I'm NOT doing

- **Not adding a `GitReader` interface** — testability via real temp repos, matching the existing `internal/git` test convention. A future slice can introduce an interface if needed for the router.
- **Not modifying `commands.go`** — the `board` command self-registers via `init()` in its own file (S51 pattern). `commands.go` stays as-is.
- **Not touching the TUI** — TUI adoption is out of scope.
- **Not touching the router (S58) or scheduler (S59)** — they consume this reader later.

## §5. Reachability plan

**CLI integration test**: `cmd/sworn/board_test.go` creates a multi-branch, multi-track temp repo with committed divergent status.json files, builds the `sworn` binary via `go build`, runs `sworn board --release test-release --json`, and asserts the authoritative state (owner branch wins, ghost ignored, blocked visible). This is the Rule 1 artefact.

## §6. Open questions for the Coach

None.