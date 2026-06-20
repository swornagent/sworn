# Design TL;DR — S26-telemetry

## §1. User-visible change

Every `sworn` invocation sends an anonymous telemetry event (command name, subcommand, duration_ms, exit_code, sworn_version, go_version, os/arch, install UUID) to `https://api.sworn.sh/v1/events` when the user has opted in. Opt-in is collected during `sworn init` (a callable `ShowConsent()` is provided; T3/S09 wires the actual prompt). Users manage consent post-init via the new `sworn telemetry on|off|status` subcommand. The first invocation of any `sworn` command on a fresh install shows a one-time disclosure on stderr. Telemetry is always non-blocking, silently drops on error, and never collects code, paths, or user identity. The `sworn telemetry *` meta-commands do NOT fire telemetry events (Coach Pin 4, option (a)). `sworn version` and `sworn help` still fire — useful version-usage signal.

## §2. Design decisions not in spec (max 5)

1. **`cmd/sworn/main.go` restructuring into `dispatch()`**: The spec's suggested `run(os.Args)` wrapper requires extracting the existing switch body into a `dispatch(args []string) int` function. Each case currently calls `os.Exit(cmdXxx(...))` directly; with the wrapper, `os.Exit` moves to after the telemetry fire. The version and help cases do NOT call os.Exit — dispatch() returns 0 for both explicitly (Coach Pin 3). The `telemetry` subcommand itself does NOT fire a telemetry event (meta-command exclusion, Coach Pin 4 option (a)).

2. **Config path hardcoded to `~/.config/sworn/`**: Following Coach Pin 5 option (a), the config directory is hardcoded as `filepath.Join(home, ".config", "sworn")` matching all spec ACs exactly. `os.UserConfigDir()` is NOT used — the cross-platform complexity is deferred to post-R3 when Windows and macOS are first-class targets.

3. **`cmd`/`sub` parsing from `os.Args`**: `cmd = os.Args[1]` (after dispatch); `sub = os.Args[2]` if present, else empty string. Only the immediate subcommand name — no flags, no arguments. Parsed inside `dispatch()` before dispatching to the specific `cmdXxx`.

4. **`ShowDisclosure` placement**: Called at the top of `main()`, before `dispatch()` — so it appears before command output. The sentinel check (`~/.config/sworn/.telemetry-disclosed`) prevents re-display. The disclosure only prints if neither opt-in nor opt-out sentinel exists (neutral/undecided state — Coach Pin 6), because once the user has been asked during `sworn init`, they've made a choice.

5. **`install-id` file write semantics**: Written on first `InstallID()` call (lazy, on first invocation that fires telemetry). If the config directory does not exist, `InstallID()` creates it with `os.MkdirAll(0700)`. If creation fails, returns `""` silently. Idempotent — subsequent calls read the cached in-memory value.

## §3. Files I'll touch grouped by purpose

- **New telemetry package** (`internal/telemetry/telemetry.go`): `IsEnabled()`, `InstallID()`, `Fire(cmd, sub, swornVersion string, durationMS int64, exitCode int)`, `ShowDisclosure()`, `ShowConsent(r io.Reader, w io.Writer) (bool, error)` — the core library. New file, T9 owns it.
- **Telemetry tests** (`internal/telemetry/telemetry_test.go`): All 11 tests from spec Required Tests plus TestShowConsent. New file, T9 owns it.
- **Main dispatch wrapper** (`cmd/sworn/main.go`): Extract switch into `dispatch(args []string) int`; wrap with `ShowDisclosure` + `telemetry.Fire`; add `case "telemetry":`. Additive edits.
- **Telemetry subcommand** (`cmd/sworn/telemetry.go`): `cmdTelemetry(args []string) int` with `on|off|status` sub-subcommands. New file, T9 owns it.

## §4. Things I'm NOT doing

- Not modifying `cmd/sworn/init.go` — T3/S09 owns the init flow; T9 provides `ShowConsent()` as a callable export. The spec's AC for `sworn init` consent is accepted as a cross-track dependency that will be verified when S09 lands.
- Not sequencing `cmd/sworn/main.go` merge order — T2/S04a (tui-foundation), T3/S06a (sworn-login-auth), T4/S08a (mcp-transport), and T8/S23 (memory-config) all plan this file. S26 restructures it (extracting dispatch()). The assembler must coordinate merge order across these tracks (Coach Pin 2).
- Not adding batching, buffering, retry, or local event queue (post-R3 per spec)
- Not integrating with OTel SDK (post-R3 per spec)
- Not adding `sworn config set telemetry` command (post-R3 per spec)
- Not building telemetry dashboard or query interface (out of scope)
- Not adding user-attributed telemetry linking install-id to SwornAgent account (post-R3 per spec)
- Not building `api.sworn.sh` backend — client ships ready; backend goes live separately

## §5. Reachability plan

- **Reachability artefact**: Run `rm -f ~/.config/sworn/.telemetry-disclosed && sworn version` against a clean config dir (no .telemetry-enabled or .no-telemetry present) — one-time disclosure text visible on stderr. Captured as terminal output in proof.md.
- **Integration test proof**: `go test -race ./internal/telemetry/...` passes.
- **Build proof**: `go build ./cmd/sworn/...` compiles with the new telemetry subcommand registered.

## §6. Open questions for the Coach

1. Should `sworn telemetry on|off|status` invocations themselves fire a telemetry event? **RESOLVED (Coach Pin 4, option (a))**: No — sworn telemetry * is excluded. sworn version and sworn help still fire.
2. Is the use of `os.UserConfigDir()` acceptable for the config path, or should we hardcode `~/.config/sworn/` for cross-platform consistency? **RESOLVED (Coach Pin 5, option (a))**: Hardcode `~/.config/sworn/` matching spec ACs exactly. Windows deferred post-R3.