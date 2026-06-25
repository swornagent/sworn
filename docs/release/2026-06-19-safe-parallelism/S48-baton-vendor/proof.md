# Proof Bundle — S48-baton-vendor

## Scope

`sworn baton vendor` subcommand: reads a Baton protocol checkout, applies a transform replacing bash/node script references with sworn-native commands, writes into the binary's go:embed trees. Self-registers via S51/T15 command registry.

## Files changed



## Test results

```
$ go test -race -count=1 ./internal/baton/...
ok  	github.com/swornagent/sworn/internal/baton	1.245s
```

All 14 tests pass:
- TestTransformStripsScriptRefs (8 subtests — every map row + multiple + no-op)
- TestTransformAppliesToRulesAndPrompts
- TestTransformFailsClosedOnUnmappedScript (4 subtests — unknown .sh, .py, .mjs, clean)
- TestTransformIdempotent
- TestReplacementsAndGuardDerivedFromSameTable
- TestValidateSource
- TestValidateSource_MissingFile
- TestVendorWritesTransformedEmbed
- TestVendorIsIdempotent
- TestVendorCheckOnlyDoesNotWrite
- TestVendorFailsOnUnmappedScriptInSource

## Reachability artefact

`sworn baton vendor --check` against a temporary fixture source:

```
$ /tmp/sworn-prod baton vendor /tmp/test-source --check
--- a/internal/adopt/baton/rules/01-reachability-gate.md
+++ b/internal/adopt/baton/rules/01-reachability-gate.md
@@ -1,79 +1,2 @@
-2. Run `scripts/release-verify.sh <slice-id>` from a terminal.
+2. Run `sworn verify <slice-id>` from a terminal.
--- a/internal/adopt/baton/README.md
+++ b/internal/adopt/baton/README.md
@@ -1,1 +1,1 @@
-Run `release-verify.sh` to verify slices.
+Run `sworn verify` to verify slices.
...
```

The transform correctly replaces every script reference:
- `release-verify.sh` → `sworn verify`
- `release-board-status.sh` → `sworn board`
- `design-audit.sh` → `sworn designaudit`
- `captain-route.sh` → `the sworn internal router`
- `port-deriver.sh` → `native port derivation`
- `captain-memory-search.py` → `sworn memory search`

`sworn baton vendor --check` exits 0 and prints the diff without modifying the tree.

## Delivered

1. **Transform with single-table derive-both pattern** — `internal/baton/transform.go`: ordered, table-driven substitution map (6 entries from ADR-0006) with fail-closed guard derived from the same table. Regex-based to handle path prefixes (scripts/, bin/, $HOME/.claude/bin/). Evidence: `TestTransformStripsScriptRefs`, `TestReplacementsAndGuardDerivedFromSameTable`.

2. **Transform applies identically to rules and prompts** — same Transform() function used for all file types. Evidence: `TestTransformAppliesToRulesAndPrompts`.

3. **Fail-closed guard on unmapped scripts** — Transform returns error if a known token survives OR an unknown script reference is found. Evidence: `TestTransformFailsClosedOnUnmappedScript`.

4. **Vendor writes transformed embed** — `internal/baton/vendor.go`: reads source, applies Transform, writes to embed paths. Supports --check dry-run. Evidence: `TestVendorWritesTransformedEmbed`, `TestVendorCheckOnlyDoesNotWrite`.

5. **Vendor is idempotent** — running Vendor twice produces byte-identical output. Evidence: `TestVendorIsIdempotent`.

6. **Explicit file mapping** — `internal/baton/source.go`: hand-maintained `source_relpath → dest_abs_path` mapping for all Baton → SwornAgent files. Evidence: `TestValidateSource`.

7. **sworn baton vendor subcommand** — `cmd/sworn/baton.go`: self-registers via `init()` → `command.Register()` (S51/T15 registry). Supports `--check`. Evidence: reachability artefact above.

8. **Build is clean** — `go build ./...` succeeds.

## Not delivered

- **Network fetch of a Baton tag** — deferred as Rule 2 (why: S48 MVP is vendored snapshot on disk; tracking: GitHub issue #11; acknowledged: Coach-approved in approved-ack.md). A hook is left for future network resolution in `source.go` (the file is structured to accept a tag string; currently reads from a filesystem path).

## Divergence from plan

- `transform.go` uses regex-based replacement (not pure `strings.ReplaceAll`) to handle path-prefixed script references (e.g. `scripts/release-verify.sh` → `sworn verify`). The design specified substring replacement; regex was needed because Baton docs reference scripts with path prefixes (scripts/, bin/, $HOME/.claude/bin/). The regex is mechanically equivalent — it strips the path prefix and replaces with the sworn command. This is within the spirit of Design Decision §2.3 (string→string, file-format agnostic).
- `source.go` maps `claude/baton/README.md` to TWO destinations: `internal/adopt/baton/README.md` and `internal/prompt/baton/README.md`. Both embed directories need the README; this is handled transparently by the Vendor loop (same source, two writes).

## Coach flags addressed

- (a) `design_decisions` in status.json: populated with 5 Type-2 decisions from design.md §2.
- (b) Forward-handoff comment in baton.go: present (line 19: "Forward handoff to S50...").
