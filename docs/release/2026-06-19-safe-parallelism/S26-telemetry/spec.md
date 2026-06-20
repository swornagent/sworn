---
title: 'S26-telemetry — anonymous usage telemetry with opt-out'
description: 'Instruments every sworn command with anonymous, non-PII telemetry events sent to api.sworn.sh/v1/events. Opt-out via env var or sentinel file. First-run disclosure on stderr.'
---

# Slice: `S26-telemetry`

## User outcome

On first run after install, a developer sees a one-time disclosure on stderr:

```
sworn collects anonymous usage telemetry (command names, durations, exit codes).
No code, specs, file paths, or project data is collected. To opt out:
  export SWORN_NO_TELEMETRY=1   (session)
  touch ~/.config/sworn/.no-telemetry  (permanent)
Schema: https://sworn.dev/telemetry
```

After that, every `sworn` invocation fires a non-blocking telemetry event to
`api.sworn.sh/v1/events`. The event contains only the command name, duration,
exit code, sworn version, OS/arch, and an anonymous UUID tied to the install
— nothing that identifies the user, the project, or any content. If the
endpoint is unreachable (including during the pre-launch period before
`api.sworn.sh` is live), the event is silently dropped and the command exits
normally.

## Entry point

Telemetry fires from the command dispatch wrapper in `cmd/sworn/main.go`.
No new user-invocable subcommand. Opt-out is via env var or sentinel file.

## In scope

### Event schema (narrow and public)

```json
{
  "v": 1,
  "install_id": "<random UUID written to ~/.config/sworn/install-id on first run>",
  "cmd": "run",
  "sub": "parallel",
  "duration_ms": 1234,
  "exit_code": 0,
  "sworn_version": "0.1.0",
  "go_version": "go1.26",
  "os": "linux",
  "arch": "amd64"
}
```

**`cmd`**: top-level subcommand only (e.g. `run`, `memory`, `mcp`, `login`).
**`sub`**: immediate sub-subcommand only (e.g. `build`, `search`, `status`).
Empty string if no subcommand. **Never** includes flags, arguments, file paths,
slice IDs, release names, model names, or any other content.

Schema is versioned (`v: 1`) and published at `https://sworn.dev/telemetry`.

### `internal/telemetry/` package

- `internal/telemetry/telemetry.go`:
  - `IsEnabled() bool` — checks `SWORN_NO_TELEMETRY` env var and
    `~/.config/sworn/.no-telemetry` sentinel file; returns false if either present
  - `InstallID() string` — reads or creates `~/.config/sworn/install-id`
    (random UUID, generated once, never changes); returns empty string on any
    I/O error (fail-open — missing install-id means anonymous event, not skip)
  - `Fire(cmd, sub string, durationMS int64, exitCode int)` — constructs the
    event JSON, POSTs to `https://api.sworn.sh/v1/events` in a goroutine with
    a 2-second timeout. Never blocks the caller. Never panics. Any error
    (network, non-2xx response, timeout) is silently dropped.
  - `ShowDisclosure(w io.Writer)` — prints the one-time disclosure; called only
    when `~/.config/sworn/.telemetry-disclosed` does not exist; writes the
    sentinel after printing

- `internal/telemetry/telemetry_test.go`:
  - Tests for `IsEnabled()` with both opt-out mechanisms
  - Tests for `InstallID()` idempotency (same UUID on repeated calls)
  - Tests for `Fire()` using `httptest.NewServer` — confirms correct JSON shape,
    correct `Authorization` header absent (no user auth on telemetry events),
    confirms the call is non-blocking (returns before server responds)
  - Tests for `ShowDisclosure()` — confirms sentinel file created after first call,
    not printed on second call

### `cmd/sworn/main.go` integration

Wrap the top-level command dispatch:

```go
start := time.Now()
exitCode := run(os.Args)   // existing dispatch
telemetry.Fire(cmd, sub, time.Since(start).Milliseconds(), exitCode)
os.Exit(exitCode)
```

`ShowDisclosure(os.Stderr)` called once before `run()` on first invocation.

The telemetry goroutine is fire-and-forget. `main` exits immediately after
`os.Exit(exitCode)` — the Go runtime will not wait for the goroutine if it
is still in-flight. For the 2s timeout and typical < 100ms network latency,
this means events sent to a reachable endpoint are delivered; events to an
unreachable endpoint are dropped when the process exits. This is acceptable:
telemetry is best-effort, not guaranteed delivery.

### Opt-out mechanisms

1. **Env var** `SWORN_NO_TELEMETRY=1` (or any non-empty value): checked at
   every invocation. Session-scoped; no persistence.
2. **Sentinel file** `~/.config/sworn/.no-telemetry`: permanent opt-out.
   User creates it manually or sworn may add a `sworn config set telemetry=false`
   command in a future release.
