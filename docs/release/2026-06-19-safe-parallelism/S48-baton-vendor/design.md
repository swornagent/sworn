# Design TL;DR — S48-baton-vendor

## §1. User-visible change

`sworn baton vendor <source-dir>` reads a filesystem checkout of the Baton protocol (rules + role prompts + protocol docs), applies a deterministic transform that replaces every Baton bash/node script reference with its sworn-native command equivalent, and writes the result into the binary's `go:embed` trees (`internal/adopt/baton/` and `internal/prompt/`). `--check` prints the transform diff without writing. Running the same source through the same binary produces byte-identical output (idempotent). A new `baton` subcommand self-registers via the S51/T15 command registry.

## §2. Design decisions not in spec (max 5)

1. **Single-table derive-both pattern** — the substitution map and the fail-closed guard token list are derived from one Go data structure (a `[]replacement` slice). This guarantees they can't drift apart (the Risk in spec.md). A test asserts every map entry appears in the guard derivation.
2. **Explicit file mapping, not recursive glob** — `source.go` uses a hand-maintained map of `source_relpath → dest_abs_path` rather than walking a directory tree. This is safer: a new file type upstream won't silently land in the embed without an explicit mapping decision.
3. **Transform is string→string, file-format agnostic** — `Transform(content string) string` applies regex/substring replacements uniformly; it does not parse markdown. A markdown-aware transform would break on the first upstream format change and isn't needed for script→command replacement.
4. **Source is a directory path, not a tag** — `sworn baton vendor <source-dir>` takes a path to a baton checkout (e.g. `~/projects/baton`). Tag resolution is S49's concern. The vendor command validates the source has the expected shape (rules/, role-prompts/, track-mode.md) and surfaces a clear error if not.
5. **Registration in baton.go's own init(), not commands.go** — follows the S51 pattern of per-file self-registration. `cmd/sworn/commands.go` and `cmd/sworn/main.go` are not edited.

## §3. Files I'll touch grouped by purpose

- **`internal/baton/transform.go`** — `Transform(content string) string` + the single `[]replacement` table + fail-closed guard. Core logic.
- **`internal/baton/transform_test.go`** — table-driven tests for every map row, rules+prompts fixtures, fail-closed guard, idempotence.
- **`internal/baton/source.go`** — `Source` struct that validates a source directory and enumerates the `source_relpath → dest_abs_path` file mapping.
- **`internal/baton/vendor.go`** — `Vendor(opts)` orchestrates: validate source → enumerate files → read → Transform → write (or diff for `--check`).
- **`internal/baton/vendor_test.go`** — `TestVendorWritesTransformedEmbed`, `TestVendorIsIdempotent` using fixture directories.
- **`cmd/sworn/baton.go`** — `sworn baton vendor [--check]` subcommand; `init()` self-registers `command.Register(command.Command{Name: "baton", ...})`. Does NOT edit `main.go` or `commands.go`.

## §4. Things I'm NOT doing

- **Network fetch of a Baton tag** — deferred as Rule 2 (why: S48 MVP is vendored snapshot on disk; tracking: GitHub issue #11; will surface a hook in `source.go` for future network resolution).
- **SHA → semver tag reconciliation** — S49's concern. S48 reads whatever source dir it's pointed at; the pin format is unchanged.
- **`sworn baton diff` or governance docs** — S50's concern.
- **Editing protocol content** — transform only rewrites script references → commands.
- **Editing `cmd/sworn/main.go` or `cmd/sworn/commands.go`** — baton self-registers from its own file.

## §5. Reachability plan

- **`sworn baton vendor testdata/fixture --check`** — prints the transform diff without writing; paste in proof.md. This is the integration-point artefact (Rule 1).
- **`go test -race ./internal/baton/... ./cmd/sworn/...`** — passes; includes the table-driven transform tests.
- **`go build ./...`** — clean.

## §6. Open questions for the Coach

*(empty)*