---
title: 'S48-baton-vendor — `sworn baton vendor`: semver-pinned vendor of upstream Baton + bash→sworn transform over rules AND prompts'
description: 'sworn embeds the Baton protocol, but the embed is a hand-maintained verbatim copy pinned to a raw SHA, and it carries Baton''s bash/node script references that the Go binary supersedes. S48 makes the embed a *build product*: `sworn baton vendor` reads a semver-pinned upstream Baton, applies a transform that strips script references → sworn-native commands across rules AND role-prompts, and writes the sworn-native embed. Re-running reproduces the embed deterministically, subsuming the one-time public-readiness scrub of script refs. depends_on S21 (which creates the embed targets) — see ADR-0006.'
---

# Slice: `S48-baton-vendor`

## User outcome

A maintainer runs `sworn baton vendor` and the binary regenerates its embedded Baton
protocol from a **semver-pinned** upstream Baton checkout — applying a transform that
replaces Baton's standalone bash/node script references (`release-verify.sh`,
`release-board-status.sh`, `design-audit.sh`, `captain-route.sh`, `port-deriver.sh`,
`captain-memory-search.py`) with the **sworn-native commands** that supersede them
(`sworn verify`, `sworn board`, `sworn designaudit`, the internal router, native
port derivation, `sworn memory search`) — across **both** the rule docs and the role
prompts. The result is committed as the sworn-native embed. Running it again on the
same pin produces an identical embed (deterministic), so the embed can never silently
drift from upstream and the one-time "scrub of script refs" is no longer a manual step.

## Entry point

`sworn baton vendor` — a new subcommand **self-registered** from `cmd/sworn/baton.go`
(`init()` → `command.Register(...)`, the S51/T15 registry) →
`internal/baton`. Reachable from the CLI on any checkout. (Does NOT edit `cmd/sworn/main.go`.)
`--check` (dry-run) prints the transform diff without writing.

## Background

Today the embed is two hand-maintained `go:embed` trees: `internal/adopt/baton/`
(rules `01`–`10` + `README.md` + `VERSION`) and `internal/prompt/baton/`
(`track-mode.md`; expanded to the full protocol by **S21-canonical-baton**). The pin
in `internal/adopt/baton/VERSION` is a raw 40-char SHA
(`cf158423f65c20860a3d4ec0310acb6cc7fb5aa0`), and the vendored docs reference Baton's
bash/node toolchain — correct for standalone Baton, wrong inside sworn where the Go
binary owns those capabilities. ADR-0006 records the architecture: the embed must be a
**build product** of (pinned tag + transform), not a curated copy.

This slice `depends_on S21-canonical-baton`: S21 establishes the full embed layout
(`internal/prompt/baton/**`) that the transform writes into. T14 starts only after T3
(which owns S21) merges, so the write targets exist and are not concurrently edited.

## In scope

- New package `internal/baton`:
  - `vendor.go` — `Vendor(opts)` orchestrates: resolve the pinned source (a Baton
    checkout/snapshot at the pinned **tag**), enumerate rule + role-prompt files,
    apply `Transform`, and write the sworn-native embed into `internal/adopt/baton/`
    and `internal/prompt/baton/`. Supports a dry-run (`--check`) that returns the diff
    without writing.
  - `transform.go` — `Transform(content string) string` applies an ordered,
    table-driven substitution map (the ADR-0006 table) plus a guard that **fails
    closed** if any known Baton script token survives the transform (so a new script
    reference upstream can't slip through unmapped). The map covers rules AND prompts
    identically.
  - `source.go` — resolves the pinned upstream source for a given tag. For this slice
    the source is a vendored snapshot directory (committed under the package or read
    from a configured path); fetching a tag over the network is **out of scope**
    (see Out of scope) — the pin is by tag string, the snapshot is the bytes.
- New `cmd/sworn/baton.go` — `sworn baton vendor [--check]` subcommand; usage text; **self-registers
  the `baton` verb** via `init()` → `command.Register(...)` (S51/T15 registry). Does NOT edit
  `cmd/sworn/main.go` — that file is owned solely by T15-cli-registry.
- Tests for the transform map (every row + the fail-closed guard) and for `Vendor`
  writing a transformed embed from a fixture source.

## Out of scope

- **Reconciling the pin from SHA → semver tag, and version surfacing** — that is
  **S49-baton-version**. S48 reads whatever pin S49 will set; it does not change the
  `VERSION` files' pin format. (S48 may consume a tag string but the authoritative
  pin-format change belongs to S49.)