3. **First-run disclosure** `~/.config/sworn/.telemetry-disclosed`: records
   that the disclosure has been shown. Not an opt-out mechanism — its absence
   triggers the disclosure, its presence suppresses it.

### `install-id` generation

`~/.config/sworn/install-id` is a plain text file containing a random UUID
(crypto/rand, UUIDv4 format). Written on first `sworn` invocation if absent.
Never derived from hostname, username, git config, or any identifiable source.
If the file cannot be written (permissions), `InstallID()` returns `""` and
the event fires with an empty install_id — this is acceptable for anonymity
but means the event cannot be correlated across invocations.

## Out of scope

- `sworn config set telemetry=false` subcommand (post-R3 — manual sentinel file
  covers opt-out for R3)
- Batching or local buffering (events fire immediately; if unreachable, they drop)
- Telemetry dashboard or query interface for SwornAgent
- Event schema v2 or richer event types (post-R3)
- User-attributed telemetry (linking install-id to a SwornAgent account from
  `sworn login`) — post-R3; R3 is always anonymous
- OTel SDK integration (events use a bespoke JSON schema; OTel export is
  a future migration if SwornAgent adopts OTel collector)

## Planned touchpoints

- `internal/telemetry/telemetry.go` (new)
- `internal/telemetry/telemetry_test.go` (new)
- `cmd/sworn/main.go` (additive wrap of dispatch + disclosure call)

## Acceptance checks

- [ ] On first `sworn` invocation after install (no `~/.config/sworn/.telemetry-disclosed`),
  the disclosure is printed to stderr; subsequent invocations print nothing
- [ ] `SWORN_NO_TELEMETRY=1 sworn run --task "x"` completes without firing any
  HTTP request (verified with `httptest.NewServer` in unit test; no live call needed)
- [ ] `touch ~/.config/sworn/.no-telemetry` then `sworn run` completes without
  firing any HTTP request
- [ ] A successful telemetry event POSTed to `httptest.NewServer` contains
  exactly the fields in the schema above and no others; `cmd` is the top-level
  subcommand; `sub` is the immediate sub-subcommand or empty string
- [ ] `sworn run` exits within 10ms of the run completing regardless of whether
  the telemetry endpoint is reachable (non-blocking confirmed)
- [ ] `install-id` file contains a valid UUIDv4; running `sworn` twice in the
  same install produces the same install-id
- [ ] If `~/.config/sworn/` cannot be created (e.g. `/dev/null` pointed at it),
  sworn runs normally and telemetry fires with `install_id: ""`; no panic
- [ ] `go test -race ./internal/telemetry/...` passes

## Required tests

- **Unit**: `internal/telemetry/telemetry_test.go`
  - `TestIsEnabled_EnvVar`: set `SWORN_NO_TELEMETRY=1`; `IsEnabled()` returns false
  - `TestIsEnabled_Sentinel`: create sentinel file; `IsEnabled()` returns false
  - `TestIsEnabled_Neither`: no env var, no sentinel; `IsEnabled()` returns true
  - `TestInstallIDIdempotent`: call `InstallID()` twice; same UUID returned; file
    written once
  - `TestInstallIDWriteFailure`: point config dir at unwritable path; `InstallID()`
    returns `""`, no panic
  - `TestFireSchema`: `httptest.NewServer` captures the POST; assert JSON fields
    match schema exactly (no extra fields, no missing required fields)
  - `TestFireNonBlocking`: server sleeps 5s; `Fire()` returns in <100ms (goroutine)
  - `TestFireSilentOnError`: server returns 500; no panic, no log noise
  - `TestShowDisclosure_FirstRun`: sentinel absent; disclosure printed; sentinel created
  - `TestShowDisclosure_SubsequentRun`: sentinel present; nothing printed

- **Reachability artefact**: `sworn version` (or any command) executed after
  deleting `~/.config/sworn/.telemetry-disclosed`; disclosure text appears on
  stderr. Captured in proof.md.

## Risks

- `api.sworn.sh/v1/events` may not be live at R3 ship time.
  **Mitigation**: `Fire()` silently drops on connection refused / timeout; no
  user impact. Telemetry begins flowing once the backend is live; the client
  ships ready.
- Goroutine leak if `Fire()` is called many times in a tight loop.
  **Mitigation**: `Fire()` is called once per process invocation (from `main`
  after dispatch returns); no loop risk.
- Users may distrust anonymous telemetry even with clear disclosure.
  **Mitigation**: schema is public at `https://sworn.dev/telemetry`; opt-out
  requires one env var or one file touch; no account required to opt out.

## Deferrals allowed?

Yes:
- `sworn config set telemetry=false` command — post-R3 (sentinel file covers it)
- User-attributed telemetry (linking install-id to SwornAgent account) — post-R3
- Batching / local buffer — post-R3 (best-effort fire-and-forget is sufficient)
