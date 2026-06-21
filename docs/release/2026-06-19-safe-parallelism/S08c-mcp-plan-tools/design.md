# Design TL;DR

> Revised 2026-06-21 after Coach decline (`decline.md`). All 6 Captain pins addressed;
> see **§7. Pin resolutions**. State stays `design_review` pending Captain re-review.

## §1. User-visible change
An AI assistant connected to `sworn mcp` can plan a release by calling `create_slice`,
`set_track`, and `update_intake` tools, and can read role prompts and release artefacts
via `sworn://` resources and MCP prompts. A new `docs/mcp-setup.md` guide explains how to
configure this in various AI tools.

## §2. Design decisions not in spec (max 5)
1. **`createRelease` exported.** Implemented in `internal/mcp/tools_plan.go` as exported
   `CreateRelease` so S20's `plan_release` (T7) can call it. Not a registered MCP tool here.
2. **`set_track` frontmatter via stdlib, not a YAML lib.** `index.md` frontmatter is narrow
   (known key set, single-quoted scalar values per planner.md convention), so `set_track`
   manipulates it with targeted `strings`/stdlib operations rather than adding
   `gopkg.in/yaml.v3`. This avoids a new runtime dep ([[project_dep_policy]]). *(Overridable:
   if Coach prefers a real YAML parser, add yaml.v3 behind an ADR — see §6.)*
3. **`update_intake` append-or-create.** Appends under `## <section>`; creates the heading
   at EOF if absent, never corrupting existing content.
4. **Resource path matching is bespoke.** sworn's MCP server is a hand-rolled implementation
   (no third-party SDK), so the `resources/read` handler does dynamic `sworn://` path matching
   directly — there is no "URI template where the SDK supports it" branch.
5. **`go:embed` extended in `internal/prompt/prompt.go`.** The existing embed directive
   (role prompts + `VERSION.txt`) gains `baton/track-mode.md`; the embedded FS is the
   canonical resource source — no runtime filesystem fallback.

## §3. Files I'll touch grouped by purpose
**New:**
- `internal/mcp/tools_plan.go` — `CreateRelease`, `create_slice`, `set_track`, `update_intake`; exposes `RegisterPlanTools(server, root)`.
- `internal/mcp/resources.go` — `resources/read` handler (dynamic `sworn://` path matching).
- `internal/mcp/prompts.go` — `prompts/get` handler; prompt registration.
- `internal/prompt/baton/track-mode.md` — vendored verbatim from `~/.claude/baton/track-mode.md`.
- `docs/mcp-setup.md` — setup guide (Claude Code, Codex, Cursor, Windsurf, Gemini CLI).

**Edited (these are the pin-critical wiring points):**
- `internal/mcp/server.go` — add `RegisterResource(uri, handler)` and `RegisterPrompt(name, handler)`; add `"resources/read"` and `"prompts/get"` entries to `buildMethodHandlers()`; update `handlePromptsList` to enumerate registered prompts. *(Pin 1)*
- `cmd/sworn/mcp.go` — add `mcp.RegisterPlanTools(server, ".")` at the existing `// Planning tools (S08c) register here` marker (mirrors the `RegisterOpsTools` line). *(Pin 5)*
- `internal/prompt/prompt.go` — extend the `//go:embed` directive to include `baton/track-mode.md`; add an accessor. *(Pin 3)*
- `internal/mcp/tools_test.go` — unit tests for tools, resources, prompts.

## §4. Things I'm NOT doing
- **`sworn://baton/rules` resource — DEFERRED to S21 (Rule 2).**
  - **Why**: its source, `internal/prompt/baton/rules.md`, is created by S21-canonical-baton (T3); no consolidated Baton-protocol file exists yet (`~/.claude/baton/` holds only individual rule files).
  - **Tracking**: S21-canonical-baton. Recorded as a cross-slice touchpoint note in the index — **not** a hard T4→T3 build dependency; S08c ships without this one resource.
  - **Acknowledged**: Coach, 2026-06-21 (`decline.md`).
- `resources/list` returning all `sworn://` URIs dynamically (post-R3 per spec).
- `create_release` as a registered MCP tool (internal function for S20 only).

## §5. Reachability plan
Configure `sworn mcp` in Claude Code; ask it to **"add slice S99-smoke to release
2026-06-19-mcp-test"**; observe the AI call **`create_slice`**; verify
`docs/release/2026-06-19-mcp-test/S99-smoke/{spec.md,status.json}` are created; clean up.
Capture the transcript/log in `proof.md`. *(Replaces the old artefact that referenced the
unexposed `create_release` — Pin 6.)*

## §6. Open questions for the Coach
- **yaml.v3 vs stdlib for `set_track` frontmatter (§2.2).** Design picks stdlib (no new dep).
  Confirm, or direct me to add `gopkg.in/yaml.v3` behind an ADR. Default proceeds with stdlib.

## §7. Pin resolutions (from `review.md` / `decline.md`)
1. **[Pin 1 — server dispatch gap]** `server.go` gains `RegisterResource`/`RegisterPrompt`
   methods + `"resources/read"`/`"prompts/get"` in `buildMethodHandlers()` + `handlePromptsList`
   enumeration. Without this every read/get returns JSON-RPC "Method not found". Now in §3.
2. **[Pin 2 — `sworn://baton/rules` source]** Deferred to S21 per §4 (Rule 2, no hard dep).
3. **[Pin 3 — `internal/prompt/baton/` missing]** Create the dir, vendor `track-mode.md`,
   extend the `prompt.go` `go:embed`. `sworn://baton/version` is served from the existing
   `internal/prompt/VERSION.txt` via the resource handler — no duplicate `baton/VERSION.txt`.
4. **[Pin 4 — yaml.v3 dep]** Resolved by §2.2 (stdlib, no dep) — no ADR needed unless Coach
   overrides.
5. **[Pin 5 — `cmd/sworn/mcp.go` missing from planned_files]** Added to §3 and to
   `status.json` `planned_files`; registration call goes at the marked wiring point.
6. **[Pin 6 — spec reachability references unexposed tool]** §5 rewritten to a `create_slice`
   demo; `spec.md` Required-tests reachability amended to match.

Smaller flags: (a) "MCP SDK" language dropped — server is bespoke (§2.4); (b) `prompts/list`
will enumerate registered prompts after the `handlePromptsList` update (§3, Pin 1).
