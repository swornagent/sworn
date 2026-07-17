# Design TL;DR — S01-all-ref-board-catalog

**Slice:** S01-all-ref-board-catalog · **Track:** T1-ref-aware-board · **Release:** 2026-07-17-ref-aware-board-discovery  
**State:** design_review (Rule 9 gate — no production code written)  
**User outcome:** `sworn board` without `--release` returns a deterministic catalog discovered from locally available refs, with one centrally elected high-water state and committed/uncommitted provenance per slice; named queries retain their existing envelope while using the same result.

## 1. Approach

Build a single read-only pipeline in `internal/board`:

1. `internal/git` enumerates fully qualified local-head and remote-tracking refs with `git for-each-ref`, filters remote symbolic-HEAD aliases, and bytewise-sorts the result. It never fetches, checks out, updates refs, changes directory, or examines tags/history/worktrees.
2. `board.DiscoverCatalog` inventories direct `docs/release/<release>/{board.json,index.md}` records across those ref tips, groups candidates by release-directory name, and selects one topology ref by the ratified four-class order. A canonical release-worktree ref is authoritative: if its direct record is absent, malformed, or identity-mismatched, discovery fails closed with release and fully qualified ref rather than falling through.
3. The selected record is parsed through the existing board/oracle machinery to obtain topology and `BoardState`. For every topology-declared slice, a state-evidence helper validates matching `status.json` candidates from all eligible ref tips and, only when dirty relative to `HEAD`, the active working-tree path. It applies the spec's normal/attention rank, timestamp, durability, and source-ref tie-breaks once, then writes the elected state and provenance into `SliceState`.
4. `cmd/sworn/board.go` always calls `DiscoverCatalog`. No filter renders the aggregate catalog; `--release` filters that same catalog before preserving the established `{release,tracks}` JSON/text contract. Presentation sorts releases and exposes per-slice provenance, adding `[uncommitted]` only for a working-tree winner.

This keeps topology selection, lifecycle election, and aggregate derivation inside one board-owned authority. The CLI performs filtering and rendering only, leaving S02 a reusable catalog API rather than another ref scanner.

## 2. Design choices and review pins

- **PIN-1 — mechanical: preserve existing oracle parsing instead of creating a parallel board parser.** Discovery will adapt the selected ref into the current `ReadBoard`/board-record path, then replace each topology slice's status projection with the central evidence winner. Any small extraction needed to parse a selected direct record stays package-private. This satisfies C-01 while limiting compatibility risk for existing oracle consumers.
- **PIN-2 — mechanical: ref eligibility is explicit and bounded.** The Git primitive returns only `refs/heads/*` and `refs/remotes/*`, excluding `refs/remotes/<remote>/HEAD`; every returned fully qualified ref is eligible for topology inventory and matching status evidence. Tags, history, fetches, and sibling worktrees never enter the candidate set.
- **PIN-3 — reviewer attention: canonical absence semantics.** A canonical local/remote `release-wt/<release>` ref establishes release existence even without a record. Discovery must therefore derive canonical release names from ref names before scanning files and fail immediately if the highest-ranked canonical candidate lacks a readable direct record. Lower-ranked copies cannot rescue it.
- **PIN-4 — reviewer attention: active working-tree admission.** For each topology status path, compare filesystem bytes with `HEAD:<path>`; admit `working-tree/uncommitted` only when bytes differ, including added/untracked files. If `HEAD` is unusable, use the shared filesystem fallback specified by the contract. Do not let the filesystem supply topology or enumerate another worktree.
- **PIN-5 — reviewer attention: attention-state election is timestamp-sensitive.** Normal states use ranks 0/1/2/3/5/6. `blocked`, `failed_verification`, `deferred`, or `verification.result == blocked` form attention rank 4: a valid later attention record overrides a higher normal state, and attention wins exact or missing-timestamp safety ties. Equal-class ties then prefer later RFC3339 time, committed evidence, and bytewise source ref.
- **CHOICE-A — additive provenance fields.** Add `StateSource` and `StateDurability` to `board.SliceState` with `stateSource`/`stateDurability` JSON names. They are output projections only; `slice-status-v1` is unchanged.
- **CHOICE-B — aggregate shape is CLI-owned compatibility glue.** Introduce a catalog record carrying `Release`, `SourceRef`, and complete `BoardState`. Aggregate JSON is `{ "releases": { "<name>": { "release", "sourceRef", "tracks" } } }`; named JSON continues to expose only top-level `release` and `tracks`, with provenance additive on slices.

