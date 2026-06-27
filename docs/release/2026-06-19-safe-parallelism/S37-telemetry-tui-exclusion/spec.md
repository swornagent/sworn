---
title: 'S37-telemetry-tui-exclusion — no-args/TUI launch does not emit a junk telemetry event'
description: 'After the T9/T2 merge, a no-args `sworn` launches the TUI via dispatch(); main() then fires telemetry with cmd="" and duration = the whole interactive session. Exclude the no-args/TUI path from telemetry firing, in internal/telemetry.Fire() (where the meta-command exclusion already lives) — not in the shared cmd/sworn/main.go. Tracks swornagent/sworn#7.'
---

# Slice: `S37-telemetry-tui-exclusion`

## User outcome

Running `sworn` with no subcommand (which launches the TUI) does **not** emit a
telemetry event. Today it fires one with an empty `cmd` and a `duration_ms` equal to
the entire interactive TUI session — junk data. After this slice, the no-args/TUI path
is excluded from firing, consistent with the existing `sworn telemetry *` meta-command
exclusion.

## Entry point

`internal/telemetry.Fire(...)`. Verifiable by: a unit test that `Fire` with an empty
`cmd` (the no-args/TUI signal) records/sends nothing, while `Fire("verify", ...)` still
fires.

## Background

The merge resolution of `cmd/sworn/main.go` (T9 dispatch extraction + T2 no-args TUI
launch) made `main()` call `telemetry.Fire(cmd, sub, …)` for every invocation, where
`cmd = os.Args[1]` or `""` when there is no subcommand. The no-args case is the TUI
launch — interactive, long-lived, no command name. See `swornagent/sworn#7`.

## In scope

### `internal/telemetry` — exclude the no-args/TUI path

Extend `Fire()` (the same chokepoint that already excludes `sworn telemetry *`
meta-commands) to **no-op when `cmd == ""`** (the no-args/TUI launch). Read the existing
exclusion logic first and mirror its shape. Keep the exclusion in the telemetry package
so `cmd/sworn/main.go` — the "DOCUMENTED SHARED — additive dispatch only" file — is NOT
touched (avoids a shared-file collision with T2/S34).

## Out of scope

- Editing `cmd/sworn/main.go` (keep the shared dispatch file untouched).
- Any other telemetry behaviour change.

## Planned touchpoints

- `internal/telemetry/telemetry.go` (extend `Fire`'s exclusion)
- `internal/telemetry/telemetry_test.go` (tests)

## Acceptance checks

- [ ] `telemetry.Fire("", ...)` (empty cmd / TUI launch) records or sends nothing —
  asserted by test (no event written / transport not called)
- [ ] `telemetry.Fire("verify", ...)` (a real command) still fires — no regression
- [ ] the existing `sworn telemetry *` meta-command exclusion is unchanged and still passes
- [ ] `go build ./...` and `go test ./internal/telemetry/...` pass

## Required tests

- **Unit** `internal/telemetry/telemetry_test.go`:
  - `TestFireSkipsEmptyCmd`: `Fire("", ...)` is a no-op (event not emitted)
  - `TestFireStillFiresRealCmd`: `Fire("verify", ...)` emits (guards against over-broad exclusion)
  - existing meta-command exclusion test continues to pass
- **Reachability artefact**: run the telemetry tests; capture output showing the empty-cmd
  no-op and the real-command fire. Document in proof.md.

## Risks

- Over-broad exclusion that silently drops legitimate events. The `TestFireStillFiresRealCmd`
  test guards this. `cmd == ""` is the precise no-args signal (every real command has a name).

## Deferrals allowed?

None.
