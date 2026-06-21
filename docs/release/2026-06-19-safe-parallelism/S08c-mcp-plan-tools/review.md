# Captain review — S08c-mcp-plan-tools
Date: 2026-06-21
Design commit: 8799676ae619979a3f3ccabdf3d41f2f6e650bc5

> **Drift note:** T4-mcp track is 29 commits behind `release-wt/` (from T9-telemetry and T11-infra-safety merges). These tracks are orthogonal to MCP tooling; S08c's spec/design are unaffected. Review proceeded.

## Pins

1. `[mechanical]` §3 — `resources/read` and `prompts/get` absent from server dispatch; no `RegisterResource`/`RegisterPrompt` API on `Server`
   What I observed: `server.go`'s `buildMethodHandlers()` dispatches `resources/list` and `prompts/list` but has no `"resources/read"` or `"prompts/get"` entries. No `RegisterResource()` or `RegisterPrompt()` method exists on the `Server` struct. Design §3 plans `resources.go` and `prompts.go` but says nothing about extending `server.go`.
   What to ask: Before implementing resources.go/prompts.go, add to server.go: (a) `RegisterResource(uri string, handler ResourceHandler)` and `RegisterPrompt(name string, handler PromptHandler)` registration methods, (b) `"resources/read"` and `"prompts/get"` entries in `buildMethodHandlers()`, (c) update `handlePromptsList` to enumerate registered prompts. Without these, every `resources/read` and `prompts/get` call returns JSON-RPC "Method not found" — the entire resource/prompt system ships silent-broken.

2. `[escalate]` §3 — `sworn://baton/rules` → `internal/prompt/baton/rules.md` but no source file exists
   What I observed: Spec maps `sworn://baton/rules` to `internal/prompt/baton/rules.md` ("full Baton protocol"). Neither `internal/prompt/baton/` nor any `rules.md` file exists in the repo or at `~/.claude/baton/` — baton rules are split across individual files (`adversarial-verification.md`, `no-silent-deferrals.md`, `proof-bundle.md`, etc.; `~/.claude/baton/README.md` is 139 lines describing the system but is not a consolidated protocol document). No single combined "full Baton protocol" document exists anywhere.
   What to ask: Coach must decide: (a) create a new combined `internal/prompt/baton/rules.md` from the individual baton rule files (author it as part of this slice), (b) use an existing file as-is (`README.md` or `AGENTS-fragment.md`), (c) defer `sworn://baton/rules` post-R3 with Rule 2 compliance and amend the spec accordingly. Neither the design nor the spec identifies a source — implementer cannot proceed on this resource without a decision.

3. `[mechanical]` §3 + §2.5 — `internal/prompt/baton/` directory missing; `track-mode.md` and `VERSION.txt` need sourcing
   What I observed: Spec maps `sworn://baton/track-mode` → `internal/prompt/baton/track-mode.md` and `sworn://baton/version` → `internal/prompt/baton/VERSION.txt`. The directory `internal/prompt/baton/` does not exist in the repo. Source for `track-mode.md` exists at `~/.claude/baton/track-mode.md` (vendoring needed). `VERSION.txt` already exists at `internal/prompt/VERSION.txt` — the spec references a separate copy at `baton/VERSION.txt` which would either duplicate it or point to the same file. Design §2 Decision 5 says "go:embed in internal/prompt/embed.go (or similar)" without addressing baton/ subdirectory creation or the VERSION.txt duplication question.
   What to ask: Create `internal/prompt/baton/` directory; vendor `~/.claude/baton/track-mode.md` there; decide whether to create a `baton/VERSION.txt` symlink/copy or serve `sworn://baton/version` from the existing `internal/prompt/VERSION.txt` via the resource handler (and update the spec path reference accordingly). Update the go:embed directive.

4. `[memory-cited]` §2 Decision 2 — `gopkg.in/yaml.v3` new runtime dep not covered by ADR
   What I observed: Decision 2 proposes `gopkg.in/yaml.v3` for YAML frontmatter parsing in `set_track`. `grep yaml go.mod go.sum` returns no matches — yaml.v3 is completely absent from the module graph. Per [[project_dep_policy]], each new dep requires an ADR commit before it appears in go.mod. No ADR for yaml.v3 is mentioned in the design.
   What to ask: Either (a) write a brief ADR entry (analogous to ADR-0004) before adding yaml.v3 to go.mod, or (b) implement frontmatter manipulation using stdlib strings — index.md's frontmatter is narrow in scope (known key set, single-quoted scalar values), making a targeted regex/replacement approach viable without a full YAML parser. Option (b) avoids a new dep entirely.
   Citation: [[project_dep_policy]]

