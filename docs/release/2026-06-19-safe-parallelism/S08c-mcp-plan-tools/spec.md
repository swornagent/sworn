---
title: 'S08c-mcp-plan-tools — MCP planning tools + resources + prompts + setup doc'
description: 'Registers 4 planning tools (create_release, create_slice, set_track, update_intake) and exposes sworn://prompts/* and sworn://release/* resources so any AI can plan a release via MCP and pull the Baton role prompts.'
---

# Slice: `S08c-mcp-plan-tools`

## User outcome

An AI assistant connected to `sworn mcp` can plan an entire release — calling
`create_release`, `create_slice`, and `set_track` to write all artefacts into the
repo — and pull the planner/implementer/verifier role prompts as MCP resources, without
the developer running any CLI commands or copying template files manually.

## Entry point

`tools/call` on the MCP server for planning tools; `resources/read` for sworn:// URIs;
`prompts/get` for prompt resources.

## In scope

Three planning tools registered via `server.RegisterTool()`:

> Note: the originally-planned `create_release` tool is superseded by `plan_release`
> in S20-mcp-catalog-tools (T7), which adds detection logic (new vs. existing release).
> S08c implements `create_release` as an internal function used by S20's `plan_release`;
> it is not exposed as a public MCP tool from this slice.

1. *(internal)* **`createRelease`** `(name, goal, tracking_issue)` — creates
   `docs/release/<name>/` directory; writes `intake.md` with `goal` and `tracking_issue`;
   writes `index.md` from template; creates `screenshots/.gitkeep`; returns paths.
   Called by S20's `plan_release`; not a registered MCP tool in this slice.

2. **`create_slice`** `(release: string, slice_id: string, spec_content: string, track_id: string)` →
   creates `docs/release/<release>/<slice_id>/` directory; writes `spec.md` with
   `spec_content`; writes `status.json` with `state: planned`, `track: track_id`, and
   current timestamp; returns the created paths. Errors if slice_id already exists.

3. **`set_track`** `(release: string, track_id: string, slices: []string, depends_on?: string)` →
   reads `index.md` frontmatter; adds or updates the track entry in the `tracks:` list
   (id, slices, depends_on, worktree_branch); rewrites the Tracks table in the index.md
   body to match; returns the updated frontmatter. Validates: all slice_ids exist under
   the release before writing.

4. **`update_intake`** `(release: string, section: string, content: string)` →
   appends `content` under the `## <section>` heading in `intake.md`; creates the
   heading if absent (appended at end of file). Returns the heading that was written to.

MCP Resources (registered via `server.RegisterResource()`):

> All prompt and Baton resources are served from the binary's `internal/prompt/`
> embed — NOT from `$HOME/.claude/baton/`. The binary is the canonical source.
> If the embed is absent or corrupted, the resource returns an error; it does not
> fall back to the filesystem.

- `sworn://prompts/plan` → `internal/prompt/planner.md` (embed)
- `sworn://prompts/implement` → `internal/prompt/implementer.md` (embed)
- `sworn://prompts/verify` → `internal/prompt/verifier.md` (embed)
- `sworn://baton/rules` → **DEFERRED to S21-canonical-baton** — source `internal/prompt/baton/rules.md` is not yet built; Rule-2 deferral (see design §4), Coach ack 2026-06-21. Not served by S08c.
- `sworn://baton/track-mode` → `internal/prompt/baton/track-mode.md` (embed)
- `sworn://baton/version` → `internal/prompt/baton/VERSION.txt` (embed; Baton version string)
- `sworn://release/{name}/board` → content of `docs/release/<name>/index.md`
- `sworn://release/{name}/intake` → content of `docs/release/<name>/intake.md`
- `sworn://release/{name}/{slice}/spec` → content of the slice's `spec.md`
- `sworn://release/{name}/{slice}/proof` → content of the slice's `proof.md`
  (returns empty string if proof.md does not yet exist — not an error)

MCP Prompts (`prompts/list` + `prompts/get`):
- `planner` → `sworn://prompts/plan` content as a prompt
- `implementer` → `sworn://prompts/implement` content as a prompt
- `verifier` → `sworn://prompts/verify` content as a prompt

`docs/mcp-setup.md`: setup instructions for Claude Code, Codex, Cursor, Windsurf,
Gemini CLI — includes the JSON config block for each tool, the list of available tools,
and an example planning workflow.

