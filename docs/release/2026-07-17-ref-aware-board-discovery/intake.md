---
title: 'Release intake: ref-aware board discovery'
description: 'Planning record for deterministic all-ref release-board discovery in the CLI and TUI.'
---

# Release Intake: `2026-07-17-ref-aware-board-discovery`

## Release goal

Make every release plan that already exists in the local Git object database
visible to a Sworn operator without requiring that plan to be checked out. A
shipped release lets an automation caller run `sworn board --json` from the
project root and receive a deterministic catalog, while an interactive operator
can see and open the same ref-only releases in the TUI. Existing named board
queries retain their established result shape and behaviour.

## Needs

- N-01: **Project-wide board catalog.** An operator or release-mode caller can
  discover every valid release board in local and remote-tracking refs through
  `sworn board` without supplying a release name.
- N-02: **Deterministic, fail-closed source selection.** Every catalog record
  identifies the chosen source ref, uses the ratified priority order, and
  surfaces missing or malformed canonical records instead of silently omitting
  them or selecting a different plan.
- N-03: **Ref-aware TUI navigation.** A TUI operator can see and open the same
  ref-only release catalog, including its oracle-resolved slice state.
- N-04: **Read-only compatibility.** Discovery does not fetch, create, update,
  check out, or otherwise mutate Git refs, the current worktree, or the current
  working directory; named board queries remain backward compatible.

## Source of truth

- **Human stakeholder**: repository owner / release operator
- **Tracking issue / epic**: [sworn#123](https://github.com/swornagent/sworn/issues/123)
- **TUI scope clarification**: [sworn#123 comment](https://github.com/swornagent/sworn/issues/123#issuecomment-4990122697)
- **Related captures**: none; the issue contains the live reachability repro
  and acceptance intent.

## Users and their gestures

- **Release-mode automation caller**: runs `sworn board --json` at a consumer
  project root without knowing release names and receives a deterministic
  catalog whose records name their selected source ref.
- **CLI release operator**: runs `sworn board` without `--release` and sees a
  readable section for each discovered release; continues to use
  `sworn board --release <name> [--json]` with its existing single-release
  shape.
- **TUI operator**: launches `sworn`, selects a release that exists only on a
  non-HEAD ref, and opens its board with the same ref-aware plan and current
  oracle slice state as the CLI.

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
- `internal/tui/board.go` then loads the selected plan from the filesystem, so
  merely adding a ref-only name to the list would not make the board open.

## What the human wants

- Make `--release` optional for `sworn board`; an omitted filter returns every
  discoverable release board.
- Emit a stable project-level JSON document with a `releases` object keyed by
  release name. Each record contains `release`, `sourceRef`, and the existing
  `tracks` content.
- Render every discovered release as a separate readable CLI section.
- Preserve the existing JSON and text contracts when `--release <name>` is
  supplied.
- Discover only refs already available locally, including remote-tracking refs;
  never fetch or mutate Git state as a side effect of discovery.
- Make the TUI list and open the same catalog using the same source-selection
  rules and existing slice-state oracle priority.

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
- A valid named query keeps its existing single-release output shape; the new
  aggregate shape applies only when `--release` is omitted.
- No API, network, credential, personal-data, persistence, compliance, or
  browser-facing surface is introduced. Accessibility is not applicable because
  the TUI uses its existing keyboard-operable release list and board views.
- Enumeration must remain linear in the number of available refs and board
  paths. It must not read every blob in every repository tree when a ref has no
  release-board path.

## Adjacent / out of scope

- **Ref fetching or remote configuration**: deferred because this release must
  be observational only. **Tracking**: [sworn#123](https://github.com/swornagent/sworn/issues/123).
  **Acknowledged**: repository owner, 2026-07-17.
- **Changing release-mode commands other than `board` and the TUI**: deferred
  because route, merge, lint, and run operate on an explicitly named release
  and are not part of the reported reachability gap. **Tracking**:
  [sworn#123](https://github.com/swornagent/sworn/issues/123). **Acknowledged**:
  repository owner, 2026-07-17.
- **Redesigning the TUI layout or keyboard bindings**: deferred because the
  user outcome is discovery and board opening, not a new interaction model.
  **Tracking**: [sworn#123](https://github.com/swornagent/sworn/issues/123).
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
  selection contract, so they must be independently verifiable yet implemented
  sequentially on one track.

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

## Schema-vs-spec audit notes

- The current `spec-v1` record has no typed `references` field in this branch,
  so cross-slice agreement is captured in `contracts.json` rather than adding a
  schema-invalid field. The catalog contract is a logical schema-version
  contract, not a new wire protocol.
- `BoardState` is already the canonical slice-state projection. This release
  adds a catalog around it and must not create a competing status resolver.

## Proposed slice decomposition (approved)

- `S01-all-ref-board-catalog` — a CLI or automation caller can discover a
  stable, source-attributed catalog across local and remote-tracking refs.
- `S02-tui-ref-aware-release-navigation` — a TUI operator can list and open a
  ref-only release using the S01 catalog and the existing oracle state rules.

## Track and touchpoint matrix

| File / surface | T1-ref-aware-board |
|---|---|
| `internal/git/git.go` and `internal/git/git_test.go` | ✓ |
| `internal/board/` all-ref catalog resolver and tests | ✓ |
| `cmd/sworn/board.go` and `cmd/sworn/board_test.go` | ✓ |
| `internal/tui/releases.go`, `internal/tui/board.go`, and TUI tests | ✓ |

One track is intentional: S02 consumes the catalog selection contract and
overlaps the board/TUI oracle boundary established by S01. No other track may
run in parallel with it.

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Which duplicate ref wins when one release has several valid board copies? | N-02, S01 AC-02 | Resolved by the ratified four-level ranking above. |
| A-02 | What happens when a canonical release-worktree ref exists but its board is missing or malformed? | N-02, S01 AC-03 | Resolved: report a deterministic error and return non-zero; never omit or retarget silently. |
| A-03 | Whether the TUI can load a ref-only plan after listing it. | N-03, S02 AC-01 | Resolved: consume the shared catalog/sourceRef and invoke the existing Git-ref oracle, with live-working-tree preference retained only when the selected ref is the active checkout. |

## Planning-gate triage

- **S01 initial spec-ambiguity check, PASS**: two non-blocking observations
  were retained as intentional precision boundaries. Error wording need only be
  deterministic and include release plus ref, not a brittle golden string; the
  required mutation transcript already has the canonical Rule 6 path
  `docs/release/<release>/<slice>/proof.md`.
- **S02 initial spec-ambiguity check, FAIL**: AC-02 did not explicitly cover a
  valid, uncommitted checkout status when sourceRef pointed elsewhere, and used
  the undefined word "stale". Remediation makes all non-selected checkout files
  ineligible regardless of validity, defines the exact selected-checkout test,
  and retains committed-oracle fallback for absent or malformed selected files.
- **S02 first remediation recheck, PASS**: the two low-severity observations
  are non-contractual. AC-04 deliberately preserves named, existing interaction
  tests rather than replacing stable keyboard/layout output with a new UI copy
  contract; AC-03's non-Git fallback is limited by the existing
  `git rev-parse HEAD` viability check and the named filesystem fallback test.
  Neither changes source selection, state authority, exit behaviour, or the
  ref-only user outcome, so no further recheck is warranted.

## Screenshots / references

- No screenshot is required. The issue's command-line repro and its TUI scope
  clarification are the durable references for this release.
