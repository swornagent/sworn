# Design TL;DR — S05-state-and-git

## §1. User-visible change

Two new stdlib-only Go packages land as internal building blocks for the run-loop (S07):
- **`internal/state`** reads and writes `status.json`, enforcing the Baton slice state machine (`planned → in_progress → implemented → verified | failed_verification`) and rejecting illegal jumps.
- **`internal/git`** wraps `git` via `os/exec` for branch create/checkout, stage, commit, capturing `start_commit`, and computing a slice diff (`base..HEAD`). No third-party libraries — `os/exec` keeps the binary's zero-dependency property.

These are not user-facing subcommands; they are internal packages consumed by S07.

## §2. Design decisions not in spec (max 5)

1. **Git backend: `os/exec` over go-git.** The project mandates zero runtime dependencies (AGENTS.md). The operations needed (branch, add, commit, diff, rev-parse) are all single-command invocations with stable output formats. Pulling in `go-git` would require an ADR and adds dependency surface for operations that `os/exec` handles cleanly.

2. **State transitions: explicit enum + allowed-transition map.** A `State` type (string enum) with a `Transition(to State) error` method backed by a small lookup table. Rejecting `planned → verified` is a one-line entry in that table. No FSM library — too heavy for four states.

3. **Diff range: caller-supplied base ref.** The git package accepts a base ref string; it does not hardcode `start_commit`. This keeps the package reusable outside the slice workflow (e.g. the verifier's diff against an arbitrary base).

4. **Single-writer model, documented.** The state package is not goroutine-safe. The doc comment states this explicitly and defers serialisation to the caller (S07 run-loop), consistent with the spec's risk note and the project's "one implementer per worktree" guarantee.

5. **Status.json path: caller-supplied, package-agnostic.** The state package takes a file path from the caller; it does not know about release/slice directory conventions. This keeps the package testable with temp files and avoids entangling the state layer with board layout.

## §3. Files I'll touch grouped by purpose

- **State machine (`planned_files`: `internal/state/`)**
  - `internal/state/state.go` — `State` type, transition table, `Read(path)` / `Write(path, status)` with JSON round-trip
  - `internal/state/state_test.go` — table-driven: legal transitions pass, illegal transitions error, read/write round-trip with temp files

- **Git operations (`planned_files`: `internal/git/`)**
  - `internal/git/git.go` — `Init`, `Branch`, `Checkout`, `Stage`, `Commit`, `RevParse`, `DiffRange` via `os/exec`
  - `internal/git/git_test.go` — temp git repo (`git init`), table-driven: commit creation, diff range correctness, rev-parse returns expected SHA

## §4. Things I'm NOT doing

- Worktree orchestration (create, list, remove) — S07 run-loop
- Merge operations — S07
- Track-level state or multi-slice coordination — S07
- Concurrent-safety primitives (mutexes, file locks) — spec acknowledges single-writer per slice
- A user-facing `sworn` subcommand — S07 wires these packages into a CLI surface

## §5. Reachability plan

Backend-only slice — two Go packages with no user-facing CLI affordance. Reachability artefact:
```
go test ./internal/state/ ./internal/git/ && go vet ./internal/state/ ./internal/git/
```
Output captured into `proof.md`. `go build ./...` confirms compilation. The integration point is these packages being importable and testable by the S07 run-loop; no screenshot or e2e test applies.

## §6. Open questions for the Coach

None.