## Out of scope

- Server-side resource change notifications (post-R3)
- HTTP transport or remote resource serving (post-R3)
- Validation of spec_content format (the AI is responsible for the content)

## Planned touchpoints

- `internal/mcp/tools_plan.go` (new — 4 planning tool handlers)
- `internal/mcp/resources.go` (new — resource URI handlers)
- `internal/mcp/prompts.go` (new — prompt handlers, reads embedded role prompts)
- `docs/mcp-setup.md` (new)

## Acceptance checks

- [ ] Internal `createRelease("test-mcp-release", "test goal")` creates the expected
  directory structure with intake.md containing "test goal" and index.md from template;
  cleans up after test; function is callable from S20's plan_release handler
- [ ] `create_slice("test-mcp-release", "S01-foo", "# spec content", "T1")` creates
  spec.md with the provided content and status.json with state=planned and track=T1
- [ ] `set_track` with a valid slices list updates the index.md frontmatter and Tracks
  table; `set_track` with a non-existent slice_id returns an error (not a panic)
- [ ] `update_intake` appends content under the correct section heading
- [ ] `resources/read sworn://prompts/plan` returns non-empty content from the
  `internal/prompt/` embed — NOT from `$HOME/.claude/baton/` (verified by checking
  that the resource works when `$HOME/.claude/baton/` does not exist)
- [ ] *(DEFERRED to S21 — out of scope for S08c)* `resources/read sworn://baton/rules`
  returns non-empty content from the `internal/prompt/baton/rules.md` embed (source built by
  S21-canonical-baton; Rule-2 deferral, Coach ack 2026-06-21)
- [ ] `resources/read sworn://baton/version` returns a parseable version string
- [ ] `resources/read sworn://release/2026-06-19-safe-parallelism/board` returns the
  content of this release's index.md
- [ ] `resources/read sworn://release/{name}/{slice}/proof` for a slice with no proof.md
  returns empty string (not an error)
- [ ] `docs/mcp-setup.md` exists and contains Claude Code JSON config block
- [ ] `go test ./internal/mcp/...` covers all 3 registered planning tools, the internal
  createRelease function, and resource reads

## Required tests

- **Unit**: `internal/mcp/tools_test.go` (extend)
  — `TestCreateRelease`: call internal createRelease; assert files created; cleanup
  — `TestCreateSliceDuplicate`: call create_slice twice with same id; assert error on
    second call (not silent overwrite)
  — `TestSetTrackValidation`: set_track with non-existent slice_id; assert error returned
  — `TestUpdateIntakeAppends`: call update_intake twice on same section; assert both
    contents present; assert order preserved
  — `TestResourceReadPrompt`: assert sworn://prompts/plan returns non-empty string
  — `TestResourceReadProofAbsent`: sworn://release/{name}/{slice}/proof for slice with
    no proof.md; assert empty string, no error
- **Reachability artefact**: configure sworn mcp in Claude Code; ask Claude to "add slice
  S99-smoke to release 2026-06-19-mcp-test"; observe AI calls **create_slice**; verify
  `docs/release/2026-06-19-mcp-test/S99-smoke/{spec.md,status.json}` created; clean up.
  Screenshot or log in proof.md. *(Amended 2026-06-21: the prior artefact referenced
  `create_release`, which this slice does not expose as an MCP tool — Coach ack via decline.md.)*

## Risks

- Role prompts and Baton docs are served from the `internal/prompt/` embed, NOT from
  `$HOME/.claude/baton/`. The embed is compiled in at build time; there is no runtime
  filesystem fallback. If the embed is somehow absent (should never happen in a correctly
  built binary), return a clear error: "sworn://prompts/plan: embedded prompt not found
  — this is a binary build error; please reinstall sworn."
- `set_track` rewrites the index.md frontmatter. YAML frontmatter generation must
  produce strict-YAML-safe output (single-quoted strings per the planner.md convention).
  Test with a slice title containing a colon-space.

## Deferrals allowed?

Yes, with Rule 2 compliance:
- `resources/list` returning all available sworn:// URIs dynamically: deferred post-R3.
  Why: dynamic listing requires scanning release dirs; static registration is sufficient
  for now. Tracking: TBD. Ack: now.