## 3. Files to touch

| File | Planned responsibility | ACs |
|---|---|---|
| `internal/git/git.go`, `internal/git/git_test.go` | Read-only, bytewise-sorted local/remote ref enumeration and mutation snapshots | AC-01, AC-02, AC-06 |
| `internal/board/discovery.go`, `internal/board/discovery_test.go` | Catalog inventory, canonical skew failure, source-ref ranking, selected topology records | AC-01, AC-02, AC-03 |
| `internal/board/status_evidence.go`, `internal/board/status_evidence_test.go` | Candidate validation, working-tree admission, deterministic high-water/attention election, provenance | AC-01, AC-04 |
| `internal/board/oracle.go`, `internal/board/oracle_test.go` | Add provenance to `SliceState` and reuse/extract existing topology parsing for the catalog | AC-01, AC-04, AC-05 |
| `internal/board/releaseref.go`, `internal/board/releaseref_test.go` | Align/refactor existing named-release resolution behind catalog discovery without a competing public selector | AC-02, AC-03, AC-05 |
| `cmd/sworn/board.go`, `cmd/sworn/board_test.go` | Optional filter, aggregate/named renderers, real compiled-CLI Git fixtures, read-only and mutation evidence hooks | AC-01–AC-06 |

No files outside the spec touchpoints are planned, and no runtime dependency is introduced.

## 4. AC traceability and test strategy

- **AC-01:** a compiled-CLI fixture starts on a HEAD with no release docs, creates two non-HEAD release plans plus a farther track status, runs `sworn board --json` from the consumer root, and asserts sorted release keys, selected `sourceRef`, existing tracks, and per-slice provenance/high-water state.
- **AC-02:** table-driven discovery tests remove source classes in order; a CLI assertion confirms the fully qualified selected source appears in aggregate output.
- **AC-03:** local and remote canonical fixtures cover missing record, malformed JSON, and release-identity mismatch; CLI tests assert exit 2, deterministic stderr naming release/ref, and no successful/partial stdout catalog.
- **AC-04:** election tables cover every normal rank, later attention override, safety ties, malformed/unknown/mismatched exclusion, committed tie preference, and dirty higher working-tree evidence; CLI text proves `[uncommitted]` visibility.
- **AC-05:** the same fixture invokes aggregate and named modes, compares slice state/provenance, and asserts the named top-level JSON has `release`/`tracks` but neither `releases` nor `sourceRef`.
- **AC-06:** the reachability fixture snapshots HEAD, branch, sorted refs, porcelain status, and the test process cwd around the compiled command. Implementation proof will record the required red mutation (disable non-HEAD discovery or central election), restore it, then capture targeted/full tests, vet, and clean gofmt output.

The first implementation test is the user-facing compiled `sworn board --json` reachability fixture, so wiring failure is the initial TDD red rather than a leaf-only test.

## 5. Risks and containment

- **Split authority:** make `DiscoverCatalog` the only exported discovery/election entry point used by the CLI; retain lower-level helpers only as implementation details and test seams.
- **False durability:** compare exact bytes against HEAD and make provenance part of every elected `SliceState`; committed wins exact evidence ties.
- **Silent stale fallback:** identify canonical candidates from refs independently of record readability and fail closed before considering lower classes.
- **Non-determinism:** sort refs, release names, and output explicitly; apply a total election tie-break ending in bytewise source-ref order.
- **Read-only regression:** use explicit Git argument vectors, repo-anchored command directories, and before/after integration snapshots.

## 6. Effort/complexity confirmation

The spec's **high effort / high complexity / beast** classification is confirmed. The breadth is contained to the declared Git, board, and CLI surfaces, but correctness depends on composing ref topology, canonical skew, identity validation, lifecycle/attention election, working-tree durability, compatibility rendering, and real-repository reachability without a second authority.
