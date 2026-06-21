# Design TL;DR

## §1. User-visible change
An AI assistant connected to `sworn mcp` can plan an entire release by calling `create_slice`, `set_track`, and `update_intake` tools, and can read role prompts and release artifacts via `sworn://` resources and MCP prompts. A new `docs/mcp-setup.md` guide explains how to configure this in various AI tools.

## §2. Design decisions not in spec (max 5)
1. `createRelease` will be implemented in `internal/mcp/tools_plan.go` as an exported function `CreateRelease` so it can be called by S20's `plan_release` tool later.
2. `set_track` will use a simple regex/string replacement for the Tracks table in `index.md` to avoid needing a full Markdown AST parser, while using `gopkg.in/yaml.v3` for the frontmatter.
3. `update_intake` will append to the end of the file if the section heading doesn't exist, ensuring it doesn't corrupt existing content.
4. Resources will be registered with URI templates where supported by the MCP SDK, or we will handle dynamic path matching in the `resources/read` handler if the SDK requires exact matches.
5. The embedded prompts will be accessed via `go:embed` in `internal/prompt/embed.go` (or similar) to ensure they are compiled into the binary.

## §3. Files I'll touch grouped by purpose
- `internal/mcp/tools_plan.go`: Implement `CreateRelease`, `create_slice`, `set_track`, and `update_intake` tools.
- `internal/mcp/resources.go`: Implement `resources/read` handlers for `sworn://` URIs.
- `internal/mcp/prompts.go`: Implement `prompts/list` and `prompts/get` handlers.
- `internal/mcp/mcp.go` (or similar): Register the new tools, resources, and prompts with the MCP server.
- `internal/prompt/...`: Ensure `go:embed` is set up for the prompts and Baton rules.
- `docs/mcp-setup.md`: Write the setup guide.
- `internal/mcp/tools_test.go`: Add unit tests for the new tools and resources.

## §4. Things I'm NOT doing
- `resources/list` returning all available `sworn://` URIs dynamically (deferred post-R3 per spec).
- `create_release` as a registered MCP tool (it's an internal function for S20).

## §5. Reachability plan
I will configure `sworn mcp` in Claude Code (or use a test script that acts as an MCP client), ask it to create a new slice and update the intake, and verify the files are created correctly. I will capture the output in `proof.md`.

## §6. Open questions for the Coach
- None.