5. `[mechanical]` §3 — `cmd/sworn/mcp.go` missing from planned_files
   What I observed: `cmd/sworn/mcp.go` already contains the comment "Planning tools (S08c) register here in a later slice" at the `mcp.RegisterOpsTools(...)` call site in `cmdMcp()`. This file must be edited in S08c to add the planning tool registration call (analogous to `mcp.RegisterOpsTools`). Design §3 says "internal/mcp/mcp.go (or similar)" — `internal/mcp/mcp.go` does not exist; the actual wiring point is `cmd/sworn/mcp.go`. It is absent from both design §3 and status.json `planned_files`.
   What to ask: Add `cmd/sworn/mcp.go` to §3 and status.json `planned_files`. The registration call (`mcp.RegisterPlanTools(server, ".")` or equivalent) goes there, following the S08b pattern.

6. `[escalate]` Spec reachability artefact contradicts the in-scope section
   What I observed: Required Tests reachability says "configure sworn mcp in Claude Code; ask Claude to 'create a new sworn release'; observe AI calls create_release." But the spec's In Scope section explicitly states: "`createRelease` is not exposed as a public MCP tool from this slice." A connected AI cannot call an unexposed internal function via MCP. The spec contradicts itself on its own reachability artefact.
   What to ask: Coach must amend the spec's reachability artefact to a feasible demo. The technically correct substitute is: connect Claude Code to `sworn mcp`; ask AI to "add slice S99-test to release 2026-06-19-mcp-test"; observe AI calls `create_slice`; verify `docs/release/2026-06-19-mcp-test/S99-test/` is created; clean up. Or a simpler `update_intake` demo. Whichever is chosen, Required Tests must be amended via `/replan-release`.

---

## Summary

Pins: 6 total — 3 [mechanical], 1 [memory-cited], 2 [escalate]
Critical pins: Pin 1 (resources/read + prompts/get dispatch gap — ALL resource/prompt serving ships silent-broken without this fix); Pin 2 (rules.md source undefined — blocks the baton/rules resource entirely); Pin 3 (baton/ dir missing — go:embed references non-existent files, compile-time panic)

## Smaller flags (not pins, worth one-line ack)

(a) Design §2 Decision 4 uses "where supported by the MCP SDK" language — but sworn's MCP server is a bespoke implementation with no third-party SDK. The "if SDK requires exact matches" branch is dead; implement dynamic path matching in the resource handler directly.

(b) `handleResourcesList` returns an empty list. Spec defers dynamic listing post-R3, so this is acceptable — but confirm `prompts/list` is updated to enumerate registered prompts (spec includes prompts/list as in-scope).

(c) Drift: T4-mcp is 29 commits behind release-wt/. Not a spec-staleness risk (drift is from T9/T11, orthogonal domains), but the Implementer's forward-merge step before code is still required.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design looks structurally sound but has 3 critical mechanical gaps and 2 spec-level questions that need Coach decisions first. 6 pins total:

1. **Server dispatch gap (critical).** Before implementing resources.go/prompts.go, extend server.go: add `RegisterResource()` and `RegisterPrompt()` registration methods, add `"resources/read"` and `"prompts/get"` to `buildMethodHandlers()`, and update `handlePromptsList` to enumerate registered prompts. Pattern mirrors how `RegisterTool` works.

2. **`sworn://baton/rules` source (escalate — Coach decides first).** No `rules.md` exists anywhere. Do not create this resource until Coach resolves Pin 2. If deferred, mark with Rule 2 compliance.

3. **`internal/prompt/baton/` directory (critical — apply inline).** Create `internal/prompt/baton/`; vendor `~/.claude/baton/track-mode.md` there; decide on `baton/VERSION.txt` vs reusing `internal/prompt/VERSION.txt` via the handler. Update go:embed directive.

4. **yaml.v3 ADR (apply inline).** Before adding yaml.v3 to go.mod: either write a brief ADR entry or use stdlib strings for frontmatter manipulation (simpler frontmatter structure makes this viable). See [[project_dep_policy]].

5. **`cmd/sworn/mcp.go` to planned_files (apply inline).** Add `cmd/sworn/mcp.go` to status.json planned_files and §3. The "Planning tools (S08c) register here" comment in that file is the wiring point.

6. **Spec reachability (escalate — Coach decides first).** Required Tests reachability mentions `create_release` via MCP, but it's not a registered tool. Do not write the reachability section in proof.md against this artefact — wait for Coach to amend the spec.

Flags: (a) drop "MCP SDK" language in §2 Decision 4 — the server is bespoke, use dynamic path matching directly; (b) confirm prompts/list handler reflects registered prompts.

§2 decisions 1 (CreateRelease exported), 3 (update_intake append), and 5 (go:embed for prompts) ack — sound.

Address pins 1, 3, 4, 5 inline during implementation. Hold on pins 2 and 6 pending Coach decisions. Once Coach resolves, proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pins 2 and 6 require Coach authority — Pin 2 (source of rules.md has no single right answer: create new doc, use existing file, or defer post-R3) and Pin 6 (spec reachability references a non-existent MCP tool, spec must be amended before implementer can write the proof artefact).
-->
