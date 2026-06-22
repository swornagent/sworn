# Design TL;DR — `S21-canonical-baton`

## §1. User-visible change

When a developer runs `sworn init` on a new repo, they get a minimal `AGENTS.md`
pointing at `sworn://baton/rules` via MCP — no `docs/baton/` directory is
created. The Baton protocol lives in the binary. Updating the protocol means
updating the `sworn` binary, never re-running `sworn init`. For existing repos
with legacy Baton-spliced `AGENTS.md` (containing `## Engineering Process —
Baton`), `sworn init` prints a migration warning directing them to `sworn
doctor`.

## §2. Design decisions not in spec (max 5)

1. **Single-file `rules.md` for embed, not separate files** — Simpler embed path,
   easier to look up, and the adopt package already has separate files for `sworn
   doctor --sync-baton`. `BatonAll()` just returns the one rules.md for now,
   alongside track-mode.md, session-discipline.md, brainstorm-patterns.md,
   README.md, and VERSION.txt.
2. **Keep `adopt` package intact** — Doctor (`cmd/sworn/doctor.go`) uses
   `BatonDocsFS()`, `BatonSectionHeading`, and `AgentsFragment()`. The `adopt`
   package is NOT deleted; only the calls from `init.go` are removed. The
   embedded content in `adopt`'s `go:embed` continues to serve doctor/sync
   purposes.
3. **Legacy detection uses `## Engineering Process — Baton`** — Per spec risk
   note, checked against actual `adopt.SpliceAgents` marker. No `<!--
   baton:start -->` marker exists in the codebase; the heading is the reliable
   signal.
4. **AGENTS.md template lives at `docs/templates/agents.md`** — Mirrors the
   existing pattern (`docs/templates/considerations.md`,
   `docs/templates/decisions.md`) already used by `materialiseCatalog`.
5. **`BatonAll()` returns `map[string]string` keyed by filename** —
   `"rules.md"`, `"track-mode.md"`, `"session-discipline.md"`,
   `"brainstorm-patterns.md"`, `"README.md"`, `"VERSION.txt"`. This is the map
   consumed by the MCP resource listing (which is a different slice, but the
   function signature contract is set here).

## §3. Files I'll touch grouped by purpose

**New — Embedded Baton protocol:**
- `internal/prompt/baton/rules.md` — all 10 rules concatenated from
  `internal/adopt/baton/rules/01`–`10`
- `internal/prompt/baton/session-discipline.md` — from canonical
  `~/.claude/baton/session-discipline.md`
- `internal/prompt/baton/brainstorm-patterns.md` — from canonical
  `~/.claude/baton/brainstorm-patterns.md`
- `internal/prompt/baton/README.md` — index/overview of embedded Baton docs
- `internal/prompt/baton/VERSION.txt` — version string (copied from existing
  `internal/prompt/VERSION.txt`)

**Modified — Embed wiring:**
- `internal/prompt/prompt.go` — extend `go:embed` to `baton/*`; add
  `Baton(name)`, `BatonAll()`; keep existing `TrackMode()` / `BatonVersion()`
  but they now read from the new embed paths

**Modified — `sworn init` rewrite:**
- `cmd/sworn/init.go` — remove `adopt.Materialise`, `adopt.SpliceAgents`,
  `adopt.PlanSplice`, `adopt.BatonDocsExist` calls; add AGENTS.md creation from
  `docs/templates/agents.md`; update scan/apply/final messaging

**New — Template:**
- `docs/templates/agents.md` — minimal MCP-pointer template (spec provides exact
  content)

**New — ADR:**
- `docs/adr/0005-canonical-baton.md` — architecture decision record

**Modified — Tests:**
- `cmd/sworn/init_test.go` — add `TestInitCreatesAgentsMD`,
  `TestInitSkipsExistingAgentsMD`, `TestInitWarnsLegacyBaton`,
  `TestInitDoesNotSpliceClaude`
- `internal/prompt/prompt_test.go` — add `TestBatonRulesNonEmpty`,
  `TestBatonAllKeys`, `TestBatonRulesHasAllTen`, `TestBatonMissingFile`

## §4. Things I'm NOT doing

- **NOT removing the `adopt` package** — doctor.go depends on it. It becomes a
  "legacy support" package for the doctor subcommand.
- **NOT touching `cmd/sworn/doctor.go`** — except possibly updating its embed
  check to also verify the new `internal/prompt/baton/` files exist (out of scope
  for this slice but noted).
- **NOT touching `~/.claude/baton/`** — as specified.
- **NOT adding MCP resource endpoints** — that's owned by `S08a-mcp-transport` /
  `S08b-mcp-ops-tools`. This slice only adds the embed functions that MCP will
  call.

## §5. Reachability plan

Artefact: run `sworn init` in a temp directory, capture stdout, `ls` the
directory to confirm no `docs/baton/`, `cat AGENTS.md` to confirm MCP config
block present, and run `go test ./cmd/sworn/... -run Init` and `go test
./internal/prompt/... -run Baton` for programmatic reachability. Output captured
as text in `proof.md`.

## §6. Open questions for the Coach

None.