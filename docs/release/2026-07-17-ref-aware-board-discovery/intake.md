---
title: 'Release intake: ref-aware board discovery'
description: 'Planning record for deterministic all-ref release-board discovery and high-water state evidence.'
---

# Release Intake: `2026-07-17-ref-aware-board-discovery`

## Release goal

Make every release plan already present in the local Git object database visible
to a Sworn operator without requiring that plan to be checked out, and give every
board consumer the same answer to: **what is the farthest advanced state for this
available release, track, and slice?** A shipped release lets an automation
caller run `sworn board --json` from the project root and receive a deterministic
catalog, while an interactive operator can see and open the same ref-only
releases in the TUI. When the winning slice evidence exists only in the current
working tree, the oracle and both interfaces say so explicitly rather than
silently presenting it as committed truth.

"Available" is deliberate: this release reads locally available ref tips and
the active working tree. It does not reconstruct a status that disappeared with
a crashed process before it was written to either location.

## Needs

- N-01: **Project-wide board catalog.** An operator or release-mode caller can
  discover every valid release board in local and remote-tracking refs through
  `sworn board` without supplying a release name.
- N-02: **Deterministic, fail-closed topology selection.** Every catalog record
  identifies the chosen source ref, uses the ratified priority order, and
  surfaces missing or malformed canonical records instead of silently omitting
  them or selecting a different plan.
- N-03: **One high-water state-evidence oracle.** The board package elects the
  farthest valid, available lifecycle evidence for each topology-declared slice
  across eligible ref tips and the active working tree. The CLI catalog, named
  CLI board, release-list aggregate, and TUI board consume that same result;
  none owns a second status-ranking rule.
- N-04: **Read-only bounded discovery.** Discovery does not fetch, create,
  update, check out, or otherwise mutate Git refs, the current worktree, or the
  current working directory; it does not scan Git history or sibling
  worktrees.
- N-05: **Durability provenance.** A caller can distinguish a state supported
  by a committed ref from one supported only by an uncommitted working-tree
  record, including in machine-readable output and visible TUI/text rendering.

## Source of truth

