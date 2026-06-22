# Design TL;DR: S22-sworn-doctor

## §1. User-visible change

A developer runs `sworn doctor` and gets a structured health report covering four check groups: embedded prompt integrity (length + required headings + version), repo artifact audit (legacy `docs/baton/`, AGENTS.md splice state), local Baton sync (`~/.claude/baton/` vs embedded), and dependency version freshness (project pins vs catalog, sworn's own deps). Each failing check prints `[OK]`, `[WARN]`, or `[ERROR]` with a specific repair command. `--fix` auto-removes legacy artifacts; `--sync-baton` copies embedded Baton docs to `~/.claude/baton/`. Exit codes: 0 = clean/WARN-only, 1 = any ERROR, 2 = `--fix` applied changes.

## §2. Design decisions not in spec (max 5)

1. **Spec references "baton/rules.md" as a single file — actual structure has separate rule files.** The embedded Baton docs live as `internal/adopt/baton/rules/01-*.md` through `10-*.md` plus `README.md`. There is no combined `rules.md`. Decision: doctor checks the `README.md` for the `## The seven rules` heading and verifies all 10 rule files exist and are non-empty (the protocol grew from 7 to 10 rules). This matches the actual embed structure rather than the spec's aspirational single-file model.

2. **S18 (consideration-catalog) and S19 (sworn-induction) have NOT landed yet.** The spec's group 1 heading checks for `implementer.md` ("Dependency discipline", "Deviation check") and `verifier.md` ("Catalog conformance check", "independently query the package registry") reference headings that S19 would add. Group 4 references `docs/considerations.md` which S18 would create. Decision: implement all checks as specified, but the required-heading lists are defined as Go maps that can be empty for not-yet-landed slices. For this implementation, the S19-dependent headings are included in the required-headings map but will produce `[ERROR]` if missing — which is correct behaviour (the embed IS missing them). The test `TestDoctorAllOK` will run against the actual embedded prompts and assert all currently-required headings pass; S19-dependent headings are excluded from the "all OK" test until S19 lands. Group 4 checks are skipped entirely when `docs/considerations.md` is absent (matching the spec's "Runs if ... `docs/considerations.md` exists").

3. **Legacy splice marker is `## Engineering Process — Baton`, not `<!-- baton:start -->`.** The spec says to detect `<!-- baton:start -->` but the actual `internal/adopt` package uses `BatonSectionHeading = "## Engineering Process — Baton"` as the splice marker. Decision: check for the actual marker `## Engineering Process — Baton` (via `adopt.BatonSectionHeading`) rather than the spec's `<!-- baton:start -->`. The `<!-- baton:start -->` marker was never used in this codebase.

4. **`sworn://baton/rules` MCP pointer check is deferred.** The spec's group 2 mentions checking for a `sworn://baton/rules` MCP resource reference in AGENTS.md. This MCP pointer format doesn't exist yet (no slice has implemented it). Decision: skip this check — it would always WARN on every repo. Surface as a Rule 2 deferral in proof.md.

5. **Registry reachability check is injectable.** The spec requires `go list -m -u ./...` to be mockable for the "registry unreachable" test. Decision: extract the dep-freshness check into a function variable `var checkDepFreshness = defaultCheckDepFreshness` that can be overridden in tests. This avoids network calls in tests and lets the unreachable scenario be simulated.

## §3. Files I'll touch grouped by purpose

- **`cmd/sworn/doctor.go`** (new) — the `sworn doctor` subcommand: all four check groups, `--fix` and `--sync-baton` flags, exit code logic. Imports `internal/prompt` for embedded prompt access and `internal/adopt` for Baton docs FS and splice marker constant.
- **`cmd/sworn/doctor_test.go`** (new) — table-driven tests covering every acceptance check: all-OK, legacy baton dir, legacy splice, `--fix` removes baton dir, `--fix` migrates AGENTS.md, `--sync-baton`, no baton home, group 4 stale/empty pins, registry unreachable, heading ordering.
- **`cmd/sworn/main.go`** (DOCUMENTED SHARED — additive `case "doctor"` dispatch) — one-line case addition to the switch, plus usage text update.

## §4. Things I'm NOT doing

- **`sworn://baton/rules` MCP pointer check** — the MCP resource reference format doesn't exist yet. Deferral: why = no slice has implemented the `sworn://` URI scheme; tracking = S22 spec acceptance check; acknowledgement = Coach (via design review).
- **Automatic version bumping of Baton** — explicitly out of scope per spec.
- **Network-based update checks** — explicitly out of scope per spec; `go list -m -u` is local module cache only.
- **Fixing corrupted embedded prompts** — explicitly out of scope per spec.
- **Checking consistency between `docs/considerations.md` and the embedded catalog format** — that is `sworn lint` scope per spec.

## §5. Reachability plan

Reachability artefact: run `go test ./cmd/sworn/... -run Doctor` which exercises `cmdDoctor` through the `dispatch` function (the integration point). Additionally, run `sworn doctor` on this repo and capture full output in proof.md showing all checks. The test `TestDoctorAllOK` runs `cmdDoctor` against the actual embedded prompts (not mocks), proving the command reaches the real embed FS.

## §6. Open questions for the Coach

1. The spec's group 1 says `baton/rules.md` must contain all 7 rule headings, but the actual embed structure has 10 separate rule files (01-10) plus a README.md. Should doctor check for 7 or 10 rules? (Decision: check all 10 — the protocol grew, and missing any rule file is a corruption signal.)
2. The spec's group 1 says planner.md must contain `## Phase 1` through `## Phase 4`, but the actual planner.md uses `### Phase 1` (h3, not h2). Should doctor check for `## Phase` or `### Phase`? (Decision: check for the actual heading level `### Phase` since that's what's in the embed.)