- **`sworn baton diff` and the PR-up governance/process doc** — that is
  **S50-baton-governance**.
- **Network fetch of a Baton tag** (git clone/archive download at vendor time). The
  pinned source is a committed snapshot; networked fetch is a later enhancement and
  must be surfaced as a Rule-2 deferral if a hook is left for it.
- **Editing the protocol content itself.** The transform only rewrites script
  references → sworn commands; it does not add/remove/reword rules. Protocol changes
  go upstream (S50 governance).

## Planned touchpoints

- `internal/baton/vendor.go` (new)
- `internal/baton/transform.go` (new)
- `internal/baton/source.go` (new)
- `internal/baton/vendor_test.go` (new)
- `internal/baton/transform_test.go` (new)
- `cmd/sworn/baton.go` (new — self-registers the `baton` verb via the S51/T15 `command` registry; `main.go` NOT touched)
- `internal/adopt/baton/**`, `internal/prompt/baton/**` (write targets — created/owned
  by S21/T3; T14 `depends_on T3` so these are sequential, not concurrent)

## Acceptance checks

- [ ] `Transform` replaces every reference in the ADR-0006 map: a fixture string
  containing `release-verify.sh` → contains `sworn verify` and no longer contains
  `release-verify.sh`; same for `release-board-status.sh`→`sworn board`,
  `design-audit.sh`→`sworn designaudit`, `captain-route.sh`→router,
  `port-deriver.sh`→native, `captain-memory-search.py`→`sworn memory search`
- [ ] The transform applies identically to a role-prompt fixture and a rule fixture
  (one test each) — proving it covers rules AND prompts
- [ ] Fail-closed guard: a fixture containing a known Baton script token that is NOT
  in the substitution map causes `Transform`/`Vendor` to return a non-nil error
  (the embed is not written with an unmapped script ref)
- [ ] `Vendor` against a fixture source writes transformed content to the embed paths
  and is **idempotent** — running it twice produces byte-identical output
- [ ] `sworn baton vendor --check` exits 0 and prints a diff without modifying the
  tree (assert `git status` is unchanged after `--check`)
- [ ] `go test -race ./internal/baton/... ./cmd/sworn/...` passes; `go build ./...` clean

## Required tests

- **Unit**: `internal/baton/transform_test.go` — `TestTransformStripsScriptRefs`
  (table test over every map row), `TestTransformAppliesToRulesAndPrompts`,
  `TestTransformFailsClosedOnUnmappedScript`. `internal/baton/vendor_test.go` —
  `TestVendorWritesTransformedEmbed`, `TestVendorIsIdempotent`.
- **Reachability artefact**: paste `sworn baton vendor --check` output (the transform
  diff) in `proof.md`, plus the test run. The `--check` invocation through
  `cmd/sworn` is the integration-point proof (Rule 1), not just the leaf transform.

## Risks

- The fail-closed guard's token list must be kept in step with the substitution map —
  a script added to the map but not the guard list, or vice versa, defeats it. Put
  both in one table and derive the guard from it; assert that in a test.
- Writing into `internal/prompt/baton/**` collides with S21 if T14 starts before T3
  merges — the `depends_on T3` edge prevents this; the implementer must confirm S21
  is merged (Step 0 forward-merge) before running a real vendor.
- Idempotence requires stable file ordering and newline handling — normalise on write.

## Deferrals allowed?

Only the networked tag-fetch (see Out of scope) may be deferred, and only as a Rule-2
deferral (why + tracking + acknowledgement) recorded in `proof.md`. The transform,
the fail-closed guard, and idempotent write are core and may not be deferred.