- **Human stakeholder**: repository owner / release operator
- **Tracking issue / epic**: [sworn#123](https://github.com/swornagent/sworn/issues/123)
- **TUI scope clarification**: [sworn#123 comment](https://github.com/swornagent/sworn/issues/123#issuecomment-4990122697)
- **Related captures**: none; the issue contains the live reachability repro
  and acceptance intent.

## Users and their gestures

- **Release-mode automation caller**: runs `sworn board --json` at a consumer
  project root without knowing release names and receives a deterministic
  catalog whose records name their selected topology source refs and each
  slice's state evidence source and durability.
- **CLI release operator**: runs `sworn board` without `--release` and sees a
  readable section for each discovered release; continues to use
  `sworn board --release <name> [--json]` with its established top-level
  single-release shape and the same per-slice state evidence.
- **TUI operator**: launches `sworn`, selects a release that exists only on a
  non-HEAD ref, and opens its board from the same catalog snapshot as the CLI.
  A slice whose selected high-water state is uncommitted is visibly marked.

## What's currently broken or missing

- `sworn board --json` rejects a project-root query with exit 64 because
  `--release` is mandatory, so release-mode automation must already know a
  release name before it can inspect the board.
- The named-release resolver only checks a local
  `refs/heads/release-wt/<release>` ref before falling back to `HEAD`; it cannot
  catalog remote-tracking or noncanonical refs.
- `internal/tui/releases.go` scans only the checked-out
  `docs/release/*/index.md` filesystem paths. A release that exists only on a
  release assembly or track ref is invisible.
- `internal/tui/board.go` separately loads the selected plan and elects a
  live-status override. That creates a second state authority and can make the
  CLI and TUI disagree about both the farthest state and whether it is durable.

## What the human wants

- Make `--release` optional for `sworn board`; an omitted filter returns every
  discoverable release board.
- Emit a stable project-level JSON document with a `releases` object keyed by
  release name. Each record contains `release`, `sourceRef`, and the existing
  `tracks` content.
- Use one internal `board.DiscoverCatalog` result as the topology and state
  evidence authority for the CLI and TUI. The named CLI path selects a record
  from that result; it does not run a different status resolver.
- Keep the selected `sourceRef` authoritative for plan topology, while electing
  slice state separately from all valid available evidence for the exact
  topology-declared release, track, and slice.
- Expose `stateSource` and `stateDurability` on every CLI JSON slice. A source
  is either a fully-qualified Git ref or the literal `working-tree`; durability
  is exactly `committed` or `uncommitted`. Text and TUI rows visibly append
  `[uncommitted]` when that is the elected evidence.
- Make the TUI list and open the same catalog and state snapshot. A release-list
  aggregate visibly indicates uncommitted evidence when any selected slice
  evidence is uncommitted.
- Discover only refs already available locally, including remote-tracking refs;
  never fetch or mutate Git state as a side effect of discovery.
- Do not promise recovery of a state that was never committed and no longer
  survives in the active working tree.

## Constraints and non-negotiables

- Sworn remains a native Go binary with zero new runtime dependencies.
- Discovery is read-only: no `git fetch`, checkout, ref update, worktree change,
  or current-directory change is permitted.
- The all-ref path is fail closed. A canonical release-worktree ref that lacks a
  valid board record is a reported error, not an excuse to omit that release or
  fall back silently.
- The source-ref ranking is bytewise deterministic: local canonical
  `refs/heads/release-wt/<release>`; remote-tracking canonical
  `refs/remotes/<remote>/release-wt/<release>` in lexical ref order; local
  noncanonical candidates in lexical ref order; remote noncanonical candidates
  in lexical ref order.
- The selected topology fixes the release, tracks, and slice IDs to inspect. A
  committed candidate is a status at that exact slice path on an eligible local
  head or remote-tracking ref that parses and exactly matches the topology's
  `release`, `track`, and `slice_id`. The active working-tree candidate is used
  only when its status bytes differ from `HEAD` at that path (including an
  added/untracked path); without a usable Git HEAD, the filesystem fallback is
  conservatively marked `working-tree` / `uncommitted`.
- Normal lifecycle rank is `planned` 0, `design_review` 1, `in_progress` 2,
  `implemented` 3, `verified` 5, and `shipped` 6. `blocked`,
  `failed_verification`, and `deferred`, plus `verification.result == blocked`,
  are attention evidence at rank 4. A higher normal rank wins. A valid later
  attention record wins over a higher normal record; attention wins exact or
  missing-timestamp safety ties. Equal normal or equal attention records use a
  later valid RFC3339 `last_updated_at`; then a committed candidate beats an
  uncommitted candidate; then fully-qualified source refs sort bytewise. This
  keeps the election deterministic without hiding a newer block or failure.
- A valid named query keeps its established top-level single-release output
  shape; the new aggregate shape applies only when `--release` is omitted.
  Adding per-slice `stateSource` and `stateDurability` is an intentional,
  additive JSON contract change for both named and aggregate output.
- The candidate set is bounded to locally enumerated ref tips, the source
  topology's declared slice paths, and the primary current working tree. No
  history walk, fetch, arbitrary filesystem scan, or sibling-worktree scan is
  permitted.
- No API, network, credential, personal-data, persistence, compliance, or
  browser-facing surface is introduced. Accessibility is not applicable because
  the TUI uses its existing keyboard-operable release list and board views.

## Adjacent / out of scope

- **Ref fetching or remote configuration**: deferred because this release must
  be observational only. **Tracking**: [sworn#123](https://github.com/swornagent/sworn/issues/123).
  **Acknowledged**: repository owner, 2026-07-17.
- **Git-history archaeology or recovery of vanished process-local state**:
  deferred because #123's bounded oracle is about currently available evidence,
  and a historical reconstruction would change both performance and the meaning
  of state provenance. **Tracking**: [sworn#123](https://github.com/swornagent/sworn/issues/123).
  **Acknowledged**: repository owner, 2026-07-17.
- **Changing release-mode commands other than `board` and the TUI**: deferred
  because route, merge, lint, and run have distinct gating semantics and are
  not part of the reported reachability gap. **Tracking**:
  [sworn#123](https://github.com/swornagent/sworn/issues/123). **Acknowledged**:
  repository owner, 2026-07-17.
- **Redesigning the TUI layout or keyboard bindings**: deferred because the
  user outcome is discovery, evidence visibility, and board opening, not a new
  interaction model. **Tracking**: [sworn#123](https://github.com/swornagent/sworn/issues/123).
  **Acknowledged**: repository owner, 2026-07-17.

## Decisions made during planning

### 2026-07-17 — isolate ref-aware discovery in its own release

- **Context**: #123 affects the shared board oracle, public CLI output, and TUI
  loading. The active provider and Baton-conformance plans have different user
  outcomes and should not absorb this change.
- **Options considered**: append to an unrelated active release; make one large
  CLI-and-TUI slice; create a focused release with serial CLI/core and TUI
  slices.
- **Decision**: create `2026-07-17-ref-aware-board-discovery` with
  `S01-all-ref-board-catalog` followed by `S02-tui-ref-aware-release-navigation`
  in `T1-ref-aware-board`.
- **Why**: the CLI and TUI are separate user journeys but share the catalog and
  state-evidence contract, so they must be independently verifiable yet
  implemented sequentially on one track.

### 2026-07-17 — ratify deterministic source-ref priority and fail-closed skew

- **Context**: a release can appear on local, remote-tracking, and
  noncanonical refs. #123 required a documented fallback rule.
- **Options considered**: prefer any local ref after local canonical; prefer
  remote canonical before arbitrary local branches; limit discovery to
  release-worktree refs.
- **Decision**: prefer local canonical, then remote canonical in lexical ref
  order, then local noncanonical lexical candidates, then remote noncanonical
  lexical candidates. A canonical ref without a valid board is an error.
- **Why**: canonical release assembly state is authoritative where available;
  lexical ordering removes implementation-dependent selection; failing closed
  preserves the existing skew guard.

### 2026-07-17 — replace the TUI-only status override with one State Evidence Oracle

- **Context**: the first amendment correctly established that a valid
  uncommitted record must not disappear merely because its source ref differs
  from the selected plan ref. The repository owner then made the broader
  requirement explicit: the CLI catalog, named CLI output, and TUI need the
  same farthest-advanced answer, not separate committed and live rules.
- **Options considered**: retain the selected-source committed baseline with a
  TUI-only live override; scan every worktree and Git history by timestamp; use
  one bounded `board.DiscoverCatalog` election across locally available ref tips
  and the active working tree, with provenance.
- **Decision**: S01 owns the bounded shared election and exports its chosen
  evidence through `stateSource` and `stateDurability`. S02 only renders the
  catalog snapshot. A dirty working-tree winner is explicitly
  `working-tree` / `uncommitted`; an equal durable candidate wins a tie so the
  warning means the displayed result actually depends on uncommitted evidence.
- **Why**: lifecycle progress is an operational fact, not a property of the
  interface that happens to read it. Provenance preserves the operator's ability
  to judge whether that fact survived a commit, while the bounded source set
  keeps discovery deterministic and read-only.

## Schema-vs-spec audit notes

- The current `spec-v1` record has no typed `references` field in this branch,
  so cross-slice agreement is captured in `contracts.json` rather than adding a
  schema-invalid field. The catalog contract is a logical schema-version
  contract, not a new Baton wire protocol.
- `BoardState` remains the canonical projection. This release extends each
  projected slice with selected state-evidence provenance; it does not create a
  TUI-only resolver or alter `slice-status-v1` records.
- `slice-status-v1` has no monotonic status-revision field. Lifecycle stage is
  therefore primary; `last_updated_at` is a deterministic tie and
  safety-attention discriminator, not a claim that independently authored
  branch timestamps establish global history.

## Proposed slice decomposition (approved)

- `S01-all-ref-board-catalog` — a CLI or automation caller can discover a
  stable, source-attributed catalog and its single high-water state evidence
  across local and remote-tracking refs plus the active working tree.
- `S02-tui-ref-aware-release-navigation` — a TUI operator can list and open a
  ref-only release using the S01 catalog snapshot and visibly render its elected
  state durability without re-resolving it.

## Track and touchpoint matrix

| File / surface | T1-ref-aware-board |
|---|---|
| `internal/git/git.go` and `internal/git/git_test.go` | ✓ |
| `internal/board/` catalog and state-evidence oracle/tests | ✓ |
| `cmd/sworn/board.go` and `cmd/sworn/board_test.go` | ✓ |
| `internal/tui/releases.go`, `internal/tui/board.go`, and TUI tests | ✓ |

One track is intentional: S02 consumes S01's catalog snapshot and overlaps the
board/TUI state-authority boundary established by S01. No other track may run
in parallel with it.

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Which duplicate ref wins when one release has several valid board copies? | N-02, S01 AC-02 | Resolved by the ratified four-level ranking above. |
| A-02 | What happens when a canonical release-worktree ref exists but its board is missing or malformed? | N-02, S01 AC-03 | Resolved: report a deterministic error and return non-zero; never omit or retarget silently. |
| A-03 | Which status is authoritative when lifecycle progress is split across refs and an uncommitted current working tree? | N-03, N-05, S01 AC-04, S02 AC-02 | Resolved: S01 elects valid topology-matching candidates using the lifecycle, attention, timestamp, durability, and source tie rules above; S02 and both CLI modes render that selected evidence unchanged. |
| A-04 | Can a process-local state lost before commit be recovered after a crash? | N-03, N-04 | Resolved: no. The oracle reports only current local ref-tip and active-working-tree evidence; historical reconstruction is explicitly deferred above. |

## Planning-gate triage

- **S01 initial spec-ambiguity check, PASS**: two non-blocking observations
  were retained as intentional precision boundaries. Error wording need only be
  deterministic and include release plus ref, not a brittle golden string; the
  required mutation transcript already has the canonical Rule 6 path
  `docs/release/<release>/<slice>/proof.md`.
- **S02 initial spec-ambiguity check, FAIL; first remediation recheck, PASS**:
  the earlier TUI-local arbitration and its selected-checkout limitation are
  now superseded by the owner-directed shared-oracle decision below. They remain
  historical review evidence, not implementation instructions.
- **Shared state-evidence amendment, human-directed**: the repository owner
  requires one high-water state source across CLI and TUI, with uncommitted
  winners made explicit in the oracle. The amendment moves all state election
  from S02 to S01, adds the provenance contract, and reopens one bounded
  spec-ambiguity check per materially changed slice. At most one remediation
  recheck per slice is authorised; no review fan-out is authorised.
- **S01 shared-state-evidence check, PASS**: the bounded
  `openai/gpt-5.3-codex` spec-ambiguity review found no ambiguity in the
  catalog, election, provenance, or CLI contract.
- **S02 shared-state-evidence check, FAIL then remediation PASS**: the sole
  initial pass found two presentation ambiguities: the release-list aggregate
  marker and the catalog-error observable. Remediation pins the exact suffix
  ` [uncommitted]` and the `Model.View()` `Error: <catalog error>` line; the
  one authorised recheck passed with no findings.

## Screenshots / references

- No screenshot is required. The issue's command-line repro and its TUI scope
  clarification are the durable references for this release.
