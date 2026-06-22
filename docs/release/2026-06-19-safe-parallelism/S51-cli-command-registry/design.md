# Design TL;DR — S51-cli-command-registry

## §1. User-visible change

None. Every `sworn <verb>` produces identical output and exit codes. `sworn help`
now lists commands derived from the registry (`command.All()`) instead of a
hand-maintained string — `sworn lint` is listed as `sworn lint ac|trace`, which
is a faithful rendering of the verb's summary. The dispatch path inside `func
main()` changes from a 17-case `switch` to a single registry `Lookup`, but the
observable surface is byte-for-byte unchanged.

## §2. Design decisions not in spec (max 5)

1. **`cmdVerify` moves to new `cmd/sworn/verify.go`** — it currently lives
   inside `main.go` (the only command function embedded there). The spec says
   `main.go` must contain no `case` statements; moving `cmdVerify` to its own
   file is the natural completion of that reduction. Not listed in
   `planned_files` because it is relocation of existing code, not new logic.
2. **`Command.Run` does not take `context.Context`** — all existing `cmdXxx`
   functions have signature `func cmdXxx(args []string) int`. The registry
   matches this; no existing function needs a signature change.
3. **`version` variable reference via closure** — `version` is a package-level
   `var` in `main`; `commands.go` references it directly when registering the
   version handler. No need to thread it through `Command` as a field.
4. **No `interface` for `Command.Run`** — the `Command` struct has `Run func([]string) int`.
   A method `(c Command) Run(args []string) int` would be equivalent, but a
   field keeps the zero value meaningful and matches the registration pattern:
   `command.Register(command.Command{Name: "verify", Summary: "…", Run: cmdVerify})`.
5. **`usage()` derives its listing from `command.All()` but keeps hand-written
   descriptions** — each `Command` carries a `Summary` string used in the
   auto-generated usage block. The detailed prose (per-verb paragraphs below
   the usage listing) stays hand-maintained because it is not scannable from
   function signatures.

## §3. Files I'll touch grouped by purpose

- **New registry package**: `internal/command/registry.go` — `Command` struct,
  `Register`, `Lookup`, `All`; `internal/command/registry_test.go` — unit
  coverage for registration, lookup, sorted-all, duplicate-panic.
- **Central registration**: `cmd/sworn/commands.go` (new, T15-owned) — one
  `Register` call per verb for all 19 verbs that exist on `release-wt`, plus
  `version`/`--version`/`-v` and `help`/`--help`/`-h` aliases registered as
  distinct `Command` entries pointing to the same handlers.
- **Dispatch reduction**: `cmd/sworn/main.go` — replace the 17-case `switch`
  with `command.Lookup(os.Args[1])`; move `cmdVerify` + `openDeferralsFlag` to
  new `cmd/sworn/verify.go`; keep `main()`, `dispatch()`, `usage()`, and
  `version` var in main.go.
- **Integration test**: `cmd/sworn/commands_test.go` (new) — drives
  `command.Lookup` for every expected verb, asserts resolution and handler
  identity; asserts unknown verb returns not-found.
- **Existing verb files** — UNTOUCHED (out of scope).

## §4. Things I'm NOT doing

- **NOT** adding `init()` self-registration to any existing `<verb>.go` file
  (out of scope per spec: would collide with in-flight T3 on `run.go`/
  `memory.go` and T12 on `lint.go`).
- **NOT** changing any command's flags, output, exit codes, or behaviour.
- **NOT** touching T3's `login`/`account` commands (they don't exist on
  `release-wt` yet; T3 adopts the registry on its next forward-merge).
- **NOT** rewriting `usage()`'s prose paragraphs — only the command listing
  block is generated from the registry.
- **NOT** creating a `Command` interface — the struct is concrete; future
  subcommands that need context or richer configuration can extend the struct
  without breaking existing registrations.

## §5. Reachability plan

Explicit smoke transcript captured in `proof.md`:
1. `go build -o /tmp/sworn ./cmd/sworn/` — build succeeds
2. `/tmp/sworn help` — lists all 19 verbs from the registry
3. `/tmp/sworn version` — prints `sworn <v>` + `baton-protocol <v>`
4. `/tmp/sworn lint ac 2026-06-19-safe-parallelism` — same output as pre-refactor
5. `/tmp/sworn designfit 2026-06-19-safe-parallelism` — same output as pre-refactor
6. `/tmp/sworn bogusverb` — `unknown command "bogusverb"`, exit 64
7. `grep -c 'case "' cmd/sworn/main.go` — returns 0

CLI-only slice; no Playwright, no screenshot. Terminal transcript is the
reachability artefact.

## §6. Open questions for the Coach

None.