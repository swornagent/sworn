---
title: 'S26-telemetry — anonymous usage telemetry with init-time consent'
description: 'Instruments every sworn command with anonymous, non-PII telemetry events sent to api.sworn.sh/v1/events. Consent collected during sworn init; opt-out via env var or sentinel file at any time.'
---

# Slice: `S26-telemetry`

## User outcome

During `sworn init`, after the project is configured, the user is asked a
single consent question:

```
sworn collects anonymous usage telemetry to improve the product.
Data collected: command names, durations, exit codes, sworn version, OS/arch.
No code, specs, file paths, project names, or user identity is collected.
Schema: https://sworn.dev/telemetry

Enable telemetry? [Y/n]:
```

`Y` (or Enter) writes `~/.config/sworn/.telemetry-enabled`; `n` writes
`~/.config/sworn/.no-telemetry`. Either answer is final — sworn does not ask
again. Users can change their choice at any time:
```
sworn telemetry on    # enables
sworn telemetry off   # disables (equivalent to touch ~/.config/sworn/.no-telemetry)
sworn telemetry status # shows current state
```

After opting in, every `sworn` invocation fires a non-blocking telemetry event
to `api.sworn.sh/v1/events` — command name, duration, exit code, sworn version,
OS/arch, anonymous install UUID. Silently dropped if the endpoint is unreachable.
Users who never run `sworn init` get no telemetry until they do (no first-run
fallback disclosure — consent is explicit, not implied).

## Entry point

- `sworn init` — consent question added as the final step of the init flow
- `sworn telemetry <on|off|status>` — post-init management
- Telemetry fires from the command dispatch wrapper in `cmd/sworn/main.go` when
  `~/.config/sworn/.telemetry-enabled` exists and `SWORN_NO_TELEMETRY` is unset

## In scope

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

### Consent and opt-out

**`IsEnabled()` logic (checked on every invocation):**
1. `SWORN_NO_TELEMETRY=1` env var present → disabled (session override)
2. `~/.config/sworn/.no-telemetry` exists → disabled (permanent opt-out)
3. `~/.config/sworn/.telemetry-enabled` exists → enabled
4. Neither file exists → disabled (no consent yet; user has not run `sworn init`)

**`sworn init` consent step** (final step in S09's init flow):
- Prompt printed to stdout with schema URL
- `Y`/Enter → create `~/.config/sworn/.telemetry-enabled`
- `n`/`N` → create `~/.config/sworn/.no-telemetry`
- Never asks again after either file exists
- Non-interactive mode (`sworn init --non-interactive`) defaults to disabled

**`sworn telemetry` subcommand:**
- `sworn telemetry on` → create `~/.config/sworn/.telemetry-enabled`, remove
  `.no-telemetry` if present; print confirmation
- `sworn telemetry off` → create `~/.config/sworn/.no-telemetry`, remove
  `.telemetry-enabled` if present; print confirmation
- `sworn telemetry status` → print current state (enabled/disabled + which
  mechanism: env var, sentinel file, or init not run)

**Env var override** `SWORN_NO_TELEMETRY=1` always wins regardless of sentinel
files — allows CI systems and scripts to suppress telemetry without touching
the filesystem.

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
- `cmd/sworn/main.go` (additive: dispatch wrapper + telemetry.IsEnabled check)
- `cmd/sworn/telemetry.go` (new: `sworn telemetry on|off|status` subcommand)

**Note on `sworn init` integration:** `cmd/sworn/init.go` is owned by T3/S09
(per the touchpoint matrix). T9 ships `internal/telemetry.ShowConsent()` as a
callable function; T3/S09 adds the consent question to `sworn init` by importing
it. T9 should merge before S09 starts; if it hasn't, the T3 implementer stubs
the call and wires it once T9 lands.

## Acceptance checks

- [ ] `sworn init` (interactive) presents the telemetry consent question as its
  final step; answering `Y` creates `~/.config/sworn/.telemetry-enabled`;
  answering `n` creates `~/.config/sworn/.no-telemetry`
- [ ] `sworn init --non-interactive` skips the consent question and creates
  `~/.config/sworn/.no-telemetry` (defaults to off)
- [ ] After opting in via `sworn init`, the next `sworn run` fires a telemetry
  event; after opting out, no event fires
- [ ] `sworn telemetry on` creates `~/.config/sworn/.telemetry-enabled` and
  removes `.no-telemetry` if present; `sworn telemetry off` does the reverse
- [ ] `sworn telemetry status` prints `telemetry: enabled` or `telemetry: disabled`
  and the mechanism (env var / init opted-in / init opted-out / init not run)
- [ ] `SWORN_NO_TELEMETRY=1 sworn run --task "x"` completes without firing any
  HTTP request even when `.telemetry-enabled` exists (env var wins)
- [ ] A successful telemetry event POSTed to `httptest.NewServer` contains
  exactly the fields in the schema above and no others
- [ ] `sworn run` exits within 10ms of the run completing regardless of whether
  the telemetry endpoint is reachable (non-blocking confirmed)
- [ ] `install-id` file contains a valid UUIDv4; running `sworn` twice produces
  the same install-id
- [ ] If `~/.config/sworn/` cannot be created, sworn runs normally and telemetry
  fires with `install_id: ""`; no panic
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
