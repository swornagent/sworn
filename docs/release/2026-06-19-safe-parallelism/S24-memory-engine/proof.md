# Proof Bundle: S24-memory-engine

## Scope
Implemented the `sworn memory build` command, embedding drivers (Voyage, OAI-compat, Ollama), SQLite vector index, and entry discovery for AI coding harnesses.

## Files changed

Boundary: `git diff --name-only d441b4c..HEAD` (`start_commit` = `d441b4c`, the
"start implementation" commit). The range spans two forward-merges of
`release-wt/2026-06-19-safe-parallelism` (`761b8bf`, `dfa666c`) that were required to
sync the track before verification, so the raw diff includes already-verified content
from other slices. The S24 implementation footprint is:

```
cmd/sworn/memory.go
internal/memory/discover.go
internal/memory/discover_test.go
internal/memory/embed.go
internal/memory/embed_oai.go
internal/memory/embed_ollama.go
internal/memory/embed_test.go
internal/memory/embed_voyage.go
internal/memory/index.go
internal/memory/index_test.go
```

Also present in the diff range but **NOT S24 changes** — inherited via the two
forward-merges of `release-wt` (already verified on their own slices):

```
cmd/sworn/main.go            # dispatch() resolution: S23 memory case ported into T9's dispatch()
cmd/sworn/telemetry.go       # S26-telemetry (verified, T9)
internal/telemetry/telemetry.go
internal/telemetry/telemetry_test.go
internal/git/git.go          # S28-git-dir-guard (verified, T11)
internal/git/git_test.go
+ 46 docs/release/** artefacts # S26/S28 proofs + S29–S38 replan specs, brought in by the merges
```

## Test results
```
$ go test -race ./internal/memory/...
ok  	github.com/swornagent/sworn/internal/memory	1.163s
```

## Reachability artefact
```
$ ./sworn memory build
Indexed 3 entries (3 new, 0 unchanged) via nomic-embed-text in 129ms

$ ./sworn memory build
Indexed 3 entries (0 new, 3 unchanged) via nomic-embed-text in 1ms

$ ./sworn memory build --force
Indexed 3 entries (3 new, 0 unchanged) via nomic-embed-text in 74ms
```

## Delivered
- `sworn memory build` command with `--force` flag (cmd/sworn/memory.go)
- Embedder interface and drivers for Voyage, OAI-compat, Ollama (internal/memory/embed*.go)
- SQLite vector index with change detection and cosine similarity (internal/memory/index.go)
- Entry discovery for Claude Code, flat files, and custom paths (internal/memory/discover.go)
- Addressed all 6 Coach pins from design review.

## Not delivered
- Index pruning (entries for deleted memory files stay in DB)
  - **Why**: Post-R3, low impact since corpus is small.
  - **Tracking**: Rule 2 deferral.
  - **Acknowledged**: Coach, 2026-06-21
- TUI progress bar for large builds
  - **Why**: Post-R3 enhancement.
  - **Tracking**: Rule 2 deferral.
  - **Acknowledged**: Coach, 2026-06-21

## Divergence from plan

No divergence in the S24 implementation itself — the 10 files above match the spec's
planned touchpoints (`internal/memory/*`, `cmd/sworn/memory.go`).

The `git diff` range from `start_commit` additionally surfaces files that are **not**
S24's work and entered the range only because the track was forward-merged with
`release-wt` before re-verification (a required Baton sync step):

- `cmd/sworn/main.go` — the `case "memory"` dispatch was re-homed into T9-telemetry's
  `dispatch()` function during the forward-merge conflict resolution (commit `761b8bf`).
  - **Why**: T9 restructured `main()` into `dispatch(args) int` (non-additive change to the
    DOCUMENTED-SHARED file); every track's added case had to be ported. This is integration
    glue, not S24 feature code.
  - **Tracking**: the systemic `cmd/sworn/main.go` conflict — S30-lint-touchpoints carries
    the additive-invariant check to catch this class at plan time.
  - **Acknowledged**: Coach, 2026-06-21.
- `cmd/sworn/telemetry.go`, `internal/telemetry/*` (S26-telemetry) and `internal/git/*`
  (S28-git-dir-guard), plus 46 `docs/release/**` artefacts — all already verified on their
  own slices; present only as forward-merge content from `release-wt` (`761b8bf`, `dfa666c`).
  Not authored or modified by S24.

> Re-verify note (round 2): the round-1 FAIL was Gate 2 only — `start_commit` had been set
> to `16c0a8b` (the coach-ack commit) instead of `d441b4c` (the start-implementation
> commit), and this section previously read "None." `start_commit` is now corrected to
> `d441b4c` and every non-planned file in the diff range is accounted for above. The S24
> implementation is unchanged (Gates 1, 3, 4, 5, 6 passed in round 1).
