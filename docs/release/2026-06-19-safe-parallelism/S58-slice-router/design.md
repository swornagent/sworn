# Design TL;DR — S58-slice-router

## §1. User-visible change

`sworn route <slice-id> <release-name> [--pretty]` is a new subcommand that reads a slice's `status.json` (via the S57 board oracle) and deterministically computes what command to run next: implement, verify, review, merge-track, merge-release, replan-release, redesign, coach_decision, or none. It ports `~/.claude/bin/captain-route.sh`'s decision tree faithfully into Go, producing identical JSON `.next` output. No LLM is invoked — it is a pure deterministic function of committed git state.

## §2. Design decisions not in spec (max 5)

1. **`OracleReader` interface defined in `internal/board`.** The spec says this slice CONSUMES `board.OracleReader` but S57 did not define it. I will add a small interface `OracleReader` to `internal/board/oracle.go` with the two methods the router needs: `ReadSliceStatus` and `ReadBoard`. The concrete `*Oracle` already satisfies this interface. *Rationale:* the spec names the interface explicitly; placing it in `board` keeps the consumer contract co-located with the producer.

2. **`gitContentReader` exposed for commit-time queries.** The `design_review` sub-state routing needs commit-time comparison of artefact files (`design.md`, `review.md`, `decline.md`, `approved-ack.md`). `gitContentReader` is currently unexported; I will export it as `GitContentReader` and add a `LastCommitTime(ref, path string) (int64, error)` method so the router can compute artefact-newest. The production `git.Repo` already has `Show` and `CatFileExists`; `LastCommitTime` wraps `git log -1 --format=%ct`. *Rationale:* the router must not know about git internals; commit-time resolution stays behind the reader interface.

3. **`Shipped` handled as a string, not a `state.State` constant.** The spec lists `shipped → none` routing. `internal/state/state.go` has no `Shipped` constant (and this slice's touchpoints don't include it). The router will match the string `"shipped"` directly, same as captain-route.sh does. *Rationale:* avoids a cross-package schema change outside planned_files; `"shipped"` is a terminal state from the release board, not a workflow state in the implementer loop.

4. **`Route` returns `(Decision, error)`; CLI stamps `GeneratedAt`.** The `Route` function stays pure — no timestamps. `GeneratedAt` is set in `cmd/sworn/route.go` after `Route` returns, mirroring the shell script's `date -u` at emit time. *Rationale:* keeps unit tests deterministic and the function boundary clean.

5. **Parity test uses a JSON golden file for port-fidelity.** The parity test (`parity_test.go`) runs `captain-route.sh` over fixture refs, pipes its JSON through `jq` to strip `generated_at`, and compares the stripped `Decision` shape to the Go router's output (also stripped). A `t.Skip` with a message fires if `captain-route.sh` isn't on PATH. *Rationale:* the shell script is the literal oracle; every branch of the tree must match.

## §3. Files I'll touch grouped by purpose

- **Router core (new):** `internal/router/router.go` — the pure `Route` function + `Decision` struct + `NextType` enum. Table-testable with a fake reader.
- **Router unit tests (new):** `internal/router/router_test.go` — table-driven per-state-branch tests: `TestBlockedPrecedesState`, `TestDesignReviewCommitTimeNewest`, `TestFailedVerificationGateClassification`, `TestVerifiedWalksTrackThenMerges`, `TestGhostSliceFiltered`, etc.
- **Router parity test (new):** `internal/router/parity_test.go` — golden-file comparison of Go vs `captain-route.sh` over fixture refs.
- **CLI command (new):** `cmd/sworn/route.go` — the `sworn route` subcommand: wires flags, constructs the `GitContentReader` + `Oracle`, calls `Route`, stamps `generated_at`, prints JSON (compact or `--pretty` coloured).
- **CLI integration test (new):** `cmd/sworn/route_test.go` — runs `sworn route` against a real repo with committed fixtures; asserts JSON decision shape (Reachability Rule 1).
- **Interface definition (edit):** `internal/board/oracle.go` — add `OracleReader` interface, export `gitContentReader` → `GitContentReader`, add `LastCommitTime` method. *Why:* the spec names `board.OracleReader` as the consumer contract; it belongs in the board package.

## §4. Things I'm NOT doing

- **Not building the scheduler/dispatcher (S59).** `route` only decides; it never dispatches.
- **Not reading the working tree.** All state comes from committed git refs via the oracle reader; the stale-read trap stays in the reader's domain.
- **Not handling auto-ack/coach-loop orchestration.** Those are T17 scheduler concerns — route only produces a decision.
- **Not modifying `cmd/sworn/main.go`.** The subcommand self-registers via `init()` + the S51 command registry.

## §5. Reachability plan

`cmd/sworn/route_test.go` will create a temporary git repo with committed `status.json` fixtures covering every state branch, then run `sworn route <slice> <release> --json` and assert the output `Decision` shape. The test proves the CLI command is reachable, wired, and produces correct JSON — the Rule 1 artefact.

## §6. Open questions for the Coach

*(none)*